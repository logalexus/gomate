// UI fixtures only; real container execution is tested separately.
const {chromium}=require('../../web/node_modules/playwright');
const fs=require('node:fs'),assert=require('node:assert/strict'),path=require('node:path'),{spawn}=require('node:child_process');
process.chdir(path.resolve(__dirname,'../..'));
(async()=>{
 const base='http://127.0.0.1:4177',server=spawn(process.execPath,['node_modules/vite/bin/vite.js','--host','127.0.0.1','--port','4177','--strictPort'],{cwd:'web',stdio:'ignore'});let browser;
 const report={passed:false,mocked:true},delay=ms=>new Promise(r=>setTimeout(r,ms));
 try{
  for(let i=0;i<60;i++){try{if((await fetch(base)).ok)break}catch{}await delay(150)}
  browser=await chromium.launch({headless:true,executablePath:process.env.CHROMIUM_PATH||'/home/logalex/.cache/ms-playwright/chromium-1187/chrome-linux/chrome'});
  const page=await browser.newPage({viewport:{width:1100,height:760},reducedMotion:'reduce'}),errors=[],submitted=[],cancelled=[];let available=true,job=null,polls=0;
  page.on('pageerror',e=>errors.push(e.message));await page.routeWebSocket('**/api/v2/events',()=>{});
  const stream={id:7,service:18080,server:'192.0.2.1:18080',client:'192.0.2.2:50000',protocol:'TCP',startTimestamp:Date.now(),sizeBytes:100,packetsCount:1};
  const packet={id:11,incoming:true,timestamp:Date.now(),matches:[],content:Buffer.from('POST /api/login?x=1 HTTP/1.1\r\nHost: shop\r\nCookie: token=demo\r\nContent-Type: application/json\r\nContent-Length: 7\r\n\r\n{"a":1}').toString('base64')};
  await page.route('**/api/**',async route=>{
   const req=route.request(),url=new URL(req.url());let data=[];
   if(url.pathname==='/api/v2/snapshot')data={services:[{port:18080,name:'Shop',http:true}],patterns:[],metrics:{captured_total:1,persisted_chunks_total:1},captureRunning:true,capture:{interface:'eth1'}};
   else if(url.pathname==='/api/stream/7'&&req.method()==='GET')data=stream;
   else if(url.pathname.startsWith('/api/stream/'))data=[stream];
   else if(url.pathname.startsWith('/api/packet/'))data=[packet];
   else if(url.pathname==='/api/v2/runner')data={available,maxCodeBytes:262144,maxTimeoutSeconds:120};
   else if(url.pathname==='/api/v2/runner/jobs'&&req.method()==='POST'){const body=req.postDataJSON();submitted.push(body);polls=0;job={id:String(submitted.length).padStart(32,'a'),status:'running',stdout:'',stderr:'',elapsedMs:0,exitCode:null};data=job}
   else if(url.pathname.startsWith('/api/v2/runner/jobs/')){
    if(req.method()==='DELETE'){cancelled.push(job.id);job={...job,status:'cancelled',exitCode:137};data=job}
    else{if(!submitted.at(-1).code.includes('sleep')&&++polls>=2)job={...job,status:'completed',stdout:'<img src=x onerror=alert(1)>\nedited script output',stderr:'debug message',elapsedMs:150,exitCode:0};data=job}
   }
   await route.fulfill({json:data});
  });
  await page.goto(base+'/#/all/7');await page.locator('.gm-packet').waitFor();
  async function open(format){await page.locator('.gm-packet').getByRole('button',{name:'Копировать',exact:true}).click();await page.getByRole('menuitem',{name:format,exact:true}).click();await page.getByLabel('Редактируемый скрипт').waitFor();await page.waitForFunction(()=>document.querySelector('.script-editor')?.__vue__?.value.length>0)}
  await open('curl…');const dialog=page.getByRole('dialog'),editor=page.getByLabel('Редактируемый скрипт');const fillEditor=async text=>{await editor.fill(text);await page.waitForFunction(expected=>document.querySelector('.script-editor')?.__vue__?.value===expected,text)};assert((await editor.evaluate(el=>el.closest('.script-editor').__vue__.value)).includes('curl'));assert.equal(await page.getByLabel('Адрес сервиса',{exact:true}).inputValue(),'192.0.2.1');
  // Actual syntax spans, responsive split geometry, independent scrolling and both themes.
  for(const [width,height,theme] of [[1100,760,'dark'],[1440,900,'light'],[800,600,'dark']]){
   await page.setViewportSize({width,height});await page.evaluate(theme=>document.body.dataset.theme=theme,theme);
   await page.waitForFunction(()=>document.querySelectorAll('.cm-line span[class]').length>2);
   const [left,right]=await dialog.evaluate(el=>['.workbench-editor-pane','.workbench-result'].map(selector=>el.querySelector(selector).getBoundingClientRect().toJSON()));
   assert(left.width>right.width);assert(right.x>=left.x+left.width);assert(Math.abs(left.y-right.y)<2);assert(left.height>=300);assert(left.width+right.width>width*.85);
   assert(await dialog.evaluate(el=>el.scrollWidth<=el.clientWidth+1));
  }
  await page.setViewportSize({width:1100,height:760});
  await fillEditor('echo "highlight" # shell comment');await editor.press('End');await editor.press('Enter');await editor.pressSequentially('echo undo');await editor.press('Control+z');
  assert(!(await editor.evaluate(el=>el.closest('.script-editor').__vue__.value)).includes('echo undo'));
  await editor.press('Tab');assert(!(await editor.evaluate(el=>el===document.activeElement)),'Tab must escape editor');
  await editor.focus();await editor.press('Control+a');
  await editor.evaluate((el,text)=>{const data=new DataTransfer();data.setData('text/plain',text);el.dispatchEvent(new ClipboardEvent('paste',{bubbles:true,cancelable:true,clipboardData:data}))},Array.from({length:4000},(_,i)=>`# line ${i}`).join('\n'));
  await page.waitForFunction(()=>{const el=document.querySelector('.cm-scroller');return el.scrollHeight>el.clientHeight*10});
  assert(await editor.locator('.cm-line').count()<300,'editor must virtualize long scripts');
  const runBounds=await dialog.getByRole('button',{name:'Запустить',exact:true}).boundingBox();assert(runBounds.y+runBounds.height<=760);
  const edited="printf '%s\\n' 'edited by user'";await fillEditor(edited);await page.getByLabel('Адрес сервиса',{exact:true}).fill('192.0.2.9');assert.equal(await editor.evaluate(el=>el.closest('.script-editor').__vue__.value),edited);await dialog.getByText('Скрипт изменён вручную.',{exact:false}).waitFor();
  await page.evaluate(()=>{Object.defineProperty(navigator,'clipboard',{value:{writeText:async text=>{window.copiedScript=text}},configurable:true});document.execCommand=command=>{if(command==='copy')window.copiedScript=document.activeElement.value;return true}});await dialog.getByRole('button',{name:'Копировать скрипт',exact:true}).click();assert.equal(await page.evaluate(()=>window.copiedScript),edited);
  await dialog.getByRole('button',{name:'Запустить',exact:true}).click();await dialog.getByText('edited script output',{exact:false}).waitFor();assert.deepEqual(submitted[0],{language:'bash',code:edited,timeoutSeconds:30});assert.equal(await dialog.locator('.workbench-result img').count(),0);assert.equal(await dialog.locator('#runner-stderr').innerText(),'debug message');
  await dialog.getByRole('button',{name:'Сгенерировать заново',exact:true}).click();assert((await editor.evaluate(el=>el.closest('.script-editor').__vue__.value)).includes('192.0.2.9'));
  await fillEditor('sleep 30');await dialog.getByRole('button',{name:'Запустить',exact:true}).click();await dialog.getByRole('button',{name:'Остановить',exact:true}).waitFor();assert(await dialog.getByRole('button',{name:'Запустить',exact:true}).isDisabled());await dialog.getByRole('button',{name:'Остановить',exact:true}).click();await dialog.getByText('Остановлено',{exact:true}).waitFor();assert.equal(cancelled.length,1);
  await fillEditor('sleep 40');await dialog.getByRole('button',{name:'Запустить',exact:true}).click();await dialog.getByRole('button',{name:'Остановить',exact:true}).waitFor();const cancelOnClose=page.waitForRequest(r=>r.method()==='DELETE'&&r.url().includes('/runner/jobs/'));await page.keyboard.press('Escape');await cancelOnClose;await dialog.waitFor({state:'hidden'});assert.equal(cancelled.length,2);
  await open('Python requests…');assert((await editor.evaluate(el=>el.closest('.script-editor').__vue__.value)).includes('import requests'));await fillEditor('import requests\n# python comment\nprint("hello", 42)');await page.waitForFunction(()=>document.querySelectorAll('.cm-line span[class]').length>3);
  const colors=await editor.locator('.cm-line span[class]').evaluateAll(nodes=>[...new Set(nodes.map(n=>getComputedStyle(n).color))]);assert(colors.length>=3,'Python tokens need distinct syntax colors');
  await page.getByLabel('Язык выполнения',{exact:true}).selectOption('bash');assert((await editor.evaluate(el=>el.closest('.script-editor').__vue__.value)).includes('import requests'),'language switch must preserve edits');
  await fillEditor('x'.repeat(262145));assert(await dialog.getByRole('button',{name:'Запустить',exact:true}).isDisabled());await page.keyboard.press('Escape');await dialog.waitFor({state:'hidden'});
  available=false;await page.setViewportSize({width:375,height:640});await open('pwntools…');await dialog.getByText('Среда запуска недоступна.',{exact:false}).waitFor();assert(await dialog.getByRole('button',{name:'Запустить',exact:true}).isDisabled());assert(!(await dialog.getByRole('button',{name:'Копировать скрипт',exact:true}).isDisabled()));assert(await dialog.evaluate(el=>el.scrollWidth<=el.clientWidth+1));const mobileLeft=await dialog.locator('.workbench-editor-pane').boundingBox(),mobileRight=await dialog.locator('.workbench-result').boundingBox();assert(mobileRight.y>=mobileLeft.y+mobileLeft.height);
  assert.deepEqual(errors,[]);Object.assign(report,{passed:true,submittedRuns:submitted.length,cancelledRuns:cancelled.length,checks:['editable generated requests/curl/pwntools','captured target defaults','manual edits survive generator settings','exact edited code copied and submitted','stdout/stderr rendered as text','run/stop/close lifecycle','script limit','copy without runner','375px stacked layout','Python/Bash syntax highlighting','800/1100/1440px split layout, larger editor','light/dark themes','undo and Tab escape','4000-line script virtualization and visible Run button'],pageErrors:errors});
 }finally{await browser?.close();server.kill('SIGTERM');fs.writeFileSync('reports/request-workbench-ui.json',JSON.stringify(report,null,2)+'\n')}
 console.log(JSON.stringify(report));
})().catch(e=>{console.error(e);process.exitCode=1});
