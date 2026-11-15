const {chromium}=require('../../web/node_modules/playwright');
const fs=require('node:fs'),assert=require('node:assert/strict'),path=require('node:path');
process.chdir(path.resolve(__dirname,'../..'));
(async()=>{
 const env=Object.fromEntries(fs.readFileSync('.cache/kali.env','utf8').split('\n').filter(l=>l&&!l.startsWith('#')).map(l=>{const n=l.indexOf('=');return[l.slice(0,n),l.slice(n+1)]}));
 const base='http://192.168.56.103:65000',browser=await chromium.launch({headless:true,executablePath:process.env.CHROMIUM_PATH||'/home/logalex/.cache/ms-playwright/chromium-1187/chrome-linux/chrome'});
 const ctx=await browser.newContext({httpCredentials:{username:'packmate',password:env.WEB_PASSWORD},viewport:{width:1440,height:900},reducedMotion:'reduce'}),page=await ctx.newPage(),errors=[],failures=[],checks=[];
 page.on('pageerror',e=>errors.push(e.message));page.on('response',r=>{if(r.url().includes('/api/')&&r.status()>=400)failures.push({url:new URL(r.url()).pathname,status:r.status()})});
 const report={passed:false,checks};
 try{
  await page.goto(base);await page.locator('#traffic-service').waitFor();
  const streams=await (await ctx.request.post(base+'/api/stream/all',{data:{pageSize:10}})).json();const target=streams[0]?.id;
  for(const width of [1440,375]){
   await page.setViewportSize({width,height:width===375?812:900});
   for(const theme of ['dark','light']){
    if(await page.evaluate(()=>document.body.dataset.theme)!==theme)await page.getByRole('button',{name:'Toggle theme',exact:true}).click();
    for(const route of ['stream','containers','chat','anomalies']){
     await page.goto(base+'/#/'+(route==='stream'?(target?'all/'+target:''):route));
     await page.locator('main h1').waitFor();await page.waitForTimeout(350);
     assert.equal(await page.evaluate(()=>document.body.dataset.theme),theme);
     const layout=await page.evaluate(()=>({width:innerWidth,documentWidth:document.documentElement.scrollWidth,appWidth:document.querySelector('.gm-app').scrollWidth,navLinks:[...document.querySelectorAll('.gm-navigation a')].map(a=>({text:a.textContent.trim(),width:a.getBoundingClientRect().width})),main:document.querySelector('main').getBoundingClientRect().width}));
     assert(layout.documentWidth<=width+1,route+' document overflow');assert(layout.appWidth<=width+1,route+' app overflow');assert.equal(layout.navLinks.length,4);assert(layout.main>200);
     if(route==='stream'&&width===1440){const height=await page.locator('.session-list-area').evaluate(el=>el.getBoundingClientRect().height);assert(height>100,'session list clipped');}
     checks.push({route,width,theme,passed:true});
     // Compressed screenshots only; never fullPage or original PNG captures.
     if(width===1440&&(theme==='dark'||route==='stream')){
      await page.setViewportSize({width:1100,height:760});await page.waitForTimeout(150);
      await page.screenshot({path:'reports/redesign-'+route+(theme==='light'?'-light':'')+'.jpg',type:'jpeg',quality:30,scale:'css'});
      await page.setViewportSize({width,height:900});
     }
    }
   }
  }
  await page.setViewportSize({width:1440,height:600});await page.goto(base);await page.locator('.gm-session-sidebar').waitFor();
  const sidebar=await page.locator('.gm-session-sidebar').boundingBox(),footer=await page.locator('.gm-statusbar').boundingBox();assert(sidebar.y+sidebar.height<=footer.y+2,'sidebar overlaps status at short viewport');
  assert.deepEqual(errors,[]);assert.deepEqual(failures,[]);Object.assign(report,{passed:true,pageErrors:errors,apiFailures:failures,shortViewport:true});
 }finally{await browser.close();fs.writeFileSync('reports/ui-shell-redesign.json',JSON.stringify(report,null,2)+'\n')}
 console.log(JSON.stringify(report));
})().catch(e=>{console.error(e);process.exitCode=1});
