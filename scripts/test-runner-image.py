#!/usr/bin/env python3
"""Run inside the runner image: docker run ... -i IMAGE python3 - < this_file."""
import http.server
import json
import os
import socket
import subprocess
import tempfile
import threading

import requests
from pwn import ELF, context, disasm, remote
from unicorn import Uc, UC_ARCH_X86, UC_MODE_64
from unicorn.x86_const import UC_X86_REG_RAX
from cryptography.hazmat.primitives.ciphers.aead import AESGCM
from zstandard import backend_c, backend_cffi

checks = []
assert os.getuid() == 65532
checks.append('nonroot Python and dependency imports')


class Handler(http.server.BaseHTTPRequestHandler):
    def do_POST(self):
        body = self.rfile.read(int(self.headers['Content-Length']))
        self.send_response(200)
        self.end_headers()
        self.wfile.write(body)

    def log_message(self, *args):
        pass


server = http.server.ThreadingHTTPServer(('127.0.0.1', 0), Handler)
threading.Thread(target=server.serve_forever, daemon=True).start()
url = 'http://127.0.0.1:%d/api/echo' % server.server_port
payload = b'gomate runner smoke test'
try:
    response = requests.post(url, data=payload, timeout=5)
    response.raise_for_status()
    assert response.content == payload
    assert subprocess.check_output(
        ['bash', '-c', 'curl --fail --silent --show-error --max-time 5 --data-binary "$1" "$2"',
         'smoke', payload.decode(), url], timeout=10) == payload
finally:
    server.shutdown()
    server.server_close()
checks.append('requests and Bash/curl HTTP POST round trips')

context.clear(arch='amd64', log_level='error')
listener = socket.socket()
listener.bind(('127.0.0.1', 0))
listener.listen(1)
listener.settimeout(5)


def echo():
    connection, _ = listener.accept()
    with connection:
        connection.settimeout(5)
        connection.sendall(connection.recv(1024))


thread = threading.Thread(target=echo, daemon=True)
thread.start()
with remote('127.0.0.1', listener.getsockname()[1], timeout=5) as tube:
    tube.sendline(payload)
    assert tube.recvline() == payload + b'\n'
thread.join(timeout=5)
listener.close()
checks.append('pwntools TCP send/receive')

# Exercise the installed GNU tools directly: pwntools.asm also needs cpp,
# which is not included in either the original image or this smaller image.
with tempfile.TemporaryDirectory() as directory:
    obj = directory + '/sample.o'
    binary = directory + '/sample.bin'
    subprocess.run(['as', '--64', '-o', obj],
                   input=b'.text\n.global _start\n_start:\nmov $42, %rax\n', check=True)
    subprocess.run(['objcopy', '-O', 'binary', '-j', '.text', obj, binary], check=True)
    with open(binary, 'rb') as stream:
        machine_code = stream.read()
assert 'mov' in disasm(machine_code)
assert ELF('/bin/bash', checksec=False).arch == 'amd64'
import capstone
assert list(capstone.Cs(capstone.CS_ARCH_X86, capstone.CS_MODE_64).disasm(machine_code, 0))
emulator = Uc(UC_ARCH_X86, UC_MODE_64)
emulator.mem_map(0x1000, 0x1000)
emulator.mem_write(0x1000, machine_code)
emulator.emu_start(0x1000, 0x1000 + len(machine_code))
assert emulator.reg_read(UC_X86_REG_RAX) == 42
checks.append('ELF, GNU assembler/disassembler, Capstone and Unicorn execution')

for backend in (backend_c, backend_cffi):
    compressed = backend.ZstdCompressor().compress(payload)
    assert backend.ZstdDecompressor().decompress(compressed) == payload
cipher = AESGCM(AESGCM.generate_key(bit_length=128))
nonce = os.urandom(12)
assert cipher.decrypt(nonce, cipher.encrypt(nonce, payload, None), None) == payload
checks.append('both zstandard backends and cryptography native libraries')
print(json.dumps({'passed': True, 'checks': checks}, indent=2))
