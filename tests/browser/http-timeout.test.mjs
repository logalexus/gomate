import test from 'node:test';
import assert from 'node:assert/strict';
import {createServer} from 'node:http';
import {createHTTPClient} from '../../web/src/http.mjs';

test('common timeout covers initial requests, saved settings, all screens and cancellation', async t=>{
 let seconds=10, configReads=0, writes=0;
 const server=createServer((req,res)=>{
  res.setHeader('Content-Type','application/json');
  if(req.url==='/api/v2/config'){configReads++;return res.end(JSON.stringify({timeoutSeconds:seconds}))}
  if(req.url==='/api/v2/settings'&&req.method==='PUT'){
   writes++;seconds=30;return res.end(JSON.stringify({settings:{requestTimeoutSeconds:seconds}}));
  }
  if(req.url.endsWith('/slow')){setTimeout(()=>res.end('{}'),6000).unref();return}
  res.end('{}');
 });
 await new Promise(resolve=>server.listen(0,'127.0.0.1',resolve));
 t.after(()=>{server.closeAllConnections();server.close()});
 const client=createHTTPClient({baseURL:`http://127.0.0.1:${server.address().port}/api`});
 const initial=await Promise.all(['/v2/snapshot','/v2/chat','/v2/settings'].map(url=>client.get(url)));
 assert.equal(configReads,1,'initial requests share config discovery');
 assert(initial.every(r=>r.config.timeout===15000));
 const slow=await client.get('/v2/chat/slow');
 assert.equal(slow.status,200,'ordinary requests survive the former 5s ceiling');
 await client.put('/v2/settings',{});
 for(const url of ['/stream','/v2/chat','/v2/anomalies','/v2/containers','/v2/runner']){
  const r=await client.get(url,{timeout:5000});
  assert.equal(r.config.timeout,35000,url);
 }
 const controller=new AbortController();
 const request=client.get('/v2/chat/slow',{signal:controller.signal});
 controller.abort();
 await assert.rejects(request,e=>e.code==='ERR_CANCELED');
 assert.equal(writes,1,'never retry writes');
});
