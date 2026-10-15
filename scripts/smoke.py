#!/usr/bin/env python3
"""Optional live endpoint smoke test. Requires grpcurl and a private env key.
No key is stored in the repository or printed. Not required to run the binary.
"""
import argparse,json,os,pathlib,socket,subprocess,tempfile,time
p=argparse.ArgumentParser();p.add_argument('--binary',required=True);args=p.parse_args()
binary=str(pathlib.Path(args.binary).resolve());grpcurl=os.environ.get('GRPCURL','grpcurl');key=os.environ.get('SUI_GRPC_API_KEY','')
if not key:raise SystemExit('Set SUI_GRPC_API_KEY privately before running this optional smoke test')
upstream=os.environ.get('SUI_GRPC_UPSTREAM','https://sui-mainnet-grpc.giverep.com:443')
baseenv={k:v for k,v in os.environ.items() if k not in ['SUI_GRPC_API_KEY','SUI_GRPC_UPSTREAM','FORWARDER_LISTEN','FORWARDER_ALLOW_NON_LOOPBACK']}
result={}
with tempfile.TemporaryDirectory(prefix='hashi-forwarder-smoke-',dir=os.environ.get('TMPDIR')) as d:
 missing=subprocess.run([binary],cwd=d,env=baseenv,capture_output=True,text=True,timeout=10)
 assert missing.returncode!=0;result['missing_key_fails_closed']=True
 sock=socket.socket();sock.bind(('127.0.0.1',0));port=sock.getsockname()[1];sock.close()
 envfile=pathlib.Path(d)/'.env';fd=os.open(envfile,os.O_WRONLY|os.O_CREAT|os.O_EXCL,0o600)
 with os.fdopen(fd,'w') as f:f.write(f'SUI_GRPC_API_KEY={key}\nSUI_GRPC_UPSTREAM={upstream}\nFORWARDER_LISTEN=127.0.0.1:{port}\n')
 def start(env):
  child=subprocess.Popen([binary],cwd=d,env=env,stdout=subprocess.PIPE,stderr=subprocess.PIPE,text=True)
  deadline=time.monotonic()+5
  while time.monotonic()<deadline:
   if child.poll() is not None:
    out,err=child.communicate();assert key not in out+err;raise AssertionError('forwarder startup failed')
   try:
    with socket.create_connection(('127.0.0.1',port),timeout=.2):return child
   except OSError:time.sleep(.05)
  child.kill();child.communicate();raise AssertionError('listener readiness timed out')
 def stop(child):
  child.terminate();out,err=child.communicate(timeout=10)
  assert key not in out+err and child.returncode==0
  with socket.socket() as s:assert s.connect_ex(('127.0.0.1',port))!=0
 def call(method,data='{}',seconds='10',extra=None):
  return subprocess.run([grpcurl,'-plaintext','-max-time',seconds,*(extra or []),'-d',data,f'127.0.0.1:{port}',method],capture_output=True,text=True,timeout=int(seconds)+5)
 child=start(baseenv)
 try:
  unary=call('sui.rpc.v2.LedgerService/GetServiceInfo',extra=['-H','x-api-key: caller-key-must-be-overwritten'])
  assert unary.returncode==0;info=json.loads(unary.stdout)
  result['unary']={'chain':info['chain'],'checkpoint':info.get('checkpointHeight')};result['caller_key_overwritten']=True
  stream=call('sui.rpc.v2.SubscriptionService/SubscribeCheckpoints','{"readMask":{"paths":["sequence_number"]}}','8')
  decoder=json.JSONDecoder();text=stream.stdout;messages=[]
  while text.strip():
   obj,idx=decoder.raw_decode(text.lstrip());messages.append(obj);text=text.lstrip()[idx:]
  assert len(messages)>=2 and int(messages[-1]['cursor'])>int(messages[0]['cursor'])
  assert 'DeadlineExceeded' in stream.stderr
  result['stream']={'messages':len(messages),'first':messages[0]['cursor'],'last':messages[-1]['cursor'],'test_deadline_seconds':8}
 finally:stop(child)
 result['clean_shutdown_and_listener_cleanup']=True
 child=start(dict(baseenv,SUI_GRPC_API_KEY='invalid-smoke-fixture-key'))
 try:
  bad=call('sui.rpc.v2.LedgerService/GetServiceInfo');assert bad.returncode!=0
  # Some gRPC clients report a trailers error for an upstream's HTML401.
  # Independently inspect HTTP/2 status instead of requiring one client error spelling.
  status=subprocess.run(['curl','--http2-prior-knowledge','-sS','-o',os.devnull,'-w','%{http_code}','-X','POST',f'http://127.0.0.1:{port}/sui.rpc.v2.LedgerService/GetServiceInfo'],capture_output=True,text=True,timeout=10)
  assert status.returncode==0 and status.stdout=='401'
  result['exported_env_overrides_dotenv_and_wrong_key_denied']=True
  result['wrong_key_http_status']=status.stdout
 finally:stop(child)
 unexpected=[x.name for x in pathlib.Path(d).iterdir() if x.name!='.env'];assert not unexpected
 result['no_config_autosave_in_workdir']=True;result['key_absent_from_process_output']=True
print(json.dumps(result,indent=2))
