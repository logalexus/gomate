// Read-only verification of the Kali deployment. Requires .cache/kali.env.
const {chromium}=require('../../web/node_modules/playwright');
const fs=require('node:fs'),assert=require('node:assert/strict');
const {execFileSync}=require('node:child_process');
process.chdir(require('node:path').resolve(__dirname,'../..'));
(async()=>{
 const base=process.env.GOMATE_URL||'http://192.168.56.103:65000';
 const password=fs.readFileSync('.cache/kali.env','utf8').split('\n').find(s=>s.startsWith('WEB_PASSWORD=')).slice('WEB_PASSWORD='.length).replace(/^['"]|['"]$/g,'');
 const browser=await chromium.launch({headless:true});
 const report={passed:false,at:new Date().toISOString(),readOnly:true,errors:[],checks:[]};
 try{
  const page=await browser.newPage({viewport:{width:1280,height:768},httpCredentials:{username:'packmate',password}});
  page.setDefaultTimeout(15000);page.on('pageerror',e=>report.errors.push(e.message));
  const get=async path=>{const r=await page.request.get(base+path);assert.equal(r.status(),200,path);return r.json()};
  report.captureBefore=(await get('/api/v2/snapshot')).capture;
  assert.equal((await fetch(base+'/api/v2/monitor')).status,401);
  // Rates need two samples after deploying/restarting the gateway.
  let first;
  for(let i=0;i<20;i++){
   first=await get('/api/v2/monitor?minutes=60');
   if(first.hostHistory?.length>=2&&first.containers?.some(c=>c.name==='gomate-lab-app-1'&&c.point.cpu!=null))break;
   await page.waitForTimeout(1000);
  }
  assert(!first.error&&!first.host.error&&!first.host.diskError&&!first.host.networkError,JSON.stringify(first.host));
  assert(first.host.cpu>=0&&first.host.cpu<=100);assert(first.host.memoryUsed>0&&first.host.memoryLimit>first.host.memoryUsed);assert(first.host.diskUsed>0&&first.host.diskTotal>first.host.diskUsed);
  assert(first.containers.length>=4&&!first.truncated);assert(first.containers.filter(c=>c.state==='running').every(c=>!c.error&&c.point.memoryUsed!=null));
  const app=first.containers.find(c=>c.name==='gomate-lab-app-1');assert(app&&app.point.cpu!=null);
  const hostReference=JSON.parse(execFileSync('ssh',['-o','BatchMode=yes','kali@192.168.56.103',"python3 -c 'import json,os; m={x.split()[0].rstrip(\":\"):int(x.split()[1])*1024 for x in open(\"/proc/meminfo\") if x.startswith(\"MemTotal:\")}; s=os.statvfs(\"/\"); print(json.dumps(dict(memoryTotal=m[\"MemTotal\"],diskTotal=s.f_blocks*s.f_frsize,cpus=os.cpu_count())))'"],{encoding:'utf8'}));
  assert.equal(first.host.memoryLimit,hostReference.memoryTotal);assert.equal(first.host.diskTotal,hostReference.diskTotal);assert.equal(first.host.cpus,hostReference.cpus);report.hostReference=hostReference;
  await page.goto(base+'/#/monitoring');await page.locator('.monitor-table tbody tr').first().waitFor();
  await page.getByRole('button',{name:'Пауза',exact:true}).click();
  await page.getByLabel('Поиск контейнеров',{exact:true}).fill(app.name);
  await page.getByRole('button',{name:app.name,exact:true}).click();await page.locator('.monitor-detail svg').first().waitFor();
  assert.equal(await page.locator('.monitor-detail .resource-chart').count(),4);
  assert.equal(await page.locator('.monitor-kpis [role="meter"]').count(),3);
  for(const meter of await page.locator('.monitor-kpis [role="meter"], .monitor-table [role="meter"]').all()){
   const percent=Number(await meter.getAttribute('aria-valuenow'));
   const level=percent>=90?'danger':percent>=70?'warning':'normal';
   assert.match(await meter.locator('..').getAttribute('class'),new RegExp('gm-load-'+level));
  }
  assert.equal(await page.locator('.resource-chart.is-load').count(),5);
  const gradientIds=await page.locator('linearGradient').evaluateAll(elements=>elements.map(e=>e.id));
  assert.equal(gradientIds.length,5);assert.equal(new Set(gradientIds).size,5);
  await page.locator('.monitor-detail').scrollIntoViewIfNeeded();
  await page.screenshot({path:'reports/monitoring-live.jpg',type:'jpeg',quality:35});
  const history=await get('/api/v2/monitor?minutes=60&container='+app.id);
  assert.equal(history.selectedId,app.id);assert(history.containerHistory.length>=2&&history.hostHistory.length>=2);
  await page.waitForTimeout(5500);
  const next=await get('/api/v2/monitor?minutes=5&container='+app.id);assert(next.sampledAt>first.sampledAt);assert(next.containerHistory.at(-1).time>history.containerHistory.at(-1).time);
  const latencies=[];
  await Promise.all(Array.from({length:4},async()=>{for(let i=0;i<5;i++){const started=Date.now();await get('/api/v2/monitor?minutes=60&container='+app.id);latencies.push(Date.now()-started)}}));
  report.snapshotReads={count:latencies.length,concurrency:4,minMs:Math.min(...latencies),maxMs:Math.max(...latencies),meanMs:Math.round(latencies.reduce((a,b)=>a+b,0)/latencies.length)};
  report.host=next.host;report.containerCount=next.containers.length;report.containers=next.containers.map(c=>({name:c.name,state:c.state,error:c.error||'',cpu:c.point.cpu,memoryUsed:c.point.memoryUsed}));
  report.historyPoints={host:history.hostHistory.length,container:history.containerHistory.length};
  report.captureAfter=(await get('/api/v2/snapshot')).capture;
  assert(report.captureAfter.running);assert.equal(report.captureAfter.startedAt,report.captureBefore.startedAt);assert.deepEqual(report.errors,[]);
  report.checks=['authentication','host metrics match actual host RAM, CPU count and root filesystem','running containers have metrics','real charts and container selection','load colors and meter levels match the displayed usage','host and container charts have unique load gradients','history advances while UI paused','20 shared snapshot reads with 4 clients','capture remains running'];report.passed=true;
 }finally{await browser.close();fs.writeFileSync('reports/monitoring-live-check.json',JSON.stringify(report,null,2)+'\n')}
 console.log(JSON.stringify(report));
})().catch(e=>{console.error(e);process.exitCode=1});
