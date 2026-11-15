const {chromium}=require('../../web/node_modules/playwright');
const fs=require('node:fs'),assert=require('node:assert/strict'),path=require('node:path');
process.chdir(path.resolve(__dirname,'../..'));
(async()=>{
 const env=Object.fromEntries(fs.readFileSync('.cache/kali.env','utf8').split('\n').filter(l=>l&&!l.startsWith('#')).map(l=>{const n=l.indexOf('=');return[l.slice(0,n),l.slice(n+1)]}));
 const browser=await chromium.launch({headless:true,executablePath:process.env.CHROMIUM_PATH||'/home/logalex/.cache/ms-playwright/chromium-1187/chrome-linux/chrome'}),base='http://192.168.56.103:65000';
 const ctx=await browser.newContext({httpCredentials:{username:'packmate',password:env.WEB_PASSWORD},viewport:{width:1200,height:800},reducedMotion:'reduce'}),page=await ctx.newPage(),errors=[];page.on('pageerror',e=>errors.push(e.message));
 const report={passed:false};
 try{
  await page.goto(base+'/#/containers');await page.getByRole('textbox',{name:'Search containers'}).fill('gomate-ui-ops');
  const row=page.locator('tr[data-container="gomate-ui-ops"]');await row.waitFor();await row.getByRole('button',{name:'Restart',exact:true}).click();const restarted=page.waitForResponse(r=>r.url().endsWith('/restart')&&r.request().method()==='POST');await page.getByRole('dialog').getByRole('button',{name:'Restart',exact:true}).click();assert((await restarted).ok());await page.getByRole('dialog').waitFor({state:'hidden'});await page.waitForFunction(()=>document.querySelector('tr[data-container="gomate-ui-ops"]')?.textContent.includes('Running'));
  await row.getByRole('button',{name:'Logs',exact:true}).click();await page.locator('#logs-tail').selectOption('2000');await page.locator('.log-row').first().waitFor();
  const count=await page.locator('.log-row').count();assert(count>0&&count<70);
  await page.getByRole('textbox',{name:'Search logs'}).fill('LOG_UI_ERROR');await page.getByRole('combobox',{name:'Log stream'}).selectOption('stderr');await page.waitForFunction(()=>{const rows=[...document.querySelectorAll('.log-row')];return rows.length>0&&rows.every(r=>r.textContent.includes('LOG_UI_ERROR'))});
  await page.locator('.log-row').first().click();await page.locator('#logLine').waitFor();await page.locator('#logLine').getByRole('button',{name:'Close',exact:true}).last().click();await page.locator('#logLine').waitFor({state:'hidden'});
  await page.getByRole('link',{name:'Team chat',exact:true}).click();let logRequests=0;page.on('request',r=>{if(r.url().includes('/logs?'))logRequests++});await page.waitForTimeout(2500);assert.equal(logRequests,0);assert.deepEqual(errors,[]);Object.assign(report,{passed:true,logDOMRows:count,pageErrors:errors,checks:['restart isolated fixture with confirmation','real Docker log API','bounded virtual rows','stderr/text filter','line detail','polling stops on navigation']});
 }finally{await browser.close();fs.writeFileSync('reports/ui-container-live-redesign.json',JSON.stringify(report,null,2)+'\n')}
 console.log(JSON.stringify(report));
})().catch(e=>{console.error(e);process.exitCode=1});
