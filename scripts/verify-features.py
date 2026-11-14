#!/usr/bin/env python3
"""Exercise decoded protocols and checkpointed lookback over the real Kali NIC."""
import base64, json, os, re, subprocess, time, urllib.request
from pathlib import Path
os.chdir(Path(__file__).resolve().parents[1])
env=dict(l.split('=',1) for l in Path('.cache/kali.env').read_text().splitlines() if l and not l.startswith('#'))
headers={'Authorization':'Basic '+base64.b64encode(('packmate:'+env['WEB_PASSWORD']).encode()).decode(),'Content-Type':'application/json'}
base='http://192.168.56.103:65000'
def api(path,body=None):
    data=None if body is None else json.dumps(body).encode()
    with urllib.request.urlopen(urllib.request.Request(base+path,data=data,headers=headers),timeout=10) as r:return json.load(r)
def sql(query):
    raw=subprocess.check_output(['ssh','kali@192.168.56.103','cd ~/gomate-test && docker compose exec -T db psql -U gomate -d gomate -tA'],input=query,text=True,timeout=20)
    return json.loads(raw.strip())
run='features_'+str(time.time_ns())
api('/api/service/',{'port':18080,'name':'Mock HTTP','http':True,'parseWebSockets':True,'urldecodeHttpRequests':True})
api('/api/service/',{'port':15051,'name':'Mock gRPC','http':True})
before=api('/api/v2/snapshot')['metrics']
generated=[]
for protocol,port in [('http-gzip',18080),('ws',18080),('grpc-gzip',15051),('grpc-stream',15051)]:
    r=subprocess.run(['bin/mock','-mode','client','-protocol',protocol,'-address',f'192.168.56.103:{port}','-run',run,'-count','10','-rps','5'],capture_output=True,text=True,timeout=40)
    generated.append(json.loads(r.stdout));assert r.returncode==0,r.stderr
query=f"""SELECT coalesce(json_agg(t),'[]'::json) FROM (
SELECT p.id,p.stream_id,p.details->>'kind' kind,p.details->>'ungzipped' ungzipped,p.details->'metadata' metadata,p.matches,s.user_agent_hash
FROM gomate_packets p JOIN gomate_streams s ON s.id=p.stream_id WHERE NOT p.incoming AND COALESCE(p.details->>'kind','')<>'' AND encode(p.content,'escape') LIKE '%{run}%' ORDER BY p.id) t;"""
deadline=time.monotonic()+20
while True:
    records=sql(query)
    if len(records)==60 or time.monotonic()>deadline:break
    time.sleep(.5)
assert len(records)==60,f'expected 60 decoded replies, got {len(records)}'
assert sum(p['kind']=='http' and p['ungzipped']=='true' for p in records)==10
assert sum(p['kind']=='websocket' for p in records)==10
assert sum(p['kind']=='grpc' and p['ungzipped']=='true' for p in records)==10
assert sum(p['kind']=='grpc' and p['metadata'].get('path')=='/gomate.mock.Echo/Watch' for p in records)==30
pattern=api('/api/pattern/',{'name':run,'value':'FLAG_'+run,'color':'#2288cc','searchType':'SUBSTRING','directionType':'OUTPUT','actionType':'FIND'})
job=api(f"/api/pattern/{pattern['id']}/lookback",1)
deadline=time.monotonic()+90
while True:
    jobs=api('/api/v2/jobs');status=next(j for j in jobs if j['id']==job['id'])
    if status['status']=='completed' or time.monotonic()>deadline:break
    time.sleep(.5)
assert status['status']=='completed',status
records=sql(query)
assert all(any(m['patternId']==pattern['id'] for m in (p['matches'] or [])) for p in records),'lookback missed a decoded message'
# The UI-facing API must expose actual semantic data, not only the SQL rows.
for record in [records[0],next(p for p in records if p['kind']=='grpc')]:
    blocks=api(f"/api/packet/{record['stream_id']}",{'pageSize':100,'representation':'decoded'})
    assert any(b['id']==record['id'] and b['kind']==record['kind'] for b in blocks)
after=api('/api/v2/snapshot')['metrics'];delta={k:after[k]-before[k] for k in before if k.endswith('_total')}
assert not any(v for k,v in delta.items() if 'drops' in k or k=='decode_errors_total'),delta
report={'passed':True,'run':run,'generated':generated,'decodedReplies':len(records),'lookback':status,'metrics_delta':delta,'diagnostics':api('/api/v2/diagnostics'),'target':'192.168.56.103 eth1; traffic generated on 192.168.56.102','records':records}
Path('reports/kali-features.json').write_text(json.dumps(report,indent=2)+'\n')
print(json.dumps({'passed':True,'report':'reports/kali-features.json','decodedReplies':len(records),'lookback':status['status']}))
