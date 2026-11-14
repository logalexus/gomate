#!/usr/bin/env python3
"""Create only project-owned fixture containers; never capture host interfaces."""
import base64
import json
import os
from pathlib import Path
import shutil
import subprocess
import time
import urllib.request

root = Path(__file__).resolve().parents[1]
os.chdir(root)
def run(*cmd):
    return subprocess.run(cmd, check=True)
def output(*cmd):
    return subprocess.check_output(cmd, text=True).strip()
env = dict(l.split('=',1) for l in Path('.env').read_text().splitlines() if l and not l.startswith('#'))
for name in ['gomate-fixture', 'gomate-live-test']:
    if subprocess.run(['docker','inspect',name], stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL).returncode == 0:
        raise SystemExit(f'{name} already exists; stop/remove this test fixture explicitly before recreating it')
go = shutil.which('go') or str(root / '.tools/go/bin/go')
buildenv = os.environ.copy(); buildenv['CGO_ENABLED'] = '0'
for name, source in [('packmate','./cmd/packmate'),('traffic','./bench/traffic')]:
    subprocess.run([go,'build','-trimpath','-o',f'bin/{name}',source], env=buildenv, check=True)
run('npm','run','build','--prefix','web')
run('docker','compose','up','-d','--wait','db')
psql = ['docker','compose','exec','-T','db','psql','-U','gomate','-d','postgres']
if output(*psql,'-tAc',"SELECT 1 FROM pg_database WHERE datname='gomate_live'") != '1':
    run(*psql,'-c','CREATE DATABASE gomate_live')
if subprocess.run(['docker','network','inspect','gomate-test'], stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL).returncode != 0:
    run('docker','network','create','gomate-test')
db = output('docker','compose','ps','-q','db')
networks = json.loads(output('docker','inspect',db))[0]['NetworkSettings']['Networks']
if 'gomate-test' not in networks:
    run('docker','network','connect','--alias','gomate-test-db','gomate-test',db)
db_ip = json.loads(output('docker','inspect',db))[0]['NetworkSettings']['Networks']['gomate-test']['IPAddress']
run('docker','run','-d','--name','gomate-fixture','--network','gomate-test','--cpus=1','--memory=128m','-p','127.0.0.1:18080:18080','-p','127.0.0.1:16500:65000','-v',str(root/'bin/traffic')+':/traffic:ro','--entrypoint','/traffic','alpine:3.22','-listen',':18080')
ip = json.loads(output('docker','inspect','gomate-fixture'))[0]['NetworkSettings']['Networks']['gomate-test']['IPAddress']
values = {'DATABASE_URL':f"postgres://gomate:{env['DB_PASSWORD']}@{db_ip}:5432/gomate_live?sslmode=disable", 'WEB_LOGIN':env.get('WEB_LOGIN','packmate'),'WEB_PASSWORD':env['WEB_PASSWORD'],'INTERFACE':'eth0','LOCAL_IPS':ip,'LISTEN':'0.0.0.0:65000','WEB_DIR':'/web','GOMAXPROCS':'1','GOMEMLIMIT':'384MiB'}
Path('.cache').mkdir(exist_ok=True)
file = Path('.cache/live.env'); file.write_text(''.join(k+'='+v+'\n' for k,v in values.items())); file.chmod(0o600)
run('docker','run','-d','--name','gomate-live-test','--network','container:gomate-fixture','--cpus=1','--memory=512m','--cap-drop=ALL','--cap-add=NET_RAW','--env-file',str(file.resolve()),'-v',str(root/'bin/packmate')+':/packmate:ro','-v',str(root/'web/dist')+':/web:ro','--entrypoint','/packmate','alpine:3.22')
headers = {'Authorization':'Basic '+base64.b64encode((values['WEB_LOGIN']+':'+values['WEB_PASSWORD']).encode()).decode(),'Content-Type':'application/json'}
for attempt in range(30):
    try:
        with urllib.request.urlopen(urllib.request.Request('http://127.0.0.1:16500/health/ready',headers=headers),timeout=2) as r:
            if r.status == 200: break
    except OSError: time.sleep(1)
else: raise SystemExit('fixture did not become ready')
for path, body in [('/service/',{'port':18080,'name':'Load fixture'}),('/pattern/',{'name':'Synthetic flags','value':'FLAG_','color':'#ff0000','searchType':'SUBSTRING','directionType':'OUTPUT','actionType':'FIND'})]:
    with urllib.request.urlopen(urllib.request.Request('http://127.0.0.1:16500/api'+path,data=json.dumps(body).encode(),headers=headers)) as r:
        r.read()
print('Fixture ready on http://127.0.0.1:16500; load target http://127.0.0.1:18080')
