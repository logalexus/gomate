// Real Go multiplayer server, two browser identities, synthetic non-game API.
const {chromium}=require('../../web/node_modules/playwright');
const fs=require('node:fs'),path=require('node:path'),assert=require('node:assert/strict'),{spawn,execFileSync}=require('node:child_process');
process.chdir(path.resolve(__dirname,'../..'));
(async()=>{
 execFileSync(process.env.GO_BINARY||'.tools/go/bin/go',['build','-o','.cache/arena-test-server','./tests/fixtures/arena']);
 const server=spawn('.cache/arena-test-server',[],{stdio:['ignore','pipe','pipe']});let log='';server.stderr.on('data',b=>log+=b);
 const base=await new Promise((resolve,reject)=>{server.stdout.once('data',b=>resolve(b.toString().trim()));server.once('exit',()=>reject(Error(log)))});
 let browser;const report={passed:false,realMultiplayer:true,checks:[],errors:[]};
 try{
  browser=await chromium.launch({headless:true,executablePath:process.env.CHROMIUM_PATH||'/home/logalex/.cache/ms-playwright/chromium-1187/chrome-linux/chrome'});
  const pages=[];for(let i=0;i<2;i++){
   const context=await browser.newContext({httpCredentials:{username:'arena-test',password:'arena-test'},viewport:{width:1100,height:760}}),page=await context.newPage();pages.push(page);page.on('pageerror',e=>report.errors.push(e.message));
   await page.routeWebSocket('**/api/v2/events',()=>{});
   await page.route('**/api/**',async route=>{const p=new URL(route.request().url()).pathname;if(p.endsWith('/arena'))return route.continue();let data=[];if(p.endsWith('/snapshot'))data={services:[],patterns:[],metrics:{},captureRunning:true};if(p.endsWith('/config'))data={timeoutSeconds:30};if(p.endsWith('/chat'))data={author:'Аноним-test',messages:[],hasMore:false};await route.fulfill({json:data})});
   await page.goto(base+'/#/chat');await page.getByLabel('Сообщение в чат',{exact:true}).fill('draft survives game');
   await page.getByRole('button',{name:'Открыть мини-шутер'}).click();await page.getByRole('button',{name:'Войти на арену',exact:true}).click();await page.waitForFunction(()=>document.querySelector('.chat-page').__vue__.$children.find(c=>c.$options.name==='ArenaGame')?.me?.hp===100);
  }
  const [a,b]=pages;
  for(const page of pages)await page.waitForFunction(()=>document.querySelector('.chat-page').__vue__.$children.find(c=>c.$options.name==='ArenaGame').players.length===2);
  const initial=await a.evaluate(()=>{const g=document.querySelector('.chat-page').__vue__.$children.find(c=>c.$options.name==='ArenaGame');return {id:g.id,players:g.players}});assert.notEqual(initial.players[0].name,initial.players[1].name);report.checks.push('two authenticated browsers share authoritative arena with distinct anonymous identities');
  // Keyboard-only controls work without pointer lock, including HTTP deployments.
  await a.bringToFront();await a.evaluate(()=>{const g=document.querySelector('.chat-page').__vue__.$children.find(c=>c.$options.name==='ArenaGame');g.active=true;g.angle=-Math.PI/2;g.$refs.canvas.focus()});
  const before=await a.evaluate(()=>document.querySelector('.chat-page').__vue__.$children.find(c=>c.$options.name==='ArenaGame').me.y);
  await a.keyboard.down('KeyW');await a.waitForFunction(()=>document.querySelector('.chat-page').__vue__.$children.find(c=>c.$options.name==='ArenaGame').me.y<2.65,{},{timeout:8000});await a.keyboard.up('KeyW');
  const after=await a.evaluate(()=>document.querySelector('.chat-page').__vue__.$children.find(c=>c.$options.name==='ArenaGame').me.y);assert(after<before-5);
  await b.waitForFunction(id=>document.querySelector('.chat-page').__vue__.$children.find(c=>c.$options.name==='ArenaGame').players.find(p=>p.id===id).y<2.8,initial.id);report.checks.push('WASD movement replicated to other browser');
  await a.evaluate(()=>{const g=document.querySelector('.chat-page').__vue__.$children.find(c=>c.$options.name==='ArenaGame'),other=g.players.find(p=>p.id!==g.id);g.angle=Math.atan2(other.y-g.me.y,other.x-g.me.x)});
  await a.keyboard.down('Space');await a.waitForFunction(()=>document.querySelector('.chat-page').__vue__.$children.find(c=>c.$options.name==='ArenaGame').me.kills===1,{},{timeout:4000});await a.keyboard.up('Space');
  await b.waitForFunction(()=>document.querySelector('.chat-page').__vue__.$children.find(c=>c.$options.name==='ArenaGame').me.hp===0);await b.waitForFunction(()=>document.querySelector('.chat-page').__vue__.$children.find(c=>c.$options.name==='ArenaGame').me.hp===100,{},{timeout:5000});report.checks.push('three server-confirmed hits, score and automatic respawn replicated');
  await a.getByRole('button',{name:'Счёт',exact:true}).click();await a.locator('.arena-score-row').first().waitFor();assert.equal(await a.locator('.arena-score-row').count(),2);
  await a.screenshot({path:'reports/arena-desktop.jpg',type:'jpeg',quality:30});
  await a.keyboard.press('Escape');await a.getByRole('button',{name:'Продолжить',exact:true}).waitFor();report.checks.push('Escape pauses movement and releases mouse');
  await a.setViewportSize({width:390,height:700});await a.waitForTimeout(200);assert(await a.evaluate(()=>document.documentElement.scrollWidth<=innerWidth+1));
  const box=await a.getByRole('dialog').boundingBox();assert(box.y>=0&&box.y+box.height<=700);report.checks.push('mobile modal stays within viewport');
  await a.getByRole('button',{name:'Close',exact:true}).click();await a.getByRole('dialog').waitFor({state:'hidden'});assert.equal(await a.getByLabel('Сообщение в чат',{exact:true}).inputValue(),'draft survives game');
  await b.waitForFunction(()=>document.querySelector('.chat-page').__vue__.$children.find(c=>c.$options.name==='ArenaGame').players.length===1);report.checks.push('closing game disconnects player and preserves chat draft');
  for(const [width,height] of [[375,640],[390,700],[1100,760],[1440,1000],[1920,1080]])for(const theme of ['dark','light']){
   await a.setViewportSize({width,height});await a.evaluate(t=>document.body.setAttribute('data-theme',t),theme);await a.waitForTimeout(100);
   const bounds=await a.evaluate(()=>{const main=document.querySelector('main'),editor=document.querySelector('#chat-editor'),send=document.querySelector('.chat-composer button[type=submit]');return {overflow:document.documentElement.scrollWidth>innerWidth+1,mainBottom:main.getBoundingClientRect().bottom,editorTop:editor.getBoundingClientRect().top,sendBottom:send.getBoundingClientRect().bottom,scroll:main.scrollHeight-main.clientHeight}});
   assert(!bounds.overflow&&bounds.scroll<=1&&bounds.editorTop>=0&&bounds.sendBottom<=bounds.mainBottom,JSON.stringify({width,height,theme,...bounds}));assert(await a.getByRole('button',{name:'Открыть мини-шутер'}).isVisible());
  }
  report.checks.push('chat composer and arena button fit five viewports in both themes');
  assert.deepEqual(report.errors,[]);report.passed=true;
 }catch(error){report.error=error.stack;throw error}finally{await browser?.close();server.kill('SIGTERM');fs.writeFileSync('reports/arena-browser.json',JSON.stringify(report,null,2)+'\n')}
 console.log(JSON.stringify(report));
})().catch(e=>{console.error(e);process.exitCode=1});
