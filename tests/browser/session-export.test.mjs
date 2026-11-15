import {test} from 'node:test';
import assert from 'node:assert/strict';
import http from 'node:http';
import net from 'node:net';
import {spawn} from 'node:child_process';
import {prepareSessionExport,generateSessionExport,MAX_SESSION_BYTES} from '../../web/src/requestExport.mjs';
import {loadSessionExport} from '../../web/src/sessionExport.mjs';
const packet=(raw,incoming=true,extra={})=>({incoming,content:Buffer.from(raw,'latin1').toString('base64'),...extra});
const stream={id:7,protocol:'TCP',server:'127.0.0.1:8080',packetsCount:4,sizeBytes:100,endTimestamp:10};
const request='GET /one HTTP/1.1\r\nHost: 127.0.0.1\r\n\r\n';
const generate=(packets,extra={})=>generateSessionExport({packets,stream,...extra});
function python(code,compile=false){return new Promise((resolve,reject)=>{
 const p=spawn('python3',compile?['-c','import sys; compile(sys.stdin.read(), "<generated>", "exec")']:['-'],{stdio:['pipe','pipe','pipe']});let out=Buffer.alloc(0),err='';const timer=setTimeout(()=>{p.kill('SIGKILL');reject(Error('timeout'))},12000);
 p.stdout.on('data',b=>out=Buffer.concat([out,b]));p.stderr.on('data',b=>err+=b);p.on('error',reject);p.on('close',exit=>{clearTimeout(timer);exit?reject(Error(err)):resolve(out)});p.stdin.end(code);
})}
test('session HTTP joins raw fragments, omits decoded copies and preserves all requests',()=>{
 const packets=[packet(request.slice(0,8)),packet(request.slice(8)),packet(request,true,{kind:'http'}),packet('HTTP/1.1 200 OK\r\n\r\n',false),packet('POST /two HTTP/1.1\r\nContent-Length: 3\r\n\r\n\x00\xffx')];
 const prepared=prepareSessionExport(packets,stream);assert.equal(prepared.error,'');assert.equal(prepared.httpError,'');assert.equal(prepared.requests.length,2);assert.equal(prepared.requests[1].body,'\x00\xffx');assert.equal(prepared.events.length,3);
 const result=generate(packets);assert.equal(result.error,'');assert.match(result.code,/requests.Session/);assert.equal(result.code.match(/"\/one"/g).length,1);assert.match(result.code,/"\/two"/);
});
test('whole-session export rejects gaps, cleared content, decoded-only, malformed and oversized input',()=>{
 for(const fields of [{incomplete:true},{payloadPurged:true},{protocol:'UDP'}])assert.equal(generate([packet(request)],{stream:{...stream,...fields}}).code,'');
 for(const packets of [[packet(request,true,{incomplete:true})],[packet(request,true,{kind:'http'})],[packet(request,true,{content:'!bad'})],[]])for(const format of ['requests','tcp'])assert.equal(generate(packets,{format}).code,'');
 assert.equal(generate([packet(request+'GET /cut')]).code,'');
 assert(generate([packet('random\0tcp')],{format:'tcp'}).code);
 assert.equal(generate([packet('\x16\x03\x03TLS')],{format:'tcp'}).code,'');
 assert.equal(generate(Array.from({length:5},()=>packet('x'.repeat(MAX_SESSION_BYTES/4))),{format:'tcp'}).code,'');
 assert.equal(generate([packet(request.repeat(129))]).code,'');
 assert.equal(generate([packet('GET / HTTP/1.1\r\nUpgrade: websocket\r\n\r\n')]).code,'');
});
test('generated Python compiles for binary bytes, quotes and TLS option',async()=>{
 const p=[packet('GET /"quote HTTP/1.1\r\nX-Test: \'$`\\\r\n\r\n')];
 for(const format of ['requests','tcp'])for(const scheme of ['http','https']){const g=generate(p,{format,scheme});assert.equal(g.error,'');await python(g.code,true);}
});
test('HTTP session preserves order and binary bodies, updates and expires cookies with Host different from destination',async()=>{
 const observed=[];const server=http.createServer((req,res)=>{const chunks=[];req.on('data',b=>chunks.push(b));req.on('end',()=>{observed.push({url:req.url,cookie:req.headers.cookie,body:Buffer.concat(chunks)});if(observed.length===1)res.setHeader('Set-Cookie','sid=fresh; Path=/');if(observed.length===2)res.setHeader('Set-Cookie','sid=; Max-Age=0; Path=/');res.end('ok')})});
 await new Promise(resolve=>server.listen(0,'127.0.0.1',resolve));
 try{
  const packets=[packet('GET /login HTTP/1.1\r\nHost: shop\r\nCookie: sid=old; stable=keep\r\n\r\n'),packet('POST /play HTTP/1.1\r\nHost: shop\r\nCookie: sid=obsolete\r\nContent-Length: 3\r\n\r\n\x00\xffx'),packet('GET /done HTTP/1.1\r\nHost: shop\r\nCookie: sid=obsolete\r\n\r\n')];
  const generated=generate(packets,{port:server.address().port});assert.equal(generated.error,'');await python(generated.code);
  assert.deepEqual(observed.map(x=>x.url),['/login','/play','/done']);assert.match(observed[0].cookie,/sid=old/);assert.match(observed[1].cookie,/sid=fresh/);assert.doesNotMatch(observed[1].cookie,/old|obsolete/);assert.doesNotMatch(observed[2].cookie,/sid=/);assert.match(observed[2].cookie,/stable=keep/);assert.deepEqual(observed[1].body,Buffer.from([0,255,120]));
 }finally{server.closeAllConnections();await new Promise(resolve=>server.close(resolve))}
});
test('TCP uses one connection, server banner, exact bytes and fragmented replies',async()=>{
 let connections=0;const got=[];const server=net.createServer(socket=>{connections++;socket.write('B');setTimeout(()=>socket.write('ANNER'),10);let buffer=Buffer.alloc(0),step=0;socket.on('data',chunk=>{buffer=Buffer.concat([buffer,chunk]);if(step===0&&buffer.length>=3){got.push(buffer.subarray(0,3));buffer=buffer.subarray(3);step++;socket.write(Buffer.from([0]));setTimeout(()=>socket.write(Buffer.from([255,1])),10)}if(step===1&&buffer.length>=4){got.push(buffer.subarray(0,4));step++;socket.end('DONE')}})});
 await new Promise(resolve=>server.listen(0,'127.0.0.1',resolve));
 try{
  const packets=[packet('BAN',false),packet('NER',false),packet('\x00\xff',true),packet('x',true),packet('\x00\xff\x01',false),packet('next'),packet('DONE',false)];
  const generated=generate(packets,{format:'tcp',port:server.address().port});assert.equal(generated.error,'');const output=await python(generated.code);assert.equal(connections,1);assert.deepEqual(got,[Buffer.from([0,255,120]),Buffer.from('next')]);assert.deepEqual(output,Buffer.concat([Buffer.from('BANNER'),Buffer.from([0,255,1]),Buffer.from('DONE')]));
 }finally{await new Promise(resolve=>server.close(resolve))}
});
test('loader follows X-Has-More with raw cursor and stable metadata',async()=>{
 const pages=[],progress=[];let gets=0;
 const client={get:async()=>{gets++;return {data:stream}},post:async(path,body)=>{pages.push(body);return body.startingFrom?{data:[packet('b',false,{id:6})],headers:{'x-has-more':'false'}}:{data:[packet('a',true,{id:2})],headers:{'x-has-more':'true'}}}};
 const result=await loadSessionExport(client,7,{onProgress:x=>progress.push(x)});assert.equal(result.packets.length,2);assert.equal(gets,2);assert.deepEqual(pages.map(x=>x.startingFrom),[null,2]);assert(pages.every(x=>x.representation==='raw'));assert.equal(progress[1].bytes,2);
});
test('loader fails closed on retention, live changes, bad pagination, limits and abort',async()=>{
 for(const after of [{...stream,sizeBytes:101},{...stream,payloadPurged:true},{}]){let gets=0;const client={get:async()=>({data:++gets===1?stream:after}),post:async()=>({data:[packet('a',true,{id:1})],headers:{'x-has-more':'false'}})};await assert.rejects(loadSessionExport(client,7));}
 for(const response of [{data:[],headers:{'x-has-more':'true'}},{data:[packet('a',true,{id:1})],headers:{}},{data:[packet('a',true,{id:1}),packet('b',true,{id:1})],headers:{'x-has-more':'false'}},{data:[packet('a',true,{id:1,kind:'http'})],headers:{'x-has-more':'false'}},{data:[packet('a'.repeat(MAX_SESSION_BYTES+1),true,{id:1})],headers:{'x-has-more':'false'}}])await assert.rejects(loadSessionExport({get:async()=>({data:stream}),post:async()=>response},7));
 const controller=new AbortController();let calls=0;await assert.rejects(loadSessionExport({get:async()=>({data:stream}),post:async()=>{calls++;controller.abort();return {data:[packet('a',true,{id:1})],headers:{'x-has-more':'true'}}}},7,{signal:controller.signal}));assert.equal(calls,1);
});

test('TCP fails explicitly on early EOF instead of sending later steps',async()=>{
 let received='';const server=net.createServer(socket=>{socket.on('data',b=>{received+=b;socket.end('short')})});
 await new Promise(resolve=>server.listen(0,'127.0.0.1',resolve));
 try{const g=generate([packet('first'),packet('longer response',false),packet('must not send')],{format:'tcp',port:server.address().port});await assert.rejects(python(g.code),/EOFError: Expected 15 bytes, received 5/);assert.equal(received,'first')}finally{await new Promise(resolve=>server.close(resolve))}
});
