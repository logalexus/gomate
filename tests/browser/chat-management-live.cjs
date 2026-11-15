const {chromium}=require('../../web/node_modules/playwright');
const fs=require('node:fs'),assert=require('node:assert/strict'),path=require('node:path');
process.chdir(path.resolve(__dirname,'../..'));
(async()=>{
 const env=Object.fromEntries(fs.readFileSync('.cache/kali.env','utf8').split('\n').filter(l=>l&&!l.startsWith('#')).map(l=>{const i=l.indexOf('=');return[l.slice(0,i),l.slice(i+1).replace(/^['"]|['"]$/g,'')]}));
 const base=process.env.GOMATE_URL||'http://192.168.56.103:65000',run='chat_management_'+Date.now(),report={passed:false,run,messageIDs:[],checks:[],pageErrors:[]};
 const browser=await chromium.launch({headless:true});
 const options={httpCredentials:{username:'packmate',password:env.WEB_PASSWORD},viewport:{width:1100,height:800}};
 const a=await browser.newContext(options),b=await browser.newContext(options),page=await a.newPage(),peer=await b.newPage();
 for(const p of [page,peer]){p.setDefaultTimeout(15000);p.on('pageerror',e=>report.pageErrors.push(e.message))}
 fs.writeFileSync('reports/chat-management-live-check.json',JSON.stringify(report,null,2)+'\n');
 const action=async(p,id,name)=>{const row=p.locator(`[data-message-id="${id}"]`);await row.getByRole('button',{name:'Message actions',exact:true}).click();await row.getByRole('menuitem',{name,exact:true}).click()};
 try{
  await page.goto(base+'/#/chat');await peer.goto(base+'/#/chat');
  await page.waitForFunction(()=>!document.querySelector('.chat-page').textContent.includes('Connecting…'));
  const composer=page.getByRole('textbox',{name:'Chat message',exact:true});
  await composer.fill(run+' **original**');
  const posted=page.waitForResponse(r=>r.url().endsWith('/api/v2/chat')&&r.request().method()==='POST');await composer.press('Enter');
  const sent=await(await posted).json();assert(sent.id);report.messageIDs.push(sent.id);
  const row=p=>p.locator(`[data-message-id="${sent.id}"]`);
  await row(peer).waitFor();assert.equal(await row(peer).locator('.chat-you').count(),0);
  await composer.fill('Keep this unsent draft');
  await action(peer,sent.id,'Edit');const peerEdit=peer.getByRole('textbox',{name:'Edit message',exact:true});
  assert.equal(await peerEdit.inputValue(),run+' **original**');
  const edited=run+' **edited by teammate**\n\n```python\nprint("hello")\n```';await peerEdit.fill(edited);await peer.getByRole('button',{name:'Save',exact:true}).click();
  await row(page).locator('pre code').waitFor();assert.equal(await row(page).locator('pre code').textContent(),'print("hello")\n');
  assert.equal(await row(page).locator('.chat-edited').textContent(),'(edited)');assert.equal(await composer.inputValue(),'Keep this unsent draft');
  report.checks.push('teammate edits foreign message, Markdown rendered and synchronized, author retained, composer draft preserved');
  await action(page,sent.id,'Edit');const edit=page.getByRole('textbox',{name:'Edit message',exact:true});assert.equal(await edit.inputValue(),edited);
  await edit.fill('discard this edit');await edit.press('Escape');assert.equal(await edit.count(),0);assert.equal(await composer.inputValue(),'Keep this unsent draft');
  report.checks.push('Escape cancels edit without changing message or new-message draft');
  // Keep an old editor open while the other browser saves a newer revision.
  await action(page,sent.id,'Edit');await edit.fill(run+' stale revision');
  await action(peer,sent.id,'Edit');await peerEdit.fill(run+' **newer revision**');await peer.getByRole('button',{name:'Save',exact:true}).click();await row(peer).locator('.chat-markdown strong').filter({hasText:'newer revision'}).waitFor();
  const conflict=page.waitForResponse(r=>r.request().method()==='PATCH');await edit.press('Control+Enter');assert.equal((await conflict).status(),409);await page.locator('#chat-edit-error').waitFor();assert.equal(await edit.inputValue(),run+' stale revision');
  await page.getByRole('button',{name:'Cancel',exact:true}).click();await row(page).locator('.chat-markdown strong').filter({hasText:'newer revision'}).waitFor();
  await action(page,sent.id,'Edit');await edit.fill(run+' final');await edit.press('Control+Enter');await row(peer).getByText(run+' final',{exact:true}).waitFor();
  report.checks.push('stale concurrent save rejected with draft intact; reopening uses current version; Ctrl+Enter saves');
  for(const width of [390,1100]){await peer.setViewportSize({width,height:844});await action(peer,sent.id,'Edit');assert(await peerEdit.isVisible());assert(await peer.evaluate(()=>document.documentElement.scrollWidth<=innerWidth+1),'horizontal overflow');await peerEdit.press('Escape')}
  report.checks.push('edit controls and message menu usable at 390px and desktop');
  await action(peer,sent.id,'Delete');let modal=peer.getByRole('dialog',{name:'Delete message',exact:true});await modal.getByRole('button',{name:'Cancel',exact:true}).click();await modal.waitFor({state:'hidden'});assert(await row(peer).isVisible());
  // A remote deletion closes an open editor and removes the message everywhere.
  await action(page,sent.id,'Edit');await edit.fill('unsaved edit before remote deletion');
  await action(peer,sent.id,'Delete');modal=peer.getByRole('dialog',{name:'Delete message',exact:true});await modal.getByRole('button',{name:'Delete',exact:true}).click();
  await row(peer).waitFor({state:'detached'});await row(page).waitFor({state:'detached'});assert.equal(await edit.count(),0);assert.equal(await composer.inputValue(),'Keep this unsent draft');
  await page.reload();await page.getByRole('textbox',{name:'Chat message',exact:true}).waitFor();const data=await(await a.request.get(base+'/api/v2/chat')).json();assert(!data.messages.some(m=>m.id===sent.id));assert.equal(await row(page).count(),0);
  report.checks.push('delete confirmation cancel/confirm, foreign message deleted for everyone, open remote editor closed, deletion survives reload');
  assert.deepEqual(report.pageErrors,[]);report.passed=true;
 }finally{
  // Only remove this test's own messages if a check failed midway.
  for(const id of report.messageIDs)await b.request.delete(base+'/api/v2/chat/'+id).catch(()=>{});
  fs.writeFileSync('reports/chat-management-live-check.json',JSON.stringify(report,null,2)+'\n');await browser.close();
 }
 console.log(JSON.stringify(report));
})().catch(e=>{console.error(e);process.exitCode=1});
