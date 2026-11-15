import base64
import json
import os
from pathlib import Path
import subprocess
import sys
import tempfile
import threading
import unittest
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

CLIENT = Path(__file__).resolve().parents[1] / 'internal/farm/client.py'

class ClientTest(unittest.TestCase):
    def test_downloaded_client_executes_and_submits_with_auth(self):
        posted = []
        case = self
        class Handler(BaseHTTPRequestHandler):
            def log_message(self, *_): pass
            def do_GET(self):
                case.assertEqual(self.headers['Authorization'], 'Basic ' + base64.b64encode(b'test:pass').decode())
                data = {'targets': [{'name': 'mock', 'host': '127.0.0.1'}], 'flagFormat': '[A-Z0-9]{31}=', 'interval': 1, 'workers': 2, 'timeout': 2}
                self.send_response(200); self.end_headers(); self.wfile.write(json.dumps(data).encode())
            def do_POST(self):
                posted.append(json.loads(self.rfile.read(int(self.headers['Content-Length']))))
                self.send_response(200); self.end_headers(); self.wfile.write(b'{"inserted":1}')
        server = ThreadingHTTPServer(('127.0.0.1', 0), Handler)
        thread = threading.Thread(target=server.serve_forever, daemon=True); thread.start()
        try:
            with tempfile.TemporaryDirectory() as folder:
                exploit = Path(folder) / 'sploit.py'
                exploit.write_text('print("A" * 31 + "=", flush=True)\n')
                result = subprocess.run([sys.executable, str(CLIENT), '--server', f'http://127.0.0.1:{server.server_port}', '--name', 'test', '--once', '--spool', str(Path(folder)/'outbox.db'), '--', sys.executable, str(exploit)], env={**os.environ, 'GOMATE_USER': 'test', 'GOMATE_PASSWORD': 'pass'}, capture_output=True, timeout=15)
                self.assertEqual(result.returncode, 0, result.stderr)
                self.assertEqual(posted, [{'flags': ['A'*31+'='], 'team': 'mock', 'sploit': 'test'}])
        finally:
            server.shutdown(); server.server_close(); thread.join()

if __name__ == '__main__': unittest.main()
