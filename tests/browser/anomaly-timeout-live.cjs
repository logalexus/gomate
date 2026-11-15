const {chromium}=require('../../web/node_modules/playwright');
const fs=require('node:fs'),assert=require('node:assert/strict');
process.chdir(require('node:path').resolve(__dirname,'../..'));
(async()=>{
 const base='http://192.168.56.103:65000';
 const password=fs.readFileSync('.cache/kali.env','utf8').split('\n').find(s=>s.startsWith('WEB_PASSWORD=')).split('=').slice(1).join('=').replace(/^['"]|['"]$/g,'');
 const browser=await chromium.launch({headless:true});
 const report={passed:false,at:new Date().toISOString(),readOnly:true,errors:[],writes:[]};
 try{
  const page=await browser.newPage({viewport:{width:1366,height:768},httpCredentials:{username:'packmate',password}});
  const snapshot=async()=> (await (await page.request.get(base+'/api/v2/snapshot')).json()).capture;
  report.captureBefore=await snapshot();
  const config=await page.request.get(base+'/api/v2/anomalies/config');
  assert.equal(config.status(),200);assert.equal(config.headers()['cache-control'],'no-store');
  report.config=await config.json();
  page.on('pageerror',e=>report.errors.push(e.message));
  await page.routeWebSocket('**/api/v2/events',()=>{});
  await page.route('**/api/**',r=>{
   if(r.request().method()!=='GET'){report.writes.push(r.request().url());return r.abort()}
   return r.continue();
  });
  const started=Date.now();
  await page.goto(base+'/#/anomalies');
  await page.getByRole('button',{name:'Pause',exact:true}).click();
  await page.locator('.anomaly-row').first().waitFor();
  report.loadLatencyMs=Date.now()-started;
  report.summary=await page.locator('.anomaly-results-heading').innerText();
  await page.locator('.anomaly-row').first().click();
  await page.getByRole('link',{name:'Open session',exact:true}).waitFor();
  report.sessionLink=await page.getByRole('link',{name:'Open session',exact:true}).getAttribute('href');
  await page.getByRole('button',{name:'Settings',exact:true}).click();
  await page.locator('#capture-interface').waitFor();
  await page.getByRole('tab',{name:'Anomalies',exact:true}).click();
  const input=page.getByLabel('Response timeout, seconds',{exact:true});
  assert.equal(Number(await input.inputValue()),report.config.timeoutSeconds);
  assert.equal(await input.getAttribute('min'),'5');assert.equal(await input.getAttribute('max'),'300');
  await input.fill('30');assert.equal(await input.inputValue(),'30');
  report.layouts=[];
  for(const width of [1366,375]){
   await page.setViewportSize({width,height:768});
   assert(await page.locator('#settingsModal .modal-content').evaluate(el=>el.scrollWidth<=el.clientWidth+1));
   assert(await input.isVisible());report.layouts.push({width,noOverflow:true});
  }
  await page.getByRole('button',{name:'Close',exact:true}).last().click();
  report.configAfter=await (await page.request.get(base+'/api/v2/anomalies/config')).json();
  assert.deepEqual(report.configAfter,report.config);
  report.captureAfter=await snapshot();
  assert.equal(report.captureAfter.startedAt,report.captureBefore.startedAt);assert(report.captureAfter.running);
  assert.deepEqual(report.errors,[]);assert.deepEqual(report.writes,[]);report.passed=true;
 }finally{
  await browser.close();fs.writeFileSync('reports/anomaly-timeout-live-check.json',JSON.stringify(report,null,2)+'\n');
 }
 console.log(JSON.stringify(report));
})().catch(e=>{console.error(e);process.exitCode=1});
