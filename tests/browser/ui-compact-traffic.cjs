// Synthetic fixtures: verifies layout and client request semantics, not SQL filtering.
const {chromium}=require('../../web/node_modules/playwright');
const assert=require('node:assert/strict'),fs=require('node:fs'),path=require('node:path'),{spawn}=require('node:child_process');
process.chdir(path.resolve(__dirname,'../..'));
(async()=>{
 const port=4176,base='http://127.0.0.1:'+port,server=spawn(process.execPath,['node_modules/vite/bin/vite.js','--host','127.0.0.1','--port',String(port),'--strictPort'],{cwd:'web',stdio:'ignore'});
 let browser;const report={passed:false,mocked:true,layouts:[]},delay=ms=>new Promise(r=>setTimeout(r,ms));
 try{
  for(let i=0;i<60;i++){try{if((await fetch(base)).ok)break}catch{}await delay(200)}
  browser=await chromium.launch({headless:true,executablePath:process.env.CHROMIUM_PATH||'/home/logalex/.cache/ms-playwright/chromium-1187/chrome-linux/chrome'});
  const page=await browser.newPage({viewport:{width:1440,height:760},reducedMotion:'reduce'}),errors=[];page.on('pageerror',e=>errors.push(e.message));
  const patterns=[{id:1,name:'SQL injection',actionType:'FIND'},{id:2,name:'Flag',actionType:'FIND'},{id:3,name:'Ignored',actionType:'IGNORE'}];let latestBody;let many=false;
  const rows=Array.from({length:25},(_,i)=>({id:100-i,service:8080,protocol:'HTTP',startTimestamp:Date.now(),sizeBytes:112,packetsCount:2,foundPatternsIds:i===0?[1,2]:[1],firstRequestPath:i===0?'/api/login':'/api/items/'+i,favorite:i===0}));
  await page.routeWebSocket('**/api/v2/events',()=>{});
  await page.route('**/api/**',async route=>{
   const req=route.request(),url=new URL(req.url());let data=[];
   if(url.pathname==='/api/v2/snapshot')data={services:[{port:8080,name:'Shop'}],patterns:many?[...patterns,...Array.from({length:60},(_,i)=>({id:10+i,name:'Long template label '+i,actionType:'FIND'}))]:patterns,metrics:{captured_total:500,persisted_chunks_total:25,capture_drops_total:0,queue_drops_total:0,flow_drops_total:0,write_drops_total:0},captureRunning:true,capture:{interface:'eth1'}};
   if(url.pathname.startsWith('/api/stream/')){latestBody=req.postDataJSON();data=rows.filter(r=>(latestBody.patternIds||[]).every(id=>r.foundPatternsIds.includes(id))&&(!latestBody.favorites||r.favorite))}
   if(url.pathname.startsWith('/api/packet/'))data=[{id:201,timestamp:Date.now(),incoming:true,content:Buffer.from('POST /api/login HTTP/1.1\r\nHost: shop\r\n\r\nusername=ctf').toString('base64'),matches:[],metadata:{path:'/api/login'}}];
   await route.fulfill({json:data});
  });
  await page.goto(base);await page.getByRole('button',{name:'SQL injection',exact:true}).waitFor();
  const chip=name=>page.getByRole('button',{name,exact:true});
  await chip('SQL injection').click();await chip('Flag').click();await page.waitForFunction(()=>location.hash.includes('patterns=1,2')||location.hash.includes('patterns=1%2C2'));
  await page.waitForTimeout(150);assert.deepEqual(latestBody.patternIds,[1,2]);assert.equal(await chip('SQL injection').getAttribute('aria-pressed'),'true');assert.equal(await chip('Flag').getAttribute('aria-pressed'),'true');assert.equal(await chip('Ignored').count(),0);
  await chip('Избранное').click();await page.waitForTimeout(100);assert(latestBody.favorites);
  await page.getByLabel('Сервис',{exact:true}).selectOption('8080');await page.waitForTimeout(100);assert.deepEqual(latestBody.patternIds,[1,2]);
  await page.locator('#stream-100 .stream-link').click();await page.locator('.gm-packet').waitFor();assert.equal(await page.locator('#stream-100 .stream-path').innerText(),'/api/login');assert.match(page.url(),/patterns=/);
  await page.reload();await page.locator('.gm-packet').waitFor();assert.equal(await chip('Flag').getAttribute('aria-pressed'),'true');
  await chip('SQL injection').focus();await page.keyboard.press('Space');await page.waitForTimeout(120);assert.deepEqual(latestBody.patternIds,[2]);
  await page.getByRole('button',{name:'Сбросить все фильтры'}).click();await page.waitForTimeout(120);assert.deepEqual(latestBody.patternIds,[]);assert.equal(latestBody.favorites,false);assert.equal(await page.getByLabel('Сервис',{exact:true}).inputValue(),'');
  for(const width of [1440,1100,375,667])for(const theme of ['dark','light']){
   const height=width===667?375:760;
   await page.setViewportSize({width,height});await page.goto(base+'/#/all/100');await page.locator('.gm-packet').waitFor();await page.evaluate(theme=>document.body.dataset.theme=theme,theme);await page.waitForTimeout(150);
   const layout=await page.evaluate(()=>{const r=s=>{const b=document.querySelector(s).getBoundingClientRect();return {height:b.height,top:b.top,bottom:b.bottom}};return {docWidth:document.documentElement.scrollWidth,list:r('.session-list-area'),filters:r('.gm-traffic-filters'),pagination:r('.session-pagination'),heading:r('.traffic-detail-heading'),toolbar:r('.packet-view-toolbar'),main:r('main'),packet:r('.gm-packet')}});
   assert(layout.docWidth<=width+1,JSON.stringify(layout));assert(layout.pagination.height<=40,JSON.stringify(layout));assert(layout.heading.height<(width>=1100?42:80),JSON.stringify(layout));
   if(width>=1100){assert(layout.list.height>440,JSON.stringify(layout));assert(layout.packet.top-layout.main.top<100,JSON.stringify(layout))}
   report.layouts.push({width,height,theme,...layout});
   if(width===1100)await page.screenshot({path:'reports/compact-traffic-'+theme+'.jpg',type:'jpeg',quality:30,scale:'css'});
  }
  await page.setViewportSize({width:375,height:640});await page.goto(base+'/#/all/');await page.locator('#stream-100 .stream-link').click();
  await page.waitForFunction(()=>{const main=document.querySelector('.gm-main-traffic'),workspace=document.querySelector('.gm-workspace');return main&&workspace&&main.getBoundingClientRect().top<=workspace.getBoundingClientRect().top+24&&document.activeElement===main});
  many=true;await page.setViewportSize({width:1100,height:760});await page.reload();await chip('Long template label 59').waitFor({state:'attached'});assert(await page.locator('.session-list-area').evaluate(e=>e.clientHeight)>350);await chip('Long template label 59').click();assert.equal(await chip('Long template label 59').getAttribute('aria-pressed'),'true');
  assert.deepEqual(errors,[]);Object.assign(report,{passed:true,pageErrors:errors,checks:['multiple labels and AND request semantics','favorites and service preserve label selection','deep link reload','keyboard toggle','clear all','IGNORE excluded','first request path displayed','compact layout in both themes','phone and landscape overflow','many wrapping labels preserve list height']});
 }finally{await browser?.close();server.kill('SIGTERM');fs.writeFileSync('reports/ui-compact-traffic.json',JSON.stringify(report,null,2)+'\n')}
 console.log(JSON.stringify(report));
})().catch(e=>{console.error(e);process.exitCode=1});
