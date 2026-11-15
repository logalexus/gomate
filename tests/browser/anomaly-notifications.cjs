const {chromium}=require('../../web/node_modules/playwright');
const assert=require('node:assert/strict'),fs=require('node:fs'),path=require('node:path'),{spawn}=require('node:child_process');
process.chdir(path.resolve(__dirname,'../..'));
const delay=ms=>new Promise(r=>setTimeout(r,ms));
(async()=>{
 const base='http://127.0.0.1:4186';
 const server=spawn(process.execPath,['node_modules/vite/bin/vite.js','--host','127.0.0.1','--port','4186','--strictPort'],{cwd:'web',stdio:'ignore'});
 let browser;const report={passed:false,checks:[],layouts:[]};
 try {
  for(let i=0;i<60;i++){try{if((await fetch(base)).ok)break}catch{}await delay(200)}
  browser=await chromium.launch();const page=await browser.newPage({viewport:{width:1366,height:900}});
  const errors=[],feed=[];let requests=0,fail=false,gap=false;
  const add=(id,score)=>feed.push({streamId:id,service:8080,score});
  add(1,100); // Historical event must not show at startup.
  page.on('pageerror',e=>errors.push(e.message));
  await page.routeWebSocket('**/api/v2/events',()=>{});
  await page.route('**/api/**',async route=>{
   const u=new URL(route.request().url());let data=[];
   if(u.pathname==='/api/v2/config')data={timeoutSeconds:10};
   if(u.pathname==='/api/v2/snapshot')data={services:[{port:8080,name:'HTTP'}],patterns:[],metrics:{},captureRunning:true};
   if(u.pathname==='/api/v2/anomalies/groups')data={groups:[],totalGroups:0,totalSessions:0,totalAnomalies:0};
   if(u.pathname==='/api/v2/anomalies/stats')data={services:[],warmAfter:100};
   if(u.pathname==='/api/v2/settings')data={settings:{revision:1,localIPs:[],packetRetentionSeconds:0,sessionRetentionSeconds:3600,requestTimeoutSeconds:10},capture:{running:true}};
   if(u.pathname==='/api/v2/anomalies/notifications'){
    requests++;if(fail){fail=false;return route.fulfill({status:503,json:{error:'temporary'}})}
    const cursor=u.searchParams.get('cursor');data={cursor:String(feed.length),items:cursor?feed.slice(+cursor):[],gap,reset:false};gap=false;
   }
   return route.fulfill({json:data});
  });
  await page.goto(base+'/#/anomalies');
  await page.waitForFunction(()=>{
   const app=document.querySelector('#app')?.__vue__;
   const notice=app?.$children.find(c=>c.$options.methods?.scheduleHide);
   if(!notice?.cursor)return false;window.notice=notice;clearInterval(notice.timer);return true;
  });
  const poll=()=>page.evaluate(async()=>{await window.notice.poll();await window.notice.$nextTick()});
  const box=page.getByTestId('anomaly-notification');
  assert.equal(await box.count(),0);report.checks.push('No historical notifications');
  add(10,39.9);await poll();assert.equal(await box.count(),0);
  add(10,40);await poll();await box.waitFor();assert.match(await box.innerText(),/Session #10/);
  await page.getByRole('button',{name:'Dismiss notification',exact:true}).click();
  add(10,90);await poll();assert.equal(await box.count(),0);
  report.checks.push('Inclusive threshold; old session crossing threshold; duplicate suppression');
  await page.getByRole('button',{name:'Settings',exact:true}).click();
  await page.getByRole('tab',{name:'Notifications',exact:true}).click();
  const input=page.getByLabel('Minimum score',{exact:true});
  assert.equal(await input.inputValue(),'40');
  await input.fill('0');await page.locator('.invalid-feedback').filter({hasText:'Current threshold: 40'}).waitFor();
  assert.equal(await page.evaluate(()=>window.notice.threshold),40);
  await input.fill('75');await delay(100);await poll();
  assert.equal(await page.evaluate(()=>JSON.parse(localStorage.vuex).anomalyNotificationScore),75);
  const enabled=page.getByLabel('Notify me about new anomalies');
  await enabled.uncheck({force:true});const before=requests;add(11,100);await poll();assert.equal(requests,before);
  await enabled.check({force:true});await delay(100);await poll();assert.equal(await box.count(),0);
  report.checks.push('Settings persist; invalid threshold rejected; disabled polling; no disabled backlog');
  await page.locator('#settingsModal .close').click();await page.locator('#settingsModal').waitFor({state:'hidden'});
  add(12,74.9);await poll();assert.equal(await box.count(),0);
  fail=true;add(13,75);await poll();assert.equal(await box.count(),0);await poll();await box.waitFor();
  assert.match(await box.innerText(),/Session #13/);report.checks.push('Retry preserves cursor and event');
  for(let i=20;i<120;i++)add(i,80+i);await poll();
  assert.equal(await box.locator('li').count(),3);assert.match(await box.innerText(),/101/);
  assert.match(await box.locator('li a').first().getAttribute('href'),/8080\/119$/);
  await box.hover();
  for(const width of [1366,375])for(const theme of ['dark','light']){
   await page.setViewportSize({width,height:900});await page.evaluate(t=>document.body.dataset.theme=t,theme);
   assert(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth+1));
   assert(await box.evaluate(e=>e.scrollWidth<=e.clientWidth+1));
   await page.screenshot({path:`reports/anomaly-notifications-${width}-${theme}.jpg`,type:'jpeg',quality:70});report.layouts.push({width,theme});
  }
  await page.evaluate(()=>{window.location.hash='#/containers'});await delay(150);assert(await box.isVisible());
  report.checks.push('Burst coalescing; three links maximum; notifications survive route changes; compact light/dark layouts');
  await page.reload();await page.waitForFunction(()=>{
   const app=document.querySelector('#app')?.__vue__,n=app?.$children.find(c=>c.$options.methods?.scheduleHide);
   if(!n?.cursor)return false;window.notice=n;clearInterval(n.timer);return true;
  });
  assert.equal(await box.count(),0);assert.equal(await page.evaluate(()=>window.notice.threshold),75);
  add(119,999);await poll();assert.equal(await box.count(),0);
  gap=true;await poll();await box.waitFor();assert.match(await box.innerText(),/Some events were skipped/);
  report.checks.push('Reload preserves settings and deduplication; overflow warning');
  await page.mouse.move(0,899);await delay(12500);assert.equal(await box.count(),0);
  add(999,100);await poll();await box.waitFor();await box.hover();await delay(12500);assert(await box.isVisible());
  await page.getByRole('button',{name:'Dismiss notification',exact:true}).click();assert.equal(await box.count(),0);
  report.checks.push('Auto-hide after 12 seconds; hover pauses dismissal');
  assert.deepEqual(errors,[]);report.passed=true;
 }finally{if(browser)await browser.close();server.kill();fs.writeFileSync('reports/anomaly-notifications-browser.json',JSON.stringify(report,null,2)+'\n')}
 console.log(JSON.stringify(report));
})().catch(e=>{console.error(e);process.exitCode=1});
