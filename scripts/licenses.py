#!/usr/bin/env python3
"""Collect shipped-module notices from the verified Go module cache.
Run after go mod download. This is an inventory, not a legal compatibility opinion.
"""
import json,pathlib,subprocess,sys
raw=subprocess.check_output(['go','list','-deps','-json','.'],text=True)
decoder=json.JSONDecoder();modules={}
while raw.strip():
 obj,end=decoder.raw_decode(raw.lstrip());raw=raw.lstrip()[end:];m=obj.get('Module')
 if m and not m.get('Main'):
  m=m.get('Replace',m);modules[(m['Path'],m.get('Version',''))]=pathlib.Path(m['Dir'])
parts=['Third-party license and notice inventory for hashi-grpc-forwarder.\nGenerated from runtime build dependencies, not a relicense of them.\n']
root=pathlib.Path(subprocess.check_output(['go','env','GOROOT'],text=True).strip())
parts+=['\n=== Go standard library/runtime ===\n'+(root/'LICENSE').read_text()]
missing=[]
for (name,version),directory in sorted(modules.items()):
 files=[p for p in directory.iterdir() if p.is_file() and (p.name.upper().startswith(('LICENSE','LICENCE','COPYING','NOTICE','COPYRIGHT')) or p.name.upper()=='PATENTS')]
 if not files:missing.append(name+' '+version);continue
 parts.append('\n=== '+name+' '+version+' ===\n')
 for p in sorted(files):parts.append('\n--- '+p.name+' ---\n'+p.read_text(errors='replace'))
if missing:raise SystemExit('Missing top-level notices; inspect before release: '+', '.join(missing))
out=pathlib.Path(sys.argv[1] if len(sys.argv)>1 else 'THIRD_PARTY_LICENSES.txt');out.write_text('\n'.join(parts));print(json.dumps({'runtime_dependency_modules':len(modules),'output':str(out),'bytes':out.stat().st_size}))
