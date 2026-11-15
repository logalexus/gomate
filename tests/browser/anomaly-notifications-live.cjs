// Read-only live check. Changes only this isolated browser's notification preference.
const {chromium}=require('../../web/node_modules/playwright');
const fs=require('node:fs'),assert=require('node:assert/strict'),path=require('node:path');
process.chdir(path.resolve(__dirname,'../..'));
(async()=>{
 const base=process.env.GOMATE_URL||'http://192.168.56.103:65000';
 const password=process.env.GOMATE_PASSWORD||fs.readFileSync('.cache/kali.env','utf8').split('\n').find(x=>x.startsWith('WEB_PASSWORD=')).slice(13).replace(/^['"]|['"]$/g,'');
 const browser=await chromium.launch(),report={passed:false,readOnly:true,errors:[],writes:[],events:[]};
 try{
  const page=await browser.newPage({httpCredentials:{username:process.env.GOMATE_USER||'packmate',password},viewport:{width:1366,height:900}});
  page.on('pageerror',e=>report.errors.push(e.message));
  await page.route('**/api/**',r=>{if(r.request().method()!=='GET'){report.writes.push(r.request().method()+' '+new URL(r.request().url()).pathname);return r.abort()}return r.continue()});
  page.on('response',async r=>{if(r.url().includes('/api/v2/anomalies/notifications')&&r.ok())try{const d=await r.json();report.events.push(...d.items.slice(0,5));report.events=report.events.slice(-20)}catch{}});
  await page.goto(base+'/#/anomalies');
  await page.getByRole('button',{name:'Settings',exact:true}).click();
  await page.getByRole('tab',{name:'Notifications',exact:true}).click();
  const score=page.getByLabel('Minimum score',{exact:true});assert.equal(await score.inputValue(),'40');report.defaultThreshold=40;
  // Existing traffic may no longer reach 40 once the model has learned it.
  await score.fill('6');report.observationThreshold=6;
  await page.screenshot({path:'reports/anomaly-notifications-settings-live.jpg',type:'jpeg',quality:75});
  await page.locator('#settingsModal .close').click();await page.locator('#settingsModal').waitFor({state:'hidden'});
  const box=page.getByTestId('anomaly-notification');await box.waitFor({timeout:180000});await box.hover();
  report.notification=await box.innerText();assert.match(report.notification,/Session #/);
  report.sessionLink=await box.locator('li a').first().getAttribute('href');assert.match(report.sessionLink,/#\/\d+\/\d+$/);
  await page.screenshot({path:'reports/anomaly-notifications-live.jpg',type:'jpeg',quality:75});
  const snapshot=await page.request.get(base+'/api/v2/snapshot');assert(snapshot.ok());report.captureRunning=(await snapshot.json()).captureRunning;assert(report.captureRunning);
  assert.deepEqual(report.errors,[]);assert.deepEqual(report.writes,[]);report.passed=true;
 }finally{await browser.close();fs.writeFileSync('reports/anomaly-notifications-live.json',JSON.stringify(report,null,2)+'\n')}
 console.log(JSON.stringify(report));
})().catch(e=>{console.error(e);process.exitCode=1});
