// Mocked API UI regression. Run: node tests/browser/ui-operations-redesign.cjs
// Optional: UI_BASE_URL for an existing frontend; UI_TEST_PORT (default 4174),
// CHROMIUM_PATH to select a browser. No SSH, .env, or real Docker is used.
const {chromium}=require('../../web/node_modules/playwright');
const assert=require('node:assert/strict'),fs=require('node:fs'),path=require('node:path'),os=require('node:os'),{spawn}=require('node:child_process');
const root=path.resolve(__dirname,'../..');
process.chdir(root);
const delay=ms=>new Promise(resolve=>setTimeout(resolve,ms));
async function startFrontend(){
 if(process.env.UI_BASE_URL)return {url:process.env.UI_BASE_URL.replace(/\/$/,''),stop:async()=>{}};
 const port=Number(process.env.UI_TEST_PORT||4174);assert(Number.isInteger(port)&&port>1024&&port<65536,'invalid UI_TEST_PORT');
 const url='http://127.0.0.1:'+port;
 const server=spawn(process.execPath,[path.join(root,'web/node_modules/vite/bin/vite.js'),'--host','127.0.0.1','--port',String(port),'--strictPort'],{cwd:path.join(root,'web'),stdio:['ignore','pipe','pipe']});
 let output='';server.stdout.on('data',b=>output=(output+b).slice(-4000));server.stderr.on('data',b=>output=(output+b).slice(-4000));
 const stop=async()=>{if(server.exitCode===null){server.kill('SIGTERM');await Promise.race([new Promise(resolve=>server.once('exit',resolve)),delay(3000)]);if(server.exitCode===null)server.kill('SIGKILL')}};
 try{for(let i=0;i<60;i++){if(server.exitCode!==null)throw Error('Vite exited: '+output);try{const response=await fetch(url,{signal:AbortSignal.timeout(1000)});if(response.ok){await delay(150);if(server.exitCode!==null)throw Error('Vite exited: '+output);return {url,stop}}}catch{}await delay(150)}throw Error('Vite not ready: '+output)}catch(error){await stop();throw error}
}
(async()=>{
 const frontend=await startFrontend();let browser;
 const report={passed:false,mocked:true,dataSource:'synthetic API fixtures; no real services or backend lifecycle actions',widths:[1440,375,1100],viewports:[{width:1440,height:1000},{width:375,height:1000},{width:1100,height:760}],composerVisibility:[],themes:['dark','light'],layouts:[],logDOMRows:[],chatDOMRows:[]};
 try{
 const fallback=path.join(os.homedir(),'.cache/ms-playwright/chromium-1187/chrome-linux/chrome');
 const executablePath=process.env.CHROMIUM_PATH||(fs.existsSync(fallback)?fallback:undefined);
 browser=await chromium.launch({headless:true,...(executablePath?{executablePath}:{})});
 const page=await browser.newPage({viewport:{width:1440,height:1000}}),errors=[];page.on('pageerror',e=>errors.push(e.message));
 const services=[{port:8080,name:'Web shop',http:true}],containers=[{id:'a'.repeat(64),name:'web-shop',image:'local/shop:latest',state:'running',status:'Up 4 hours',ports:['0.0.0.0:8080 → 80/tcp'],protected:false},{id:'b'.repeat(64),name:'gomate',image:'gomate:test',state:'running',status:'Up 4 hours',ports:[],protected:true}];
 const logs=Array.from({length:2000},(_,i)=>({id:String(i),time:new Date().toISOString(),stream:i%3?'stdout':'stderr',text:i%3?'request finished '+i:'synthetic error '+i}));
 const messages=Array.from({length:100},(_,i)=>({id:i+1,author:i%2?'Аноним-12345678':'Аноним-87654321',text:i===99?'**result**\n```go\nfmt.Println("OK")\n```':'Сообщение команды '+i,createdAt:new Date().toISOString()}));
 await page.routeWebSocket('**/api/v2/events',()=>{});
 await page.route('**/api/**',async route=>{const req=route.request(),url=new URL(req.url()),p=url.pathname;let data=[];
 if(p==='/api/v2/snapshot')data={services,patterns:[],metrics:{captured_total:123,persisted_chunks_total:100},captureRunning:true,capture:{interface:'eth1'}};
 else if(p==='/api/service/'||p==='/api/service')data=services;
 else if(p==='/api/v2/containers')data=containers;
 else if(p.endsWith('/logs'))data={lines:logs,tty:false,truncated:false,fetchedAt:new Date().toISOString()};
 else if(p==='/api/v2/chat'){if(req.method()==='POST'){const v=req.postDataJSON();messages.push({id:messages.length+1,author:'Аноним-12345678',text:v.text,createdAt:new Date().toISOString()});data=messages.at(-1)}else data={author:'Аноним-12345678',messages:url.searchParams.has('after')?messages.filter(m=>m.id>Number(url.searchParams.get('after'))):messages,hasMore:false}}
 else if(p==='/api/v2/anomalies/stats')data={services:[{port:8080,warm:true,messages:200,endpoints:4,reported:1}],drops:0,flagPattern:true,warmAfter:100};
 else if(p==='/api/v2/anomalies')data=[{streamId:123,service:8080,score:18,kind:'anomaly',summary:'POST /login',startTimestamp:Date.now(),reasons:[{bits:18,detector:'chars',field:'password',text:'Необычные символы',template:'POST /login'}]}];
 await route.fulfill({json:data});});
 for(const {width,height} of report.viewports)for(const theme of ['dark','light']){
 await page.setViewportSize({width,height});
 for(const route of ['containers','chat','anomalies']){
 await page.goto(frontend.url+'/#/'+route);await page.waitForSelector('.gm-page-title');await page.evaluate(t=>document.body.setAttribute('data-theme',t),theme);await page.waitForTimeout(250);
 const over=await page.evaluate(()=>({w:innerWidth,doc:document.documentElement.scrollWidth,main:document.querySelector('main').scrollWidth,client:document.querySelector('main').clientWidth}));assert(over.doc<=width+1,JSON.stringify({route,theme,width,over}));assert(over.main<=over.client+1,JSON.stringify({route,theme,width,over}));report.layouts.push({route,theme,width,documentWidth:over.doc,mainWidth:over.main,mainClientWidth:over.client});
 if(route==='containers'){
 await page.getByRole('textbox',{name:'Поиск контейнеров'}).fill('no-such-container');await page.getByText('Ничего не найдено',{exact:true}).waitFor();await page.getByRole('button',{name:'Показать все контейнеры',exact:true}).click();await page.locator('tr[data-container="web-shop"]').getByRole('button',{name:'Логи',exact:true}).click();await page.locator('.log-row').first().waitFor();const logRows=await page.locator('.log-row').count();assert(logRows>0&&logRows<60);report.logDOMRows.push({width,theme,loaded:2000,rendered:logRows});await page.getByRole('textbox',{name:'Поиск в логах'}).fill('synthetic error');await page.waitForTimeout(300);assert((await page.locator('.log-row').allTextContents()).every(t=>t.includes('synthetic error')));
 }
 if(route==='chat'){
 const editor=page.getByRole('textbox',{name:'Сообщение в чат'});await editor.fill('**preview**');await page.getByRole('button',{name:'Предпросмотр',exact:true}).click();await page.getByLabel('Предпросмотр сообщения').locator('strong').waitFor();await page.getByRole('button',{name:'Редактировать',exact:true}).click();assert.equal(await editor.inputValue(),'**preview**');if(width===1100&&height===760){const bounds=await page.evaluate(()=>{const main=document.querySelector('main'),editor=document.querySelector('#chat-editor'),send=document.querySelector('.chat-composer button[type=submit]'),timeline=document.querySelector('.chat-timeline');return {mainTop:main.getBoundingClientRect().top,mainBottom:main.getBoundingClientRect().bottom,editorTop:editor.getBoundingClientRect().top,editorBottom:editor.getBoundingClientRect().bottom,sendBottom:send.getBoundingClientRect().bottom,timelineHeight:timeline.clientHeight,scrollHeight:main.scrollHeight,clientHeight:main.clientHeight,scrollTop:main.scrollTop}});assert(bounds.editorTop>=bounds.mainTop&&bounds.sendBottom<=bounds.mainBottom,JSON.stringify(bounds));assert(bounds.scrollHeight<=bounds.clientHeight+1,JSON.stringify(bounds));assert.equal(bounds.scrollTop,0);report.composerVisibility.push({theme,width,height,...bounds})}const chatRows=await page.locator('.chat-message').count();assert(chatRows>0&&chatRows<40);report.chatDOMRows.push({width,theme,loaded:100,rendered:chatRows});
 }
 if(route==='anomalies'){const card=page.locator('.service-card').first();await card.focus();await page.keyboard.press('Enter');assert.equal(await card.getAttribute('aria-pressed'),'true');await page.getByRole('button',{name:'Сбросить фильтры',exact:true}).click();assert.equal(await card.getAttribute('aria-pressed'),'false');}
 }
 }
 assert.deepEqual(errors,[]);Object.assign(report,{passed:true,pageErrors:errors,checks:['responsive operations/chat/anomalies','container filtering and reset','virtualized filtered logs','Markdown preview preserves draft','virtualized chat','keyboard service selection']});
 }catch(error){report.error=error.message;throw error}finally{await browser?.close();await frontend.stop();fs.mkdirSync('reports',{recursive:true});fs.writeFileSync('reports/ui-operations-redesign.json',JSON.stringify(report,null,2)+'\n')}
 console.log(JSON.stringify(report));
})().catch(e=>{console.error(e);process.exitCode=1});
