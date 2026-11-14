const {chromium}=require('../../web/node_modules/playwright');
const fs=require('node:fs'),assert=require('node:assert/strict'),path=require('node:path'),http=require('node:http'),{execFileSync}=require('node:child_process');
process.chdir(path.resolve(__dirname,'../..'));
(async()=>{
 const env=Object.fromEntries(fs.readFileSync('.cache/kali.env','utf8').split('\n').filter(l=>l&&!l.startsWith('#')).map(l=>{const n=l.indexOf('=');return[l.slice(0,n),l.slice(n+1)]}));
 const base='http://192.168.56.103:65000',run='decode_'+Date.now(),sql="' OR 1=1 --",blob=Buffer.from(sql).toString('base64');
 const jwt=Buffer.from('{"alg":"none"}').toString('base64url')+'.'+Buffer.from(JSON.stringify({sub:run,exp:1700000000,blob})).toString('base64url')+'.';
 await new Promise((resolve,reject)=>{const req=http.get('http://192.168.56.103:18080/?marker='+run+'&blob='+encodeURIComponent(blob),{agent:false,headers:{Authorization:'Bearer '+jwt,'X-Decode-Sample':blob,Connection:'close'}},r=>{r.resume();r.on('end',()=>r.statusCode===200?resolve():reject(Error('mock HTTP '+r.statusCode)))});req.on('error',reject);req.setTimeout(5000,()=>req.destroy(Error('mock timeout')))});
 const query=`SELECT coalesce(json_agg(t),'[]'::json) FROM (SELECT id,stream_id FROM gomate_packets WHERE incoming AND position('${run}' in encode(content,'escape'))>0 ORDER BY id LIMIT 1) t;`;
 let record;
 for(let i=0;i<25&&!record;i++){const raw=execFileSync('ssh',['-o','BatchMode=yes','kali@192.168.56.103','docker exec -i gomate-lab-db-1 psql -X -qAt -U gomate -d gomate'],{input:query,encoding:'utf8',timeout:15000});record=JSON.parse(raw)[0];if(!record)await new Promise(r=>setTimeout(r,400))}
 assert(record,'capture did not persist the real-interface HTTP fixture');
 const browser=await chromium.launch({headless:true,executablePath:process.env.CHROMIUM_PATH||'/home/logalex/.cache/ms-playwright/chromium-1187/chrome-linux/chrome'});
 const ctx=await browser.newContext({httpCredentials:{username:'packmate',password:env.WEB_PASSWORD},viewport:{width:1100,height:800},reducedMotion:'reduce'});
 const page=await ctx.newPage(),errors=[];page.on('pageerror',e=>errors.push(e.message));
 const report={passed:false,run,streamID:record.stream_id,packetID:record.id};
 try{
  const packets=async()=>{const r=await ctx.request.post(base+'/api/packet/'+record.stream_id,{data:{pageSize:50}});assert(r.ok());return r.json()};
  const before=await packets();
  await page.goto(base+'/#/18080/'+record.stream_id);
  const packet=page.locator('main .packet-incoming').filter({hasText:run}).first();await packet.waitFor();
  await packet.getByRole('button',{name:'Декодировать',exact:true}).click();
  const modal=page.getByRole('dialog'),input=modal.getByLabel('Входные данные'),output=modal.locator('#traffic-decode-output');
  await modal.locator('.decoder-candidate').filter({hasText:sql}).first().click();assert.equal(await output.inputValue(),sql);
  await modal.locator('.decoder-candidate').filter({hasText:'JWT (header + payload)'}).first().click();
  assert.deepEqual(JSON.parse(await modal.getByLabel('JWT payload',{exact:true}).inputValue()),{sub:run,exp:1700000000,blob});
  await modal.getByText('Подпись JWT не проверялась.',{exact:false}).waitFor();
  const payload=modal.getByLabel('JWT payload',{exact:true});await payload.focus();await payload.evaluate((el,value)=>{const start=el.value.indexOf(value);el.setSelectionRange(start,start+value.length)},blob);
  await modal.getByRole('button',{name:'Выделение результата → вход',exact:true}).click();await page.waitForFunction(expected=>document.querySelector('#traffic-decode-output')?.value===expected,sql);assert.equal(await input.inputValue(),blob);
  await modal.getByRole('button',{name:'Назад',exact:true}).click();await modal.locator('.decoder-candidate').first().waitFor();assert((await input.inputValue()).includes(run));
  await input.fill(Buffer.from(blob).toString('base64'));await modal.getByRole('button',{name:'Распознать автоматически',exact:true}).click();await page.waitForFunction(expected=>document.querySelector('#traffic-decode-output')?.value===expected,blob);
  await modal.getByRole('button',{name:'Результат → вход',exact:true}).click();await page.waitForFunction(expected=>document.querySelector('#traffic-decode-output')?.value===expected,sql);
  const manual=async(value,method)=>{await input.fill(value);await modal.getByLabel('Ручной декодер').selectOption(method);await modal.getByRole('button',{name:'Декодировать',exact:true}).click()};
  await manual('%27+OR+1%3D1+--','form');await page.waitForFunction(expected=>document.querySelector('#traffic-decode-output')?.value===expected,sql);
  await manual('invalid%%%','base64');await modal.getByRole('alert').filter({hasText:'Некорректный Base64'}).waitFor();assert.equal(await output.count(),0,'stale result survived invalid input');
  await manual(Buffer.from('<img src=x onerror=window.decodeXSS=1>').toString('base64'),'base64');await output.waitFor();assert.equal(await modal.locator('img,script').count(),0);assert.equal(await page.evaluate(()=>window.decodeXSS),undefined);
  await manual('/wAA','base64');await modal.getByText('Бинарный результат:',{exact:false}).waitFor();assert.equal(await output.inputValue(),'ff 00 00');
  // A deliberately stalled worker makes cancellation and timeout deterministic.
  await page.evaluate(()=>{window.RealDecodeWorker=window.Worker;window.Worker=class{postMessage(){}terminate(){window.decodeTerminations=(window.decodeTerminations||0)+1}}});
  await input.fill(blob);await modal.getByRole('button',{name:'Распознать автоматически',exact:true}).click();await modal.getByRole('button',{name:'Отмена',exact:true}).click();await modal.getByText('Обработка отменена.',{exact:true}).waitFor();
  await modal.getByRole('button',{name:'Распознать автоматически',exact:true}).click();await modal.getByText('Превышено время обработки',{exact:false}).waitFor();assert(await page.evaluate(()=>window.decodeTerminations>=2));
  await page.evaluate(()=>{window.Worker=window.RealDecodeWorker});
  await input.fill('a'.repeat(65536));const started=Date.now();await modal.getByRole('button',{name:'Распознать автоматически',exact:true}).click();await modal.getByRole('status').waitFor();report.largeBlobMilliseconds=Date.now()-started;
  await input.fill('a'.repeat(65537));assert(await modal.getByRole('button',{name:'Распознать автоматически',exact:true}).isDisabled());
  await manual(blob,'base64');await page.waitForFunction(expected=>document.querySelector('#traffic-decode-output')?.value===expected,sql);
  for(const theme of ['light','dark']){await page.evaluate(theme=>document.body.setAttribute('data-theme',theme),theme);const colors=await input.evaluate(el=>({background:getComputedStyle(el).backgroundColor,color:getComputedStyle(el).color}));const luminance=rgb=>{const values=rgb.match(/[\d.]+/g).slice(0,3).map(Number).map(n=>{n/=255;return n<=.04045?n/12.92:((n+.055)/1.055)**2.4});return values[0]*.2126+values[1]*.7152+values[2]*.0722};const a=luminance(colors.background),b=luminance(colors.color);colors.contrast=(Math.max(a,b)+.05)/(Math.min(a,b)+.05);assert(colors.contrast>=4.5,theme+' input contrast');report[theme+'InputColors']=colors}
  await page.setViewportSize({width:375,height:812});assert(await modal.evaluate(el=>el.scrollWidth<=window.innerWidth),'modal horizontal overflow');
  await page.keyboard.press('Escape');await modal.waitFor({state:'hidden'});await page.setViewportSize({width:1100,height:800});
  // Selecting text inside a packet scopes the decoder to that selection.
  await packet.locator('p').evaluate((el,value)=>{const walker=document.createTreeWalker(el,NodeFilter.SHOW_TEXT);let node;while(node=walker.nextNode()){const at=node.textContent.indexOf(value);if(at>=0){const range=document.createRange();range.setStart(node,at);range.setEnd(node,at+value.length);const s=getSelection();s.removeAllRanges();s.addRange(range);return}}throw Error('selection fixture missing')},blob);
  await packet.getByRole('button',{name:'Декодировать',exact:true}).click();await input.waitFor();assert.equal(await input.inputValue(),blob);await page.waitForFunction(expected=>document.querySelector('#traffic-decode-output')?.value===expected,sql);
  await page.keyboard.press('Escape');await modal.waitFor({state:'hidden'});await page.waitForFunction(()=>document.activeElement?.textContent.trim()==='Декодировать');
  assert.deepEqual((await packets()).map(p=>[p.id,p.content]),before.map(p=>[p.id,p.content]));assert.deepEqual(errors,[]);
  Object.assign(report,{passed:true,pageErrors:errors,checks:['real HTTP capture on eth1','auto Base64 and JWT','JWT payload selection','nested Base64 and undo','manual form decoding','errors clear stale output','HTML stays text','binary HEX','worker cancel and deadline','64 KiB input boundary','light/dark and 375px modal','packet text selection','Escape and return focus','original captured bytes unchanged']});
 }finally{await browser.close();fs.writeFileSync('reports/ui-traffic-decode.json',JSON.stringify(report,null,2)+'\n')}
 console.log(JSON.stringify(report));
})().catch(e=>{console.error(e);process.exitCode=1});
