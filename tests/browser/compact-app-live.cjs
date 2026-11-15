// Read-only instance check: no saved settings, Docker changes, chat messages or runner jobs.
const {chromium}=require('../../web/node_modules/playwright');
const fs=require('node:fs'),assert=require('node:assert/strict');
(async()=>{
 const base=process.env.GOMATE_URL||'http://192.168.56.103:65000',password=fs.readFileSync('.cache/kali.env','utf8').split('\n').find(x=>x.startsWith('WEB_PASSWORD=')).slice(13).replace(/^['"]|['"]$/g,'');
 const browser=await chromium.launch({headless:true}),report={passed:false,readOnly:true,layouts:[],errors:[],blockedWrites:[]};
 try{
  const page=await browser.newPage({viewport:{width:1366,height:900},httpCredentials:{username:'packmate',password},reducedMotion:'reduce'});
  page.on('pageerror',e=>report.errors.push(e.message));
  await page.route('**/api/**',r=>{const p=new URL(r.request().url()).pathname,m=r.request().method();if(m!=='GET'&&!(m==='POST'&&/^\/api\/(stream|packet)\//.test(p))){report.blockedWrites.push(m+' '+p);return r.abort()}return r.continue()});
  const before=await(await page.request.get(base+'/api/v2/snapshot')).json();
  const close=async id=>{await page.locator(id).getByRole('button',{name:/^(Close|Закрыть)$/}).first().click();await page.locator(id).waitFor({state:'hidden'})};
  const fits=async(selector='main')=>{assert(!(await page.evaluate(()=>document.documentElement.scrollWidth>innerWidth+1)));assert(await page.locator(selector).evaluate(el=>el.scrollWidth<=el.clientWidth+1),selector+' overflow')};
  for(const width of [1366,800,375])for(const theme of ['dark','light']){
   await page.setViewportSize({width,height:900});await page.goto(base+'/#/');await page.locator('#traffic-service').waitFor();await page.evaluate(t=>document.body.dataset.theme=t,theme);
   await page.getByRole('button',{name:'Добавить сервис',exact:true}).click();const service=page.locator('#serviceModal');await service.locator('#docker-service-search').waitFor();await service.locator('.candidate,.service-empty').first().waitFor();await fits('#serviceModal .modal-body');
   const candidate=service.locator('.candidate').filter({has:page.locator('button:not([disabled])')}).first();
   if(await candidate.count()){
    const expectedName=await candidate.locator('.candidate-name strong').textContent();await candidate.getByRole('button').click();await page.locator('#service-name').waitFor();assert.equal(await page.locator('#service-name').inputValue(),expectedName);assert(await page.locator('#service-port').isDisabled());await fits('#serviceModal .modal-body');report.dockerDraftChecked=true;
   }else{await service.getByRole('tab',{name:'Вручную',exact:true}).click();await page.locator('#service-name').waitFor()}
   await service.getByRole('button',{name:'Отмена',exact:true}).click();await service.waitFor({state:'hidden'});
   await page.getByRole('button',{name:'Настройки',exact:true}).click();await page.locator('#capture-interface').waitFor();
   for(const tab of ['Захват трафика','Хранение и очистка','Приложение','Отображение']){await page.locator('#settingsModal').getByRole('tab',{name:tab,exact:true}).click();await fits('#settingsModal .modal-body')}
   await close('#settingsModal');await page.getByRole('button',{name:/^Шаблоны/}).click();await page.locator('#patterns-search').waitFor();await fits('#patternsManager .modal-body');await close('#patternsManager');
   await page.goto(base+'/#/chat');await page.getByRole('textbox',{name:'Сообщение в чат'}).waitFor();await fits();
   const geometry=await page.evaluate(()=>{const bar=document.querySelector('.gm-topbar').getBoundingClientRect(),main=document.querySelector('main'),composer=document.querySelector('.chat-composer').getBoundingClientRect();return{topbarHeight:bar.height,composerBottom:composer.bottom,mainBottom:main.getBoundingClientRect().bottom,mainHeight:main.clientHeight,mainScrollHeight:main.scrollHeight}});
   assert(geometry.composerBottom<=geometry.mainBottom+1);assert(geometry.mainScrollHeight<=geometry.mainHeight+1);if(width===1366)assert(geometry.topbarHeight<=50);report.layouts.push({width,theme,...geometry});
  }
  const after=await(await page.request.get(base+'/api/v2/snapshot')).json();assert.deepEqual(after.services,before.services);assert(after.capture.running);assert.equal(after.capture.startedAt,before.capture.startedAt);assert.deepEqual(report.errors,[]);assert.deepEqual(report.blockedWrites,[]);report.captureRunning=true;report.servicesUnchanged=true;report.passed=true;
 }finally{await browser.close();fs.writeFileSync('reports/compact-app-live.json',JSON.stringify(report,null,2)+'\n')}
 console.log(JSON.stringify(report));
})().catch(e=>{console.error(e);process.exitCode=1});
