#!/usr/bin/env python3
"""Run race + integration tests against this project's isolated test database."""
import os
from pathlib import Path
import shutil
import subprocess
from urllib.parse import quote

root = Path(__file__).resolve().parents[1]
os.chdir(root)
values = dict(line.split('=', 1) for line in (root / '.env').read_text().splitlines()
              if line and not line.startswith('#'))
subprocess.run(['docker', 'compose', 'up', '-d', '--wait', 'db'], check=True)
psql = ['docker', 'compose', 'exec', '-T', 'db', 'psql', '-U', 'gomate', '-d', 'postgres']
exists = subprocess.check_output(psql + ['-tAc', "SELECT 1 FROM pg_database WHERE datname='gomate_test'"], text=True)
if exists.strip() != '1':
    subprocess.run(psql + ['-c', 'CREATE DATABASE gomate_test'], check=True)
env = os.environ.copy()
env['TEST_DATABASE_URL'] = f"postgres://gomate:{quote(values['DB_PASSWORD'], safe='')}@127.0.0.1:{values.get('DB_PORT', '65432')}/gomate_test?sslmode=disable"
go = shutil.which('go') or str(root / '.tools/go/bin/go')
raise SystemExit(subprocess.call([go, 'test', '-race', './...'], env=env))
