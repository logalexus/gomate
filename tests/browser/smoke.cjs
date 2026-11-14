const {chromium} = require('../../web/node_modules/playwright');
const fs = require('node:fs');
const assert = require('node:assert/strict');
const path = require('node:path');
process.chdir(path.resolve(__dirname, '../..'));

(async () => {
  const env = Object.fromEntries(fs.readFileSync(process.env.CREDENTIALS_FILE || '.env', 'utf8').split('\n').filter(l => l && !l.startsWith('#')).map(l => {const i=l.indexOf('=');return [l.slice(0,i),l.slice(i+1)];}));
  const options = {headless: true};
  if (process.env.CHROMIUM_PATH) options.executablePath = process.env.CHROMIUM_PATH;
  const browser = await chromium.launch(options);
  try {
    const context = await browser.newContext({httpCredentials: {username: env.WEB_LOGIN || 'packmate', password: env.WEB_PASSWORD}, viewport: {width: 1440, height: 900}});
    const page = await context.newPage();
    const errors = [];
    page.on('pageerror', e => errors.push(e.message));
    await page.goto(process.env.BASE_URL || 'http://127.0.0.1:16500');
    await page.waitForSelector('.stream-item');
    assert((await page.locator('.stream-item').count()) <= 100);
    await page.locator('.stream-item .nav-link').first().click();
    await page.getByText('Copy HEX', {exact: true}).first().waitFor();
    await page.waitForFunction(() => document.body.innerText.includes('FLAG_'));
    await page.getByRole('button', {name: 'Hex / Text', exact: true}).click();
    await page.waitForFunction(() => document.body.innerText.includes('0000000000:'));
    await page.getByRole('button', {name: 'Hex / Text', exact: true}).click();
    await page.getByRole('button', {name: 'Pause', exact: true}).click();
    await page.getByRole('button', {name: 'Resume', exact: true}).click();
    await page.goto((process.env.BASE_URL || 'http://127.0.0.1:16500') + '/#/' + (process.env.SERVICE_PORT || '18080') + '?pattern=1');
    await page.waitForSelector('.stream-item');
    await page.locator('.stream-item .nav-link').first().click();
    await page.waitForFunction(() => document.body.innerText.includes('FLAG_'));
    await page.screenshot({path: process.env.SCREENSHOT || 'reports/browser-smoke.png'});
    assert.deepEqual(errors, [], `browser exceptions: ${errors.join('; ')}`);
    console.log(JSON.stringify({passed: true, streamRows: await page.locator('.stream-item').count(), pageErrors: errors}));
  } finally {await browser.close();}
})().catch(e => {console.error(e);process.exitCode=1;});
