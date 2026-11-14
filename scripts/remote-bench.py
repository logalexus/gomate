#!/usr/bin/env python3
"""Generate traffic HERE; mocks and capture run on Kali's external interface."""
import argparse
import base64
import concurrent.futures
import json
import os
from pathlib import Path
import re
import subprocess
import time
import urllib.request

os.chdir(Path(__file__).resolve().parents[1])
p=argparse.ArgumentParser()
p.add_argument('--host',default='192.168.56.103')
p.add_argument('--count',type=int,default=100)
p.add_argument('--rps',type=int,default=50,help='per protocol: aggregate offered unary RPS = 3x this value')
p.add_argument('--tag',default='smoke')
args=p.parse_args()
if not re.fullmatch(r'[a-zA-Z0-9_-]+',args.tag) or not re.fullmatch(r'[0-9.]+',args.host):raise SystemExit('invalid tag/host')
env=dict(l.split('=',1) for l in Path('.cache/kali.env').read_text().splitlines() if l and not l.startswith('#'))
headers={'Authorization':'Basic '+base64.b64encode(('packmate:'+env['WEB_PASSWORD']).encode()).decode(),'Content-Type':'application/json'}
base=f'http://{args.host}:65000'
def api(path,body=None):
    data=None if body is None else json.dumps(body).encode()
    with urllib.request.urlopen(urllib.request.Request(base+path,data=data,headers=headers),timeout=10) as r:return json.load(r)
for port,name in [(18080,'Mock HTTP'),(19090,'Mock TCP'),(15051,'Mock gRPC')]:api('/api/service/',{'port':port,'name':name,'http':port in (18080,15051),'parseWebSockets':port==18080,'urldecodeHttpRequests':port==18080})
patterns=api('/api/pattern/')
if not any(v['name']=='Lab response markers' and not v['deleted'] for v in patterns):api('/api/pattern/',{'name':'Lab response markers','value':'FLAG_','color':'#dd3333','searchType':'SUBSTRING','directionType':'OUTPUT','actionType':'FIND'})
before=api('/api/v2/snapshot')['metrics'];run=f'{args.tag}_{time.time_ns()}'
commands=[('http',18080,args.count,args.rps),('tcp',19090,args.count,args.rps),('grpc',15051,args.count,args.rps),('grpc-stream',15051,10,5)]
def generate(spec):
    proto,port,count,rps=spec
    result=subprocess.run(['bin/mock','-mode','client','-protocol',proto,'-address',f'{args.host}:{port}','-run',run,'-count',str(count),'-rps',str(rps),'-concurrency','512'],capture_output=True,text=True,timeout=max(60,count/rps+20))
    report=json.loads(result.stdout);report['exit_code']=result.returncode;report['stderr']=result.stderr;return report
with concurrent.futures.ThreadPoolExecutor(max_workers=4) as pool:generated=list(pool.map(generate,commands))
drain=time.monotonic();deadline=drain+15
while True:
    raw=subprocess.check_output(['ssh','-o','BatchMode=yes',f'kali@{args.host}',f'python3 ~/gomate-test/count.py {run}'],text=True,timeout=25)
    stored=json.loads(raw)
    if all(stored.get(g['protocol'],0)==g['response_messages'] for g in generated) or time.monotonic()>=deadline:break
    time.sleep(.5)
after=api('/api/v2/snapshot')['metrics'];delta={k:after[k]-before[k] for k in before if k.endswith('_total')}
raw=subprocess.check_output(['ssh',f'kali@{args.host}',"docker image inspect gomate-app:kali-test gomate-mock:kali-test postgres:17.7-alpine --format '{{json .RepoTags}} {{.Id}}'"],text=True)
report={'run':run,'generated':generated,'stored_unique_response_markers':stored,'metrics_delta':delta,'metrics_before':before,'metrics_after':after,'verification_seconds':time.monotonic()-drain,'images':raw.splitlines(),'grpc_semantic_decode':True,'generator_location':'devops 192.168.56.102; Kali 192.168.56.103 eth1; no SSH data tunnel','target':args.host,'aggregate_unary_offered_rps':args.rps*3}
passed=all(g['exit_code']==0 and stored.get(g['protocol'],0)==g['response_messages'] for g in generated) and not any(v for k,v in delta.items() if 'drops' in k)
report['passed']=passed
path=Path('reports')/f'kali-{args.tag}.json';path.write_text(json.dumps(report,indent=2)+'\n')
print(json.dumps({'report':str(path),'passed':passed,'stored':stored,'drops':{k:v for k,v in delta.items() if 'drops' in k}}))
raise SystemExit(0 if passed else 1)
