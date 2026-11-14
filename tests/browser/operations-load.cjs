const {chromium}=require('../../web/node_modules/playwright');
const fs=require('node:fs'),assert=require('node:assert/strict'),{spawn}=require('node:child_process'),path=require('node:path');
process.chdir(path.resolve(__dirname,'../..'));
(async()=>{
 const env=Object.fromEntries(fs.readFileSync('.cache/kali.env','utf8').split('\n').filter(l=>l&&!l.startsWith('#')).map(l=>{const n=l.indexOf('=');return[l.slice(0,n),l.slice(n+1)]}));
 const base='http://192.168.56.103:65000',browser=await chromium.launch({headless:true,executablePath:process.env.CHROMIUM_PATH||'/home/logalex/.cache/ms-playwright/chromium-1187/chrome-linux/chrome'}),contexts=[],pages=[],errors=[],httpErrors=[];let logsResponses=0;
 try{
  for(let i=0;i<8;i++){
   const ctx=await browser.newContext({httpCredentials:{username:'packmate',password:env.WEB_PASSWORD},viewport:{width:1536,height:1000}});contexts.push(ctx);const p=await ctx.newPage();pages.push(p);p.on('pageerror',e=>errors.push(e.message));p.on('response',r=>{if(r.url().includes('/api/v2/containers/')&&r.url().includes('/logs')){logsResponses++;if(r.status()!==200)httpErrors.push(r.status())}});
   await p.goto(base+(i<4?'/#/containers':'/#/chat'));
   if(i<4){await p.locator('tr[data-container="gomate-ui-ops"]').getByRole('button',{name:'Логи',exact:true}).click();await p.locator('#logs-tail').selectOption('2000');await p.locator('.log-row').first().waitFor()}
  }
  await pages[0].waitForTimeout(5000); // settle initial rendering before timed traffic
  const result=await new Promise((resolve,reject)=>{let output='';const proc=spawn('python3',['scripts/remote-bench.py','--count','100','--rps','10','--tag','operations-smoke'],{stdio:['ignore','pipe','pipe']});proc.stdout.on('data',b=>output+=b);proc.stderr.on('data',b=>output+=b);proc.on('error',reject);proc.on('exit',code=>resolve({code,output}))});
  const rowCounts=await Promise.all(pages.slice(0,4).map(p=>p.locator('.log-row').count()));assert(rowCounts.every(n=>n>0&&n<70));assert.deepEqual(errors,[]);assert.deepEqual(httpErrors,[]);
  await pages[0].screenshot({path:'reports/ui-container-logs.jpg',type:'jpeg',quality:35,scale:'css'});await pages[4].screenshot({path:'reports/ui-chat.jpg',type:'jpeg',quality:35,scale:'css'});await pages[0].getByRole('tab',{name:'Управление',exact:true}).click();await pages[0].screenshot({path:'reports/ui-containers.jpg',type:'jpeg',quality:35,scale:'css'});
  let lateLogs=0;pages[0].on('request',r=>{if(r.url().includes('/logs?'))lateLogs++});await pages[0].getByRole('link',{name:'Чат',exact:true}).click();await pages[0].waitForTimeout(4000);assert.equal(lateLogs,0,'log polling continued after leaving');
  const report={passed:result.code===0,browserClients:8,logViewers:4,chatViewers:4,logResponses:logsResponses,logHTTPFailures:httpErrors,logDOMRows:rowCounts,pageErrors:errors,traffic:JSON.parse(result.output.trim()),logPollingStoppedAfterLeaving:true};fs.writeFileSync('reports/ui-operations-load.json',JSON.stringify(report,null,2)+'\n');console.log(JSON.stringify(report));assert.equal(result.code,0,'traffic generator did not fully serve scheduled load');
 }finally{await browser.close()}
})().catch(e=>{console.error(e);process.exitCode=1});
