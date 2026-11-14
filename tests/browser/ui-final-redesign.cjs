const {chromium}=require('../../web/node_modules/playwright');
const fs=require('node:fs'),assert=require('node:assert/strict'),path=require('node:path');
process.chdir(path.resolve(__dirname,'../..'));
(async()=>{
 const env=Object.fromEntries(fs.readFileSync('.cache/kali.env','utf8').split('\n').filter(l=>l&&!l.startsWith('#')).map(l=>{const n=l.indexOf('=');return[l.slice(0,n),l.slice(n+1)]}));
 const browser=await chromium.launch({headless:true,executablePath:process.env.CHROMIUM_PATH||'/home/logalex/.cache/ms-playwright/chromium-1187/chrome-linux/chrome'}),ctx=await browser.newContext({httpCredentials:{username:'packmate',password:env.WEB_PASSWORD},viewport:{width:1100,height:760},reducedMotion:'reduce'}),page=await ctx.newPage(),errors=[],themes=[];
 page.on('pageerror',e=>errors.push(e.message));const report={passed:false,themes};
 try{
  await page.goto('http://192.168.56.103:65000/#/chat');await page.getByRole('textbox',{name:'Сообщение в чат'}).waitFor();
  assert.equal(await page.title(),'gomate — анализ трафика');
  for(const theme of ['dark','light']){
   if(await page.evaluate(()=>document.body.dataset.theme)!==theme)await page.getByRole('button',{name:'Переключить тему'}).click();
   const metrics=await page.evaluate(()=>{const button=document.querySelector('.chat-composer button[type=submit]'),input=document.querySelector('#chat-editor'),main=document.querySelector('main'),probe=document.createElement('div');probe.style.background='var(--gm-accent)';document.body.appendChild(probe);const accent=getComputedStyle(probe).backgroundColor;probe.remove();return{accent,background:getComputedStyle(button).backgroundColor,disabled:button.disabled,buttonBottom:button.getBoundingClientRect().bottom,inputBottom:input.getBoundingClientRect().bottom,mainBottom:main.getBoundingClientRect().bottom,mainScrollHeight:main.scrollHeight,mainClientHeight:main.clientHeight}});
   assert(metrics.disabled);assert.equal(metrics.background,metrics.accent);assert(metrics.buttonBottom<=metrics.mainBottom);assert(metrics.inputBottom<=metrics.mainBottom);assert(metrics.mainScrollHeight<=metrics.mainClientHeight+2);themes.push({theme,...metrics});
   if(theme==='dark')await page.screenshot({path:'reports/redesign-chat.jpg',type:'jpeg',quality:30,scale:'css'});
  }
  await page.locator('.gm-skip-link').focus();await page.keyboard.press('Enter');assert.equal(await page.evaluate(()=>document.activeElement.id),'workspace-main');assert.equal(new URL(page.url()).hash,'#/chat');
  assert.deepEqual(errors,[]);Object.assign(report,{passed:true,pageErrors:errors,skipLinkPreservesRoute:true});
 }finally{await browser.close();fs.writeFileSync('reports/ui-final-redesign.json',JSON.stringify(report,null,2)+'\n')}
 console.log(JSON.stringify(report));
})().catch(e=>{console.error(e);process.exitCode=1});
