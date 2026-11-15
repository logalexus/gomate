// UI checks with a synthetic attachment; optional saved API response exercises a
// real captured message without putting credentials or traffic into the repository.
const {chromium}=require('../../web/node_modules/playwright');
const fs=require('node:fs'),assert=require('node:assert/strict'),path=require('node:path'),crypto=require('node:crypto'),{spawn}=require('node:child_process');
process.chdir(path.resolve(__dirname,'../..'));
(async()=>{
 const live=process.argv.includes('--live'),base=live?'http://192.168.56.103:65000':'http://127.0.0.1:4187';
 const server=live?null:spawn(process.execPath,[path.resolve('web/node_modules/vite/bin/vite.js'),'--host','127.0.0.1','--port','4187','--strictPort'],{cwd:process.env.WEB_ROOT||'web',stdio:'ignore'});
 const fixture=process.env.ATTACHMENT_FIXTURE?JSON.parse(fs.readFileSync(process.env.ATTACHMENT_FIXTURE)): [{id:1,kind:'http',incoming:true,content:Buffer.from('POST /upload HTTP/1.1\r\n\r\nPK\x03\x04binary').toString('base64'),preview:'POST /upload HTTP/1.1\r\n\r\nobject: test\nfile: [файл file]',attachments:[{name:'<img src=x onerror=alert(1)>',downloadName:'file',type:'application/zip',size:7,content:Buffer.from('PK\x03\x04abc').toString('base64')}]}];
 let browser;const report={passed:false,live,errors:[],layouts:[]};
 try{
  if(!live)for(let i=0;i<60;i++){try{if((await fetch(base)).ok)break}catch{}await new Promise(r=>setTimeout(r,200))}
  browser=await chromium.launch({headless:true});
  const password=live?fs.readFileSync('.cache/kali.env','utf8').split('\n').find(s=>s.startsWith('WEB_PASSWORD=')).slice(13).replace(/^['"]|['"]$/g,''):'';
  const page=await browser.newPage({viewport:{width:1366,height:768},...(live?{httpCredentials:{username:'packmate',password}}:{})});
  page.on('pageerror',e=>report.errors.push(e.message));await page.routeWebSocket('**/api/v2/events',()=>{});
  let packets=fixture,streamId=206209;
  if(live){const started=Date.now();const r=await page.request.post(base+'/api/packet/206209',{data:{pageSize:25,representation:'decoded'}});assert(r.ok());packets=await r.json();report.apiLatencyMs=Date.now()-started;assert(packets.every(p=>(p.attachments||[]).every(f=>!('text' in f))),'document parsing must be absent')}
  else await page.route('**/api/**',r=>{
   const u=new URL(r.request().url());let data=[];
   if(u.pathname==='/api/v2/config')data={timeoutSeconds:10};
   if(u.pathname==='/api/v2/snapshot')data={services:[{port:8070,name:'Documents'}],patterns:[],metrics:{},captureRunning:true};
   if(u.pathname==='/api/packet/206209')data=packets;
   return r.fulfill({json:data});
  });
  if(live&&!packets.some(p=>p.attachments?.length)){
   const sessions=await (await page.request.post(base+'/api/stream/8070',{data:{pageSize:25,representation:'decoded'}})).json();
   for(const session of sessions){packets=await (await page.request.post(base+'/api/packet/'+session.id,{data:{pageSize:25,representation:'decoded'}})).json();if(packets.some(p=>p.attachments?.length)){streamId=session.id;break}}
  }
  report.streamId=streamId;
  const p=packets.find(p=>p.attachments?.length);assert(p,'no attachment in session');const file=p.attachments[0];
  await page.goto(base+'/#/8070/'+streamId);
  await page.getByLabel('Content',{exact:true}).selectOption('decoded');
  const card=page.locator(`[data-packet-id="${p.id}"]`);await card.locator('.packet-attachment').waitFor();
  assert.equal(await card.locator('.packet-attachment img,.packet-attachment script').count(),0);
  assert.equal(await card.locator('.packet-content').innerText(),p.preview.replace(/\r\n/g,'\n'));
  const [download]=await Promise.all([page.waitForEvent('download'),card.getByRole('button',{name:'Download '+file.downloadName,exact:true}).click()]);
  assert.equal(download.suggestedFilename(),file.downloadName);
  const data=fs.readFileSync(await download.path());assert.deepEqual(data,Buffer.from(file.content,'base64'));
  report.download={name:download.suggestedFilename(),bytes:data.length,sha256:crypto.createHash('sha256').update(data).digest('hex')};
  await card.getByRole('button',{name:'Show raw text',exact:true}).click();assert((await card.locator('.packet-content').innerText()).includes('PK'));
  await card.getByRole('button',{name:'Show form fields',exact:true}).click();
  await page.getByRole('button',{name:'HEX',exact:true}).click();assert(await card.locator('.packet-hex').count());
  await page.getByRole('button',{name:'Text',exact:true}).click();
  for(const theme of ['light','dark'])for(const width of [1366,768,375]){
   await page.evaluate(t=>document.body.setAttribute('data-theme',t),theme);await page.setViewportSize({width,height:812});
   const overflow=await page.evaluate(()=>document.documentElement.scrollWidth>innerWidth+1);assert(!overflow,theme+' '+width+' overflow');report.layouts.push({theme,width,overflow});
  }
  assert.deepEqual(report.errors,[]);report.passed=true;
 }finally{if(browser)await browser.close();if(server)server.kill();fs.writeFileSync('reports/attachments-'+(live?'live':'ui')+'-check.json',JSON.stringify(report,null,2)+'\n')}
 console.log(JSON.stringify(report));
})().catch(e=>{console.error(e);process.exitCode=1});
