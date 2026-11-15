// UI fixtures only. Generated scripts are never executed against a live service.
const {chromium}=require('../../web/node_modules/playwright');
const assert=require('node:assert/strict'),path=require('node:path'),{spawn}=require('node:child_process');
process.chdir(path.resolve(__dirname,'../..'));
(async()=>{
 const base='http://127.0.0.1:4178',server=spawn(process.execPath,['node_modules/vite/bin/vite.js','--host','127.0.0.1','--port','4178','--strictPort'],{cwd:'web',stdio:'ignore'});let browser;
 const delay=ms=>new Promise(resolve=>setTimeout(resolve,ms));
 try{
  for(let i=0;i<60;i++){try{if((await fetch(base)).ok)break}catch{}await delay(150)}
  browser=await chromium.launch({headless:true,executablePath:process.env.CHROMIUM_PATH||'/home/logalex/.cache/ms-playwright/chromium-1187/chrome-linux/chrome'});
  const page=await browser.newPage({viewport:{width:1100,height:760},reducedMotion:'reduce'}),errors=[],pages=[];let purged=false,stall=false,release,submitted=0;
  page.on('pageerror',e=>errors.push(e.message));await page.routeWebSocket('**/api/v2/events',()=>{});
  const packets=Array.from({length:102},(_,i)=>({id:i+1,incoming:i%2===0,timestamp:100+i,matches:[],content:Buffer.from(i%2===0?`GET /step/${i/2} HTTP/1.1\r\nHost: shop\r\n\r\n`:'HTTP/1.1 200 OK\r\nContent-Length: 2\r\n\r\nok').toString('base64')}));
  const stream={id:7,service:18080,server:'192.0.2.1:18080',client:'192.0.2.2:50000',protocol:'TCP',startTimestamp:1,endTimestamp:102,sizeBytes:10000,packetsCount:204};
  await page.route('**/api/**',async route=>{
   const request=route.request(),url=new URL(request.url());let data=[],headers={};
   if(url.pathname==='/api/v2/snapshot')data={services:[{port:18080,name:'Shop',http:true}],patterns:[],metrics:{},captureRunning:true,capture:{interface:'eth1'}};
   else if(url.pathname==='/api/v2/config')data={timeoutSeconds:10};
   else if(url.pathname==='/api/stream/7'&&request.method()==='GET')data={...stream,payloadPurged:purged};
   else if(url.pathname.startsWith('/api/stream/'))data=[stream];
   else if(url.pathname==='/api/packet/7'){
    const body=request.postDataJSON();if(body.representation==='raw'){pages.push(body);if(stall){stall=false;await new Promise(resolve=>release=resolve)}}
    const filtered=packets.filter(p=>p.id>(body.startingFrom||0));data=filtered.slice(0,body.pageSize);headers={'X-Has-More':String(filtered.length>body.pageSize)};
   }else if(url.pathname==='/api/v2/runner')data={available:false};
   else if(url.pathname==='/api/v2/runner/jobs'){submitted++;data={};}
   await route.fulfill({json:data,headers}).catch(()=>{});
  });
  await page.goto(base+'/#/all/7');await page.locator('.gm-packet').first().waitFor();
  const button=page.getByRole('button',{name:'Скрипт сессии',exact:true});await button.click();
  const dialog=page.getByRole('dialog'),editor=page.getByLabel('Редактируемый скрипт');
  const value=()=>editor.evaluate(el=>el.closest('.script-editor').__vue__.value);
  await dialog.getByText('51 HTTP-запросов',{exact:false}).waitFor();
  assert.deepEqual(pages.map(p=>p.startingFrom),[null,50,100]);assert((await value()).includes('"/step/50"'));assert.equal((await value()).match(/"\/step\//g).length,51);assert.equal(submitted,0);
  assert.equal(await page.getByLabel('Адрес сервиса',{exact:true}).inputValue(),'192.0.2.1');
  await page.getByLabel('Шаблон',{exact:true}).selectOption('tcp');assert((await value()).includes('socket.create_connection'));assert.equal((await value()).match(/connection.sendall/g).length,51);
  await editor.fill('# manual session edit');await page.waitForFunction(()=>document.querySelector('.script-editor')?.__vue__?.value==='# manual session edit');await page.getByLabel('Адрес сервиса',{exact:true}).fill('192.0.2.8');assert.equal(await value(),'# manual session edit');await dialog.getByRole('button',{name:'Сгенерировать заново',exact:true}).click();assert((await value()).includes('192.0.2.8'));
  await page.evaluate(()=>Object.defineProperty(navigator,'clipboard',{value:{writeText:async text=>window.copiedScript=text},configurable:true}));await dialog.getByRole('button',{name:'Копировать скрипт',exact:true}).click();assert.equal(await page.evaluate(()=>window.copiedScript),await value());
  await page.keyboard.press('Escape');await dialog.waitFor({state:'hidden'});
  // Retention cannot leave an old valid-looking script in the editor.
  purged=true;await button.click();await dialog.getByRole('alert').filter({hasText:'очищено'}).waitFor();assert.equal(await value(),'');assert(await dialog.getByRole('button',{name:'Копировать скрипт',exact:true}).isDisabled());
  purged=false;await dialog.getByRole('button',{name:'Повторить загрузку',exact:true}).click();await dialog.getByText('51 HTTP-запросов',{exact:false}).waitFor();assert((await value()).includes('requests.Session'));
  await page.keyboard.press('Escape');await dialog.waitFor({state:'hidden'});
  stall=true;await button.click();await page.waitForFunction(()=>document.querySelector('.request-workbench')?.textContent.includes('Загружаем всю сессию'));
  for(let i=0;i<20&&!release;i++)await delay(50);assert(release);await page.keyboard.press('Escape');await dialog.waitFor({state:'hidden'});const previous=pages.length;release();await delay(300);assert.equal(pages.length,previous,'closing cancels pagination');
  await page.setViewportSize({width:375,height:640});await button.click();await dialog.getByText('51 HTTP-запросов',{exact:false}).waitFor();assert(await dialog.evaluate(el=>el.scrollWidth<=el.clientWidth+1));assert((await value()).includes('"/step/50"'));assert.deepEqual(errors,[]);assert.equal(submitted,0);
  console.log(JSON.stringify({passed:true,checks:['all 3 raw pages','51 HTTP requests without decoded duplicates','TCP full dialogue','manual edits and regeneration','copy without runner','retention and retry','cancel pending page on close','375px layout','no automatic execution'],pageErrors:errors}));
 }finally{await browser?.close();server.kill('SIGTERM')}
})().catch(e=>{console.error(e);process.exitCode=1});
