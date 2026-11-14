#!/usr/bin/env python3
"""Read-only marker verification on the lab DB. No load generation on Kali."""
import json
import os
from pathlib import Path
import re
import subprocess
import sys

os.chdir(Path(__file__).resolve().parent)
run=sys.argv[1]
if not re.fullmatch(r'[a-zA-Z0-9_-]{1,100}',run):raise SystemExit('invalid run')
sql=f"""SELECT coalesce(json_object_agg(protocol,n),'{{}}'::json) FROM (
SELECT protocol,count(DISTINCT marker) n FROM (
SELECT (m)[1] protocol,(m)[2] marker FROM gomate_packets p
CROSS JOIN LATERAL regexp_matches(encode(p.content,'escape'),'FLAG_{run}_(http|tcp|grpc-stream|grpc)_([0-9]+(?:_[0-9]+)?)','g') m
WHERE NOT p.incoming) t GROUP BY protocol) counts"""
raw=subprocess.check_output(['docker','compose','exec','-T','db','psql','-U','gomate','-d','gomate','-tAc',sql],text=True,timeout=15)
print(json.dumps(json.loads(raw)))
