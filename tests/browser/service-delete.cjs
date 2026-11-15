// Regression: opening settings must pass the selected service before @show reads it.
// All service writes are mocked; optionally exercise the deployed UI with GOMATE_URL.
const {chromium}=require('../../web/node_modules/playwright');
const fs=require('node:fs'),assert=require('node:assert/strict'),path=require('node:path'),{spawn}=require('node:child_process');
process.chdir(path.resolve(__dirname,'../..'));
(async()=>{
 const live=process.env.GOMATE_URL,base=live||'http://127.0.0.1:4191';
 const server=live?null:spawn(process.execPath,['node_modules/vite/bin/vite.js','--host','127.0.0.1','--port','4191','--strictPort'],{cwd:'web',stdio:'ignore'});
 let browser;const errors=[],writes=[],report={passed:false,mockedAPI:true,deployedUI:!!live};
 let services=[{port:18080,name:'HTTP fixture',http:true,urldecodeHttpRequests:true,parseWebSockets:false},{port:19090,name:'TCP fixture',http:false,urldecodeHttpRequests:false,parseWebSockets:true}],failDelete=true;
 try{
  if(!live)for(let i=0;i<60;i++){try{if((await fetch(base)).ok)break}catch{}await new Promise(r=>setTimeout(r,150))}
  const credentials=live?{httpCredentials:{username:'packmate',password:fs.readFileSync('.cache/kali.env','utf8').split('\n').find(x=>x.startsWith('WEB_PASSWORD=')).slice(13).replace(/^['"]|['"]$/g,'')}}:{};
  browser=await chromium.launch({headless:true});const page=await browser.newPage({...credentials,viewport:{width:1366,height:850}});
  page.on('pageerror',e=>errors.push(e.message));await page.routeWebSocket('**/api/v2/events',()=>{});
  await page.route('**/api/**',async r=>{const req=r.request(),u=new URL(req.url());let data=[];
   if(u.pathname==='/api/v2/config')data={requestTimeoutSeconds:10};
   if(u.pathname==='/api/v2/snapshot')data={services,patterns:[{id:1,name:'FLAG',actionType:'FIND'}],metrics:{},captureRunning:true};
   if(u.pathname==='/api/service/')data=services;
   if(req.method()==='DELETE'){
    writes.push({method:req.method(),path:u.pathname});
    if(failDelete)return r.fulfill({status:500,json:{error:'Не удалось удалить: тестовая ошибка'}});
    services=services.filter(s=>u.pathname!=='/api/service/'+s.port);
    return r.fulfill({status:204});
   }
   if(req.method()==='POST'&&u.pathname.startsWith('/api/service/')){writes.push({method:req.method(),path:u.pathname,data:req.postDataJSON()});data=req.postDataJSON();services=services.map(s=>s.port===data.port?data:s)}
   await r.fulfill({json:data});
  });
  await page.goto(base+'/#/18080?patterns=1');
  const settings=page.locator('#serviceModal'),open=()=>page.getByRole('button',{name:'Configure selected service'}).click();
  await open();await settings.waitFor();assert.equal(await page.locator('#service-name').inputValue(),'HTTP fixture');assert.equal(await page.locator('#service-port').inputValue(),'18080');
  assert(await settings.getByLabel('Parse HTTP, HTTP/2 and gRPC',{exact:true}).isChecked());
  await settings.getByRole('button',{name:'Delete service',exact:true}).click();
  let confirmation=page.getByRole('dialog').filter({hasText:'Delete service?'});await confirmation.waitFor();assert.match(await confirmation.textContent(),/HTTP fixture.*18080/);
  await confirmation.getByRole('button',{name:'Cancel',exact:true}).click();await confirmation.waitFor({state:'hidden'});assert.equal(writes.length,0);
  await settings.getByRole('button',{name:'Cancel',exact:true}).click();await settings.waitFor({state:'hidden'});
  // Switching services and create/edit modes must not reuse stale props.
  await page.locator('#traffic-service').selectOption('19090');await open();assert.equal(await page.locator('#service-name').inputValue(),'TCP fixture');assert.equal(await page.locator('#service-port').inputValue(),'19090');assert(!(await settings.getByLabel('Parse HTTP, HTTP/2 and gRPC',{exact:true}).isChecked()));
  await settings.getByRole('button',{name:'Cancel',exact:true}).click();await settings.waitFor({state:'hidden'});
  await page.getByRole('button',{name:'Add service',exact:true}).click();await settings.getByRole('tab',{name:'Manual',exact:true}).click();assert.equal(await page.locator('#service-name').inputValue(),'');assert.equal(await page.locator('#service-port').inputValue(),'');
  await settings.getByRole('button',{name:'Cancel',exact:true}).click();await settings.waitFor({state:'hidden'});await open();await page.locator('#service-name').fill('TCP renamed');await settings.getByRole('button',{name:'Save',exact:true}).click();await settings.waitFor({state:'hidden'});assert.equal(writes.at(-1).path,'/api/service/19090');assert.equal(writes.at(-1).data.name,'TCP renamed');
  await page.locator('#traffic-service').selectOption('18080');await open();
  await settings.getByRole('button',{name:'Delete service',exact:true}).click();await confirmation.getByRole('button',{name:'Delete',exact:true}).click();await settings.getByRole('alert').filter({hasText:'тестовая ошибка'}).waitFor();assert.equal(writes.at(-1).path,'/api/service/18080');assert.equal(await page.locator('#service-name').inputValue(),'HTTP fixture');
  // Mobile/light retry, preserving unrelated filters and leaving no deleted-port fallback.
  await page.setViewportSize({width:375,height:812});await page.evaluate(()=>document.body.dataset.theme='light');failDelete=false;
  await settings.getByRole('button',{name:'Delete service',exact:true}).click();await confirmation.waitFor();assert(!(await page.evaluate(()=>document.documentElement.scrollWidth>innerWidth+1)));
  await confirmation.getByRole('button',{name:'Delete',exact:true}).click();await settings.waitFor({state:'hidden'});
  await page.waitForFunction(()=>document.querySelector('#traffic-service')?.value==='');assert.equal(await page.locator('#traffic-service option[value="18080"]').count(),0);assert.match(page.url(),/patterns=1/);assert.equal(await page.locator('#traffic-service option[value="19090"]').count(),1);
  assert.deepEqual(writes.filter(w=>w.method==='DELETE').map(w=>w.path),['/api/service/18080','/api/service/18080']);assert.deepEqual(errors,[]);
  Object.assign(report,{passed:true,checks:['selected name/port/protocols available on first open','switch service without stale values','switch create/edit modes','save selected service','cancel sends no DELETE','confirmation identifies service','failure preserves settings and allows retry','delete targets selected port','success resets only service scope','375px light / 1366px dark'],writes,errors});
 }finally{await browser?.close();server?.kill();fs.writeFileSync(live?'reports/service-delete-live.json':'reports/service-delete-ui.json',JSON.stringify(report,null,2)+'\n')}
 console.log(JSON.stringify(report));
})().catch(e=>{console.error(e);process.exitCode=1});
