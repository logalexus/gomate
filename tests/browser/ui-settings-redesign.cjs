const {chromium}=require('../../web/node_modules/playwright');
const fs=require('node:fs'),assert=require('node:assert/strict'),path=require('node:path');
process.chdir(path.resolve(__dirname,'../..'));
(async()=>{
 const env=Object.fromEntries(fs.readFileSync('.cache/kali.env','utf8').split('\n').filter(l=>l&&!l.startsWith('#')).map(l=>{const n=l.indexOf('=');return[l.slice(0,n),l.slice(n+1)]}));
 const base='http://192.168.56.103:65000',errors=[],blockedMutations=[];
 const browser=await chromium.launch({headless:true,executablePath:process.env.CHROMIUM_PATH||'/home/logalex/.cache/ms-playwright/chromium-1187/chrome-linux/chrome'});
 const report={passed:false,readOnly:true,viewports:[],pageErrors:errors,blockedMutations};
 try{
  for(const width of [1366,375]){
   const context=await browser.newContext({httpCredentials:{username:'packmate',password:env.WEB_PASSWORD},viewport:{width,height:900},reducedMotion:'reduce'});
   // Fail closed if a selector ever accidentally activates a saving action.
   await context.route('**/api/**',async route=>{
    const request=route.request(),method=request.method(),pathname=new URL(request.url()).pathname;
    const readPost=method==='POST'&&(/^\/api\/(stream|packet)\//.test(pathname)||pathname==='/api/v2/pattern-test');
    if(!['GET','HEAD','OPTIONS'].includes(method)&&!readPost){blockedMutations.push(method+' '+pathname);await route.abort();return}
    await route.continue();
   });
   const page=await context.newPage();page.on('pageerror',e=>errors.push(e.message));
   await page.goto(base+'/#/');
   const viewport={width,checks:[]};report.viewports.push(viewport);
   const modalFits=async id=>{const dimensions=await page.locator(id).evaluate(el=>{const content=el.querySelector('.modal-content');return{viewport:innerWidth,outer:el.scrollWidth,content:content?.scrollWidth,client:content?.clientWidth}});assert(dimensions.outer<=dimensions.viewport+1,`${id} overflows ${width}px viewport`);assert(dimensions.content<=dimensions.client+1,`${id} content overflows ${width}px`)};
   const closeKeyboard=async id=>{const modal=page.locator(id);await modal.getByRole('button',{name:/^Close$/}).first().focus();await page.keyboard.press('Escape');await modal.waitFor({state:'hidden'})};

   await page.getByRole('button',{name:'Settings',exact:true}).click();
   const settings=page.locator('#settingsModal');await settings.locator('#capture-interface').waitFor();
   const choices=await settings.locator('#capture-interface option').allTextContents();assert(choices.some(v=>v.includes('eth1')));viewport.interfaceChoices=choices.length;
   assert(await settings.getByLabel('Host network interface',{exact:true}).isVisible());
   await modalFits('#settingsModal');
   await settings.getByRole('tab',{name:'Retention and cleanup',exact:true}).click();
   for(const id of ['packet-retention','session-retention','cleanup-interval','cleanup-mode','cleanup-service'])assert(await settings.locator('#'+id).isVisible());
   assert(await settings.getByRole('button',{name:'Delete…',exact:true}).isDisabled());
   await modalFits('#settingsModal');
   await settings.getByRole('tab',{name:'Display',exact:true}).click();
   for(const id of ['settings-pageSize','settings-hexdumpBlockSize','settings-hex-base'])assert(await settings.locator('#'+id).isVisible());
   await modalFits('#settingsModal');await closeKeyboard('#settingsModal');viewport.checks.push('settings NIC/storage/display labels and disabled deletion');

   await page.getByRole('button',{name:/^Patterns/}).click();
   const manager=page.locator('#patternsManager');await manager.getByRole('textbox',{name:'Search patterns',exact:true}).waitFor();
   await manager.getByRole('textbox',{name:'Search patterns',exact:true}).fill('ui_readonly_no_matching_pattern_74603819');
   await manager.locator('.pattern-manager-empty').waitFor();
   await manager.getByRole('combobox',{name:'Filter by state',exact:true}).selectOption('disabled');
   await manager.getByRole('button',{name:'Reset pattern filters',exact:true}).click();
   assert.equal(await manager.getByRole('textbox',{name:'Search patterns',exact:true}).inputValue(),'');
   assert.equal(await manager.getByRole('combobox',{name:'Filter by state',exact:true}).inputValue(),'all');
   await modalFits('#patternsManager');viewport.checks.push('pattern search, empty state and filter reset');
   await manager.getByRole('button',{name:'+ New pattern',exact:true}).click();
   const editor=page.locator('#patternModal');await editor.locator('#pattern-name').fill('Readonly UI sample');await editor.locator('#pattern-value').fill('GM_UI_SAMPLE');
   await editor.getByRole('textbox',{name:'Test fragment',exact:true}).fill('before GM_UI_SAMPLE after');
   await editor.getByRole('button',{name:'Test',exact:true}).click();
   await editor.getByText('Matches: 1',{exact:true}).waitFor();assert.equal(await editor.locator('mark').textContent(),'GM_UI_SAMPLE');
   await modalFits('#patternModal');await closeKeyboard('#patternModal');viewport.checks.push('draft-only pattern tester and nested modal keyboard close');
   await closeKeyboard('#patternsManager');

   const filters=page.getByRole('region',{name:'Session filters',exact:true});
   if(await filters.getByRole('button',{name:'Configure filters',exact:true}).getAttribute('aria-expanded')==='false')await filters.getByRole('button',{name:'Configure filters',exact:true}).click();
   await filters.getByRole('button',{name:'Add service',exact:true}).click();
   const service=page.locator('#serviceModal');await service.getByRole('textbox',{name:'Search Docker services',exact:true}).waitFor();
   await service.getByRole('textbox',{name:'Search Docker services',exact:true}).fill('ui_no_matching_container_74603819');
   await service.locator('.service-empty').waitFor();await modalFits('#serviceModal');
   await service.getByRole('tab',{name:'Manual',exact:true}).click();
   assert(await service.getByLabel('Name',{exact:true}).isVisible());assert(await service.getByLabel('Port at capture point',{exact:true}).isVisible());
   assert.equal(await service.getByRole('checkbox').count(),3);await modalFits('#serviceModal');await closeKeyboard('#serviceModal');
   viewport.checks.push('Docker picker search and manual service form without saving');
   await context.close();
  }
  assert.deepEqual(errors,[]);assert.deepEqual(blockedMutations,[]);report.passed=true;
 }finally{await browser.close();fs.writeFileSync('reports/ui-settings-redesign.json',JSON.stringify(report,null,2)+'\n')}
 console.log(JSON.stringify(report));
})().catch(e=>{console.error(e);process.exitCode=1});
