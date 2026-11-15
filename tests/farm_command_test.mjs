import assert from 'node:assert/strict';
import {execFileSync} from 'node:child_process';
import {mkdtempSync, rmSync, writeFileSync} from 'node:fs';
import {tmpdir} from 'node:os';
import {join} from 'node:path';
import test from 'node:test';
import {farmClientCommand} from '../web/src/farmClient.mjs';

test('launch command preserves credentials and arguments literally in a POSIX shell', () => {
 const cwd = mkdtempSync(join(tmpdir(), 'gomate-command-'));
 try {
  // This stand-in only reports its arguments; no client is run.
  writeFileSync(join(cwd, 'gomate_client.py'), 'import json, sys\nprint(json.dumps(sys.argv[1:]))\n');
  const input = {username: "user ' name", password: "--test'pass$(printf expanded)`printf expanded`;$HOME\nline", server: 'http://localhost:65000', filename: "example ' $(printf expanded).py"};
  const output = execFileSync('/bin/sh', ['-c', farmClientCommand(input)], {cwd, encoding:'utf8'});
  assert.deepEqual(JSON.parse(output), ['--server', input.server, '--user='+input.username, '--password='+input.password, '--name', input.filename, '--', 'python3', input.filename]);
 } finally {
  rmSync(cwd, {recursive:true, force:true});
 }
});
