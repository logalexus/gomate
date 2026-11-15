// Check English UI independently of the browser locale, while preserving user text.
const {chromium}=require('../../web/node_modules/playwright');
const assert=require('node:assert/strict'),fs=require('node:fs'),path=require('node:path'),{spawn}=require('node:child_process');
process.chdir(path.resolve(__dirname,'../..'));
const delay=ms=>new Promise(resolve=>setTimeout(resolve,ms));
(async()=>{
 const base='http://127.0.0.1:4186';
 const server=spawn(process.execPath,['node_modules/vite/bin/vite.js','--host','127.0.0.1','--port','4186','--strictPort'],{cwd:'web',stdio:'ignore'});
 const report={passed:false,browserLocale:'ru-RU',checks:[],layouts:[]};let browser;
 try{
  // Prevent untranslated copy from returning in a hidden dialog or worker.
  const walk=dir=>fs.readdirSync(dir,{withFileTypes:true}).flatMap(e=>e.isDirectory()?walk(path.join(dir,e.name)):[path.join(dir,e.name)]);
  for(const file of ['web/index.html',...walk('web/src')])if(/\.(vue|js|mjs|html)$/.test(file))assert(!/[А-Яа-яЁё]/.test(fs.readFileSync(file,'utf8')),'Untranslated copy in '+file);
  report.checks.push('all frontend source, hidden dialogs and workers contain English copy');
  for(let i=0;i<60;i++){try{if((await fetch(base)).ok)break}catch{}await delay(150)}
  browser=await chromium.launch({headless:true});
  const page=await browser.newPage({locale:'ru-RU',viewport:{width:1366,height:900},reducedMotion:'reduce'});
  page.setDefaultTimeout(10000);const errors=[];page.on('pageerror',e=>errors.push(e.message));
  await page.routeWebSocket('**/api/v2/events',()=>{});
  const settings={interface:'eth0',autoIPs:true,localIPs:['192.0.2.1'],packetRetentionSeconds:0,sessionRetentionSeconds:3600,cleanupIntervalSeconds:60,preserveFavorites:true,requestTimeoutSeconds:10};
  const farm={revision:1,enabled:false,protocol:'ructf_http',url:'',tokenSet:false,flagFormat:'[A-Z0-9]{31}=',targets:[],exclude:[],pattern:'',first:1,last:50,period:2,batch:100,lifetime:300,retention:86400,timeout:5,clientWorkers:8,clientInterval:30};
  await page.route('**/api/**',async route=>{
   const p=new URL(route.request().url()).pathname;let data=[];
   if(p==='/api/v2/config')data={timeoutSeconds:10};
   if(p==='/api/v2/snapshot')data={services:[],patterns:[],metrics:{captured_total:12345,persisted_chunks_total:100},captureRunning:true,capture:{interface:'eth0',running:true}};
   if(p==='/api/v2/settings')data={settings,capture:{running:true,interface:'eth0',localIPs:['192.0.2.1']}};
   if(p==='/api/v2/interfaces')data=[{name:'eth0',addresses:['192.0.2.1'],up:true}];
   if(p==='/api/v2/chat')data={messages:[],author:'Anonymous-test',hasMore:false};
   if(p==='/api/v2/anomalies/stats')data={services:[],warmAfter:100};
   if(p==='/api/v2/anomaly-events')data={events:[],cursor:0};
   if(p==='/api/v2/farm/settings')data=farm;
   if(p==='/api/v2/farm/summary')data={groups:[],enabled:false,protocol:'ructf_http'};
   if(p==='/api/v2/farm/flags')data={flags:[],total:0};
   if(p==='/api/v2/farm/client-credentials')data={username:'test',password:'fixture'};
   if(p==='/api/v2/monitor')data={sampledAt:Date.now(),intervalSeconds:5,retentionMinutes:60,containers:[],host:{cpus:8},hostHistory:[],containerHistory:[]};
   await route.fulfill({json:data});
  });
  const checkEnglish=async scope=>{
   const text=await scope.evaluate(el=>[el.innerText,...[...el.querySelectorAll('[aria-label],[placeholder],[title]')].filter(n=>n.getClientRects().length).flatMap(n=>['aria-label','placeholder','title'].map(a=>n.getAttribute(a)||''))].join('\n'));
   assert(!/[А-Яа-яЁё]/.test(text),'Untranslated visible text: '+text.match(/.{0,50}[А-Яа-яЁё].{0,80}/)?.[0]);
  };
  for(const width of [1366,375])for(const theme of ['dark','light']){
   await page.setViewportSize({width,height:900});
   for(const route of ['all','anomalies','containers','monitoring','farm','chat']){
    await page.goto(base+'/#/'+route);await page.locator('.gm-navigation').waitFor();
    await page.evaluate(t=>document.body.setAttribute('data-theme',t),theme);await delay(200);
    assert.equal(await page.locator('html').getAttribute('lang'),'en');assert.equal(await page.title(),'gomate — traffic analysis');
    for(const label of ['Traffic','Anomalies','Containers','Monitoring','Farm','Team chat'])assert.equal(await page.getByRole('navigation',{name:'Main navigation'}).getByRole('link',{name:label,exact:true}).count(),1);
    await checkEnglish(page.locator('body'));
    assert(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth+1),'Page overflow '+route+' '+width);
    report.layouts.push({route,width,theme});
   }
  }
  report.checks.push('all six routes, English navigation and document language in both themes at 1366 and 375 px with a Russian browser locale');
  await page.setViewportSize({width:1366,height:900});
  await page.getByRole('button',{name:'Settings',exact:true}).click();const settingsModal=page.locator('#settingsModal');await settingsModal.locator('#capture-interface').waitFor();
  for(const tab of ['Traffic capture','Retention and cleanup','Application','Notifications','Display','Farm']){
   await settingsModal.getByRole('tab',{name:tab,exact:true}).click();await delay(100);await checkEnglish(settingsModal);
  }
  await page.keyboard.press('Escape');await settingsModal.waitFor({state:'hidden'});
  await page.getByRole('button',{name:/^Patterns/}).click();const manager=page.locator('#patternsManager');await manager.getByLabel('Search patterns',{exact:true}).waitFor();await checkEnglish(manager);
  await manager.getByRole('button',{name:'+ New pattern',exact:true}).click();const editor=page.locator('#patternModal');await editor.locator('#pattern-name').waitFor();await checkEnglish(editor);
  await editor.locator('#pattern-name').fill('Пользовательский шаблон');assert.equal(await editor.locator('#pattern-name').inputValue(),'Пользовательский шаблон');
  await page.keyboard.press('Escape');await editor.waitFor({state:'hidden'});await page.keyboard.press('Escape');await manager.waitFor({state:'hidden'});
  report.checks.push('all six settings tabs, pattern manager and editor, user-entered Unicode preserved');
  const chat=page.getByRole('textbox',{name:'Chat message'});await chat.fill('Сообщение команды');assert.equal(await chat.inputValue(),'Сообщение команды');await chat.fill('');
  await page.getByRole('button',{name:'Message formatting',exact:true}).click();await page.getByRole('menuitem',{name:'Code block',exact:true}).click();assert.equal(await chat.inputValue(),'```\ncode\n```');
  assert.deepEqual(errors,[]);report.checks.push('English formatting snippets, Unicode chat draft preserved and no browser errors');report.passed=true;
 }finally{await browser?.close();server.kill();fs.writeFileSync('reports/english-ui-check.json',JSON.stringify(report,null,2)+'\n')}
 console.log(JSON.stringify(report));
})().catch(e=>{console.error(e);process.exitCode=1});
