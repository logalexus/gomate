// Shared metrics UI: chart semantics, controls, errors and responsive layouts.
const {chromium}=require('../../web/node_modules/playwright');
const assert=require('node:assert/strict');
const fs=require('node:fs');
const path=require('node:path');
const {spawn}=require('node:child_process');
process.chdir(path.resolve(__dirname,'../..'));
const delay=ms=>new Promise(resolve=>setTimeout(resolve,ms));
(async()=>{
 const base='http://127.0.0.1:4181';
 const server=spawn(process.execPath,['node_modules/vite/bin/vite.js','--host','127.0.0.1','--port','4181','--strictPort'],{cwd:'web',stdio:'ignore'});
 const report={passed:false,mocked:true,checks:[],layouts:[]};let browser;
 try{
  for(let i=0;i<60;i++){try{if((await fetch(base)).ok)break}catch{}await delay(200)}
  browser=await chromium.launch({headless:true});
  const page=await browser.newPage({viewport:{width:1366,height:768},reducedMotion:'reduce'});
  page.setDefaultTimeout(10000);const errors=[],requests=[];
  page.on('pageerror',e=>{errors.push(e.message);console.error('PAGE ERROR',e.message)});page.on('console',msg=>{if(msg.type()==='error')console.error('BROWSER',msg.text())});
  let fail=false,stale=false,slow=false,empty=false,unavailable=false,loadProfile=false;
  const containers=Array.from({length:27},(_,i)=>({id:(i+1).toString(16).padStart(64,'0'),name:'service-'+String(i).padStart(2,'0'),image:'gomate/mock:latest',state:i===26?'exited':'running',point:{cpu:i===26?null:i*10,memoryUsed:1024*1024*(27-i),memoryLimit:512*1024*1024,rx:i*1000,tx:i*500,read:100,write:200,pids:i+1}}));
  const history=now=>Array.from({length:180},(_,i)=>({time:now-(179-i)*5000,cpu:i===90?null:20+i%40,memoryUsed:(2+i/180)*1024**3,memoryLimit:16*1024**3,rx:i*10000,tx:i*5000,read:i*1000,write:10000,diskUsed:50*1024**3,diskTotal:100*1024**3}));
  await page.routeWebSocket('**/api/v2/events',()=>{});
  await page.route('**/api/**',async route=>{
   const u=new URL(route.request().url());let data=[];
   if(u.pathname==='/api/v2/chat')data={messages:[],author:'tester',hasMore:false};
   if(u.pathname==='/api/v2/config')data={timeoutSeconds:10};
   if(u.pathname==='/api/v2/snapshot')data={services:[],patterns:[],metrics:{captured_total:1500,persisted_chunks_total:42,capture_drops_total:0,queue_drops_total:0,flow_drops_total:0,write_drops_total:0},captureRunning:true};
   if(u.pathname==='/api/v2/monitor'){
    requests.push(Object.fromEntries(u.searchParams));
    if(fail)return route.fulfill({status:503,json:{error:'Тест: шлюз недоступен'}});
    const now=Date.now()-(stale?60000:0),points=history(now),id=u.searchParams.get('container');
    data={sampledAt:now,intervalSeconds:5,retentionMinutes:60,selectedId:id,containers:empty?[]:containers,host:{...points.at(-1),cpus:8,load1:1.25,uptimeSeconds:90000,interfaces:['eth0'],diskAvailable:50*1024**3},hostHistory:points,containerHistory:id?points:[]};
    if(loadProfile){
     Object.assign(data.host,{cpu:90,memoryUsed:0.7*data.host.memoryLimit,diskUsed:0.25*data.host.diskTotal});
     data.hostHistory=points.map(p=>({...p,cpu:90,memoryUsed:data.host.memoryUsed,diskUsed:data.host.diskUsed}));
     data.containers=data.containers.map((c,i)=>i>2?c:({...c,point:{...c.point,cpu:[100,560,760][i],memoryUsed:[0.2,0.7,0.95][i]*c.point.memoryLimit}}));
     const selected=data.containers.find(c=>c.id===id);
     if(selected)data.containerHistory=points.map(p=>({...p,...selected.point,time:p.time}));
    }
    if(unavailable){data.host.error='Тест: метрики хоста недоступны';data.host.cpu=null;data.hostHistory=points.map(p=>({time:p.time}));data.containers=containers.map(c=>({...c,error:'Метрики недоступны',point:{}}))}
    if(slow&&id===containers[25].id)await delay(700);
   }
   await route.fulfill({json:data});
  });
  await page.goto(base+'/#/monitoring');await page.locator('.monitor-table tbody tr').first().waitFor();
  await page.getByRole('button',{name:'Пауза',exact:true}).click();
  assert.equal(await page.locator('[data-monitor-container]').count(),20);
  assert.equal(await page.locator('[data-monitor-container]').first().getAttribute('data-monitor-container'),'service-25');
  assert.equal(await page.locator('.monitor-charts svg').count(),4);
  const chart=page.locator('.resource-chart').first();
  assert((await chart.locator('path').getAttribute('d')).split('M').length===3,'null sample breaks the line');
  await chart.locator('svg').focus();await page.keyboard.press('ArrowLeft');
  assert.match(await chart.locator('.resource-chart-values').innerText(),/CPU:/);
  await page.getByLabel('Следующая страница',{exact:true}).click();assert.equal(await page.locator('[data-monitor-container]').count(),7);
  await page.getByLabel('Поиск контейнеров',{exact:true}).fill('service-03');assert.equal(await page.locator('[data-monitor-container]').count(),1);
  await page.getByRole('button',{name:'service-03',exact:true}).click();await page.locator('.monitor-detail svg').first().waitFor();
  assert.equal(await page.locator('.monitor-detail svg').count(),4);
  await page.getByLabel('Период',{exact:true}).selectOption('60');await delay(100);
  assert.equal(requests.at(-1).minutes,'60');assert.equal(requests.at(-1).container,containers[3].id);
  await page.getByLabel('Поиск контейнеров',{exact:true}).fill('');
  await page.getByLabel('Сортировка контейнеров',{exact:true}).selectOption('memoryUsed');
  assert.equal(await page.locator('[data-monitor-container]').first().getAttribute('data-monitor-container'),'service-00');
  report.checks.push('CPU and RAM sorting, search, pagination','container selection and history period','chart gaps and keyboard values');
  await page.getByLabel('Сортировка контейнеров',{exact:true}).selectOption('cpu');
  slow=true;await page.getByRole('button',{name:'service-25',exact:true}).click();await delay(60);
  await page.getByRole('button',{name:'service-24',exact:true}).click();await page.locator('.monitor-detail svg').first().waitFor();await delay(800);
  assert.equal(await page.locator('.monitor-detail h2').innerText(),'service-24');
  slow=false;report.checks.push('late responses cannot replace the selected container');
  const paused=requests.length;await delay(5300);assert.equal(requests.length,paused,'paused tab makes no requests');
  await page.getByRole('button',{name:'Продолжить',exact:true}).click();await delay(100);assert(requests.length>paused);
  await page.getByRole('button',{name:'Пауза',exact:true}).click();report.checks.push('pause and resume');
  loadProfile=true;await page.getByRole('button',{name:'Обновить',exact:true}).click();
  const hostLoads=page.locator('.monitor-kpis .resource-load');
  await hostLoads.nth(0).filter({hasText:'Высокая'}).waitFor();
  assert.match(await hostLoads.nth(0).getAttribute('class'),/gm-load-danger/);
  assert.match(await hostLoads.nth(1).getAttribute('class'),/gm-load-warning/);
  assert.match(await hostLoads.nth(2).getAttribute('class'),/gm-load-normal/);
  assert.equal(await page.getByRole('meter',{name:'CPU хоста',exact:true}).getAttribute('aria-valuenow'),'90');
  await page.getByLabel('Поиск контейнеров',{exact:true}).fill('service-0');
  for(const [i,level,percent] of [[0,'normal','12.5'],[1,'warning','70'],[2,'danger','95']]){
   const row=page.locator('[data-monitor-container="service-0'+i+'"]');
   assert.match(await row.locator('.resource-load').nth(0).getAttribute('class'),new RegExp('gm-load-'+level));
   assert.equal(await row.getByRole('meter',{name:'CPU service-0'+i,exact:true}).getAttribute('aria-valuenow'),percent);
   assert.match(await row.locator('.resource-load').nth(1).getAttribute('class'),new RegExp('gm-load-'+level));
  }
  await page.getByRole('button',{name:'service-02',exact:true}).click();
  await page.locator('.monitor-detail .resource-chart.gm-load-danger').first().waitFor();
  const gradients=await page.locator('linearGradient').evaluateAll(elements=>elements.map(e=>e.id));
  assert.equal(new Set(gradients).size,gradients.length,'each chart has its own gradient');
  assert.equal(await page.locator('.monitor-detail .resource-threshold').count(),4);
  report.checks.push('load thresholds at 70% and 90%, host cards and container meters','Docker CPU colors normalize across 8 host cores; 100% is 12.5% of the host','history gradients, threshold bands and unique SVG IDs');
  await page.getByLabel('Поиск контейнеров',{exact:true}).fill('');
  for(const width of [1366,1920,800,375])for(const theme of ['dark','light']){
   await page.setViewportSize({width,height:768});await page.evaluate(t=>document.body.dataset.theme=t,theme);await delay(60);
   const sizes=await page.evaluate(()=>({document:document.documentElement.scrollWidth,main:document.querySelector('main').scrollWidth,charts:document.querySelectorAll('.resource-chart').length}));
   assert(sizes.document<=width+1&&sizes.main<=width+1,JSON.stringify({width,...sizes}));
   report.layouts.push({width,theme,...sizes});
   if(width===1366){
    const contrasts=await page.evaluate(()=>{
     const luminance=rgb=>rgb.match(/[\d.]+/g).slice(0,3).map(Number).map(v=>{v/=255;return v<=.04045?v/12.92:((v+.055)/1.055)**2.4}).reduce((sum,v,i)=>sum+v*[.2126,.7152,.0722][i],0);
     const probe=document.createElement('span');document.body.append(probe);
     const result=['success','warning','danger'].map(tone=>{
      probe.style.color=`var(--gm-${tone})`;const foreground=luminance(getComputedStyle(probe).color);
      return {tone,ratios:['panel',tone+'-soft'].map(background=>{probe.style.backgroundColor=`var(--gm-${background})`;const back=luminance(getComputedStyle(probe).backgroundColor);return (Math.max(foreground,back)+.05)/(Math.min(foreground,back)+.05)})};
     });probe.remove();return result;
    });
    for(const {tone,ratios} of contrasts)for(const ratio of ratios)assert(ratio>=4.5,`${theme} ${tone}: contrast ${ratio}`);
    report.layouts.at(-1).loadColorContrast=contrasts;
   }
   if(width===1366&&theme==='dark'){
    await page.locator('main').evaluate(el=>el.scrollTop=0);
    await page.screenshot({path:'reports/monitoring-dark.jpg',type:'jpeg',quality:30});
   }
  }
  report.checks.push('8 viewport/theme combinations without page overflow');
  report.checks.push('normal, warning and danger text contrast exceeds 4.5:1 on cards and badges in both themes');
  await page.setViewportSize({width:1366,height:768});
  fail=true;await page.getByRole('button',{name:'Обновить',exact:true}).click();await page.getByRole('alert').filter({hasText:'Тест: шлюз недоступен'}).waitFor();assert.equal(await page.locator('[data-monitor-container]').count(),20);
  fail=false;stale=true;await page.getByRole('button',{name:'Продолжить',exact:true}).click();await page.locator('.monitor-status').filter({hasText:'Нет свежих данных'}).waitFor();
  await page.getByRole('button',{name:'Пауза',exact:true}).click();stale=false;unavailable=true;
  await page.getByRole('button',{name:'Обновить',exact:true}).click();await page.getByRole('alert').filter({hasText:'Тест: метрики хоста недоступны'}).waitFor();
  assert.equal(await page.locator('.monitor-kpis strong').first().innerText(),'—');assert.equal(await page.locator('.monitor-charts').first().locator('svg').count(),0);
  assert.equal(await page.locator('.monitor-kpis .resource-load.gm-load-unknown').count(),1);
  assert.equal(await page.locator('.monitor-table [role="meter"]').count(),0,'unavailable metrics never show healthy usage');
  unavailable=false;empty=true;await page.getByRole('button',{name:'Обновить',exact:true}).click();await page.getByText('Контейнеров пока нет',{exact:true}).waitFor();
  const beforeLeave=requests.length;await page.goto(base+'/#/chat');await delay(5300);assert.equal(requests.length,beforeLeave);
  assert.deepEqual(errors,[]);report.checks.push('API errors retain data, stale warning, unavailable metrics and empty states','polling stops on route exit');report.passed=true;
 }finally{await browser?.close();server.kill('SIGTERM');fs.writeFileSync('reports/monitoring-ui-check.json',JSON.stringify(report,null,2)+'\n')}
 console.log(JSON.stringify(report));
})().catch(e=>{console.error(e);process.exitCode=1});
