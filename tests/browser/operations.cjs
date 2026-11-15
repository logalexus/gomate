const {chromium}=require('../../web/node_modules/playwright');
const fs=require('node:fs'),assert=require('node:assert/strict'),{execFileSync}=require('node:child_process'),path=require('node:path');
process.chdir(path.resolve(__dirname,'../..'));
(async()=>{
 const env=Object.fromEntries(fs.readFileSync('.cache/kali.env','utf8').split('\n').filter(l=>l&&!l.startsWith('#')).map(l=>{const n=l.indexOf('=');return[l.slice(0,n),l.slice(n+1)]}));
 const base='http://192.168.56.103:65000',run='ops_'+Date.now();
 const browser=await chromium.launch({headless:true,executablePath:process.env.CHROMIUM_PATH||'/home/logalex/.cache/ms-playwright/chromium-1187/chrome-linux/chrome'});
 const options={httpCredentials:{username:'packmate',password:env.WEB_PASSWORD},viewport:{width:1536,height:1000}};
 const a=await browser.newContext(options),b=await browser.newContext(options),page=await a.newPage(),peer=await b.newPage(),errors=[];for(const p of [page,peer])p.on('pageerror',e=>errors.push(e.message));
 const api=async(ctx,method,route,body)=>{const r=await ctx.request.fetch(base+'/api/v2/'+route,{method,data:body});assert(r.ok(),`${method} ${route}: ${r.status()} ${await r.text()}`);return r.json()};
 try{
  await page.goto(base+'/#/containers');const row=page.locator('tr[data-container="gomate-ui-ops"]');await row.waitFor();const containers=await api(a,'GET','containers');const fixture=containers.find(c=>c.name==='gomate-ui-ops'),tty=containers.find(c=>c.name==='gomate-ui-tty');assert(fixture&&tty,'create isolated UI fixtures first');
  assert(containers.filter(c=>c.protected).length>=3);for(const c of containers.filter(c=>c.protected))assert.equal(await page.locator(`tr[data-container="${c.name}"]`).getByRole('button',{name:'Stop',exact:true}).count(),0);
  let r=await a.request.post(base+`/api/v2/containers/${containers.find(c=>c.protected).id}/stop`,{data:{confirm:true}});assert.equal(r.status(),403);
  const action=async(label,state,confirm=false)=>{await row.getByRole('button',{name:label,exact:true}).click();if(confirm)await page.getByRole('dialog').getByRole('button',{name:label==='Pause'?'Pause':label,exact:true}).click();await page.waitForFunction(({name,state})=>{const row=document.querySelector(`tr[data-container="${name}"]`);return row?.innerText.includes(state)&&!row.innerText.includes('Running…')},{name:'gomate-ui-ops',state},{timeout:20000})};
  await action('Pause','Paused',true);await action('Resume','Running');await action('Restart','Running',true);await action('Stop','Stopped',true);await action('Run','Running');
  await page.screenshot({path:'reports/ui-containers.jpg',type:'jpeg',quality:35,scale:'css'});await row.getByRole('button',{name:'Logs',exact:true}).click();await page.locator('#logs-tail').selectOption('2000');
  await page.waitForFunction(()=>document.querySelector('.workspace-page')?.innerText.includes('of 2000 lines'));
  const rows=await page.locator('.log-row').count();assert(rows>0&&rows<70,`logs rendered ${rows} rows`);
  await page.getByRole('textbox',{name:'Search logs'}).fill('LOG_UI_ERROR');await page.getByRole('combobox',{name:'Log stream'}).selectOption('stderr');await page.waitForTimeout(500);assert((await page.locator('.log-row').allTextContents()).every(t=>t.includes('stderr')&&t.includes('LOG_UI_ERROR')));
  await page.screenshot({path:'reports/ui-container-logs.jpg',type:'jpeg',quality:35,scale:'css'});
  await page.getByRole('textbox',{name:'Search logs'}).fill('<img');await page.getByRole('combobox',{name:'Log stream'}).selectOption('all');await page.waitForTimeout(500);await page.locator('.log-row').filter({hasText:'<img'}).click();await page.locator('#logLine').waitFor();assert.equal(await page.evaluate(()=>window.logXSS),undefined);await page.locator('#logLine').getByRole('button',{name:'Close',exact:true}).last().click();
  await page.getByRole('textbox',{name:'Search logs'}).fill('');await page.locator('#logs-container').selectOption(tty.id);await page.getByText('TTY: stdout and stderr are merged.',{exact:false}).waitFor();assert(await page.getByRole('combobox',{name:'Log stream'}).isDisabled());
  await page.getByRole('tab',{name:'Manage',exact:true}).click();await page.getByRole('tab',{name:'Logs',exact:true}).click();await page.locator('.log-row').filter({hasText:'LOG_UI_TTY'}).first().waitFor();
  await page.getByRole('link',{name:'Team chat',exact:true}).click();await peer.goto(base+'/#/chat');await page.getByRole('textbox',{name:'Chat message'}).waitFor();
  for(let i=0;i<30;i++){if(await page.getByRole('button',{name:'Send',exact:true}).isEnabled())break;const author=await api(a,'GET','chat');if(author.author)break;await page.waitForTimeout(100)}
  await page.getByRole('textbox',{name:'Chat message'}).fill(run+' <img src=x onerror=window.chatXSS=1>');await page.getByRole('button',{name:'Send',exact:true}).click();await peer.getByText(run+' <img src=x onerror=window.chatXSS=1>',{exact:true}).waitFor();assert.equal(await peer.evaluate(()=>window.chatXSS),undefined);
  await peer.getByRole('textbox',{name:'Chat message'}).fill(run+' ответ');await peer.getByRole('button',{name:'Send',exact:true}).click();await page.getByText(run+' ответ',{exact:true}).waitFor();
  const ca=await api(a,'GET','chat'),cb=await api(b,'GET','chat');assert.notEqual(ca.author,cb.author);assert(ca.messages.some(m=>m.text===run+' ответ'));
  // Several independent browsers share the chat. Seed bounded synthetic history
  // through the API at <= 12 messages/s, below the global posting limit.
  const peers=await Promise.all(Array.from({length:12},()=>browser.newContext(options)));for(const ctx of peers)await api(ctx,'GET','chat');
  for(let batch=0;batch<10;batch++){await Promise.all(peers.map((ctx,i)=>api(ctx,'POST','chat',{text:`${run} участник ${i} сообщение ${batch}`,key:`${run}_${i}_${batch}`})));await page.waitForTimeout(1100)}
  await page.getByRole('button',{name:'Jump to latest messages',exact:true}).click();await page.waitForTimeout(700);assert(await page.getByRole('button',{name:'History',exact:true}).isEnabled());await page.getByRole('button',{name:'History',exact:true}).click();await page.getByText('Browsing history',{exact:true}).waitFor();const chatRows=await page.locator('.chat-message').count();assert(chatRows<60,`chat DOM grew to ${chatRows}`);
  await page.getByRole('button',{name:'Jump to latest messages',exact:true}).click();await page.waitForTimeout(500);await page.screenshot({path:'reports/ui-chat.jpg',type:'jpeg',quality:35,scale:'css'});for(const ctx of peers)await ctx.close();
  // Verify persistence through the existing deployment's app restart.
  execFileSync('ssh',['kali@192.168.56.103','cd ~/gomate-test && docker compose restart app'],{timeout:30000,stdio:['ignore','pipe','pipe']});
  let stored;for(let i=0;i<40;i++){try{stored=await api(a,'GET','chat?before='+ca.messages[ca.messages.length-1].id);break}catch{}await page.waitForTimeout(300)};assert(stored.messages.some(m=>m.text.startsWith(run)));await page.reload();await page.getByRole('textbox',{name:'Chat message'}).waitFor();assert.equal((await api(a,'GET','chat')).author,ca.author);
  assert.deepEqual(errors,[]);const report={passed:true,run,logDOMRows:rows,chatDOMRows:chatRows,anonymousClients:14,chatMessages:122,checks:['start/stop/restart/pause/unpause isolated container','protected gomate denied','bounded virtual log list','stdout/stderr and text filtering','TTY logs','literal HTML in logs and chat','two-way anonymous chat','multiple clients, history pagination','chat and anonymous identity survive restart'],pageErrors:errors};fs.writeFileSync('reports/ui-operations.json',JSON.stringify(report,null,2)+'\n');console.log(JSON.stringify(report));
 }catch(e){await page.screenshot({path:'reports/ui-operations-failure.jpg',type:'jpeg',quality:35,scale:'css'});throw e}finally{await browser.close()}
})().catch(e=>{console.error(e);process.exitCode=1});
