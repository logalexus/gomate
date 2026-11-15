// Synthetic API fixtures: interaction, race handling, and responsive layout.
const {chromium} = require('../../web/node_modules/playwright');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const {spawn} = require('node:child_process');
process.chdir(path.resolve(__dirname, '../..'));
const delay = ms => new Promise(resolve => setTimeout(resolve, ms));
(async () => {
 const base = 'http://127.0.0.1:4177';
 const server = spawn(process.execPath, ['node_modules/vite/bin/vite.js', '--host', '127.0.0.1', '--port', '4177', '--strictPort'], {cwd:'web', stdio:'ignore'});
 const report = {passed:false, mocked:true, layouts:[], checks:[]};
 let browser;
 try {
  for (let i=0;i<60;i++) { try { if ((await fetch(base)).ok) break; } catch {} await delay(200); }
  browser = await chromium.launch({headless:true});
  const page = await browser.newPage({viewport:{width:1366,height:768}, reducedMotion:'reduce'});
  page.setDefaultTimeout(10000);
  const errors = [], mutations = [], queries = [];
  page.on('pageerror', e => errors.push(e.message));
  const rows = Array.from({length:25}, (_,i) => ({
   streamId:100+i, service:8080, kind:'anomaly', score:300-i*10, startTimestamp:Date.now()-i*1000, dismissed:false,
   summary:'POST /api/items/'+i+'?q='+('long-value-'.repeat(40)),
   reasons:[{detector:'enum', field:'json.tags[]', bits:9.2, template:'POST /api/items/'+i, text:'Новое значение traffic; обычно demo'},
    {detector:'shape', field:'json.payload', bits:7.1, template:'POST /api/items/'+i, text:'Редкая форма: '+('x'.repeat(400))}],
  }));
  rows.splice(1,0,{...rows[0],streamId:200,score:290,summary:'POST /api/items/0?q=second-session'});
  rows.push({...rows[0],streamId:300,dismissed:true});
  rows.push({...rows[0],streamId:400,kind:'theft',score:40});
  let slow=false, fail=false;
  await page.routeWebSocket('**/api/v2/events',()=>{});
  await page.route('**/api/**',async route => {
   const req=route.request(), u=new URL(req.url()); let data=[];
   if(u.pathname.startsWith('/api/packet/'))return route.fulfill({json:[{id:1,incoming:true,timestamp:Date.now(),kind:'raw',content:Buffer.from('TCP session '+u.pathname.split('/').pop()).toString('base64')}]});
   if (u.pathname==='/api/v2/config') data={timeoutSeconds:10};
   if (u.pathname==='/api/v2/snapshot') data={services:[{port:8080,name:'HTTP mock'},{port:9000,name:'TCP mock'}], patterns:[], metrics:{},captureRunning:true};
   if (u.pathname==='/api/v2/anomalies/groups') data={groups:[],totalGroups:0,totalSessions:0,totalAnomalies:0};
   if (u.pathname==='/api/v2/anomalies/stats') data={services:[{port:8080,messages:3000,reported:800,warm:true},{port:9000,messages:80,reported:0,warm:false}],drops:0,flagPattern:false,warmAfter:100};
   if (u.pathname==='/api/v2/anomalies') {
    queries.push(Object.fromEntries(u.searchParams));
    if (fail) return route.fulfill({status:500,json:{error:'test unavailable'}});
    data=rows.filter(a=>(!u.searchParams.has('service') || a.service===+u.searchParams.get('service')) && a.score>=+u.searchParams.get('minScore') && (!a.dismissed || u.searchParams.get('dismissed')==='1'));
    data=structuredClone(data);
    if (slow && u.searchParams.get('service')==='8080') await delay(650);
   }
   if (req.method()==='POST' && u.pathname.includes('/anomalies/')) {
    mutations.push(u.pathname);const p=u.pathname.split('/');rows.find(a=>a.streamId===+p[4]).dismissed=p[5]==='normal';
    return route.fulfill({status:204});
   }
   await route.fulfill({json:data});
  });
  await page.goto(base+'/#/anomalies');await page.getByLabel('Показать',{exact:true}).selectOption('alerts');await page.locator('.anomaly-row').first().waitFor();
  assert.equal(await page.locator('.anomaly-detail').count(),0);
  assert.equal(await page.locator('.anomaly').count(),26);
  await page.getByRole('button',{name:'Пауза',exact:true}).click();
  const row=()=>page.locator('.anomaly-row').first();
  await row().focus();await page.keyboard.press('Enter');
  assert.equal(await row().getAttribute('aria-expanded'),'true');
  await page.locator('.anomaly-session-picker select').selectOption('200');
  assert.match(await page.getByRole('link',{name:'Открыть сессию',exact:true}).getAttribute('href'),/8080\/200$/);
  assert.match(await page.locator('.anomaly-full-summary').innerText(),/second-session/);
  assert.equal(await page.locator('.anomaly-flow-panel').count(),0);
  await page.getByRole('button',{name:'Поток сессии',exact:true}).click();
  await page.locator('.anomaly-flow-panel').getByText('TCP session 200',{exact:false}).waitFor();
  await page.getByRole('button',{name:'Это норма',exact:true}).click();
  await page.waitForFunction(()=>!document.querySelector('.anomaly-detail'));
  assert.deepEqual(mutations,['/api/v2/anomalies/200/normal']);
  await page.locator('label').filter({hasText:/^Включая скрытые$/}).click();await page.waitForTimeout(100);
  // Active and dismissed sessions must never be grouped together.
  const hidden = page.locator('.anomaly.dismissed');assert.equal(await hidden.count(),1);
  await hidden.locator('.anomaly-row').click();
  await page.locator('.anomaly-session-picker select').selectOption('200');
  await page.getByRole('button',{name:'Вернуть в список',exact:true}).click();
  await page.waitForFunction(()=>!document.querySelector('.anomaly-detail'));
  assert.deepEqual(mutations,['/api/v2/anomalies/200/normal','/api/v2/anomalies/200/restore']);
  report.checks.push('keyboard disclosure','session-specific link and normal/restore mutation','dismissed groups separated');
  await page.getByLabel('Минимум баллов',{exact:true}).selectOption('40');await page.waitForTimeout(100);
  assert.equal(queries.at(-1).minScore,'40');
  assert(!(await page.locator('#anomaly-level').innerText()).includes('краж'));
  assert((await page.locator('.anomaly:not(.theft)').count())>0);
  // Later filters win even if the earlier request resolves last.
  slow=true;await page.getByLabel('Сервис',{exact:true}).selectOption('8080');await page.waitForTimeout(50);
  await page.getByLabel('Сервис',{exact:true}).selectOption('9000');
  await page.getByRole('heading',{name:'Нет срабатываний по этим фильтрам'}).waitFor();
  await page.waitForTimeout(750);assert.equal(await page.locator('.anomaly-row').count(),0);
  slow=false;await page.getByRole('button',{name:'Сбросить фильтры',exact:true}).click();await row().waitFor();
  report.checks.push('score filter does not claim theft','latest filter response wins');
  await page.getByRole('button',{name:'Продолжить автообновление',exact:true}).click();
  await row().click();const before=queries.length;await page.waitForTimeout(4300);assert.equal(queries.length,before);
  await row().focus();await page.keyboard.press('Space');
  await page.waitForFunction(()=>document.querySelector('.anomaly-row').getAttribute('aria-expanded')==='false');
  for (let i=0;i<20&&queries.length===before;i++)await delay(200);
  assert(queries.length>before);
  await page.getByRole('button',{name:'Пауза',exact:true}).click();await delay(100);
  const paused=queries.length;await page.waitForTimeout(4200);assert.equal(queries.length,paused);
  await page.getByRole('button',{name:'Обновить',exact:true}).click();await delay(100);assert(queries.length>paused);
  report.checks.push('details freeze live updates including App polling','pause and resume','manual refresh while paused');
  for (const width of [1366,1920,800,375,667]) for(const theme of ['dark','light']) {
   const height=width===667?375:768;await page.setViewportSize({width,height});
   await page.evaluate(theme=>document.body.dataset.theme=theme,theme);
   await page.locator('main').evaluate(e=>e.scrollTop=0);await delay(80);
   const layout=await page.evaluate(()=>{
    const rect=e=>{const r=e.getBoundingClientRect();return {top:r.top,bottom:r.bottom,height:r.height}};
    return {docWidth:document.documentElement.scrollWidth,mainWidth:document.querySelector('main').scrollWidth,
     first:rect(document.querySelector('.anomaly-row')),visibleRows:[...document.querySelectorAll('.anomaly-row')].filter(e=>{const r=e.getBoundingClientRect();return r.top>=0&&r.bottom<=innerHeight-30}).length};
   });
   assert(layout.docWidth<=width+1,JSON.stringify(layout));assert(layout.mainWidth<=width+1,JSON.stringify(layout));
   if(width>=1366){assert(layout.first.top<300,JSON.stringify(layout));assert(layout.visibleRows>=6,JSON.stringify(layout));assert(layout.first.height<75,JSON.stringify(layout));}
   if(width===375)assert(layout.first.top<600,JSON.stringify(layout));
   report.layouts.push({width,height,theme,...layout});
   if(width===1366)await page.screenshot({path:`reports/anomalies-compact-${theme}.jpg`,type:'jpeg',quality:55});
   await row().click();await page.getByRole('button',{name:'Как работать',exact:true}).click();
   const overflow=await page.evaluate(()=>({document:document.documentElement.scrollWidth,main:document.querySelector('main').scrollWidth}));
   assert(overflow.document<=width+1&&overflow.main<=width+1,JSON.stringify(overflow));
   await page.getByRole('button',{name:'Как работать',exact:true}).click();await row().click();
  }
  report.checks.push('compact list in 10 viewport/theme combinations','expanded details and help do not overflow');
  await page.setViewportSize({width:1366,height:768});fail=true;
  await page.getByLabel('Сервис',{exact:true}).selectOption('9000');await page.getByRole('alert').filter({hasText:'Не удалось загрузить'}).waitFor();
  assert.equal(await page.getByRole('heading',{name:'Нет срабатываний по этим фильтрам'}).count(),0);
  fail=false;await page.getByRole('button',{name:'Обновить',exact:true}).click();await page.getByRole('heading',{name:'Нет срабатываний по этим фильтрам'}).waitFor();
  assert.deepEqual(errors,[]);report.checks.push('API failure and recovery');report.passed=true;report.pageErrors=errors;
 } finally {
  await browser?.close();server.kill('SIGTERM');fs.writeFileSync('reports/anomalies-ui-check.json',JSON.stringify(report,null,2)+'\n');
 }
 console.log(JSON.stringify(report));
})().catch(e=>{console.error(e);process.exitCode=1});
