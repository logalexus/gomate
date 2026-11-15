// Docker selection opens an editable draft; all writes are mocked.
const {chromium}=require('../../web/node_modules/playwright');
const fs=require('node:fs'),assert=require('node:assert/strict'),path=require('node:path'),{spawn}=require('node:child_process');
process.chdir(path.resolve(__dirname,'../..'));
(async()=>{
 const live=process.env.GOMATE_URL,base=live||'http://127.0.0.1:4192';
 const server=live?null:spawn(process.execPath,['node_modules/vite/bin/vite.js','--host','127.0.0.1','--port','4192','--strictPort'],{cwd:'web',stdio:'ignore'});
 let browser;const errors=[],writes=[],report={passed:false,mockedAPI:true,deployedUI:!!live};
 const first={id:'http',name:'shop-api',image:'registry.test/team/shop:latest',port:18080,privatePort:8080,protocol:'TCP',hostIP:'0.0.0.0',available:true};
 const second={...first,id:'tcp',name:'tcp-service',port:19090,privatePort:9090};
 const candidates=[first,second,{...first,id:'tracked',name:'already-tracked',port:8070},{...first,id:'private',name:'private-only',port:0,available:false,reason:'Port is not published'}];
 let services=[{port:8070,name:'Tracked'}],failSave=true;
 try{
  if(!live)for(let i=0;i<60;i++){try{if((await fetch(base)).ok)break}catch{}await new Promise(r=>setTimeout(r,150))}
  const credentials=live?{httpCredentials:{username:'packmate',password:fs.readFileSync('.cache/kali.env','utf8').split('\n').find(x=>x.startsWith('WEB_PASSWORD=')).slice(13).replace(/^['"]|['"]$/g,'')}}:{};
  browser=await chromium.launch({headless:true});const page=await browser.newPage({...credentials,viewport:{width:1366,height:900}});
  page.on('pageerror',e=>errors.push(e.message));await page.routeWebSocket('**/api/v2/events',()=>{});
  await page.route('**/api/**',async r=>{const req=r.request(),u=new URL(req.url());let data=[];
   if(u.pathname==='/api/v2/config')data={requestTimeoutSeconds:10};
   if(u.pathname==='/api/v2/snapshot')data={services,patterns:[],metrics:{},captureRunning:true};
   if(u.pathname==='/api/service/')data=services;
   if(u.pathname==='/api/v2/docker-services'){
    data=candidates;
    if(req.method()==='POST'){
     writes.push({path:u.pathname,body:req.postDataJSON()});
     if(failSave)return r.fulfill({status:409,json:{error:'Порт уже отслеживается: тестовая ошибка'}});
     data={...req.postDataJSON().service,port:candidates.find(c=>c.id===req.postDataJSON().id).port};services.push(data);
    }
   }else if(req.method()==='POST'&&u.pathname==='/api/service/'){writes.push({path:u.pathname,body:req.postDataJSON()});data=req.postDataJSON();services.push(data)}
   await r.fulfill({json:data});
  });
  await page.goto(base+'/#/');const modal=page.locator('#serviceModal'),open=()=>page.getByRole('button',{name:'Add service',exact:true}).click();
  await open();await modal.getByRole('button',{name:'Add shop-api 18080',exact:true}).waitFor();
  assert(await modal.getByRole('button',{name:'Add already-tracked 8070',exact:true}).isDisabled());assert(await modal.getByRole('button',{name:'Add private-only 0',exact:true}).isDisabled());
  await modal.getByRole('button',{name:'Add shop-api 18080',exact:true}).click();await page.locator('#service-name').waitFor();assert.equal(writes.length,0);
  assert.equal(await page.locator('#service-name').inputValue(),'shop-api');assert.equal(await page.locator('#service-port').inputValue(),'18080');assert(await page.locator('#service-port').isDisabled());
  await page.waitForFunction(()=>document.activeElement?.id==='service-name');
  await page.locator('#service-name').fill('Edited HTTP API');
  await modal.locator('label').filter({hasText:'Parse HTTP, HTTP/2 and gRPC'}).click();await modal.locator('label').filter({hasText:'Decode URL and form-urlencoded'}).click();await modal.locator('label').filter({hasText:'Parse WebSocket'}).click();
  assert.equal(writes.length,0);await modal.getByRole('button',{name:'Save',exact:true}).click();await modal.getByRole('alert').filter({hasText:'тестовая ошибка'}).waitFor();assert.equal(await page.locator('#service-name').inputValue(),'Edited HTTP API');
  assert.deepEqual(writes[0],{path:'/api/v2/docker-services',body:{id:'http',service:{name:'Edited HTTP API',port:18080,http:true,urldecodeHttpRequests:true,parseWebSockets:true}}});
  failSave=false;await modal.getByRole('button',{name:'Save',exact:true}).click();await modal.waitFor({state:'hidden'});assert.deepEqual(writes[0],writes[1]);
  await open();await modal.getByRole('button',{name:'Add tcp-service 19090',exact:true}).click();assert.equal(await page.locator('#service-name').inputValue(),'tcp-service');assert(!(await modal.getByLabel('Parse HTTP, HTTP/2 and gRPC',{exact:true}).isChecked()));
  await modal.getByRole('button',{name:'Cancel',exact:true}).click();await modal.waitFor({state:'hidden'});assert.equal(writes.length,2);
  // Reopen from scratch, choose another candidate, and explicitly opt into manual port editing.
  services=services.filter(s=>s.port!==18080);
  await open();await modal.getByRole('button',{name:'Add tcp-service 19090',exact:true}).click();await modal.getByRole('tab',{name:'From Docker',exact:true}).click();
  // The store may still track the first candidate until snapshot refresh; use a fresh fixture candidate.
  candidates.push({...first,id:'other',name:'other-service',port:20080});
  await modal.getByRole('button',{name:'Refresh',exact:true}).click();await modal.getByRole('button',{name:'Add other-service 20080',exact:true}).click();
  assert.equal(await page.locator('#service-name').inputValue(),'other-service');assert.equal(await page.locator('#service-port').inputValue(),'20080');
  await modal.getByRole('button',{name:'Edit port manually',exact:true}).click();assert(!(await page.locator('#service-port').isDisabled()));await page.locator('#service-port').fill('21080');
  await modal.getByRole('button',{name:'Save',exact:true}).click();await modal.waitFor({state:'hidden'});assert.equal(writes.at(-1).path,'/api/service/');assert.equal(writes.at(-1).body.port,21080);
  // Compact layout still exposes settings and actions at phone/desktop sizes and both themes.
  for(const theme of ['light','dark'])for(const width of [1366,375]){
   await page.setViewportSize({width,height:812});await page.evaluate(t=>document.body.dataset.theme=t,theme);await open();await modal.getByRole('button',{name:'Add tcp-service 19090',exact:true}).click();await page.locator('#service-name').waitFor();
   assert(!(await page.evaluate(()=>document.documentElement.scrollWidth>innerWidth+1)));assert(await modal.evaluate(el=>el.scrollWidth<=el.clientWidth+1));
   await page.screenshot({path:`reports/service-docker-form-${width}-${theme}.jpg`,type:'jpeg',quality:75});
   await modal.getByRole('button',{name:'Cancel',exact:true}).click();await modal.waitFor({state:'hidden'});
  }
  assert.equal(writes.length,3);assert.deepEqual(errors,[]);Object.assign(report,{passed:true,checks:['Docker candidate opens prefilled draft without writes','published port and editable protocol options','name receives keyboard focus','tracked/unpublished candidates disabled','explicit Save sends edited options','save failure preserves draft and retries','cancel never creates service','switch candidate resets fields','explicit manual port mode','1366/375px light/dark'],writes,errors});
 }finally{await browser?.close();server?.kill();fs.writeFileSync(live?'reports/service-docker-form-live.json':'reports/service-docker-form-ui.json',JSON.stringify(report,null,2)+'\n')}
 console.log(JSON.stringify(report));
})().catch(e=>{console.error(e);process.exitCode=1});
