// Mocked API checks interaction and cancellation; SQL/content matching has integration coverage.
const {chromium}=require('../../web/node_modules/playwright');
const assert=require('node:assert/strict'),fs=require('node:fs'),{spawn}=require('node:child_process');
const delay=ms=>new Promise(r=>setTimeout(r,ms));
(async()=>{
 const base='http://127.0.0.1:4189',report={passed:false,checks:[],layouts:[],pageErrors:[]};
 const server=spawn(process.execPath,['node_modules/vite/bin/vite.js','--host','127.0.0.1','--port','4189','--strictPort'],{cwd:'web',stdio:'ignore'});
 let browser;
 try{
  for(let i=0;i<60;i++){try{if((await fetch(base)).ok)break}catch{}await delay(150)}
  browser=await chromium.launch({headless:true});const page=await browser.newPage({viewport:{width:1366,height:900}});page.setDefaultTimeout(12000);page.on('pageerror',e=>report.pageErrors.push(e.message));
  const rows=Array.from({length:25},(_,i)=>({id:100-i,service:8080,protocol:'HTTP',startTimestamp:Date.now(),sizeBytes:100,packetsCount:2,foundPatternsIds:[1,2],firstRequestPath:'/api/login',favorite:true}));
  let requests=[],socket,slowCalls=0;
  await page.routeWebSocket('**/api/v2/events',ws=>{socket=ws});
  await page.route('**/api/**',async route=>{
   const req=route.request(),p=new URL(req.url()).pathname;
   if(p==='/api/v2/config')return route.fulfill({json:{timeoutSeconds:10}});
   if(p==='/api/v2/snapshot')return route.fulfill({json:{services:[{port:8080,name:'Shop'}],patterns:[{id:1,name:'SQL',actionType:'FIND'},{id:2,name:'Flag',actionType:'FIND'}],metrics:{},captureRunning:true,capture:{interface:'eth1'}}});
   if(p==='/api/v2/traffic/search'){
    const body=req.postDataJSON();requests.push(body);
    if(body.query==='[')return route.fulfill({status:400,json:{error:'invalid regular expression: missing closing ]'}});
    if(body.query==='slow'){slowCalls++;await delay(150);return route.fulfill({json:{streams:[],next:{before:10000-slowCalls},scanned:128}})}
    if(body.query==='pages')return route.fulfill({json:{streams:body.cursor?[{...rows[0],id:1}]:rows,next:body.cursor?null:{before:76},scanned:25}});
    return route.fulfill({json:{streams:body.cursor?rows.slice(0,2):[],next:body.cursor?null:{before:200},scanned:body.cursor?2:128}});
   }
   if(p.startsWith('/api/stream/'))return route.fulfill({json:req.method()==='GET'?rows[0]:rows});
   if(p.startsWith('/api/packet/'))return route.fulfill({json:[]});
   return route.fulfill({json:[]});
  });
  await page.goto(base+'/#/all');await page.locator('#stream-100').waitFor();
  const input=page.getByRole('searchbox',{name:'Search saved traffic'}),submit=page.getByRole('button',{name:'Search traffic',exact:true}),regex=page.getByRole('button',{name:'Use regular expression'});
  await input.fill('login.php?x=1');await delay(250);assert.equal(requests.length,0,'typing must not trigger history scan');await input.press('Enter');
  await page.waitForFunction(()=>document.querySelector('.search-progress')?.textContent.includes('Search complete'));assert.equal(await page.locator('.stream-item').count(),2);assert.equal(requests[0].mode,'text');assert.equal(requests[0].query,'login.php?x=1');assert.equal(requests[1].cursor.before,200);
  report.checks.push('explicit text search follows empty intermediate batches to actual results');
  const count=requests.length;socket.send('refresh');await delay(250);assert.equal(requests.length,count,'LIVE must not repeat historical search');
  await page.getByRole('button',{name:'SQL',exact:true}).click();await page.getByRole('button',{name:'Flag',exact:true}).click();await page.getByRole('button',{name:'Favorites',exact:true}).click();await page.getByLabel('Service',{exact:true}).selectOption('8080');
  await page.waitForFunction(()=>document.querySelector('.search-progress')?.textContent.includes('Search complete'));
  assert.deepEqual(requests.at(-1).patternIds,[1,2]);assert.equal(requests.at(-1).favorites,true);assert.equal(requests.at(-1).port,8080);
  await page.locator('#stream-100 .stream-link').click();assert.match(page.url(),/q=login/);await page.reload();await page.waitForFunction(()=>document.querySelector('.search-progress')?.textContent.includes('Search complete'));assert.equal(await input.inputValue(),'login.php?x=1');
  report.checks.push('service, all labels, Favorites and URL reload/session navigation preserve search; LIVE does not restart it');
  await regex.click();await input.fill('token=[a-z]+');await submit.click();await page.waitForFunction(()=>location.hash.includes('searchMode=regex'));await page.waitForFunction(()=>document.querySelector('.search-progress')?.textContent.includes('Search complete'));assert.equal(requests.at(-1).mode,'regex');
  await input.fill('[');await input.press('Enter');await page.locator('#traffic-search-error').waitFor();assert.equal(await input.getAttribute('aria-invalid'),'true');
  await input.fill('pages');await submit.click();await page.getByRole('button',{name:'Older',exact:true}).waitFor();await page.waitForFunction(()=>document.querySelector('.session-total')?.textContent.trim()==='25');await page.getByRole('button',{name:'Older',exact:true}).click();await page.waitForFunction(()=>document.querySelector('#stream-1'));assert.equal(await page.locator('.stream-item').count(),1);await page.getByRole('button',{name:'Newer sessions'}).click();await page.waitForFunction(()=>document.querySelector('.session-total')?.textContent.trim()==='25');
  report.checks.push('regex mode and inline invalid-expression feedback; Older/Newer pages use search checkpoints');
  await input.fill('slow');await submit.click();await page.getByRole('button',{name:'Stop search'}).waitFor();await delay(220);await page.getByRole('button',{name:'Stop search'}).click();await page.getByRole('button',{name:'Continue',exact:true}).waitFor();const stopped=slowCalls;await delay(300);assert.equal(slowCalls,stopped);await page.getByRole('button',{name:'Continue',exact:true}).click();await page.waitForFunction(()=>document.querySelector('.search-progress')?.textContent.includes('Searching'));
  await input.fill('login');await input.press('Enter');await page.waitForFunction(()=>document.querySelector('.search-progress')?.textContent.includes('Search complete'));await delay(200);assert.equal(await page.locator('.stream-item').count(),2);
  report.checks.push('stop/continue and changing query cancel stale work without replacing current results');
  for(const width of [1366,667,375])for(const theme of ['dark','light']){
   await page.setViewportSize({width,height:width===667?375:900});await page.evaluate(t=>document.body.dataset.theme=t,theme);await delay(80);
   const inputBox=await input.boundingBox(),serviceBox=await page.locator('#traffic-service').boundingBox(),filters=await page.locator('.gm-traffic-filters').boundingBox();
   assert(inputBox.y+inputBox.height<=serviceBox.y,'search is above filters');assert(inputBox.width>80);assert(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth+1),'horizontal overflow');assert(filters.height<160,'filters too large');report.layouts.push({width,theme,filtersHeight:filters.height,inputWidth:inputBox.width});
  }
  await input.press('Escape');await page.waitForFunction(()=>!location.hash.includes('q='));assert.equal(await input.inputValue(),'');assert.equal(await regex.getAttribute('aria-pressed'),'false');
  await input.fill('login');await input.press('Enter');await page.waitForFunction(()=>document.querySelector('.search-progress')?.textContent.includes('Search complete'));await page.getByRole('button',{name:'Reset all filters'}).click();await page.waitForFunction(()=>!location.hash.includes('q=')&&!location.hash.includes('patterns='));assert.equal(await input.inputValue(),'');
  report.checks.push('compact search above filters at desktop/mobile sizes in both themes; Escape and Reset clear query and mode');
  assert.deepEqual(report.pageErrors,[]);report.passed=true;
 }finally{await browser?.close();server.kill();fs.writeFileSync('reports/traffic-search-ui-check.json',JSON.stringify(report,null,2)+'\n')}
 console.log(JSON.stringify(report));
})().catch(e=>{console.error(e);process.exitCode=1});
