const {chromium} = require('../../web/node_modules/playwright');
const fs=require('node:fs'),assert=require('node:assert/strict'),path=require('node:path');
process.chdir(path.resolve(__dirname,'../..'));
(async()=>{
 const env=Object.fromEntries(fs.readFileSync('.cache/kali.env','utf8').split('\n').filter(l=>l&&!l.startsWith('#')).map(l=>{const n=l.indexOf('=');return[l.slice(0,n),l.slice(n+1)];}));
 const fixture=JSON.parse(fs.readFileSync('reports/kali-features.json','utf8'));
 const base='http://192.168.56.103:65000',errors=[],contexts=[];
 const browser=await chromium.launch({headless:true,executablePath:process.env.CHROMIUM_PATH || '/home/logalex/.cache/ms-playwright/chromium-1187/chrome-linux/chrome'});
 try{
  const n=Number(process.env.BROWSERS || 20),start=Date.now();
  for(let i=0;i<n;i++){
   const context=await browser.newContext({httpCredentials:{username:'packmate',password:env.WEB_PASSWORD},viewport:{width:1440,height:900}});contexts.push(context);
   const page=await context.newPage();page.on('pageerror',e=>errors.push(e.message));
   await page.goto(base);await page.waitForSelector('.stream-item');
   assert(await page.locator('.stream-item').count()<25,'virtual list rendered entire page');
  }
  const page=contexts[0].pages()[0],record=fixture.records.find(p=>p.kind==='grpc');
  await page.goto(`${base}/#/15051/${record.stream_id}`);await page.locator('select').selectOption('decoded');
  await page.getByText('Copy HEX',{exact:true}).first().waitFor();await page.waitForFunction(()=>document.body.innerText.includes('/gomate.mock.Echo/'));
  assert(!(await page.locator('main strong').allTextContents()).includes('raw'));
  await page.getByRole('button',{name:'Hex / Text',exact:true}).click();await page.waitForFunction(()=>document.body.innerText.includes('0000000000:'));
  await page.getByRole('button',{name:'Hex / Text',exact:true}).click();
  // Exercise snapshot recovery: a disconnected context must display current data.
  await contexts[0].setOffline(true);await page.waitForTimeout(1500);await contexts[0].setOffline(false);await page.waitForFunction(()=>document.querySelector('.capture-status').innerText.startsWith('LIVE'),{},{timeout:20000});
  const hexMarker='hexcheck_'+Date.now();
  const response=await contexts[0].request.post(base+'/api/pattern/',{data:{name:hexMarker,value:Buffer.from('FLAG_'+hexMarker).toString('hex'),color:'#123abc',searchType:'SUBBYTES',directionType:'OUTPUT',actionType:'FIND',serviceId:19090}});
  assert(response.ok());const bytePattern=await response.json();
  require('node:child_process').execFileSync('bin/mock',['-mode','client','-protocol','tcp','-address','192.168.56.103:19090','-run',hexMarker,'-count','1','-rps','1'],{timeout:15000});
  let streams=[];
  for(let i=0;i<30&&!streams.length;i++){const r=await contexts[0].request.post(base+'/api/stream/19090',{data:{pageSize:10,pattern:{id:bytePattern.id}}});streams=await r.json();if(!streams.length)await page.waitForTimeout(500);}
  assert(streams.length);await page.goto(`${base}/#/19090/${streams[0].id}`);await page.locator('select').selectOption('');await page.waitForFunction(marker=>document.querySelector('main').innerText.includes(marker),hexMarker);await page.getByText('Copy HEX',{exact:true}).first().waitFor();
  await page.getByRole('button',{name:'Hex / Text',exact:true}).click();await page.locator('main span[style*="background-color"]').first().waitFor();
  assert.equal(await page.locator('main span[style*="background-color"]').first().textContent(),'46');
  await contexts[0].request.delete(base+`/api/pattern/${bytePattern.id}`);
  await page.waitForTimeout(5000);assert.deepEqual(errors,[]);
  await page.screenshot({path:'reports/kali-features-browser.png'});
  const report={passed:true,browsers:n,elapsedSeconds:(Date.now()-start)/1000,pageErrors:errors,checks:['virtual list','decoded gRPC view','hex/text','WebSocket reconnect snapshot','SUBBYTES live matching and hex highlighting']};
  fs.writeFileSync('reports/kali-browser-features.json',JSON.stringify(report,null,2)+'\n');console.log(JSON.stringify(report));
 }finally{await browser.close();}
})().catch(e=>{console.error(e);process.exitCode=1;});
