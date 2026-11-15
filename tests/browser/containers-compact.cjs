// Compact container management; all Docker reads and actions are mocked.
const {chromium}=require('../../web/node_modules/playwright');
const assert=require('node:assert/strict'),fs=require('node:fs'),path=require('node:path'),{spawn}=require('node:child_process');
process.chdir(path.resolve(__dirname,'../..'));
const delay=ms=>new Promise(r=>setTimeout(r,ms));
(async()=>{
 const base='http://127.0.0.1:4183';
 const server=spawn(process.execPath,['node_modules/vite/bin/vite.js','--host','127.0.0.1','--port','4183','--strictPort'],{cwd:'web',stdio:'ignore'});
 const report={passed:false,mocked:true,checks:[],layouts:[]};let browser;
 try {
  for(let i=0;i<60;i++){try{if((await fetch(base)).ok)break}catch{}await delay(200)}
  browser=await chromium.launch({headless:true});const page=await browser.newPage({viewport:{width:1366,height:768}});
  const errors=[],writes=[],logQueries=[];
  const containers=Array.from({length:28},(_,i)=>({id:i.toString(16).padStart(64,'a'),name:i===0?'web-api':i===1?'gomate':i===2?'worker':'service-'+i,image:'registry.internal.example/team/deeply/nested/application:'+('release-'.repeat(16))+i,state:i===2?'exited':i===3?'paused':'running',status:i===2?'Exited (0) 2 minutes ago':'Up 2 hours (healthy)',ports:i===0?['0.0.0.0:8080 → 80/tcp','[::]:8443 → 443/tcp','0.0.0.0:9000 → 9000/tcp']:[],protected:i===1}));
  let failContainers=false;
  page.on('pageerror',e=>errors.push(e.message));
  await page.routeWebSocket('**/api/v2/events',()=>{});
  await page.route('**/api/**',async route=>{
   const req=route.request(),u=new URL(req.url());let data=[];
   if(req.method()!=='GET'){
    writes.push({path:u.pathname,body:req.postDataJSON()});
    const c=containers.find(c=>u.pathname.includes(c.id));
    if(c)c.state=u.pathname.endsWith('/stop')?'exited':u.pathname.endsWith('/pause')?'paused':'running';
    return route.fulfill({status:204});
   }
   if(u.pathname==='/api/v2/config')data={timeoutSeconds:10};
   if(u.pathname==='/api/v2/snapshot')data={services:[],patterns:[],metrics:{},captureRunning:true};
   if(u.pathname==='/api/v2/containers'){
    if(failContainers)return route.fulfill({status:503,json:{error:'Docker unavailable'}});
    data=containers;
   }
   if(u.pathname.endsWith('/logs')){
    logQueries.push(u.toString());data={tty:false,truncated:false,fetchedAt:new Date().toISOString(),lines:[{id:'1',time:new Date().toISOString(),stream:'stdout',text:'GET /health 200'},{id:'2',time:new Date().toISOString(),stream:'stderr',text:'connection failed '+('detail '.repeat(90))},{id:'3',time:new Date().toISOString(),stream:'stdout',text:'ready for requests'}]};
   }
   return route.fulfill({json:data});
  });
  await page.goto(base+'/#/containers');await page.locator('.container-row').first().waitFor();
  assert.equal(await page.locator('.container-row').count(),25);
  await page.getByRole('button',{name:'Container details web-api',exact:true}).focus();await page.keyboard.press('Enter');
  const details=page.locator('[data-container="web-api"] .container-detail');await details.waitFor();
  assert.match(await details.innerText(),new RegExp(containers[0].id));assert.match(await details.innerText(),/9000/);
  assert.equal(await page.getByRole('button',{name:'Container details web-api',exact:true}).getAttribute('aria-expanded'),'true');
  await page.keyboard.press('Enter');assert.equal(await details.count(),0);
  await page.getByRole('button',{name:'All container ports web-api',exact:true}).click();await details.waitFor();
  report.checks.push('keyboard disclosure, full image and ID, all published ports');
  await page.getByRole('button',{name:'Stop',exact:true}).click();
  await page.getByRole('button',{name:'Cancel',exact:true}).click();await page.locator('.modal').waitFor({state:'hidden'});assert.equal(writes.length,0);
  await page.getByRole('button',{name:'Stop',exact:true}).click();
  await page.getByRole('dialog').getByRole('button',{name:'Stop',exact:true}).click();await page.locator('.modal').waitFor({state:'hidden'});
  await details.getByRole('button',{name:'Run',exact:true}).waitFor();assert.deepEqual(writes,[{path:`/api/v2/containers/${containers[0].id}/stop`,body:{confirm:true}}]);
  await details.getByRole('button',{name:'Run',exact:true}).click();await details.getByRole('button',{name:'Pause',exact:true}).waitFor();
  await page.getByRole('button',{name:'Container details gomate',exact:true}).click();
  assert.equal(await page.locator('[data-container="gomate"] .container-detail button').count(),0);
  assert.match(await page.locator('[data-container="gomate"] .container-detail').innerText(),/view only/);
  report.checks.push('cancel does not submit, confirmed stop and start reflect new state, protected containers have no management actions');
  await page.getByLabel('Search containers',{exact:true}).fill('8443');assert.equal(await page.locator('.container-row').count(),1);
  await page.getByLabel('Search containers',{exact:true}).fill(containers[2].id);assert.equal(await page.locator('.container-row').count(),1);
  await page.getByRole('button',{name:'Reset filters',exact:true}).click();
  await page.getByLabel('Container states',{exact:true}).selectOption('paused');assert.equal(await page.locator('.container-row').count(),1);
  await page.getByRole('button',{name:'Reset filters',exact:true}).click();
  await page.getByLabel('Search containers',{exact:true}).fill('does-not-exist');await page.getByText('Nothing found',{exact:true}).waitFor();
  await page.getByRole('button',{name:'Show all containers',exact:true}).click();
  await page.locator('.container-pagination .page-item').filter({hasText:/^2$/}).locator('button').click();assert.equal(await page.locator('.container-row').count(),3);
  await page.locator('.container-pagination .page-item').filter({hasText:/^1$/}).locator('button').click();
  report.checks.push('port and ID search, state filter, empty state reset, pagination');
  await page.getByRole('button',{name:'Container logs web-api',exact:true}).click();await page.locator('.log-row').first().waitFor();
  assert.equal(await page.getByLabel('Container',{exact:true}).inputValue(),containers[0].id);
  await page.locator('label').filter({hasText:/^LIVE · 2 s$/}).click();
  await page.getByLabel('Log stream',{exact:true}).selectOption('stderr');assert.equal(await page.locator('.log-row').count(),1);
  await page.locator('.log-row').click();await page.getByRole('dialog').waitFor();assert.match(await page.locator('.full-log').innerText(),/connection failed/);
  await page.getByRole('button',{name:'Close',exact:true}).last().click();await page.locator('.modal').waitFor({state:'hidden'});
  await page.getByLabel('Log stream',{exact:true}).selectOption('all');
  await page.getByLabel('Search logs',{exact:true}).fill('/health');await page.waitForFunction(()=>document.querySelectorAll('.log-row').length===1);
  await page.getByLabel('Period',{exact:true}).selectOption('0');await page.getByLabel('Last lines',{exact:true}).selectOption('100');
  await page.waitForFunction(()=>document.querySelector('.logs-source-controls .btn:not(:disabled)'));
  assert(logQueries.some(q=>q.includes('since=0')));assert(logQueries.some(q=>q.includes('tail=100')));
  report.checks.push('logs retain selected container, stream and text filters, full line, time and tail controls');
  await page.getByRole('alert').filter({hasText:'action completed'}).getByRole('button',{name:'Close'}).last().click();
  for(const width of [1366,1920,800,375,667])for(const theme of ['dark','light']){
   await page.setViewportSize({width,height:width===667?375:768});await page.evaluate(t=>document.body.dataset.theme=t,theme);
   await page.getByRole('tab',{name:'Manage',exact:true}).click();
   await page.getByRole('button',{name:'Container details web-api',exact:true}).click();
   const layout=await page.evaluate(()=>({docWidth:document.documentElement.scrollWidth,mainWidth:document.querySelector('main').scrollWidth,firstRowTop:document.querySelector('.container-row').getBoundingClientRect().top,rowHeight:document.querySelector('.container-row').getBoundingClientRect().height}));
   assert(layout.docWidth<=width+1&&layout.mainWidth<=width+1,JSON.stringify(layout));
   if(width>=1366){assert(layout.rowHeight<=65,JSON.stringify(layout));assert(layout.firstRowTop<300,JSON.stringify(layout));}
   if(width===1366||width===375)await page.screenshot({path:`reports/containers-compact-${width}-${theme}.jpg`,type:'jpeg',quality:65});
   await page.getByRole('button',{name:'Container details web-api',exact:true}).click();
   await page.getByRole('tab',{name:'Logs',exact:true}).click();
   assert(await page.evaluate(()=>document.querySelector('main').scrollWidth)<=width+1);
   report.layouts.push({width,theme,...layout});
  }
  await page.getByRole('tab',{name:'Manage',exact:true}).click();failContainers=true;
  await page.getByRole('button',{name:'Refresh',exact:true}).click();await page.getByRole('alert').filter({hasText:'Docker unavailable'}).waitFor();
  failContainers=false;await page.getByRole('button',{name:'Refresh',exact:true}).click();await page.getByRole('alert').filter({hasText:'Docker unavailable'}).waitFor({state:'hidden'});
  report.checks.push('dark/light desktop, tablet, mobile and landscape layouts; Docker failure and retry');
  assert.deepEqual(errors,[]);assert.equal(writes.length,2);report.pageErrors=errors;report.passed=true;
 }finally{await browser?.close();server.kill('SIGTERM');fs.writeFileSync('reports/containers-compact-ui.json',JSON.stringify(report,null,2)+'\n')}
 console.log(JSON.stringify(report));
})().catch(e=>{console.error(e);process.exitCode=1});
