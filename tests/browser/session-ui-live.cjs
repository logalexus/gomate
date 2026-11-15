// Read-only check: no chat sends, container mutations or runner jobs on the instance.
const {chromium}=require('../../web/node_modules/playwright');
const fs=require('node:fs'),assert=require('node:assert/strict');
(async()=>{
 const base=process.env.GOMATE_URL||'http://192.168.56.103:65000',password=fs.readFileSync('.cache/kali.env','utf8').split('\n').find(x=>x.startsWith('WEB_PASSWORD=')).slice(13).replace(/^['"]|['"]$/g,'');
 const browser=await chromium.launch({headless:true}),report={passed:false,readOnly:true,at:new Date().toISOString(),errors:[],blockedWrites:[],exports:[],layouts:[]};
 try{
  const page=await browser.newPage({viewport:{width:1440,height:900},httpCredentials:{username:'packmate',password}});page.on('pageerror',e=>report.errors.push(e.message));await page.routeWebSocket('**/api/v2/events',()=>{});
  await page.route('**/api/**',r=>{const u=new URL(r.request().url());if(r.request().method()!=='GET'&&!(r.request().method()==='POST'&&/^\/api\/(packet|stream)\//.test(u.pathname))){report.blockedWrites.push(u.pathname);return r.abort()}return r.continue()});
  const snapshot=await (await page.request.get(base+'/api/v2/snapshot')).json();report.captureBefore=snapshot.capture;
  await page.goto(base+'/#/anomalies');await page.getByLabel('Порядок',{exact:true}).selectOption('time');await page.locator('.anomaly-row').first().waitFor();await page.locator('.anomaly-row').first().click();
  const flow=page.locator('.anomaly-flow-panel');await flow.waitFor();await flow.getByRole('button',{name:'Скрипт сессии',exact:true}).waitFor();assert(page.url().endsWith('/#/anomalies'));report.flowVisible=true;
  await flow.getByRole('button',{name:'В общий чат',exact:true}).click();let dialog=page.getByRole('dialog');await dialog.locator('.session-share-preview a').waitFor();assert.match(await dialog.locator('.session-share-preview a').getAttribute('href'),/\/#\/\d+\/\d+$/);await dialog.getByRole('button',{name:'Отмена',exact:true}).click();await dialog.waitFor({state:'hidden'});
  for(const theme of ['light','dark']){await page.evaluate(t=>document.body.dataset.theme=t,theme);assert(!(await page.evaluate(()=>document.documentElement.scrollWidth>innerWidth+1)));report.layouts.push({view:'anomalies',width:1440,theme})}
  await page.setViewportSize({width:375,height:812});await page.getByRole('dialog').waitFor();assert(!(await page.evaluate(()=>document.documentElement.scrollWidth>innerWidth+1)));await page.getByRole('button',{name:'Закрыть поток',exact:true}).click();await page.getByRole('dialog').waitFor({state:'hidden'});report.layouts.push({view:'anomalies',width:375});
  await page.goto(base+'/#/containers');await page.locator('.container-row').first().waitFor();report.containers=await page.locator('.container-row').count();await page.locator('.container-disclosure').first().click();await page.locator('.container-detail').waitFor();
  for(const width of [375,1440]){await page.setViewportSize({width,height:900});assert(!(await page.evaluate(()=>document.documentElement.scrollWidth>innerWidth+1)));report.layouts.push({view:'containers',width})}
  const row=page.locator('.container-row').filter({hasText:'gomate-lab-app'});if(await row.count()){await row.getByRole('button',{name:/Логи контейнера/}).click();await page.locator('.log-row').first().waitFor();report.logsVisible=true}
  // Pick fresh complete sessions, rather than retention-expired anomaly examples.
  for(const expected of ['http','tcp']){
   let checked=false;
   for(const service of snapshot.services.filter(s=>expected==='http'?s.http:!s.http)){
    if(checked)break;const response=await page.request.post(base+'/api/stream/'+service.port,{data:{pageSize:10}});if(!response.ok())continue;const streams=await response.json();if(!Array.isArray(streams))continue;
    for(const stream of streams.filter(s=>s.protocol==='TCP'&&!s.payloadPurged&&!s.incomplete&&s.sizeBytes>0).slice(0,3)){
     await page.goto(base+'/#/'+service.port+'/'+stream.id);await page.getByRole('button',{name:'Скрипт сессии',exact:true}).click();dialog=page.getByRole('dialog');await dialog.getByLabel('Шаблон',{exact:true}).waitFor();
     await page.waitForFunction(()=>document.querySelector('#export-host')&&!document.querySelector('#export-host').disabled);
     const code=await dialog.locator('.script-editor').evaluate(el=>el.__vue__.value);const format=await dialog.getByLabel('Шаблон',{exact:true}).inputValue();
     if(code&&((expected==='http'&&format==='requests')||(expected==='tcp'&&format==='tcp'))){assert(code.includes(expected==='http'?'requests.Session()':'socket.create_connection'));report.exports.push({kind:expected,streamId:stream.id,codeBytes:Buffer.byteLength(code)});checked=true}
     await dialog.locator('.script-editor').click();await page.keyboard.press('Escape');await dialog.waitFor({state:'hidden'});if(checked)break;
    }
   }
   assert(checked,'No fresh '+expected+' session successfully exported');
  }
  report.captureAfter=(await (await page.request.get(base+'/api/v2/snapshot')).json()).capture;assert.equal(report.captureAfter.startedAt,report.captureBefore.startedAt);assert(report.captureAfter.running);assert.deepEqual(report.errors,[]);assert.deepEqual(report.blockedWrites,[]);report.passed=true;
 }finally{await browser.close();fs.writeFileSync('reports/session-ui-live.json',JSON.stringify(report,null,2)+'\n')}
 console.log(JSON.stringify(report));
})().catch(e=>{console.error(e);process.exitCode=1});
