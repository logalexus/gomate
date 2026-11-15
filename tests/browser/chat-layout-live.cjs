// Read-only smoke against the configured Kali test deployment; does not post messages.
const {chromium}=require('../../web/node_modules/playwright');
const fs=require('node:fs'),assert=require('node:assert/strict');
process.chdir(require('node:path').resolve(__dirname,'../..'));
(async()=>{
 const env=Object.fromEntries(fs.readFileSync('.cache/kali.env','utf8').split('\n').filter(l=>l&&!l.startsWith('#')).map(l=>{const i=l.indexOf('=');return [l.slice(0,i),l.slice(i+1)]}));
 const browser=await chromium.launch({executablePath:'/home/logalex/.cache/ms-playwright/chromium-1187/chrome-linux/chrome',headless:true});
 const ctx=await browser.newContext({httpCredentials:{username:'packmate',password:env.WEB_PASSWORD},viewport:{width:1100,height:760}}),page=await ctx.newPage(),errors=[];
 page.on('pageerror',e=>errors.push(e.message));
 const base='http://192.168.56.103:65000',report={passed:false,readOnly:true,checks:[]};
 try{
  const ready=await ctx.request.get(base+'/health/ready');assert(ready.ok());
  await page.goto(base+'/#/chat');await page.locator('.chat-input-shell').waitFor();
  await page.waitForFunction(()=>!!document.querySelector('.chat-page')?.__vue__?.author);
  const editor=page.getByRole('textbox',{name:'Chat message'});
  await editor.fill('**Локальный предпросмотр**');
  await page.getByRole('button',{name:'Message formatting',exact:true}).click();await page.getByRole('menuitem',{name:'Preview',exact:true}).click();
  await page.getByLabel('Message preview').locator('strong').waitFor();await page.getByRole('button',{name:'Edit',exact:true}).click();assert.equal(await editor.inputValue(),'**Локальный предпросмотр**');await editor.fill('');
  report.layout=await page.evaluate(()=>{const main=document.querySelector('main'),chat=document.querySelector('.chat-page'),input=document.querySelector('.chat-input-shell'),timeline=document.querySelector('.chat-timeline');return {mainWidth:main.clientWidth,chatWidth:chat.clientWidth,mainHeight:main.clientHeight,mainScrollHeight:main.scrollHeight,timelineHeight:timeline.clientHeight,inputHeight:input.clientHeight}});
  assert.equal(report.layout.mainWidth,report.layout.chatWidth);assert(report.layout.mainScrollHeight<=report.layout.mainHeight+1);assert(report.layout.inputHeight<55);assert(report.layout.timelineHeight>500);
  const snapshot=await (await ctx.request.get(base+'/api/v2/snapshot')).json();assert(snapshot.captureRunning);report.captureRunning=snapshot.captureRunning;
  await page.evaluate(()=>document.body.setAttribute('data-theme','dark'));await page.screenshot({path:'reports/chat-redesign-live.jpg',type:'jpeg',quality:30});
  assert.deepEqual(errors,[]);Object.assign(report,{passed:true,pageErrors:errors,checks:['ready endpoint','anonymous chat loads','menu preview preserves draft','full workspace width','one-line compact composer','capture still running','no chat messages sent']});
 }finally{await browser.close();fs.writeFileSync('reports/chat-redesign-live.json',JSON.stringify(report,null,2)+'\n')}
 console.log(JSON.stringify(report));
})().catch(e=>{console.error(e);process.exitCode=1});
