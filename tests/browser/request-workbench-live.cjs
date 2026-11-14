// Real Kali replay against this project's own HTTP mock; never execute service data as code.
const {chromium}=require('../../web/node_modules/playwright');
const fs=require('node:fs'),assert=require('node:assert/strict'),path=require('node:path'),http=require('node:http'),{execFileSync}=require('node:child_process');
process.chdir(path.resolve(__dirname,'../..'));
(async()=>{
 const env=Object.fromEntries(fs.readFileSync('.cache/kali.env','utf8').split('\n').filter(l=>l&&!l.startsWith('#')).map(l=>{const n=l.indexOf('=');return[l.slice(0,n),l.slice(n+1)]}));
 const base='http://192.168.56.103:65000',run='workbench_'+Date.now(),since=Date.now()-5000;
 await new Promise((resolve,reject)=>{const req=http.get('http://192.168.56.103:18080/?marker='+run,{agent:false,headers:{Connection:'close'}},r=>{r.resume();r.on('end',()=>r.statusCode===200?resolve():reject(Error('mock HTTP '+r.statusCode)))});req.on('error',reject);req.setTimeout(5000,()=>req.destroy(Error('mock timeout')))});
 const query=`SELECT coalesce(json_agg(t),'[]'::json) FROM (SELECT id,stream_id FROM gomate_packets WHERE incoming AND timestamp_ms>=${since} AND coalesce(details->>'kind','')='' AND position('${run}' in encode(content,'escape'))>0 ORDER BY id LIMIT 1) t;`;
 let record;for(let i=0;i<25&&!record;i++){record=JSON.parse(execFileSync('ssh',['-o','BatchMode=yes','kali@192.168.56.103','docker exec -i gomate-lab-db-1 psql -X -qAt -U gomate -d gomate'],{input:query,encoding:'utf8',timeout:15000}))[0];if(!record)await new Promise(r=>setTimeout(r,400))}assert(record,'fixture not captured');
 const browser=await chromium.launch({headless:true,executablePath:process.env.CHROMIUM_PATH||'/home/logalex/.cache/ms-playwright/chromium-1187/chrome-linux/chrome'}),ctx=await browser.newContext({httpCredentials:{username:'packmate',password:env.WEB_PASSWORD},viewport:{width:1100,height:760},reducedMotion:'reduce'}),page=await ctx.newPage(),errors=[],jobs=[];
 page.on('pageerror',e=>errors.push(e.message));const report={passed:false,run,streamID:record.stream_id,packetID:record.id,jobs,checks:[]};
 try{
  const capabilities=await (await ctx.request.get(base+'/api/v2/runner')).json();assert(capabilities.available,'runner unavailable');
  await page.goto(base+'/#/all/'+record.stream_id);const packet=page.locator('[data-packet-id="'+record.id+'"]');await packet.waitFor();await packet.getByRole('button',{name:'Копировать',exact:true}).click();await page.getByRole('menuitem',{name:'Python requests…',exact:true}).click();
  const dialog=page.getByRole('dialog'),editor=page.getByLabel('Редактируемый скрипт');await page.waitForFunction(()=>document.querySelector('.script-editor')?.__vue__?.value.includes('import requests'));
  assert.equal(await page.getByLabel('Адрес сервиса',{exact:true}).inputValue(),'192.168.56.103');assert.equal(await page.getByLabel('Порт',{exact:true}).inputValue(),'18080');
  async function start(){const response=page.waitForResponse(r=>r.request().method()==='POST'&&r.url().endsWith('/api/v2/runner/jobs'));await dialog.getByRole('button',{name:'Запустить',exact:true}).click();const res=await response;assert.equal(res.status(),202);const job=await res.json();jobs.push(job.id);return job.id}
  async function complete(id,status='finished'){let result;for(let i=0;i<180;i++){result=await (await ctx.request.get(base+'/api/v2/runner/jobs/'+id)).json();if(!['starting','running','stopping'].includes(result.status))break;await page.waitForTimeout(200)}assert.equal(result.status,status,JSON.stringify(result));return result}
  await editor.fill((await editor.evaluate(el=>el.closest('.script-editor').__vue__.value))+'\nprint("edited_'+run+'")\n');
  await page.evaluate(()=>{const original=document.execCommand;document.execCommand=function(command,...args){if(command==='copy')window.copiedWorkbench=document.activeElement.value;return original.call(this,command,...args)}});
  await dialog.getByRole('button',{name:'Копировать скрипт',exact:true}).click();assert.equal(await page.evaluate(()=>window.copiedWorkbench),await editor.evaluate(el=>el.closest('.script-editor').__vue__.value));
  let result=await complete(await start());assert.equal(result.exitCode,0);assert(result.stdout.includes(run)&&result.stdout.includes('edited_'+run));await dialog.locator('#runner-stdout').filter({hasText:'edited_'+run}).waitFor();report.checks.push('edited generated requests executes from Kali, copy contains exact edits');
  for(const format of ['curl','pwntools']){
   await page.getByLabel('Шаблон',{exact:true}).selectOption(format);await dialog.getByRole('button',{name:'Сгенерировать заново',exact:true}).click();await dialog.getByRole('button',{name:'Запустить',exact:true}).waitFor();result=await complete(await start());assert.equal(result.exitCode,0,JSON.stringify(result));assert(result.stdout.includes(run),JSON.stringify(result));await page.waitForFunction(()=>document.querySelector('.workbench-result-heading')?.textContent.includes('exit 0'));report.checks.push(format+' generated script reaches real HTTP mock');
  }
  await page.waitForFunction(()=>document.querySelectorAll('.cm-line span[class]').length>3);
  report.layout=await dialog.evaluate(el=>Object.fromEntries(['.workbench-editor-pane','.workbench-result','.cm-editor'].map(selector=>[selector,el.querySelector(selector).getBoundingClientRect().toJSON()])));
  assert(report.layout['.workbench-result'].x>report.layout['.workbench-editor-pane'].x+report.layout['.workbench-editor-pane'].width);
  assert(report.layout['.cm-editor'].height>350);report.checks.push('syntax highlighting and expanded side-by-side editor on Kali');
  await page.screenshot({path:'reports/request-workbench-live.jpg',type:'jpeg',quality:30,scale:'css'});
  await editor.fill('import sys\nprint("DEBUG_STDOUT")\nprint("DEBUG_STDERR",file=sys.stderr)\nraise SystemExit(7)');await page.getByLabel('Язык выполнения',{exact:true}).selectOption('python');result=await complete(await start());assert.equal(result.exitCode,7);assert(result.stdout.includes('DEBUG_STDOUT')&&result.stderr.includes('DEBUG_STDERR'));await dialog.locator('#runner-stderr').waitFor();report.checks.push('arbitrary edited script, stderr and nonzero exit');
  await editor.fill('import time\nprint("TIMEOUT",flush=True)\ntime.sleep(60)');await page.getByLabel('Таймаут, с',{exact:true}).fill('2');await complete(await start(),'timed_out');await dialog.getByText('Таймаут',{exact:true}).waitFor();report.checks.push('server timeout in UI');
  await page.getByLabel('Таймаут, с',{exact:true}).fill('30');await editor.fill('import time\nprint("STOP_READY",flush=True)\ntime.sleep(30)');const stopID=await start();await dialog.locator('#runner-stdout').filter({hasText:'STOP_READY'}).waitFor();await dialog.getByRole('button',{name:'Остановить',exact:true}).click();await complete(stopID,'canceled');await dialog.getByText('Остановлено',{exact:true}).waitFor();report.checks.push('Stop removes running container');
  await page.keyboard.press('Escape');await dialog.waitFor({state:'hidden'});assert.deepEqual(errors,[]);report.pageErrors=errors;report.passed=true;
 }finally{
  for(const id of jobs)await ctx.request.delete(base+'/api/v2/runner/jobs/'+id).catch(()=>{});
  await browser.close();fs.writeFileSync('reports/request-workbench-live.json',JSON.stringify(report,null,2)+'\n');
 }
 console.log(JSON.stringify(report));
})().catch(e=>{console.error(e);process.exitCode=1});
