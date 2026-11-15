// Read-only verification against captured traffic on the authorized Kali host.
const {chromium}=require('../../web/node_modules/playwright');
const fs=require('node:fs'),assert=require('node:assert/strict');
(async()=>{
 const env=Object.fromEntries(fs.readFileSync('.cache/kali.env','utf8').split('\n').filter(l=>l&&!l.startsWith('#')).map(l=>{const i=l.indexOf('=');return[l.slice(0,i),l.slice(i+1).replace(/^['"]|['"]$/g,'')]}));
 const base=process.env.GOMATE_URL||'http://192.168.56.103:65000',report={passed:false,readOnly:true,checks:[],errors:[]};
 const browser=await chromium.launch({headless:true});
 try{
  const page=await browser.newPage({viewport:{width:1366,height:900},httpCredentials:{username:env.WEB_LOGIN||'packmate',password:env.WEB_PASSWORD}});page.setDefaultTimeout(30000);page.on('pageerror',e=>report.errors.push(e.message));
  await page.route('**/api/**',async route=>{const r=route.request(),p=new URL(r.url()).pathname;if(!['GET','HEAD'].includes(r.method())&&!(r.method()==='POST'&&(/^\/api\/(stream|packet)\//.test(p)||p==='/api/v2/traffic/search')))throw Error('Unexpected mutation');await route.continue()});
  const snapshot=await page.request.get(base+'/api/v2/snapshot');assert.equal(snapshot.status(),200);report.captureBefore=(await snapshot.json()).capture;
  const listing=await page.request.post(base+'/api/stream/all',{data:{pageSize:25}});assert.equal(listing.status(),200);const rows=await listing.json();assert(rows.length);const target=rows.find(r=>r.firstRequestPath)||rows[0];
  // Do not record traffic contents or queries, which can contain user data.
  const query=target.firstRequestPath?.split('?')[0]||target.server;assert(query);
  await page.goto(base+'/#/all');await page.locator('#traffic-search').waitFor();await page.getByLabel('Service',{exact:true}).selectOption(String(target.service));
  const input=page.getByRole('searchbox',{name:'Search saved traffic'});await input.fill(query);const started=Date.now();await input.press('Enter');
  await page.waitForFunction(()=>/Search complete|More history available/.test(document.querySelector('.search-progress')?.textContent||''));
  const total=Number(await page.locator('.session-total').textContent());assert(total>0);report.firstPageMs=Date.now()-started;report.resultCount=total;report.service=target.service;
  await page.locator('.stream-item .stream-link').first().click();await page.locator('.gm-traffic-detail').waitFor();assert.match(page.url(),/q=/);assert.equal(await input.inputValue(),query);report.checks.push('text search returns real saved traffic for the selected service; query persists when opening a session');
  await page.getByRole('button',{name:'Use regular expression'}).click();await input.fill(query.replace(/[.*+?^${}()|[\]\\]/g,'\\$&'));await input.press('Enter');await page.waitForFunction(()=>/Search complete|More history available/.test(document.querySelector('.search-progress')?.textContent||''));assert(Number(await page.locator('.session-total').textContent())>0);report.checks.push('RE2 regex finds real saved sessions');
  await input.fill('[');await input.press('Enter');await page.locator('#traffic-search-error').waitFor();assert.equal(await input.getAttribute('aria-invalid'),'true');report.checks.push('invalid regex rejected with inline feedback');
  await input.press('Escape');await page.waitForFunction(()=>!location.hash.includes('q='));await page.locator('.session-mode.is-live').waitFor();report.checks.push('clearing search returns to LIVE');
  const after=await page.request.get(base+'/api/v2/snapshot');assert.equal(after.status(),200);report.captureAfter=(await after.json()).capture;assert.equal(report.captureAfter.running,true);assert.equal(report.captureAfter.interface,report.captureBefore.interface);assert.deepEqual(report.errors,[]);report.passed=true;
 }finally{await browser.close();fs.writeFileSync('reports/traffic-search-live-check.json',JSON.stringify(report,null,2)+'\n')}
 console.log(JSON.stringify(report));
})().catch(e=>{console.error(e);process.exitCode=1});
