#!/usr/bin/env python3
"""Synthetic A/D traffic: checker sessions behind one NAT address plus a few attacks."""
import json, random, string, struct, sys, time

out = open(sys.argv[1], 'wb')
out.write(struct.pack('<IHHiIII', 0xa1b2c3d4, 2, 4, 0, 0, 65535, 1))
CLIENT, SERVER, PORT = bytes([10, 80, 0, 1]), bytes([10, 0, 0, 1]), 8080
rnd = random.Random(7)
now = time.time() - 600
sport = 20000

def csum(b):
    if len(b) % 2: b += b'\0'
    s = sum(struct.unpack('!%dH' % (len(b) // 2), b))
    while s >> 16: s = (s & 0xffff) + (s >> 16)
    return ~s & 0xffff

def frame(src, dst, sp, dp, seq, ack, flags, data=b''):
    global now
    now += 0.002
    tcp = struct.pack('!HHIIBBHHH', sp, dp, seq, ack, 5 << 4, flags, 65535, 0, 0) + data
    pseudo = src + dst + struct.pack('!BBH', 0, 6, len(tcp))
    tcp = tcp[:16] + struct.pack('!H', csum(pseudo + tcp)) + tcp[18:]
    ip = struct.pack('!BBHHHBBH4s4s', 0x45, 0, 20 + len(tcp), 0, 0x4000, 64, 6, 0, src, dst)
    ip = ip[:10] + struct.pack('!H', csum(ip)) + ip[12:]
    eth = b'\x02\0\0\0\0\x02' + b'\x02\0\0\0\0\x01' + b'\x08\x00'
    pkt = eth + ip + tcp
    out.write(struct.pack('<IIII', int(now), int((now % 1) * 1e6), len(pkt), len(pkt)) + pkt)

def connection(exchanges):
    global sport
    sport += 1
    c, s = rnd.randrange(1 << 31), rnd.randrange(1 << 31)
    frame(CLIENT, SERVER, sport, PORT, c, 0, 0x02); c += 1
    frame(SERVER, CLIENT, PORT, sport, s, c, 0x12); s += 1
    frame(CLIENT, SERVER, sport, PORT, c, s, 0x10)
    for req, resp in exchanges:
        for i in range(0, len(req), 1400):
            frame(CLIENT, SERVER, sport, PORT, c, s, 0x18, req[i:i + 1400]); c += len(req[i:i + 1400])
        for i in range(0, len(resp), 1400):
            frame(SERVER, CLIENT, PORT, sport, s, c, 0x18, resp[i:i + 1400]); s += len(resp[i:i + 1400])
    frame(CLIENT, SERVER, sport, PORT, c, s, 0x11); c += 1
    frame(SERVER, CLIENT, PORT, sport, s, c, 0x11); s += 1
    frame(CLIENT, SERVER, sport, PORT, c, s, 0x10)

tok = lambda n, a=string.ascii_letters + string.digits: ''.join(rnd.choice(a) for _ in range(n))
flag = lambda: tok(31, string.ascii_uppercase + string.digits) + '='

def req(method, path, body=None, cookie=None, ctype='application/x-www-form-urlencoded', ua='python-requests/2.31.0'):
    h = f'{method} {path} HTTP/1.1\r\nHost: 10.0.0.1:8080\r\nUser-Agent: {ua}\r\nAccept: */*\r\n'
    if cookie: h += f'Cookie: session={cookie}\r\n'
    b = (body or '').encode()
    if body is not None: h += f'Content-Type: {ctype}\r\nContent-Length: {len(b)}\r\n'
    return h.encode() + b'\r\n' + b

def resp(body, status='200 OK', cookie=None, ctype='application/json'):
    b = body.encode()
    h = f'HTTP/1.1 {status}\r\nContent-Type: {ctype}\r\nContent-Length: {len(b)}\r\n'
    if cookie: h += f'Set-Cookie: session={cookie}; Path=/; HttpOnly\r\n'
    return h.encode() + b'\r\n' + b

notes = []
def checker():
    user, pw, f, cookie, nid = tok(rnd.randint(6, 10)).lower(), tok(rnd.randint(10, 16)), flag(), tok(32), rnd.randint(1000, 99999)
    connection([(req('POST', '/api/register', f'username={user}&password={pw}'), resp(json.dumps({'ok': True}), cookie=cookie)),
                (req('POST', '/api/notes', json.dumps({'title': tok(8), 'text': f}), cookie, 'application/json'), resp(json.dumps({'id': nid})))])
    cookie = tok(32)
    connection([(req('POST', '/api/login', f'username={user}&password={pw}'), resp(json.dumps({'ok': True}), cookie=cookie)),
                (req('GET', f'/api/notes/{nid}', cookie=cookie), resp(json.dumps({'id': nid, 'title': tok(8), 'text': f, 'owner': user})))])
    notes.append((nid, f, user))

def attacks():
    nid, f, user = rnd.choice(notes)
    return [
        # SQL injection in login
        lambda: connection([(req('POST', '/api/login', "username=admin'+OR+'1'%3D'1'--&password=x"), resp(json.dumps({'ok': True}), cookie=tok(32)))]),
        # IDOR: someone else's note without logging in as the owner
        lambda: connection([(req('GET', f'/api/notes/{nid}'), resp(json.dumps({'id': nid, 'title': tok(8), 'text': f, 'owner': user})))]),
        # hidden endpoint dumping everything
        lambda: connection([(req('GET', '/api/admin/export?all=1'), resp(json.dumps([{'id': n, 'text': x} for n, x, _ in notes[-40:]])))]),
        # SSTI in note text
        lambda: connection([(req('POST', '/api/register', f'username={tok(8).lower()}&password={tok(12)}'), resp('{"ok":true}', cookie=(c := tok(32)))),
                            (req('POST', '/api/notes', json.dumps({'title': tok(8), 'text': "{{config.__class__.__init__.__globals__['os'].popen('cat /flag*').read()}}"}), c, 'application/json'), resp('{"id":1}'))]),
        # mass assignment on register
        lambda: connection([(req('POST', '/api/register', f'username={tok(8).lower()}&password={tok(12)}&is_admin=true'), resp('{"ok":true}', cookie=tok(32)))]),
        # path traversal
        lambda: connection([(req('GET', '/api/notes/..%2f..%2f..%2fetc%2fpasswd'), resp('root:x:0:0:root:/root:/bin/bash\n', ctype='text/plain'))]),
    ]

for i in range(500):
    checker()
    if i > 250 and i % 25 == 0:
        a = attacks()
        a[(i // 25) % len(a)]()
for a in attacks():
    a()
print(len(notes), 'checker pairs')
