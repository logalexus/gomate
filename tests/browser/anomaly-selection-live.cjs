const {chromium}=require('../../web/node_modules/playwright');
const fs=require('node:fs'),assert=require('node:assert/strict');
process.chdir(require('node:path').resolve(__dirname,'../..'));
(async()=>{
 const before=process.argv.includes('--before'),base='http://192.168.56.103:65000';
 const password=fs.readFileSync('.cache/kali.env','utf8').split('\n').find(s=>s.startsWith('WEB_PASSWORD=')).split('=').slice(1).join('=').replace(/^['"]|['"]$/g,'');
 const browser=await chromium.launch({headless:true});
 const report={passed:false,beforeFix:before,at:new Date().toISOString(),readOnly:true,errors:[],writes:[]};
 try{
  const page=await browser.newPage({viewport:{width:1366,height:768},httpCredentials:{username:'packmate',password}});
  const snapshot=await page.request.get(base+'/api/v2/snapshot');
  report.captureBefore=(await snapshot.json()).capture;
  let delayGroups=false;
  page.on('pageerror',e=>report.errors.push(e.message));
  await page.routeWebSocket('**/api/v2/events',()=>{});
  await page.route('**/api/**',async r=>{
   if(r.request().method()!=='GET'){report.writes.push(r.request().url());return r.abort()}
   if(delayGroups&&r.request().url().includes('/anomalies/groups')){
    const response=await r.fetch();await new Promise(resolve=>setTimeout(resolve,6000));
    return r.fulfill({response}).catch(()=>{});
   }
   return r.continue();
  });
  await page.goto(base+'/#/anomalies');
  await page.getByRole('button',{name:'Пауза',exact:true}).click();
  await page.getByLabel('Показать',{exact:true}).selectOption('alerts');
  await page.locator('.anomaly-row').first().waitFor();
  report.alertRows=await page.locator('.anomaly-row').count();
  delayGroups=true;const t=Date.now();
  await page.getByLabel('Показать',{exact:true}).selectOption('all');
  if(before){
   await page.locator('.anomaly-notice.error').waitFor();
   report.errorText=await page.locator('.anomaly-notice.error').innerText();
   assert.match(report.errorText,/5000/);report.reproduced=true;
  }else{
   await page.locator('.anomaly-row').first().waitFor();
   assert.equal(await page.locator('.anomaly-notice.error').count(),0);
   report.summary=await page.locator('.anomaly-results-heading').innerText();
   report.allRows=await page.locator('.anomaly-row').count();
  }
  report.switchLatencyMs=Date.now()-t;delayGroups=false;
  if(!before){
   await page.locator('.anomaly-row').first().click();await page.getByRole('link',{name:'Открыть сессию',exact:true}).waitFor();
   report.sessionLink=await page.getByRole('link',{name:'Открыть сессию',exact:true}).getAttribute('href');
   await page.getByRole('button',{name:'Настройки',exact:true}).click();
   await page.locator('#capture-interface').waitFor();
   await page.getByRole('tab',{name:'Хранение и очистка',exact:true}).click();
   await page.getByText('Удалится только содержимое пакетов. Сессии, аномалии и фингерпринты останутся.',{exact:true}).waitFor();
   report.cleanupJobs=await page.locator('.settings-job').allTextContents();
   assert(report.cleanupJobs.some(s=>s.includes('содержимое пакетов')&&s.includes('фингерпринты сохранены')));
   await page.locator('#cleanup-mode').selectOption('sessions');
   await page.getByText('Удалятся сессии, их содержимое и связанные срабатывания.',{exact:false}).waitFor();
   assert(await page.getByRole('button',{name:'Удалить…',exact:true}).isDisabled());
   for(const width of [1366,375]){
    await page.setViewportSize({width,height:768});
    assert(await page.locator('#settingsModal .modal-content').evaluate(el=>el.scrollWidth<=el.clientWidth+1));
   }
  }
  const snapshotAfter=await page.request.get(base+'/api/v2/snapshot');
  report.captureAfter=(await snapshotAfter.json()).capture;
  assert.equal(report.captureAfter.startedAt,report.captureBefore.startedAt);
  assert(report.captureAfter.running);
  assert.deepEqual(report.errors,[]);assert.deepEqual(report.writes,[]);report.passed=true;
 }finally{
  await browser.close();fs.writeFileSync('reports/anomaly-selection-'+(before?'before':'live')+'-check.json',JSON.stringify(report,null,2)+'\n');
 }
 console.log(JSON.stringify(report));
})().catch(e=>{console.error(e);process.exitCode=1});
