// Read-only verification of the English interface deployed on Kali.
const {chromium}=require('../../web/node_modules/playwright');
const fs=require('node:fs'),assert=require('node:assert/strict'),path=require('node:path');
process.chdir(path.resolve(__dirname,'../..'));
(async()=>{
 const env=Object.fromEntries(fs.readFileSync('.cache/kali.env','utf8').split('\n').filter(l=>l&&!l.startsWith('#')).map(l=>{const i=l.indexOf('=');return[l.slice(0,i),l.slice(i+1).replace(/^['"]|['"]$/g,'')]}));
 const base=process.env.GOMATE_URL||'http://192.168.56.103:65000';
 const report={passed:false,readOnly:true,checks:[],errors:[]};
 const browser=await chromium.launch({headless:true});
 try{
  const page=await browser.newPage({locale:'ru-RU',viewport:{width:1366,height:900},httpCredentials:{username:env.WEB_LOGIN||'packmate',password:env.WEB_PASSWORD}});
  page.setDefaultTimeout(15000);page.on('pageerror',e=>report.errors.push(e.message));
  // Reject accidental mutation; session list POST endpoints are read-only queries.
  await page.route('**/api/**',async route=>{
   const r=route.request(),p=new URL(r.url()).pathname;
   if(!['GET','HEAD'].includes(r.method())&&!(r.method()==='POST'&&/^\/api\/(stream|packet)\//.test(p)))throw Error('Unexpected mutation: '+r.method()+' '+p);
   await route.continue();
  });
  const get=async p=>{const r=await page.request.get(base+p);assert.equal(r.status(),200,p);return r.json()};
  const snapshot=await get('/api/v2/snapshot');report.captureBefore=snapshot.capture;
  for(const [route,heading] of [['all',null],['anomalies','Anomalies'],['containers','Containers'],['monitoring','Monitoring'],['farm','Farm'],['chat','general']]){
   await page.goto(base+'/#/'+route);await page.getByRole('navigation',{name:'Main navigation'}).waitFor();
   assert.equal(await page.locator('html').getAttribute('lang'),'en');assert.equal(await page.title(),'gomate — traffic analysis');
   for(const name of ['Traffic','Anomalies','Containers','Monitoring','Farm','Team chat'])assert.equal(await page.getByRole('navigation',{name:'Main navigation'}).getByRole('link',{name,exact:true}).count(),1);
   if(heading)await page.getByRole('heading',{name:heading,exact:true}).waitFor();
  }
  report.checks.push('six deployed routes and English navigation with Russian browser locale');
  await page.getByRole('button',{name:'Settings',exact:true}).click();const settings=page.locator('#settingsModal');await settings.locator('#capture-interface').waitFor();
  for(const name of ['Traffic capture','Retention and cleanup','Application','Notifications','Display','Farm'])await settings.getByRole('tab',{name,exact:true}).click();
  await page.keyboard.press('Escape');await settings.waitFor({state:'hidden'});report.checks.push('all English settings tabs');
  const chat=await get('/api/v2/chat');assert.match(chat.author,/^Anonymous-[0-9a-f]{8}$/);
  for(const m of chat.messages)assert(!m.author.startsWith('Аноним-'),'legacy anonymous author was not translated');
  report.checks.push('English anonymous display names including stored chat history');
  const invalid=await page.request.get(base+'/api/v2/chat?after=-1');assert.equal(invalid.status(),400);assert.equal((await invalid.json()).error,'Invalid chat cursor');
  report.checks.push('English server validation errors');
  const after=await get('/api/v2/snapshot');report.captureAfter=after.capture;
  assert.equal(after.capture?.running,snapshot.capture?.running);assert.equal(after.capture?.interface,snapshot.capture?.interface);
  assert.deepEqual(report.errors,[]);report.passed=true;
 }finally{await browser.close();fs.writeFileSync('reports/english-live-check.json',JSON.stringify(report,null,2)+'\n')}
 console.log(JSON.stringify(report));
})().catch(e=>{console.error(e);process.exitCode=1});
