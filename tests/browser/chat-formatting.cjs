const {chromium}=require('../../web/node_modules/playwright');
const fs=require('node:fs'),assert=require('node:assert/strict'),path=require('node:path');
process.chdir(path.resolve(__dirname,'../..'));
(async()=>{
 const env=Object.fromEntries(fs.readFileSync('.cache/kali.env','utf8').split('\n').filter(l=>l&&!l.startsWith('#')).map(l=>{const n=l.indexOf('=');return[l.slice(0,n),l.slice(n+1)]}));
 const base='http://192.168.56.103:65000',run='markdown_'+Date.now(),errors=[];
 const browser=await chromium.launch({headless:true,executablePath:process.env.CHROMIUM_PATH||'/home/logalex/.cache/ms-playwright/chromium-1187/chrome-linux/chrome'});
 const opts={httpCredentials:{username:'packmate',password:env.WEB_PASSWORD},viewport:{width:1100,height:800}};
 const a=await browser.newContext(opts),b=await browser.newContext(opts),page=await a.newPage(),peer=await b.newPage();
 // Persist the run marker even if an assertion fails, so only our messages can be cleaned up.
 const report={passed:false,run,sentIDs:[]};fs.writeFileSync('reports/ui-chat-formatting.json',JSON.stringify(report,null,2)+'\n');
 for(const p of [page,peer])p.on('pageerror',e=>errors.push(e.message));
 try{
  await page.goto(base+'/#/chat');await peer.goto(base+'/#/chat');
  const editor=page.getByRole('textbox',{name:'Chat message'});
  await editor.waitFor();await page.waitForFunction(()=>!document.querySelector('.chat-page').textContent.includes('Connecting…'));
  await editor.fill('selected');await editor.evaluate(el=>el.select());await page.getByRole('button',{name:'Message formatting',exact:true}).click();await page.getByRole('menuitem',{name:'Bold',exact:true}).click();assert.equal(await editor.inputValue(),'**selected**');
  await editor.fill('');await page.getByRole('button',{name:'Message formatting',exact:true}).click();await page.getByRole('menuitem',{name:'Code block',exact:true}).click();assert.equal(await editor.inputValue(),'```\ncode\n```');
  await editor.press('ArrowRight');await editor.press('Enter');assert.equal(await editor.inputValue(),'```\ncode\n\n```'); // newline, not accidental send
  const code='  if a < b {\n\tfmt.Println("<script>")\n  }\n';
  const text=run+' **жирный** *курсив* ~~удалён~~ `inline`\n\n- первый\n- второй\n\n> цитата\n\n[документация](https://example.com)\n\n```go\n'+code+'```\n\n<img src=x onerror=window.chatXSS=1>\n[x](javascript:alert%281%29)\n![image](https://example.com/pixel)';
  await editor.fill(text);await page.getByRole('button',{name:'Message formatting',exact:true}).click();await page.getByRole('menuitem',{name:'Preview',exact:true}).click();
  const preview=page.getByLabel('Message preview');await preview.locator('strong').waitFor();assert.equal(await preview.locator('pre code').textContent(),code);assert.equal(await preview.locator('img,script,svg').count(),0);
  await page.getByRole('button',{name:'Edit',exact:true}).click();assert.equal(await editor.inputValue(),text);
  const posted=page.waitForResponse(r=>r.url().endsWith('/api/v2/chat')&&r.request().method()==='POST');await editor.press('Control+Enter');assert((await posted).ok());
  const row=peer.locator('.chat-message').filter({hasText:run});await row.locator('strong').filter({hasText:'жирный'}).waitFor();
  assert.equal(await row.locator('pre code').textContent(),code);assert.equal(await row.locator('em').textContent(),'курсив');assert.equal(await row.locator('ul li').count(),2);assert.equal((await row.locator('blockquote').textContent()).trim(),'цитата');
  assert.equal(await row.locator('img,script,svg').count(),0);assert.equal(await peer.evaluate(()=>window.chatXSS),undefined);assert.equal(await row.locator('a[href^="javascript:"]').count(),0);
  assert.equal(await row.getByRole('link',{name:'документация'}).getAttribute('rel'),'noopener noreferrer');
  await peer.evaluate(()=>{const original=document.execCommand;document.execCommand=function(command,...args){if(command==='copy')window.copiedChatCode=document.activeElement.value;return original.call(this,command,...args)}});
  await row.getByRole('button',{name:'Copy code',exact:true}).click();await row.getByRole('status').filter({hasText:'Code copied'}).waitFor();assert.equal(await peer.evaluate(()=>window.copiedChatCode),code);
  const data=await (await a.request.get(base+'/api/v2/chat')).json();const saved=data.messages.find(m=>m.text===text);assert(saved);report.sentIDs.push(saved.id);
  await peer.reload();await peer.locator(`[data-message-id="${saved.id}"] pre code`).waitFor();
  // Plain Enter still sends, while Shift+Enter does not.
  await editor.fill(run+' plain');await editor.press('Shift+Enter');assert.equal(await editor.inputValue(),run+' plain\n');
  const plainPosted=page.waitForResponse(r=>r.url().endsWith('/api/v2/chat')&&r.request().method()==='POST');await editor.press('Enter');assert((await plainPosted).ok());
  await peer.locator('.chat-message').filter({hasText:run+' plain'}).waitFor();
  await page.setViewportSize({width:390,height:844});await editor.fill('```\n'+'long_code_'.repeat(150)+'\n```');await page.getByRole('button',{name:'Message formatting',exact:true}).click();await page.getByRole('menuitem',{name:'Preview',exact:true}).click();
  assert(await page.evaluate(()=>document.documentElement.scrollWidth<=window.innerWidth+1),'mobile horizontal overflow');
  assert.deepEqual(errors,[]);Object.assign(report,{passed:true,pageErrors:errors,checks:['toolbar selection','Enter inside fenced code','preview matches sent content','Ctrl+Enter and Shift+Enter','cross-client formatted messages','copy preserves whitespace over HTTP','HTML and unsafe URLs inert','raw Markdown stored and survives reload','390px layout with long code']});
 }finally{fs.writeFileSync('reports/ui-chat-formatting.json',JSON.stringify(report,null,2)+'\n');await browser.close()}
 console.log(JSON.stringify(report));
})().catch(e=>{console.error(e);process.exitCode=1});
