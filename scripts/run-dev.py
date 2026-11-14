#!/usr/bin/env python3
"""Run the built binary with local Compose credentials; capture is opt-in."""
import os
from pathlib import Path
from urllib.parse import quote

root = Path(__file__).resolve().parents[1]
os.chdir(root)
values = dict(line.split('=', 1) for line in (root / '.env').read_text().splitlines()
              if line and not line.startswith('#'))
for name, value in values.items():
    os.environ.setdefault(name, value)
os.environ.setdefault('DATABASE_URL', f"postgres://gomate:{quote(values['DB_PASSWORD'], safe='')}@127.0.0.1:{values.get('DB_PORT', '65432')}/gomate?sslmode=disable")
os.execv(str(root / 'bin/packmate'), ['packmate'])
