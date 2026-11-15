import {test} from 'node:test';
import assert from 'node:assert/strict';
import http from 'node:http';
import net from 'node:net';
import dgram from 'node:dgram';
import {spawn} from 'node:child_process';
import {prepareExport,generateExport,MAX_EXPORT_BYTES} from '../../web/src/requestExport.mjs';
const packet=(raw,extra={})=>({incoming:true,kind:'raw',content:Buffer.from(raw,'latin1').toString('base64'),...extra});
const exported=(raw,format='curl',extra={})=>generateExport({packet:packet(raw),format,host:'127.0.0.1',port:8080,...extra});
function child(command,args,source){
 return new Promise((resolve,reject)=>{
  const process=spawn(command,args,{stdio:['pipe','pipe','pipe']});let stdout='',stderr='';
  const timer=setTimeout(()=>{process.kill('SIGKILL');reject(Error('test child timeout'))},12000);
  process.stdout.on('data',chunk=>stdout=(stdout+chunk).slice(-16384));process.stderr.on('data',chunk=>stderr=(stderr+chunk).slice(-16384));
  process.on('error',error=>{clearTimeout(timer);reject(error)});process.on('close',code=>{clearTimeout(timer);code===0?resolve({stdout,stderr}):reject(Error(command+' exited '+code+': '+stderr))});
  process.stdin.end(source);
 });
}
const compile=code=>child('python3',['-c','import sys; compile(sys.stdin.read(), "<generated>", "exec")'],code);

test('HTTP framing preserves binary bodies and splits complete pipelines',()=>{
 const first='POST /one HTTP/1.1\r\nHost: original\r\nContent-Length: 3\r\n\r\n\x00\xffx';
 const second='GET /two HTTP/1.1\r\nHost: original\r\n\r\n';
 const p=prepareExport(packet(first+second),{server:'[::1]:9000',service:8080});
 assert.equal(p.httpError,'');assert.equal(p.requests.length,2);assert.equal(p.requests[0].body,'\x00\xffx');assert.equal(p.requests[0].raw,first);assert.equal(p.requests[1].raw,second);assert.deepEqual(p.endpoint,{host:'::1',port:9000,scheme:'http'});
 assert.match(generateExport({packet:packet(first+second),format:'curl',host:'::1',port:9000,requestIndex:1}).code,/--request-target '\/two'/);
 assert.deepEqual(prepareExport(packet(second),{service:8123}).endpoint,{host:'original',port:8123,scheme:'http'});
 assert.deepEqual(prepareExport(packet('GET / HTTP/1.1\r\nHost: [::1]:444\r\n\r\n')).endpoint,{host:'::1',port:444,scheme:'http'});
});
test('partial, ambiguous and malformed HTTP is rejected without truncating raw',()=>{
 const cases=['GET / HTTP/1.1\r\nHost: x\r\n','POST / HTTP/1.1\r\nContent-Length: 5\r\n\r\nabc','GET / HTTP/1.1\r\n\r\nGET /partial','POST / HTTP/1.1\r\nContent-Length: 1\r\nContent-Length: 1\r\n\r\nx','POST / HTTP/1.1\r\nContent-Length: 1\r\nTransfer-Encoding: chunked\r\n\r\nx','GET / HTTP/1.1\r\n Host: x\r\n\r\n','GET / HTTP/1.1\nHost: x\n\n'];
 for(const raw of cases){const p=prepareExport(packet(raw));assert(p.httpError,raw);assert.equal(p.requests.length,0);assert.equal(p.raw,raw);assert.equal(exported(raw,'requests').code,'');assert(exported(raw,'pwntools').code)}
 assert(prepareExport(packet('x'.repeat(MAX_EXPORT_BYTES+1))).httpError.includes('256 KiB'));
 assert.equal(generateExport({packet:packet('x'.repeat(MAX_EXPORT_BYTES+1)),format:'pwntools',host:'localhost',port:1}).code,'');
});
test('chunked bodies are assembled with bounded validation and raw wire preserved',()=>{
 const raw='POST / HTTP/1.1\r\nHost: x\r\nTransfer-Encoding: chunked\r\nTrailer: X-Trailer\r\n\r\n3;name=value\r\na\x00b\r\n2\r\ncd\r\n0\r\nX-Trailer: done\r\n\r\n';
 const p=prepareExport(packet(raw));assert.equal(p.httpError,'');assert.equal(p.requests[0].body,'a\x00bcd');assert.equal(p.requests[0].raw,raw);assert(p.warnings.some(v=>v.includes('trailers')));
 const curl=exported(raw);assert(!curl.code.includes('--header \'Transfer-Encoding'));assert(!curl.code.includes('--header \'Trailer:'));
 for(const suffix of ['3\r\nab','3\r\nabcXX','0\r\n','ffffffffffff\r\n'])assert(prepareExport(packet('POST / HTTP/1.1\r\nTransfer-Encoding: chunked\r\n\r\n'+suffix)).httpError);
});
test('decoded HTTP restores metadata target, removes stale framing/gzip and rejects irreversible form decoding',()=>{
 const raw='POST /hello world HTTP/1.1\r\nHost: example\r\nContent-Length: 999\r\nTransfer-Encoding: chunked\r\nContent-Encoding: gzip\r\nContent-Type: text/plain\r\n\r\nhi\x00';
 const selected=packet(raw,{kind:'http',ungzipped:true,metadata:{method:'POST',path:'/hello%20world'}});
 const p=prepareExport(selected);assert.equal(p.httpError,'');assert.equal(p.requests[0].target,'/hello%20world');assert.equal(p.requests[0].body,'hi\x00');assert(!p.requests[0].headers.some(h=>/content-length|transfer-encoding|content-encoding/i.test(h.name)));assert(p.requests[0].raw.endsWith('Content-Length: 3\r\n\r\nhi\x00'));
 const polluted=packet('GET /x\r\nX-Fake: yes HTTP/1.1\r\nHost: example\r\n\r\n',{kind:'http',metadata:{method:'GET',path:'/x%0D%0AX-Fake:%20yes'}});
 const restored=prepareExport(polluted);assert.equal(restored.httpError,'');assert(!restored.requests[0].headers.some(h=>h.name==='X-Fake'));
 const form=packet('POST / HTTP/1.1\r\nHost: example\r\nContent-Type: application/x-www-form-urlencoded\r\n\r\na=b c',{kind:'http',metadata:{method:'POST',path:'/'}});
 for(const format of ['requests','curl','pwntools'])assert.equal(generateExport({packet:form,service:{urldecodeHttpRequests:true},format,host:'127.0.0.1',port:1}).code,'');
 assert(prepareExport(packet('malformed',{kind:'http'})).httpError);
});
test('unsupported representations and response/incomplete blocks do not pretend to be HTTP requests',()=>{
 const raw='GET / HTTP/1.1\r\nHost: x\r\n\r\n';
 for(const kind of ['grpc','http2-headers','http2-body','websocket'])for(const format of ['requests','curl','pwntools'])assert.equal(exported(raw,format,{packet:packet(raw,{kind})}).code,'');
 for(const format of ['requests','curl','pwntools'])assert.equal(exported(raw,format,{packet:packet(raw,{incoming:false})}).code,'');
 for(const format of ['requests','curl'])assert.equal(exported(raw,format,{packet:packet(raw,{incomplete:true})}).code,'');
 assert(exported(raw,'pwntools',{packet:packet(raw,{incomplete:true})}).code);
 assert.equal(exported('HTTP/1.1 200 OK\r\nContent-Length: 0\r\n\r\n','curl').code,'');
 assert.equal(exported('PRI * HTTP/2.0\r\n\r\nSM\r\n\r\n','requests').code,'');
 assert(exported('PRI * HTTP/2.0\r\n\r\nSM\r\n\r\n','pwntools').warnings.some(v=>v.includes('HTTP/2/TLS')));
 assert.match(exported('\x00\x01','pwntools',{stream:{protocol:'udp'}}).code,/typ="udp"/);
 assert.equal(exported(raw,'requests',{stream:{protocol:'udp'}}).code,'');
});
test('duplicate headers reject requests while curl and raw pwntools preserve them',()=>{
 const raw='GET / HTTP/1.1\r\nHost: x\r\nX-Repeat: one\r\nx-repeat: two\r\nCookie: secret\r\nAuthorization: Bearer secret\r\n\r\n';
 assert.match(exported(raw,'requests').error,/duplicate/);assert.equal((exported(raw,'curl').code.match(/--header '[Xx]-[Rr]epeat:/g)||[]).length,2);assert(exported(raw,'pwntools').code.includes('X-Repeat: one\\r\\n'));
});
test('all generated Python compiles with captured quotes/backticks/shell syntax as data',async()=>{
 const raw='POST /a\'$(printf%20oops) HTTP/1.1\r\nHost: captured.example\r\nX-Quoted: \'";print("oops");# $(printf injected) `printf injected` \\ tail\r\nContent-Length: 4\r\n\r\n\xff\x00\'"';
 for(const format of ['requests','pwntools']){const result=exported(raw,format);assert.equal(result.error,'');await compile(result.code)}
 assert.equal(generateExport({packet:packet(raw),format:'curl',host:'localhost;printf bad',port:1}).code,'');
 assert.equal(generateExport({packet:packet(raw),format:'curl',host:'localhost',port:'1;printf bad'}).code,'');
 assert.equal(generateExport({packet:packet(raw),format:'curl',host:'localhost',port:1,scheme:'file'}).code,'');
 assert.doesNotThrow(()=>prepareExport(null,null,null));assert.equal(generateExport().code,'');assert.equal(generateExport(null).code,'');
});
test('generated requests/curl run only against a local recording server and preserve body/header bytes',async()=>{
 const captures=[];const server=http.createServer((req,res)=>{const chunks=[];req.on('data',c=>chunks.push(c));req.on('end',()=>{captures.push({method:req.method,url:req.url,headers:req.headers,rawHeaders:req.rawHeaders,body:Buffer.concat(chunks)});res.end('recorded')})});
 await new Promise(resolve=>server.listen(0,'127.0.0.1',resolve));const port=server.address().port;
 const body=Buffer.from([0,255,13,10,39,34,96,36,40,41]);const header='quotes \'" $(printf injected) `printf injected` \\';
 try{for(const format of ['requests','curl']){
  const target=format==='curl'?"/a/../b?x=%2f&quote='$(printf%20injected)'":'/record';
  const raw='POST '+target+' HTTP/1.1\r\nHost: captured.example\r\nAuthorization: Bearer retained\r\nCookie: auth=retained\r\nX-Quoted: '+header+'\r\nContent-Type: application/octet-stream\r\n'+(format==='curl'?'X-Repeat: one\r\nX-Repeat: two\r\nX-Bytes: \xff\r\n':'')+'Content-Length: '+body.length+'\r\n\r\n'+body.toString('latin1');
  const result=generateExport({packet:packet(raw),format,host:'127.0.0.1',port});assert.equal(result.error,'');
  await child(format==='curl'?'sh':'python3',format==='curl'?['-s']:['-'],result.code);
  const got=captures.at(-1);assert.equal(got.method,'POST');assert.equal(got.url,target);assert.deepEqual(got.body,body);assert.equal(got.headers.host,'captured.example');assert.equal(got.headers.authorization,'Bearer retained');assert.equal(got.headers.cookie,'auth=retained');assert.equal(got.headers['x-quoted'],header);assert.equal(Number(got.headers['content-length']),body.length);
  if(format==='curl'){assert.equal(got.rawHeaders.filter(v=>v==='X-Repeat').length,2);assert.equal(got.headers['x-bytes'],'\xff')}
 }}finally{await new Promise(resolve=>server.close(resolve))}
});
test('generated pwntools preserves raw TCP/UDP bytes and exits without an interactive TTY',async()=>{
 const raw='\x00\xff\x01hello';let received=Buffer.alloc(0);
 const server=net.createServer(socket=>{socket.on('data',chunk=>{received=Buffer.concat([received,chunk]);if(received.length>=raw.length)socket.end('received')})});
 await new Promise(resolve=>server.listen(0,'127.0.0.1',resolve));
 try{const result=generateExport({packet:packet(raw),stream:{protocol:'tcp'},format:'pwntools',host:'127.0.0.1',port:server.address().port});assert.equal(result.error,'');assert(!result.code.includes('.interactive('));await child('python3',['-'],result.code);assert.deepEqual(received,Buffer.from(raw,'latin1'))}finally{await new Promise(resolve=>server.close(resolve))}
 const udp=dgram.createSocket('udp4');let datagram;
 udp.on('message',(message,peer)=>{datagram=message;udp.send(Buffer.from('received'),peer.port,peer.address)});await new Promise(resolve=>udp.bind(0,'127.0.0.1',resolve));
 try{const result=generateExport({packet:packet(raw),stream:{protocol:'udp'},format:'pwntools',host:'127.0.0.1',port:udp.address().port});await child('python3',['-'],result.code);assert.deepEqual(datagram,Buffer.from(raw,'latin1'))}finally{udp.close()}
});

test('readable ASCII and UTF-8 exports remain byte exact, including UTF-8 BOM',async()=>{
 const captures=[];const server=http.createServer((req,res)=>{const chunks=[];req.on('data',chunk=>chunks.push(chunk));req.on('end',()=>{captures.push(Buffer.concat(chunks));res.end('OK')})});
 await new Promise(resolve=>server.listen(0,'127.0.0.1',resolve));
 try{for(const text of ['editable "body" \' $(printf untouched) `printf untouched`\r\nnext','Привет 🦊','\ufeffBOM stays'])for(const format of ['requests','curl']){
  const body=Buffer.from(text,'utf8'),raw='POST / HTTP/1.1\r\nHost: original\r\nContent-Length: '+body.length+'\r\n\r\n'+body.toString('latin1');
  const result=generateExport({packet:packet(raw),format,host:'127.0.0.1',port:server.address().port});assert.equal(result.error,'');assert(!result.code.includes('base64.b64decode'));assert(!result.code.includes('| base64 --decode'));
  await child(format==='curl'?'sh':'python3',format==='curl'?['-s']:['-'],result.code);assert.deepEqual(captures.at(-1),body);
 }}finally{await new Promise(resolve=>server.close(resolve))}
 const wire='POST /editable HTTP/1.1\r\nHost: original\r\nContent-Length: 5\r\n\r\nhello';let received=Buffer.alloc(0);
 const tcp=net.createServer(socket=>socket.on('data',chunk=>{received=Buffer.concat([received,chunk]);if(received.length>=wire.length)socket.end('OK')}));await new Promise(resolve=>tcp.listen(0,'127.0.0.1',resolve));
 try{const result=generateExport({packet:packet(wire),format:'pwntools',host:'127.0.0.1',port:tcp.address().port});assert(result.code.includes('b"POST /editable HTTP/1.1\\r\\n"'));assert(result.code.includes('b"hello"'));assert(!result.code.includes('base64'));await child('python3',['-'],result.code);assert.deepEqual(received,Buffer.from(wire))}finally{await new Promise(resolve=>tcp.close(resolve))}
});
