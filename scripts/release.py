#!/usr/bin/env python3
"""Build static Linux release executables and a checksum manifest."""
import hashlib,os,pathlib,re,shutil,subprocess,sys
version=sys.argv[1] if len(sys.argv)>1 else 'dev'
if not re.fullmatch(r'[A-Za-z0-9._-]{1,64}',version):raise SystemExit('invalid release version')
root=pathlib.Path(__file__).resolve().parent.parent;os.chdir(root);dist=root/'dist';dist.mkdir(exist_ok=True)
subprocess.run([sys.executable,'scripts/licenses.py','THIRD_PARTY_LICENSES.txt'],check=True)
for arch in ['amd64','arm64']:
 subprocess.run(['go','build','-p','4','-trimpath','-buildvcs=false','-ldflags','-s -w -X main.version='+version,'-o',str(dist/('hashi-grpc-forwarder_linux_'+arch)),'.'],env=dict(os.environ,CGO_ENABLED='0',GOOS='linux',GOARCH=arch),check=True)
for source,target in [('THIRD_PARTY_LICENSES.txt','THIRD_PARTY_LICENSES.txt'),('LICENSE','LICENSE'),('NOTICE','NOTICE'),('README.md','README.md'),('.env.example','forwarder.env.example'),('examples/hashi-grpc-forwarder.service','hashi-grpc-forwarder.service')]:shutil.copyfile(root/source,dist/target)
names=['hashi-grpc-forwarder_linux_amd64','hashi-grpc-forwarder_linux_arm64','THIRD_PARTY_LICENSES.txt','LICENSE','NOTICE','README.md','forwarder.env.example','hashi-grpc-forwarder.service']
(dist/'SHA256SUMS').write_text(''.join(hashlib.sha256((dist/n).read_bytes()).hexdigest()+'  '+n+'\n' for n in names))
print('Built '+version+'; checksums at '+str(dist/'SHA256SUMS'))
