#!/usr/bin/env python3
"""GoMate farm client. Python 3.9+, standard library only.
GOMATE_USER / GOMATE_PASSWORD contain the GoMate login.
Usage: python3 gomate_client.py --server http://host:65000 --name demo -- python3 sploit.py
The target IP is appended as the last argument to each exploit invocation.
"""
import argparse
import base64
import concurrent.futures
import json
import os
import re
import signal
import sqlite3
import subprocess
import tempfile
import time
import urllib.request


def api(args, path, body=None):
    credentials = (os.environ.get('GOMATE_USER', 'packmate') + ':' + os.environ.get('GOMATE_PASSWORD', '')).encode()
    headers = {'Authorization': 'Basic ' + base64.b64encode(credentials).decode(), 'Content-Type': 'application/json'}
    request = urllib.request.Request(args.server.rstrip('/') + '/api/v2/farm/' + path,
                                    data=None if body is None else json.dumps(body).encode(), headers=headers)
    with urllib.request.urlopen(request, timeout=10) as response:
        return json.loads(response.read(2 * 1024 * 1024))


def attack(command, target, config):
    # A file avoids pipe deadlocks. Stop output floods before reading into memory.
    with tempfile.TemporaryFile() as output:
        proc = subprocess.Popen(command + [target['host']], stdout=output, stderr=subprocess.DEVNULL,
                                start_new_session=(os.name == 'posix'))
        deadline = time.monotonic() + config['timeout']
        try:
            while proc.poll() is None:
                if time.monotonic() >= deadline or os.fstat(output.fileno()).st_size > 1048576:
                    break
                time.sleep(.05)
        finally:
            if os.name == 'posix':
                try:
                    os.killpg(proc.pid, signal.SIGKILL)
                except ProcessLookupError:
                    pass
            elif proc.poll() is None:
                proc.kill()
            proc.wait()
        output.seek(0)
        text = output.read(1048576).decode(errors='replace')
        return target['name'], list(dict.fromkeys(m.group(0) for m in re.finditer(config['flagFormat'], text)))[:1000]


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--server', required=True)
    parser.add_argument('--name', required=True)
    parser.add_argument('--spool', default='gomate-flags.sqlite3')
    parser.add_argument('--once', action='store_true')
    parser.add_argument('command', nargs=argparse.REMAINDER)
    args = parser.parse_args()
    command = args.command[1:] if args.command[:1] == ['--'] else args.command
    if not command:
        parser.error('specify exploit command after --')
    db = sqlite3.connect(args.spool)
    db.execute('CREATE TABLE IF NOT EXISTS flags(flag TEXT PRIMARY KEY, team TEXT, sploit TEXT, created REAL)')
    while True:
        started = time.monotonic()
        try:
            config = api(args, 'config')
            with concurrent.futures.ThreadPoolExecutor(max_workers=config['workers']) as pool:
                futures = [pool.submit(attack, command, target, config) for target in config['targets']]
                for future in concurrent.futures.as_completed(futures):
                    try:
                        team, flags = future.result()
                        db.executemany('INSERT OR IGNORE INTO flags VALUES(?,?,?,?)', [(f, team, args.name, time.time()) for f in flags])
                        db.commit()
                    except Exception as error:
                        print('Exploit failed:', error, flush=True)
            # Bounded durable outbox; server deduplication makes retries idempotent.
            db.execute('DELETE FROM flags WHERE created < ?', (time.time() - 86400,))
            db.execute('DELETE FROM flags WHERE rowid NOT IN (SELECT rowid FROM flags ORDER BY created DESC LIMIT 100000)')
            db.commit()
            for flag, team, sploit in db.execute('SELECT flag,team,sploit FROM flags LIMIT 1000').fetchall():
                api(args, 'flags', {'flags': [flag], 'team': team, 'sploit': sploit})
                db.execute('DELETE FROM flags WHERE flag=?', (flag,))
            db.commit()
            print('Round complete; pending flags:', db.execute('SELECT count(*) FROM flags').fetchone()[0], flush=True)
        except Exception as error:
            print('Farm unavailable; outbox retained:', error, flush=True)
            config = {'interval': 5}
        if args.once:
            break
        time.sleep(max(0, config['interval'] - (time.monotonic() - started)))
    db.close()


if __name__ == '__main__':
    try:
        main()
    except KeyboardInterrupt:
        pass
