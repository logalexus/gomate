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
  const errors=[],writes=[],queries=[],packetQueries=[];let failMembers=false,slow=false,expired=false,groupDelay=0,memberDelay=0,failGroups=false;
  let settings={revision:1,captureEnabled:false,interface:'',autoIPs:true,localIPs:[],packetRetentionSeconds:0,sessionRetentionSeconds:3600,cleanupIntervalSeconds:5,preserveFavorites:true,requestTimeoutSeconds:10};
  page.on('pageerror',e=>errors.push(e.message));
  const groups=Array.from({length:105},(_,i)=>({service:8080,protocol:'TCP',method:'GET',path:'/items/'+i,fingerprint:i.toString(16).padStart(32,'0'),fields:['1:http/json.command = cat /srv/flag; id'],variableFields:['1:http/json.nonce','1:http/cookie.session'],count:i?1:251,anomalyCount:i?0:1,foundPatternsIds:i?[]:[1,2,3,4],dismissedCount:i?0:1,score:i?0:20,startTimestamp:Date.now()}));
  const normal=(id)=>({streamId:id,service:8080,protocol:'TCP',startTimestamp:Date.now(),foundPatternsIds:id===251?[1]:[2],hasAnomaly:false,score:0,reasons:[],summary:'',request:'GET /items/0?q='+id,payloadPurged:id===250});
  const sessions=Array.from({length:251},(_,i)=>normal(251-i));
  sessions[1]={...sessions[1],hasAnomaly:true,score:20,reasons:[{detector:'enum',bits:20,text:'Редкое значение'}],summary:'GET /items/0'};
  sessions[2]={...sessions[2],hasAnomaly:true,dismissed:true,score:30};
  await page.routeWebSocket('**/api/v2/events',()=>{});
  await page.route('**/api/**',async route=>{
   const req=route.request(),u=new URL(req.url());let data=[];
   if(u.pathname.startsWith('/api/packet/')) {
    const id=Number(u.pathname.split('/').pop()),body=req.postDataJSON();packetQueries.push({id,...body});
    const next=body.startingFrom!=null;
    return route.fulfill({headers:{'x-has-more':String(!next)},json:[{id:next?id*10+3:id*10+1,incoming:true,timestamp:Date.now(),kind:'http-request',content:Buffer.from('GET /session/'+id+(next?'/second':'')+' HTTP/1.1\r\nHost: mock\r\n\r\n').toString('base64')},...next?[]:[{id:id*10+2,incoming:false,timestamp:Date.now()+1,kind:'http-response',content:Buffer.from('HTTP/1.1 200 OK\r\n\r\nresponse-for-'+id).toString('base64')}]]});
   }
   if(req.method()!=='GET'){
    writes.push(u.pathname);
    if(req.method()==='PUT'&&u.pathname==='/api/v2/settings'){
     settings={...req.postDataJSON(),revision:settings.revision+1};
     return route.fulfill({json:{settings,capture:{running:false}}});
    }
    return route.fulfill({status:204});
   }
   if(u.pathname==='/api/v2/settings') data={settings,capture:{running:false}};
   if (u.pathname==='/api/v2/config') data={timeoutSeconds:settings.requestTimeoutSeconds};
   if(u.pathname==='/api/v2/snapshot') data={services:[{port:8080,name:'HTTP mock'}],patterns:[{id:1,name:'Флаги',color:'#ff0000',actionType:'FIND'},{id:2,name:'SQL injection',color:'#ffaa00',actionType:'FIND'},{id:3,name:'Очень длинное название шаблона для проверки компактности',actionType:'FIND',deleted:true}],metrics:{},captureRunning:true};
   if(u.pathname==='/api/v2/anomalies/stats') data={services:[{port:8080,messages:30000,warm:true}],drops:0,flagPattern:false,warmAfter:100};
   if(u.pathname==='/api/v2/anomalies/groups') {
    queries.push(u.toString());
    if(failGroups)return route.fulfill({status:500,json:{error:'test unavailable'}});
    if(groupDelay)await delay(groupDelay);
    const offset=+u.searchParams.get('offset');
    data={groups:groups.slice(offset,offset+100),totalGroups:105,totalSessions:355,totalAnomalies:1};
   }
   if(u.pathname==='/api/v2/anomalies') data=[sessions[1]];
   if(u.pathname==='/api/v2/anomalies/sessions') {
    if(memberDelay)await delay(memberDelay);
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
  assert.equal(await page.getByLabel('Объединять',{exact:true}).inputValue(),'behavior');
  assert(queries.some(q=>q.includes('grouping=behavior')));
  assert.match(await page.locator('.anomaly-reason-preview').first().innerText(),/cat \/srv\/flag/);
  assert.equal(await page.locator('.theft-label').count(),0);
  assert.match(await page.locator('.anomaly-row .pattern-labels').first().innerText(),/Флаги[\s\S]*SQL injection[\s\S]*удалён[\s\S]*\+1/);
  assert.match(await page.locator('.anomaly-results-heading').innerText(),/355 сохранённых сессий · 105 групп · 1 со срабатыванием/);
  await page.getByRole('button',{name:'Пауза',exact:true}).click();
  await page.locator('.anomaly-row').first().click();await page.getByRole('link',{name:'Открыть сессию',exact:true}).waitFor();
  assert.match(await page.locator('.anomaly-full-summary').innerText(),/q=251/);
  assert.equal(await page.locator('.session-patterns .pattern-labels').innerText(),'Флаги');
  const flow=()=>page.locator('.anomaly-flow-panel');
  await flow().getByText('response-for-251',{exact:false}).waitFor();
  assert.match(await flow().innerText(),/Поток сессии #251/);
  assert.match(page.url(),/#\/anomalies$/);
  await flow().getByRole('button',{name:'Следующие блоки',exact:true}).click();
  await flow().getByText('/session/251/second',{exact:false}).waitFor();
  assert(packetQueries.some(q=>q.id===251&&q.startingFrom===2512));
  await flow().getByRole('button',{name:'Предыдущие блоки',exact:true}).click();
  await flow().getByText('response-for-251',{exact:false}).waitFor();
  assert(queries.at(-1).includes('fingerprint=00000000000000000000000000000000'));
  assert.match(await page.locator('.variable-features').innerText(),/Меняющиеся поля[\s\S]*nonce/);
  assert(await page.locator('.common-features').evaluate(e=>e.open));
  assert.match(await page.locator('.behavior-details ul').innerText(),/cat \/srv\/flag/);
  assert.equal(await page.getByRole('button',{name:'Это норма',exact:true}).count(),0);
  assert.match(await page.getByRole('link',{name:'Открыть сессию',exact:true}).getAttribute('href'),/8080\/251$/);
  await page.getByRole('button',{name:/Ещё сессии/}).click();
  await page.waitForFunction(()=>document.querySelectorAll('.anomaly-session-picker option').length===100);
  await page.locator('.anomaly-session-picker select').selectOption('250');
  assert.equal(await page.locator('.session-patterns .pattern-labels').innerText(),'SQL injection');
  await flow().getByText('response-for-250',{exact:false}).waitFor();
  assert.equal(await flow().getByText('response-for-251',{exact:false}).count(),0);
  await page.getByRole('button',{name:'Закрыть поток',exact:true}).click();
  assert.equal(await flow().count(),0);
  await page.getByRole('button',{name:'Поток сессии',exact:true}).click();
  await flow().getByText('response-for-250',{exact:false}).waitFor();
  report.checks.push('side flow keeps anomaly route, shows requests and responses, pages through session, follows selection, closes and reopens');
  report.checks.push('group pattern union, compact overflow, deleted pattern label, selected session labels independent of score');
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
  await page.getByText('Сессии удалены или разбиение на группы изменилось.',{exact:false}).waitFor();expired=false;await page.locator('.anomaly-row').first().click();
  report.checks.push('stale member response cannot replace another group','member failure and retry','retention during inspection');
  await page.getByLabel('Показать',{exact:true}).selectOption('alerts');
  await page.waitForFunction(()=>document.querySelectorAll('.anomaly-row').length===1);
  groupDelay=6000;
  await page.getByLabel('Показать',{exact:true}).selectOption('all');
  await page.waitForFunction(()=>document.querySelectorAll('.anomaly-row').length===100);
  assert.equal(await page.locator('.anomaly-notice.error').count(),0);groupDelay=0;
  memberDelay=6000;await page.locator('.anomaly-row').first().click();
  await page.getByRole('link',{name:'Открыть сессию',exact:true}).waitFor();memberDelay=0;
  await page.locator('.anomaly-row').first().click();
  report.checks.push('alerts to all sessions with response exceeding five seconds','slow group members remain selectable');
  failGroups=true;await page.getByRole('button',{name:'Обновить',exact:true}).click();
  await page.getByRole('heading',{name:'Результаты недоступны'}).waitFor();
  assert.equal(await page.locator('.anomaly-row').count(),0);
  failGroups=false;await page.getByRole('button',{name:'Обновить',exact:true}).click();
  await page.waitForFunction(()=>document.querySelectorAll('.anomaly-row').length===100);
  report.checks.push('failed refresh clears obsolete rows and retry recovers');
  await page.getByRole('button',{name:'Настройки',exact:true}).click();
  await page.locator('#capture-interface').waitFor();
  await page.getByRole('tab',{name:'Приложение',exact:true}).click();
  const timeoutInput=page.getByLabel('Время ожидания ответа, секунд',{exact:true});
  assert.equal(await timeoutInput.inputValue(),'10');
  await timeoutInput.fill('4');await page.getByRole('button',{name:'Сохранить и применить',exact:true}).click();
  await page.getByText('Время ожидания ответа: целое число от 5 до 300 секунд.',{exact:true}).waitFor();
  assert.deepEqual(writes,[]);
  await timeoutInput.fill('30');await page.getByRole('button',{name:'Сохранить и применить',exact:true}).click();
  await page.getByText('Настройки сохранены',{exact:true}).waitFor();
  assert.equal(settings.requestTimeoutSeconds,30);
  await page.getByRole('button',{name:'Закрыть',exact:true}).click();
  await page.locator('#settingsModal').waitFor({state:'hidden'});
  await page.getByLabel('Показать',{exact:true}).selectOption('alerts');
  await page.waitForFunction(()=>document.querySelectorAll('.anomaly-row').length===1);
  groupDelay=16000;
  await page.getByLabel('Показать',{exact:true}).selectOption('all');
  await page.waitForFunction(()=>document.querySelectorAll('.anomaly-row').length===100);
  assert.equal(await page.locator('.anomaly-notice.error').count(),0);groupDelay=0;
  report.checks.push('timeout validation and save without reload','configured timeout allows response beyond old fifteen-second limit');
  for(const width of [1366,1920,800,375,667]) for(const theme of ['dark','light']) {
   await page.setViewportSize({width,height:width===667?375:768});await page.evaluate(t=>document.body.dataset.theme=t,theme);
   await page.locator('main').evaluate(e=>e.scrollTop=0);await delay(50);
   const layout=await page.evaluate(()=>({docWidth:document.documentElement.scrollWidth,mainWidth:document.querySelector('main').scrollWidth,firstRowTop:document.querySelector('.anomaly-row').getBoundingClientRect().top,rowHeight:document.querySelector('.anomaly-row').getBoundingClientRect().height}));
   assert(layout.docWidth<=width+1&&layout.mainWidth<=width+1,JSON.stringify(layout));if(width>=1366)assert(layout.firstRowTop<300,JSON.stringify(layout));
   if(width===1366)await page.screenshot({path:`reports/anomaly-groups-${theme}.jpg`,type:'jpeg',quality:60});
   await page.locator('.anomaly-row').first().click();await page.getByRole('link',{name:'Открыть сессию',exact:true}).waitFor();
   assert(await page.evaluate(()=>document.querySelector('main').scrollWidth)<=width+1);
   if(width<1200) {
    assert.equal(await page.locator('#anomaly-flow-modal').count(),0);
    await page.getByRole('button',{name:'Поток сессии',exact:true}).click();
    await page.getByRole('dialog').getByText('response-for-251',{exact:false}).waitFor();
    assert(await page.getByRole('dialog').evaluate(e=>e.scrollWidth)<=width+1);
    if(width===375)await page.screenshot({path:`reports/anomaly-flow-mobile-${theme}.jpg`,type:'jpeg',quality:65});
    await page.waitForFunction(()=>document.activeElement?.closest('#anomaly-flow-modal'));
    if(width===375) {
     await page.locator('#anomaly-flow-modal').getByRole('button',{name:'В общий чат',exact:true}).click();
     const share=page.getByRole('dialog',{name:'Отправить сессию в общий чат',exact:true});
     await share.getByLabel('Комментарий',{exact:false}).fill('Проверка вложенного окна');
     assert.match(await share.innerText(),/сессию #251/);
     await share.getByRole('button',{name:'Отмена',exact:true}).click();
     await share.waitFor({state:'hidden'});
     await page.waitForFunction(()=>document.activeElement?.closest('.session-share'));
     assert.match(await page.locator('#anomaly-flow-modal').innerText(),/response-for-251/);
    }
    await page.keyboard.press('Escape');await page.getByRole('dialog').waitFor({state:'hidden'});
    await page.waitForFunction(()=>document.activeElement?.classList.contains('open-session-flow'));
   } else {
    await flow().getByText('response-for-251',{exact:false}).waitFor();
    const bounds=await page.evaluate(()=>({list:document.querySelector('.anomaly-list').getBoundingClientRect().right,flow:document.querySelector('.anomaly-flow-panel').getBoundingClientRect().left}));
    assert(bounds.flow>bounds.list,JSON.stringify(bounds));
    if(width===1366)await page.screenshot({path:`reports/anomaly-flow-desktop-${theme}.jpg`,type:'jpeg',quality:65});
   }
   await page.locator('.anomaly-row').first().click();report.layouts.push({width,theme,...layout});
  }
  assert.deepEqual(errors,[]);assert.deepEqual(writes,['/api/v2/settings']);report.pageErrors=errors;report.passed=true;
 }finally{await browser?.close();server.kill('SIGTERM');fs.writeFileSync('reports/anomaly-groups-ui-check.json',JSON.stringify(report,null,2)+'\n')}
 console.log(JSON.stringify(report));
})().catch(e=>{console.error(e);process.exitCode=1});
