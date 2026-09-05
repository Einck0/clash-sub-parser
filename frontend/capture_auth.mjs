import { chromium } from 'playwright';

async function captureLoggedIn() {
  const browser = await chromium.launch({ headless: true });
  const outDir = '/home/service/hermes/workspace/tmp/visual_audit';
  const viewports = [
    { name: 'mobile-375', width: 375, height: 812 },
    { name: 'desktop-1440', width: 1440, height: 900 }
  ];

  // 1. Capture CSP with auth
  for (const vp of viewports) {
    const context = await browser.newContext({
      viewport: { width: vp.width, height: vp.height },
      deviceScaleFactor: 2
    });
    const page = await context.newPage();
    
    // Login to CSP
    await page.goto('http://127.0.0.1:18080/');
    await page.fill('input', 'aad734c9f87128c81d6e5e3c6c6a2f8258eaf7136623e00c37f0a88fdb3f5bfc');
    await page.click('button:has-text("进入")');
    await page.waitForTimeout(1500);
    
    const cspMainPath = `${outDir}/csp_logged_in_${vp.name}.png`;
    await page.screenshot({ path: cspMainPath, fullPage: true });
    console.log(`Captured: ${cspMainPath}`);

    await context.close();
  }

  // 2. Capture Eindash with auth
  for (const vp of viewports) {
    const context = await browser.newContext({
      viewport: { width: vp.width, height: vp.height },
      deviceScaleFactor: 2
    });
    const page = await context.newPage();
    
    await page.goto('http://127.0.0.1:8600/');
    await page.fill('input[type="password"]', 'einck2026');
    await page.click('button:has-text("进入面板")');
    await page.waitForTimeout(2000);
    
    const eindashMainPath = `${outDir}/eindash_logged_in_${vp.name}.png`;
    await page.screenshot({ path: eindashMainPath, fullPage: true });
    console.log(`Captured: ${eindashMainPath}`);

    await context.close();
  }

  await browser.close();
}

captureLoggedIn().catch(console.error);
