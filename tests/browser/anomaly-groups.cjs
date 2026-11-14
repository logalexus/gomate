// All retained traffic: paging, normal sessions, stale detail responses, retention.
const {chromium}=require('../../web/node_modules/playwright');
const assert=require('node:assert/strict'),fs=require('node:fs'),path=require('node:path'),{spawn}=require('node:child_process');
process.chdir(path.resolve(__dirname,'../..'));
const delay=ms=>new Promise(r=>setTimeout(r,ms));
(async()=>{
 const base='http://127.0.0.1:4179';
 const server=spawn(process.execPath,['node_modules/vite/bin/vite.js','--host','127.0.0.1','--port','4179','--strictPort'],{cwd:'web',stdio:'ignore'});
 const report={passed:false,mocked:true,checks:[],layouts:[]};let browser;
 try {
  for(let i=0;i<60;i++){try{if((await fetch(base)).ok)break}catch{}await delay(200)}
  browser=await chromium.launch({headless:true});const page=await browser.newPage({viewport:{width:1366,height:768}});
  const errors=[],writes=[],queries=[];let failMembers=false,slow=false,expired=false;
  page.on('pageerror',e=>errors.push(e.message));
  const groups=Array.from({length:105},(_,i)=>({service:8080,protocol:'TCP',method:'GET',path:'/items/'+i,count:i?1:251,anomalyCount:i?0:1,theftCount:i?0:1,dismissedCount:i?0:1,score:i?0:20,startTimestamp:Date.now()}));
  const normal=(id)=>({streamId:id,service:8080,protocol:'TCP',startTimestamp:Date.now(),hasAnomaly:false,score:0,reasons:[],summary:'',request:'GET /items/0?q='+id,payloadPurged:id===250});
  const sessions=Array.from({length:251},(_,i)=>normal(251-i));
  sessions[1]={...sessions[1],hasAnomaly:true,score:20,reasons:[{detector:'enum',bits:20,text:'Редкое значение'}],summary:'GET /items/0'};
  sessions[2]={...sessions[2],hasAnomaly:true,dismissed:true,score:30};
  await page.routeWebSocket('**/api/v2/events',()=>{});
  await page.route('**/api/**',async route=>{
   const req=route.request(),u=new URL(req.url());let data=[];
   if(req.method()!=='GET'){writes.push(u.pathname);return route.fulfill({status:204})}
   if(u.pathname==='/api/v2/snapshot') data={services:[{port:8080,name:'HTTP mock'}],patterns:[],metrics:{},captureRunning:true};
   if(u.pathname==='/api/v2/anomalies/stats') data={services:[{port:8080,messages:30000,warm:true}],drops:0,flagPattern:false,warmAfter:100};
   if(u.pathname==='/api/v2/anomalies/groups') {
    queries.push(u.toString());const offset=+u.searchParams.get('offset');
    data={groups:groups.slice(offset,offset+100),totalGroups:105,totalSessions:355,totalAnomalies:1};
   }
   if(u.pathname==='/api/v2/anomalies/sessions') {
    queries.push(u.toString());if(failMembers)return route.fulfill({status:500,json:{error:'test unavailable'}});
    const before=+u.searchParams.get('before'),alerts=u.searchParams.get('alerts')==='1';
    const rows=expired?[]:u.searchParams.get('path')==='/items/0'?sessions.filter(a=>(!before||a.streamId<before)&&(!alerts||(a.hasAnomaly&&!a.dismissed))):[normal(777)];
    data={items:rows.slice(0,50),nextBefore:rows.length>50?rows[49].streamId:0};
    if(slow&&u.searchParams.get('path')==='/items/0')await delay(600);
   }
   return route.fulfill({json:data});
  });
  await page.goto(base+'/#/anomalies');await page.locator('.anomaly-row').first().waitFor();
  assert.equal(await page.getByLabel('Показать',{exact:true}).inputValue(),'all');
  assert.equal(await page.locator('#anomaly-level').count(),0);
  assert.equal(await page.locator('.anomaly.theft .theft-label').count(),1);
  assert.match(await page.locator('.anomaly-results-heading').innerText(),/355 сохранённых сессий · 105 групп · 1 со срабатыванием/);
  await page.getByRole('button',{name:'Пауза',exact:true}).click();
  await page.locator('.anomaly-row').first().click();await page.getByRole('link',{name:'Открыть сессию',exact:true}).waitFor();
  assert.match(await page.locator('.anomaly-full-summary').innerText(),/q=251/);
  assert.equal(await page.getByRole('button',{name:'Это норма',exact:true}).count(),0);
  assert.match(await page.getByRole('link',{name:'Открыть сессию',exact:true}).getAttribute('href'),/8080\/251$/);
  await page.getByRole('button',{name:/Ещё сессии/}).click();
  await page.waitForFunction(()=>document.querySelectorAll('.anomaly-session-picker option').length===100);
  await page.locator('.anomaly-session-picker select').selectOption('250');
  await page.getByRole('button',{name:'Это норма',exact:true}).waitFor();
  assert.match(await page.locator('.anomaly-detail').innerText(),/Содержимое пакетов удалено/);
  await page.locator('label').filter({hasText:/^Только со срабатыванием$/}).click();
  await page.waitForFunction(()=>document.querySelector('.anomaly-session-label')?.textContent.includes('#250'));
  assert(queries.at(-1).includes('alerts=1'));report.checks.push('all traffic default with full totals','normal session link without learning action','member cursor paging','alert selection inside group','purged payload indication');
  await page.locator('.anomaly-row').first().click();await page.getByRole('button',{name:'Следующие группы',exact:true}).click();
  await page.waitForFunction(()=>document.querySelectorAll('.anomaly-row').length===5);
  assert.match(await page.locator('.anomaly-pagination').innerText(),/101–105 из 105/);
  await page.getByRole('button',{name:'Предыдущие группы',exact:true}).click();await page.waitForFunction(()=>document.querySelectorAll('.anomaly-row').length===100);
  report.checks.push('group pagination preserves global totals');
  slow=true;await page.locator('.anomaly-row').first().click();await page.locator('.anomaly-row').nth(1).click();
  await page.getByRole('link',{name:'Открыть сессию',exact:true}).waitFor();await delay(750);
  assert.match(await page.getByRole('link',{name:'Открыть сессию',exact:true}).getAttribute('href'),/8080\/777$/);slow=false;
  await page.locator('.anomaly-row').nth(1).click();failMembers=true;await page.locator('.anomaly-row').first().click();
  await page.getByRole('alert').filter({hasText:'Не удалось загрузить сессии'}).waitFor();assert.equal(await page.getByRole('link',{name:'Открыть сессию',exact:true}).count(),0);
  failMembers=false;expired=true;await page.getByRole('button',{name:'Повторить',exact:true}).click();
  await page.getByText('Сессий больше нет:',{exact:false}).waitFor();expired=false;await page.locator('.anomaly-row').first().click();
  report.checks.push('stale member response cannot replace another group','member failure and retry','retention during inspection');
  for(const width of [1366,1920,800,375,667]) for(const theme of ['dark','light']) {
   await page.setViewportSize({width,height:width===667?375:768});await page.evaluate(t=>document.body.dataset.theme=t,theme);
   await page.locator('main').evaluate(e=>e.scrollTop=0);await delay(50);
   const layout=await page.evaluate(()=>({docWidth:document.documentElement.scrollWidth,mainWidth:document.querySelector('main').scrollWidth,firstRowTop:document.querySelector('.anomaly-row').getBoundingClientRect().top,rowHeight:document.querySelector('.anomaly-row').getBoundingClientRect().height}));
   assert(layout.docWidth<=width+1&&layout.mainWidth<=width+1,JSON.stringify(layout));if(width>=1366)assert(layout.firstRowTop<300,JSON.stringify(layout));
   if(width===1366)await page.screenshot({path:`reports/anomaly-groups-${theme}.jpg`,type:'jpeg',quality:60});
   await page.locator('.anomaly-row').first().click();await page.getByRole('link',{name:'Открыть сессию',exact:true}).waitFor();
   assert(await page.evaluate(()=>document.querySelector('main').scrollWidth)<=width+1);
   await page.locator('.anomaly-row').first().click();report.layouts.push({width,theme,...layout});
  }
  assert.deepEqual(errors,[]);assert.deepEqual(writes,[]);report.pageErrors=errors;report.passed=true;
 }finally{await browser?.close();server.kill('SIGTERM');fs.writeFileSync('reports/anomaly-groups-ui-check.json',JSON.stringify(report,null,2)+'\n')}
 console.log(JSON.stringify(report));
})().catch(e=>{console.error(e);process.exitCode=1});
