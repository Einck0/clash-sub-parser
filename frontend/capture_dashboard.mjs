import { chromium } from 'playwright';

async function captureMainViews() {
  const browser = await chromium.launch({ headless: true });
  const outDir = '/home/service/hermes/workspace/tmp/visual_audit';
  const token = 'aad734c9f87128c81d6e5e3c6c6a2f8258eaf7136623e00c37f0a88fdb3f5bfc';

  // 1. Capture CSP Main Views
  const context = await browser.newContext({
    viewport: { width: 1440, height: 900 },
    deviceScaleFactor: 2
  });
  
  // Set cookie for auth
  await context.addCookies([
    { name: 'clash_auth_token', value: token, domain: '127.0.0.1', path: '/' },
    { name: 'clash_auth_hash', value: token, domain: '127.0.0.1', path: '/' }
  ]);

  const page = await context.newPage();
  
  // Go to CSP
  await page.goto('http://127.0.0.1:18080/');
  await page.evaluate((t) => {
    localStorage.setItem('clash_token', t);
    localStorage.setItem('clash_auth_hash', t);
  }, token);

  // If AuthGate is still visible, fill and submit
  try {
    const input = await page.$('input');
    if (input) {
      await input.fill(token);
      await page.click('button:has-text("进入")');
      await page.waitForTimeout(1000);
    }
  } catch (e) {}

  await page.waitForTimeout(2000);
  await page.screenshot({ path: `${outDir}/csp_dashboard_desktop.png`, fullPage: true });
  console.log('Saved: csp_dashboard_desktop.png');

  // Also capture mobile
  await page.setViewportSize({ width: 375, height: 812 });
  await page.waitForTimeout(1000);
  await page.screenshot({ path: `${outDir}/csp_dashboard_mobile.png`, fullPage: true });
  console.log('Saved: csp_dashboard_mobile.png');

  await browser.close();
}

captureMainViews().catch(console.error);
