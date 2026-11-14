// Read-only verification against the existing lab instance.
const {chromium}=require('../../web/node_modules/playwright');
const fs=require('node:fs'),assert=require('node:assert/strict');
process.chdir(require('node:path').resolve(__dirname,'../..'));
(async()=>{
 const base='http://192.168.56.103:65000';
 const password=fs.readFileSync('.cache/kali.env','utf8').split('\n').find(s=>s.startsWith('WEB_PASSWORD=')).split('=').slice(1).join('=').replace(/^['"]|['"]$/g,'');
 const browser=await chromium.launch({headless:true});
 const report={passed:false,at:new Date().toISOString(),readOnly:true,errors:[],writes:[],layouts:[]};
 try {
  const page=await browser.newPage({viewport:{width:1366,height:768},httpCredentials:{username:'packmate',password}});
  const snapshot=await (await page.request.get(base+'/api/v2/snapshot')).json();
  const patterns=new Map(snapshot.patterns.map(p=>[p.id,p]));
  const names=ids=>ids.map(id=>{const p=patterns.get(id);return (p?.name||`Шаблон #${id}`)+(p?.deleted?' (удалён)':'')});
  report.captureBefore=snapshot.capture;
  page.on('pageerror',e=>report.errors.push(e.message));
  await page.routeWebSocket('**/api/v2/events',()=>{});
  await page.route('**/api/**',r=>{if(r.request().method()!=='GET'){report.writes.push(r.request().url());return r.abort()}return r.continue()});
  const response=page.waitForResponse(r=>new URL(r.url()).pathname==='/api/v2/anomalies/groups');
  const started=Date.now();await page.goto(base+'/#/anomalies');
  await page.getByRole('button',{name:'Пауза',exact:true}).click();
  const groups=await (await response).json();await page.locator('.anomaly-row').first().waitFor();
  report.loadLatencyMs=Date.now()-started;report.summary=await page.locator('.anomaly-results-heading').innerText();
  const index=groups.groups.findIndex(g=>g.foundPatternsIds.length>0);assert(index>=0,'no live pattern matches');
  const group=groups.groups[index];
  assert.deepEqual(await page.locator('.anomaly-row').nth(index).locator('.pattern-label').allTextContents(),names(group.foundPatternsIds.slice(0,3)));
  assert.equal(await page.locator('.theft-label').count(),0);
  const membersResponse=page.waitForResponse(r=>new URL(r.url()).pathname==='/api/v2/anomalies/sessions');
  await page.locator('.anomaly-row').nth(index).click();
  const members=await (await membersResponse).json();
  await page.getByRole('link',{name:'Открыть сессию',exact:true}).waitFor();
  const member=members.items.find(m=>m.foundPatternsIds.length>0);assert(member,'no matching session in first page');
  if(members.items.length>1) await page.locator('.anomaly-session-picker select').selectOption(String(member.streamId));
  assert.deepEqual(await page.locator('.session-patterns .pattern-label').allTextContents(),names(member.foundPatternsIds));
  assert.match(await page.locator('.session-patterns').innerText(),/на баллы не влияют/);
  report.group={count:group.count,patternNames:names(group.foundPatternsIds),score:group.score};
  report.session={id:member.streamId,patternNames:names(member.foundPatternsIds),score:member.score,hasAnomaly:member.hasAnomaly};
  for(const width of [1366,375]){
   await page.setViewportSize({width,height:768});
   const layout=await page.evaluate(()=>({documentWidth:document.documentElement.scrollWidth,mainWidth:document.querySelector('main').scrollWidth}));
   assert(layout.documentWidth<=width+1&&layout.mainWidth<=width+1);report.layouts.push({width,...layout});
  }
  report.captureAfter=(await (await page.request.get(base+'/api/v2/snapshot')).json()).capture;
  assert.equal(report.captureAfter.startedAt,report.captureBefore.startedAt);assert(report.captureAfter.running);
  assert.deepEqual(report.errors,[]);assert.deepEqual(report.writes,[]);report.passed=true;
 } finally {await browser.close();fs.writeFileSync('reports/pattern-labels-live-check.json',JSON.stringify(report,null,2)+'\n')}
 console.log(JSON.stringify(report));
})().catch(e=>{console.error(e);process.exitCode=1});
