// RPS display, idle/error recovery and compact footer at desktop/mobile sizes.
const {chromium}=require('../../web/node_modules/playwright');
const assert=require('node:assert/strict'),fs=require('node:fs'),{spawn}=require('node:child_process'),path=require('node:path');
process.chdir(path.resolve(__dirname,'../..'));
const delay=ms=>new Promise(r=>setTimeout(r,ms));
(async()=>{
 const base='http://127.0.0.1:4188',report={passed:false,checks:[],layouts:[],pageErrors:[]};
 const server=spawn(process.execPath,['node_modules/vite/bin/vite.js','--host','127.0.0.1','--port','4188','--strictPort'],{cwd:'web',stdio:'ignore'});
 let browser;
 try{
  for(let i=0;i<60;i++){try{if((await fetch(base)).ok)break}catch{}await delay(150)}
  browser=await chromium.launch({headless:true});
  const page=await browser.newPage({viewport:{width:1366,height:900},locale:'ru-RU'});
  page.setDefaultTimeout(12000);page.on('pageerror',e=>report.pageErrors.push(e.message));
  let rate=1234.56,fail=false,socket;
  await page.routeWebSocket('**/api/v2/events',ws=>{socket=ws});
  await page.route('**/api/**',async route=>{
   const p=new URL(route.request().url()).pathname;
   if(p==='/api/v2/snapshot'){
    if(fail)return route.fulfill({status:503,json:{error:'Unavailable'}});
    return route.fulfill({json:{requestsPerSecond:rate,services:[],patterns:[],captureRunning:true,capture:{interface:'eth1',running:true},metrics:{captured_total:12345,persisted_chunks_total:100,capture_drops_total:0,queue_drops_total:0,flow_drops_total:0,write_drops_total:0}}});
   }
   return route.fulfill({json:p==='/api/v2/config'?{timeoutSeconds:10}:[]});
  });
  await page.goto(base+'/#/all');
  const rps=page.locator('.gm-status-rps');await rps.waitFor();
  await page.waitForFunction(()=>document.querySelector('.gm-status-rps strong')?.textContent==='1,234.6');
  assert.match(await rps.getAttribute('title'),/5-second average/);
  for(const width of [1366,667,375])for(const theme of ['dark','light']){
   await page.setViewportSize({width,height:900});await page.evaluate(t=>document.body.setAttribute('data-theme',t),theme);await delay(80);
   assert(await rps.isVisible());
   const bounds=await rps.boundingBox(),footer=await page.locator('.gm-statusbar').boundingBox();
   assert(bounds.x>=0&&bounds.x+bounds.width<=width+1,'RPS clipped');
   assert(footer.height<=(width>800?40:60),'Footer grew too tall');
   assert(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth+1),'Page overflow');
   report.layouts.push({width,theme,footerHeight:footer.height});
  }
  report.checks.push('localized one-decimal RPS with definition; visible compact footer in both themes at 1366, 667 and 375 px');
  rate=0;socket.send('refresh');await page.waitForFunction(()=>document.querySelector('.gm-status-rps strong')?.textContent==='0.0');
  report.checks.push('idle sample decays to zero');
  fail=true;socket.send('refresh');await page.waitForFunction(()=>document.querySelector('.gm-status-rps strong')?.textContent==='—');
  await page.getByText('Server disconnected',{exact:true}).waitFor();
  report.checks.push('request failure clears stale RPS');
  fail=false;rate=7.2;socket.send('refresh');await page.waitForFunction(()=>document.querySelector('.gm-status-rps strong')?.textContent==='7.2');
  rate=undefined;socket.send('refresh');await page.waitForFunction(()=>document.querySelector('.gm-status-rps strong')?.textContent==='—');
  report.checks.push('recovery and older-server missing metric fallback');
  assert.deepEqual(report.pageErrors,[]);report.passed=true;
 }finally{await browser?.close();server.kill();fs.writeFileSync('reports/status-rps-ui-check.json',JSON.stringify(report,null,2)+'\n')}
 console.log(JSON.stringify(report));
})().catch(e=>{console.error(e);process.exitCode=1});
