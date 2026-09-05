import { chromium } from 'playwright';
import fs from 'fs';

async function capture() {
  const browser = await chromium.launch({ headless: true });
  
  // Viewports to capture
  const viewports = [
    { name: 'mobile-375', width: 375, height: 812 },
    { name: 'tablet-768', width: 768, height: 1024 },
    { name: 'desktop-1440', width: 1440, height: 900 }
  ];

  const outDir = '/home/service/hermes/workspace/tmp/visual_audit';

  // 1. Capture CSP
  for (const vp of viewports) {
    const context = await browser.newContext({
      viewport: { width: vp.width, height: vp.height },
      deviceScaleFactor: 2
    });
    const page = await context.newPage();
    
    // Set localStorage auth for CSP if needed
    await page.goto('http://127.0.0.1:18080/', { waitUntil: 'networkidle' }).catch(() => {});
    await page.evaluate(() => {
      localStorage.setItem('clash_token', 'aad734c9f87128c81d6e5e3c6c6a2f8258eaf7136623e00c37f0a88fdb3f5bfc');
    });
    await page.goto('http://127.0.0.1:18080/', { waitUntil: 'networkidle' });
    await page.waitForTimeout(1000);
    
    const cspPath = `${outDir}/csp_${vp.name}.png`;
    await page.screenshot({ path: cspPath, fullPage: true });
    console.log(`Captured: ${cspPath}`);
    
    // Try click NodeLedger or open a modal
    try {
      const exportBtn = await page.$('button:has-text("快速导出"), button:has-text("导出")');
      if (exportBtn) {
        await exportBtn.click();
        await page.waitForTimeout(500);
        const cspModalPath = `${outDir}/csp_modal_${vp.name}.png`;
        await page.screenshot({ path: cspModalPath });
        console.log(`Captured: ${cspModalPath}`);
      }
    } catch (e) {}

    await context.close();
  }

  // 2. Capture Eindash
  for (const vp of viewports) {
    const context = await browser.newContext({
      viewport: { width: vp.width, height: vp.height },
      deviceScaleFactor: 2
    });
    const page = await context.newPage();
    
    await page.goto('http://127.0.0.1:8600/', { waitUntil: 'networkidle' }).catch(() => {});
    await page.waitForTimeout(1000);
    
    const eindashPath = `${outDir}/eindash_${vp.name}.png`;
    await page.screenshot({ path: eindashPath, fullPage: true });
    console.log(`Captured: ${eindashPath}`);
    
    await context.close();
  }

  await browser.close();
}

capture().catch(console.error);
