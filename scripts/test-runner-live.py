#!/usr/bin/env python3
"""Exercise disposable script containers using the local Docker daemon only."""
import json, os, subprocess, time, urllib.request, urllib.error
from pathlib import Path
root=Path(__file__).resolve().parents[1]
os.chdir(root)
base='http://127.0.0.1:65003'
token='gomate-local-runner-test-token-only'
env={**os.environ,'DISCOVERY_LISTEN':'127.0.0.1:65003','DOCKER_DISCOVERY_TOKEN':token,'RUNNER_IMAGE':'gomate-runner:latest'}
log=open('.cache/runner-live-gateway.log','w')
process=subprocess.Popen(['.cache/discoverer-runner-test'],env=env,stdout=log,stderr=log)
ids=[];report={'passed':False,'engine':'local Docker','checks':[]}
def call(path,method='GET',data=None):
 request=urllib.request.Request(base+path,method=method,data=json.dumps(data).encode() if data is not None else None,headers={'Authorization':'Bearer '+token,'Content-Type':'application/json'})
 with urllib.request.urlopen(request,timeout=10) as response:return json.load(response)
def start(language,code,timeout=30):
 job=call('/runner/jobs','POST',{'language':language,'code':code,'timeoutSeconds':timeout});ids.append(job['id']);return job['id']
def wait(job_id,limit=45):
 deadline=time.monotonic()+limit
 while time.monotonic()<deadline:
  result=call('/runner/jobs/'+job_id)
  if result['status'] not in ['starting','running','stopping']:return result
  time.sleep(.15)
 raise AssertionError('job did not finish')
try:
 for _ in range(80):
  try:
   if call('/runner')['available']:break
  except Exception:pass
  time.sleep(.15)
 else:raise AssertionError('runner unavailable')
 code="import os,sys,requests,pwn\nprint('UID',os.getuid(),flush=True)\nprint('SOCKET',os.path.exists('/var/run/docker.sock'))\nprint('SECRETS',any(k in os.environ for k in ['WEB_PASSWORD','DATABASE_URL','DOCKER_DISCOVERY_TOKEN']))\nprint('python stdout')\nprint('python stderr',file=sys.stderr)"
 result=wait(start('python',code));assert result['status']=='finished' and result['exitCode']==0,result
 assert 'UID 65532' in result['stdout'] and 'SOCKET False' in result['stdout'] and 'SECRETS False' in result['stdout'],result
 assert 'python stderr' in result['stderr'];report['checks'].append('Python dependencies, nonroot, no socket or application secrets, stdout/stderr')
 result=wait(start('bash',"printf 'bash stdout'; printf 'bash stderr' >&2; exit 7"));assert result['exitCode']==7 and result['stdout']=='bash stdout' and result['stderr']=='bash stderr',result
 report['checks'].append('edited Bash script and nonzero exit')
 result=wait(start('python','#'+'x'*150000+'\nprint("large argv safe")'));assert result['exitCode']==0 and 'large argv safe' in result['stdout'],result
 report['checks'].append('script larger than Linux single-argument limit')
 result=wait(start('python','import time\nprint("before timeout",flush=True)\ntime.sleep(60)',2));assert result['status']=='timed_out',result
 report['checks'].append('daemon timeout kills and removes container')
 job_id=start('python','import subprocess,sys,time\nsubprocess.Popen([sys.executable,"-c","import time; time.sleep(60)"])\nprint("child started",flush=True)\ntime.sleep(60)')
 for _ in range(60):
  if 'child started' in call('/runner/jobs/'+job_id)['stdout']:break
  time.sleep(.15)
 else:raise AssertionError('no incremental output')
 inspection=json.loads(subprocess.check_output(['docker','inspect','gomate-run-'+job_id],text=True))[0]
 policy=inspection['HostConfig'];assert policy['ReadonlyRootfs'] and policy['NetworkMode']=='host' and policy['PidsLimit']==64 and policy['Memory']==512*1024*1024 and policy['NanoCpus']==500000000
 assert not inspection['Mounts'] or all(m.get('Type')=='tmpfs' for m in inspection['Mounts'])
 call('/runner/jobs/'+job_id,'DELETE');result=wait(job_id);assert result['status']=='canceled',result
 report['checks'].append('incremental output, container resource policy, cancellation including child process')
 result=wait(start('python','print("x"*600000)'));assert result['truncated'] and len(result['stdout'].encode())+len(result['stderr'].encode())<=524288,result['status']
 report['checks'].append('bounded output')
 active=[start('python','import time; time.sleep(30)') for _ in range(2)]
 try:start('bash','echo overflow')
 except urllib.error.HTTPError as e:assert e.code==429
 else:raise AssertionError('concurrency limit not enforced')
 for job_id in active:call('/runner/jobs/'+job_id,'DELETE');wait(job_id)
 report['checks'].append('two concurrent jobs, third rejected')
 leftovers=subprocess.check_output(['docker','ps','-aq','--filter','label=gomate.runner.owner=gomate-script-runner-v1'],text=True).strip();assert not leftovers,leftovers
 report['checks'].append('no leftover execution containers');report['passed']=True
finally:
 for job_id in ids:
  try:call('/runner/jobs/'+job_id,'DELETE');wait(job_id,limit=15)
  except Exception:pass
 process.terminate()
 try:process.wait(timeout=5)
 except subprocess.TimeoutExpired:process.kill();process.wait()
 log.close();Path('reports/runner-live.json').write_text(json.dumps(report,indent=2)+'\n')
print(json.dumps(report))
