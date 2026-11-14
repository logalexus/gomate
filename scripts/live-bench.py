#!/usr/bin/env python3
"""Measure the isolated gomate-live-test fixture created by live-setup.py."""
import argparse
import base64
import json
import hashlib
from pathlib import Path
import subprocess
import time
import urllib.request

p = argparse.ArgumentParser()
p.add_argument('--rps', type=int, default=200)
p.add_argument('--seconds', type=int, default=30)
args = p.parse_args()
env = dict(l.split('=', 1) for l in Path('.env').read_text().splitlines() if l and not l.startswith('#'))
auth = base64.b64encode((env.get('WEB_LOGIN', 'packmate')+':'+env['WEB_PASSWORD']).encode()).decode()
def snapshot():
    req = urllib.request.Request('http://127.0.0.1:16500/api/v2/snapshot', headers={'Authorization': 'Basic '+auth})
    with urllib.request.urlopen(req, timeout=5) as r:
        return json.load(r)['metrics']

run = f'load{args.rps}-{time.time_ns()}'
before = snapshot()
result = subprocess.run(['bin/traffic', '-target', 'http://127.0.0.1:18080', '-duration', f'{args.seconds}s', '-rps', str(args.rps), '-agents', '200', '-concurrency', '512', '-prefix', run], capture_output=True, text=True)
generated = json.loads(result.stdout)
sql = f"SELECT count(DISTINCT m[1]) FROM gomate_packets p CROSS JOIN LATERAL regexp_matches(convert_from(p.content,'UTF8'), 'FLAG_{run}_([0-9]+)', 'g') AS m"
def count_markers():
    return int(subprocess.check_output(['docker','compose','exec','-T','db','psql','-U','gomate','-d','gomate_live','-tAc',sql], text=True).strip())

deadline = time.monotonic() + 15
drain_start = time.monotonic()
while True:
    stored = count_markers()
    if stored >= generated['success'] or time.monotonic() >= deadline:
        break
    time.sleep(0.5)
after = snapshot()
report = {'binary_sha256': subprocess.check_output(['docker','exec','gomate-live-test','sha256sum','/packmate'],text=True).split()[0], 'generator': generated, 'generator_exit': result.returncode, 'verification_after_generator_seconds': time.monotonic()-drain_start, 'unique_response_markers_in_db': stored, 'metrics_before': before, 'metrics_after': after, 'metrics_delta': {k: after[k]-before[k] for k in before if k.endswith('_total')}}
path = Path('reports') / f'live-{args.rps}rps-{args.seconds}s.json'
path.write_text(json.dumps(report, indent=2)+'\n')
print(json.dumps({'report': str(path), 'success': generated['success'], 'stored': stored, 'drops': {k: v for k, v in report['metrics_delta'].items() if 'drops' in k}}))
raise SystemExit(0 if result.returncode == 0 and stored == generated['success'] and not any(v for k, v in report['metrics_delta'].items() if 'drops' in k) else 1)
