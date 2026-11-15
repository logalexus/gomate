// Mocked chat writes only: retry identity/key, preview, session links and layouts.
const {chromium}=require('../../web/node_modules/playwright');
const fs=require('node:fs'),assert=require('node:assert/strict'),path=require('node:path'),{spawn}=require('node:child_process');
process.chdir(path.resolve(__dirname,'../..'));
(async()=>{
 const base='http://127.0.0.1:4188',server=spawn(process.execPath,['node_modules/vite/bin/vite.js','--host','127.0.0.1','--port','4188','--strictPort'],{cwd:'web',stdio:'ignore'});let browser;
 const report={passed:false,mocked:true},writes=[],calls=[],errors=[];
 try{
  for(let i=0;i<60;i++){try{if((await fetch(base)).ok)break}catch{}await new Promise(r=>setTimeout(r,150))}
  browser=await chromium.launch({headless:true});const page=await browser.newPage({viewport:{width:1366,height:800}});
  page.on('pageerror',e=>errors.push(e.message));await page.routeWebSocket('**/api/v2/events',()=>{});
  await page.route('**/api/**',async route=>{const req=route.request(),u=new URL(req.url());let data=[];
   if(u.pathname==='/api/v2/config')data={requestTimeoutSeconds:10};
   if(u.pathname==='/api/v2/snapshot')data={services:[{port:8070,name:'Documents'}],patterns:[],metrics:{},captureRunning:true};
   if(u.pathname==='/api/v2/chat'){calls.push(req.method());if(req.method()==='POST'){writes.push(req.postDataJSON());if(writes.length===1)return route.fulfill({status:500,json:{error:'fixture failure'}});data={id:1,...writes.at(-1)}}else data={author:'fixture',messages:[],hasMore:false}}
   if(u.pathname.startsWith('/api/packet/'))data=[{id:1,kind:'raw',incoming:true,timestamp:Date.now(),content:Buffer.from('HELLO').toString('base64')}];
   await route.fulfill({json:data});
  });
  await page.goto(base+'/#/8070/42');const trigger=page.getByRole('button',{name:'В общий чат',exact:true});await trigger.click();const dialog=page.getByRole('dialog');
  await dialog.getByLabel('Комментарий (необязательно)').fill('Проверьте повторяющийся запрос <script>alert(1)</script>');
  assert.equal(await dialog.locator('.session-share-preview a').getAttribute('href'),base+'/#/8070/42');assert.equal(await dialog.locator('.session-share-preview script').count(),0);assert.equal(writes.length,0);
  await dialog.getByRole('button',{name:'Отправить в чат',exact:true}).click();await dialog.getByRole('alert').waitFor();assert.equal(await dialog.locator('textarea').inputValue(),'Проверьте повторяющийся запрос <script>alert(1)</script>');
  await dialog.getByRole('button',{name:'Отправить в чат',exact:true}).click();await dialog.getByText('Сессия отправлена в общий чат.',{exact:false}).waitFor();assert.deepEqual(calls,['GET','POST','POST']);assert.deepEqual(writes[0],writes[1]);assert.match(writes[0].key,/^[a-f0-9]{32}$/);assert.equal(await dialog.getByRole('button',{name:'Отправить в чат',exact:true}).count(),0);
  await dialog.getByRole('button',{name:'Закрыть',exact:true}).click();await dialog.waitFor({state:'hidden'});await trigger.focus();
  for(const theme of ['light','dark'])for(const width of [1366,375]){await page.setViewportSize({width,height:812});await page.evaluate(t=>document.body.dataset.theme=t,theme);await trigger.click();await dialog.waitFor();assert(!(await page.evaluate(()=>document.documentElement.scrollWidth>innerWidth+1)));assert(await dialog.evaluate(el=>el.scrollWidth<=el.clientWidth+1));await dialog.locator('textarea').focus();await page.keyboard.press('Escape');await dialog.waitFor({state:'hidden'})}
  await page.goto(base+'/#/8070/43');await trigger.click();await dialog.waitFor();assert.equal(await dialog.locator('.session-share-preview a').getAttribute('href'),base+'/#/8070/43');assert.equal(await dialog.locator('textarea').inputValue(),'');
  assert.deepEqual(errors,[]);Object.assign(report,{passed:true,checks:['explicit send only','absolute session link rendered safely','GET identity before write','same key/text after failed response','draft retained on failure','success prevents duplicate send','session change resets preview','Escape and light/dark 1366/375px'],pageErrors:errors});
 }finally{await browser?.close();server.kill();fs.writeFileSync('reports/session-share-ui.json',JSON.stringify(report,null,2)+'\n')}
 console.log(JSON.stringify(report));
})().catch(e=>{console.error(e);process.exitCode=1});
