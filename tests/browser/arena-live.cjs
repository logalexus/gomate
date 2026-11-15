// Live deployment smoke: join two temporary players, move, leave; never post chat messages.
const {chromium}=require('../../web/node_modules/playwright');
const fs=require('node:fs'),path=require('node:path'),assert=require('node:assert/strict');
process.chdir(path.resolve(__dirname,'../..'));
(async()=>{
 const env=Object.fromEntries(fs.readFileSync('.cache/kali.env','utf8').split('\n').filter(l=>l&&!l.startsWith('#')).map(l=>{const i=l.indexOf('=');return[l.slice(0,i),l.slice(i+1)]}));
 const base=process.env.ARENA_BASE_URL||'http://192.168.56.103:65000',browser=await chromium.launch({headless:true,executablePath:process.env.CHROMIUM_PATH||'/home/logalex/.cache/ms-playwright/chromium-1187/chrome-linux/chrome'}),report={passed:false,checks:[],errors:[]},pages=[];
 try{
  for(let i=0;i<2;i++){
   const context=await browser.newContext({httpCredentials:{username:'packmate',password:env.WEB_PASSWORD},viewport:{width:1100,height:760}}),page=await context.newPage();pages.push(page);page.on('pageerror',e=>report.errors.push(e.message));await page.goto(base+'/#/chat');
   await page.waitForFunction(()=>!!document.querySelector('.chat-page').__vue__.author);const name=await page.evaluate(()=>document.querySelector('.chat-page').__vue__.author);
   await page.getByRole('button',{name:'Open mini shooter'}).click();await page.getByRole('button',{name:'Join arena',exact:true}).click();
   await page.waitForFunction(()=>!!document.querySelector('.chat-page').__vue__.$children.find(c=>c.$options.name==='ArenaGame')?.me);assert.equal(await page.evaluate(()=>document.querySelector('.chat-page').__vue__.$children.find(c=>c.$options.name==='ArenaGame').me.name),name);
  }
  const webgl=await pages[0].evaluate(()=>{const g=document.querySelector('.chat-page').__vue__.$children.find(c=>c.$options.name==='ArenaGame');return g.renderer.gl instanceof WebGLRenderingContext&&g.renderer.gl.isEnabled(g.renderer.gl.DEPTH_TEST)});assert(webgl);report.checks.push('deployed frontend renders real 3D geometry with WebGL depth testing');
  const ids=[];for(const page of pages)ids.push(await page.evaluate(()=>document.querySelector('.chat-page').__vue__.$children.find(c=>c.$options.name==='ArenaGame').id));
  for(const page of pages)await page.waitForFunction(ids=>ids.every(id=>document.querySelector('.chat-page').__vue__.$children.find(c=>c.$options.name==='ArenaGame').players.some(p=>p.id===id)),ids);report.checks.push('two clients connected to deployed Go arena with their chat identities');
  const a=pages[0];await a.bringToFront();const before=await a.evaluate(()=>{const g=document.querySelector('.chat-page').__vue__.$children.find(c=>c.$options.name==='ArenaGame');g.active=true;g.pitch=.2;g.$refs.canvas.focus();return {...g.me}});await a.keyboard.down('KeyW');await a.waitForFunction(before=>{const me=document.querySelector('.chat-page').__vue__.$children.find(c=>c.$options.name==='ArenaGame').me;return Math.hypot(me.x-before.x,me.y-before.y)>.1&&me.pitch>.1},before,{timeout:5000});await a.keyboard.up('KeyW');const after=await a.evaluate(()=>document.querySelector('.chat-page').__vue__.$children.find(c=>c.$options.name==='ArenaGame').me);assert(Math.hypot(after.x-before.x,after.y-before.y)>.1);assert(after.pitch>.1);report.checks.push('movement and vertical aim accepted by deployed server');
  for(const page of pages){await page.getByRole('button',{name:'Close',exact:true}).last().click();await page.getByRole('dialog').waitFor({state:'hidden'})}
  const snapshot=await (await a.context().request.get(base+'/api/v2/snapshot')).json();assert(snapshot.captureRunning);assert((await a.context().request.get(base+'/health/ready')).ok());report.checks.push('capture running and readiness healthy after game');assert.deepEqual(report.errors,[]);report.passed=true;
 }finally{await browser.close();fs.writeFileSync('reports/arena-live.json',JSON.stringify(report,null,2)+'\n')}
 console.log(JSON.stringify(report));
})().catch(e=>{console.error(e);process.exitCode=1});
