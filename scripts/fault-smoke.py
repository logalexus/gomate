#!/usr/bin/env python3
"""Pause only the dedicated Compose DB, then verify LIVE recovers without restart."""
import base64
import json
from pathlib import Path
import subprocess
import time
import urllib.error
import urllib.request

env = dict(l.split('=',1) for l in Path('.env').read_text().splitlines() if l and not l.startswith('#'))
headers = {'Authorization':'Basic '+base64.b64encode((env.get('WEB_LOGIN','packmate')+':'+env['WEB_PASSWORD']).encode()).decode()}
base = 'http://127.0.0.1:16500'
def get(path):
    with urllib.request.urlopen(urllib.request.Request(base+path,headers=headers),timeout=5) as r:
        return r.status, r.read()
def snapshot(): return json.loads(get('/api/v2/snapshot')[1])['metrics']
db = subprocess.check_output(['docker','compose','ps','-q','db'],text=True).strip()
if not db: raise SystemExit('start the dedicated gomate fixture first')
before = snapshot()
started_at = subprocess.check_output(['docker','inspect','gomate-live-test','--format','{{.State.StartedAt}}'],text=True).strip()
prefix = f'fault{time.time_ns()}'
gen = subprocess.Popen(['bin/traffic','-target','http://127.0.0.1:18080','-duration','12s','-rps','100','-agents','200','-prefix',prefix],stdout=subprocess.PIPE,text=True)
paused = False
try:
    time.sleep(1)
    subprocess.run(['docker','pause',db],check=True,stdout=subprocess.DEVNULL);paused=True
    pause_start=time.monotonic()
    try: during_status=get('/health/ready')[0]
    except urllib.error.HTTPError as e: during_status=e.code
    time.sleep(max(0,5-(time.monotonic()-pause_start)))
finally:
    if paused: subprocess.run(['docker','unpause',db],check=True,stdout=subprocess.DEVNULL)
out,_=gen.communicate(timeout=30)
generated=json.loads(out)
deadline=time.monotonic()+15
sql=f"SELECT count(DISTINCT m[1]) FROM gomate_packets p CROSS JOIN LATERAL regexp_matches(convert_from(p.content,'UTF8'),'FLAG_{prefix}_([0-9]+)','g') AS m"
while True:
    stored=int(subprocess.check_output(['docker','compose','exec','-T','db','psql','-U','gomate','-d','gomate_live','-tAc',sql],text=True).strip())
    if stored>=generated['success'] or time.monotonic()>=deadline: break
    time.sleep(.5)
after=snapshot()
same_start=started_at==subprocess.check_output(['docker','inspect','gomate-live-test','--format','{{.State.StartedAt}}'],text=True).strip()
report={'paused_seconds':5,'readiness_during_pause':during_status,'readiness_after':get('/health/ready')[0],'same_process_start':same_start,'generator':generated,'stored_markers':stored,'metrics_before':before,'metrics_after':after}
Path('reports/fault-smoke.json').write_text(json.dumps(report,indent=2)+'\n')
print(json.dumps({'during_status':during_status,'same_process_start':same_start,'stored':stored,'expected':generated['success']}))
assert during_status==503 and report['readiness_after']==200 and same_start and stored==generated['success'] and generated['errors']==0
