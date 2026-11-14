import {test} from 'node:test';
import assert from 'node:assert/strict';
import {decode,detect,MAX_INPUT,MAX_RESULTS} from '../../web/src/trafficDecode.mjs';
const b64=value=>Buffer.from(value).toString('base64');
const url64=value=>Buffer.from(JSON.stringify(value)).toString('base64url');

test('Base64 variants preserve UTF-8 and whitespace and reject invalid input',()=>{
 const text="' OR 1=1 --\nПривет 🦊";
 for(const encoded of [b64(text),b64(text).replace(/=/g,''),Buffer.from(text).toString('base64url'),b64(text).match(/.{1,8}/g).join('\n')])assert.equal(decode(encoded,'base64').text,text);
 for(const invalid of ['a','abc===','@@@','aGVsbG8==','Zh=='])assert.throws(()=>decode(invalid,'base64'));
 assert.equal(decode('/wAA','base64').binary,true);
 assert.equal(decode('/wAA','base64').hex,'ff 00 00');
});
test('JWT separates header, claims and timestamp without claiming validation',()=>{
 const payload={sub:'admin',exp:1700000000,blob:b64("' OR 1=1 --")};
 const token=url64({alg:'none'})+'.'+url64(payload)+'.';
 const decoded=decode('Bearer '+token,'jwt');assert.deepEqual(JSON.parse(decoded.jwt.payload),payload);assert.match(decoded.warning,/не проверялась/);assert.equal(decoded.jwt.dates[0].value,'2023-11-14T22:13:20.000Z');
 assert(detect('Cookie: token='+token+'; other=plain').results.some(v=>v.operation==='jwt'));
 assert.throws(()=>decode('e30.e30.e30.e30.e30','jwt'),/JWE/);
 assert.throws(()=>decode(url64({alg:'none'})+'.'+url64([])+'.','jwt'),/JSON-объект/);
 assert.throws(()=>decode('e30.invalid.','jwt'));
});
test('manual URL, form, hex, JSON and nested decoding are explicit and lossless',()=>{
 assert.equal(decode('a+b%2Bc','url').text,'a+b+c');
 assert.equal(decode('a+b%2Bc','form').text,'a b+c');
 assert.throws(()=>decode('%ff','url'));assert.throws(()=>decode('%x1','url'));
 assert.equal(decode('0x53 45 4c 45 43 54','hex').text,'SELECT');
 assert.equal(decode('\\x53\\x51\\x4c','hex').text,'SQL');assert.throws(()=>decode('abc','hex'));
 assert.equal(decode('"\\u0027 OR 1=1 --"','json').text,"' OR 1=1 --");
 const first=decode(b64(b64('SELECT 1')),'base64');assert.equal(decode(first.text,'base64').text,'SELECT 1');
 assert.deepEqual(JSON.parse(decode('{"ok":true}','json').text),{ok:true});
});
test('auto-detection finds encoded values in query strings, headers and JSON',()=>{
 const sql="' OR 1=1 --",encoded=b64(sql);
 const input='GET /?blob='+encoded+'&x=%27%20OR%201%3D1%20-- HTTP/1.1\r\nAuthorization: Basic '+b64('admin:password')+'\r\n\r\n'+JSON.stringify({nested:b64(b64(sql))});
 const results=detect(input).results;
 assert(results.some(v=>v.operation==='base64'&&v.text===sql));
 assert(results.some(v=>v.operation==='url'&&v.text===sql));
 assert(results.some(v=>v.text==='admin:password'));
 assert(results.some(v=>v.text===encoded));
 for(const r of results)assert.equal(input.slice(r.offset,r.offset+r.source.length),r.source);
 assert.equal(detect('HTTP/1.1 200 OK\r\nContent-Type: text/plain').results.length,0);
});
test('bounded scanning, binary preview and hostile strings do not grow unbounded',()=>{
 assert.throws(()=>decode('a'.repeat(MAX_INPUT+1),'base64'),/Лимит/);
 assert.throws(()=>decode('['.repeat(1000)+'0'+']'.repeat(1000),'json'),/32 уровней/);
 const many=Array.from({length:100},(_,i)=>b64('SELECT '+i)).join(' '),scan=detect(many);
 assert.equal(scan.results.length,MAX_RESULTS);assert(scan.limited);assert(scan.attempts<=512);
 const binary=decode(Buffer.alloc(5000).toString('base64'),'base64');assert(binary.hexTruncated);assert.equal(binary.hex.split(' ').length,4096);
 const text='<img src=x onerror=alert(1)>';assert.equal(decode(b64(text),'base64').text,text);
 const start=performance.now();detect('a'.repeat(MAX_INPUT));assert(performance.now()-start<1000);
});
