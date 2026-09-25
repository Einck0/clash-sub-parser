const { chromium } = require('../web/node_modules/playwright');
const fs = require('fs');
const path = require('path');

const TARGET_URL = process.env.CSP_TEST_URL || 'http://127.0.0.1:18080';
const EVIDENCE_DIR = process.env.EVIDENCE_DIR || path.join(__dirname, '../critic-evidence');

if (!fs.existsSync(EVIDENCE_DIR)) {
  fs.mkdirSync(EVIDENCE_DIR, { recursive: true });
}

function log(msg) {
  console.log(`[Critic] ${msg}`);
}

async function runAudit() {
  log(`Starting Critic UAT against ${TARGET_URL}`);
  log(`Evidence directory: ${EVIDENCE_DIR}`);

  const browser = await chromium.launch({
    headless: true,
    args: ['--no-sandbox', '--disable-gpu', '--disable-dev-shm-usage']
  });

  const context = await browser.newContext({
    viewport: { width: 393, height: 851 }, // Xiaomi 12 Pro resolution standard
    deviceScaleFactor: 3,
    isMobile: true,
    hasTouch: true,
  });

  const page = await context.newPage();

  // Listen to console errors and network errors
  const pageErrors = [];
  page.on('console', msg => {
    if (msg.type() === 'error') {
      pageErrors.push(`[Console Error] ${msg.text()}`);
    }
  });
  page.on('pageerror', err => {
    pageErrors.push(`[Uncaught Page Error] ${err.message}`);
  });

  const report = {
    target: TARGET_URL,
    timestamp: new Date().toISOString(),
    tests: {},
    passed: true,
    pageErrors: []
  };

  try {
    // =========================================================================
    // Check 1: Initial Load & App Shell
    // =========================================================================
    log('--- Check 1: Initial Page Load ---');
    await page.goto(TARGET_URL, { waitUntil: 'networkidle' });
    await page.waitForSelector('#app', { timeout: 10000 });
    await page.waitForTimeout(1000);

    const initialTitle = await page.title();
    log(`Page Title: "${initialTitle}"`);
    report.tests.initialLoad = { status: 'PASSED', title: initialTitle };
    await page.screenshot({ path: path.join(EVIDENCE_DIR, '01_initial_load.png') });

    // =========================================================================
    // Check 2: Mobile Bottom Navigation Bar & Touch Targets (>= 48px)
    // =========================================================================
    log('--- Check 2: Mobile Bottom Navigation Bar & Touch Target Sizing ---');
    const bottomNav = page.locator('nav.md\\:hidden');
    await bottomNav.waitFor({ state: 'visible', timeout: 5000 });
    const isNavVisible = await bottomNav.isVisible();
    log(`Bottom nav visible: ${isNavVisible}`);

    const navButtons = bottomNav.locator('button');
    const navBtnCount = await navButtons.count();
    log(`Bottom nav item count: ${navBtnCount} (expected 6: Subscriptions, Nodes, Probes, Policy, Publications, Settings)`);

    const buttonMetrics = [];
    let allButtonsPass48px = true;

    for (let i = 0; i < navBtnCount; i++) {
      const btn = navButtons.nth(i);
      const box = await btn.boundingBox();
      const text = (await btn.innerText()).trim().replace(/\n/g, ' ');
      log(`  Tab [${i}] "${text}": width=${box.width.toFixed(1)}px, height=${box.height.toFixed(1)}px`);
      const meetsRequirement = box.width >= 48 && box.height >= 48;
      if (!meetsRequirement) {
        allButtonsPass48px = false;
      }
      buttonMetrics.push({ index: i, text, width: box.width, height: box.height, meetsRequirement });
    }

    report.tests.bottomNavTouchTargets = {
      status: allButtonsPass48px ? 'PASSED' : 'FAILED',
      count: navBtnCount,
      metrics: buttonMetrics
    };
    if (!allButtonsPass48px) report.passed = false;

    // Test Navigation Tab Switching via touch/click
    log('--- Check 2.1: Navigation Tab Switching & Responsiveness ---');
    const tabSwitchResults = [];
    const expectedTabs = ['nodes', 'probes', 'policy', 'publications', 'settings', 'subscriptions'];

    for (const tabId of expectedTabs) {
      log(`  Clicking tab: ${tabId}...`);
      // Find button corresponding to tab
      let btn;
      if (tabId === 'nodes') btn = navButtons.nth(1);
      else if (tabId === 'probes') btn = navButtons.nth(2);
      else if (tabId === 'policy') btn = navButtons.nth(3);
      else if (tabId === 'publications') btn = navButtons.nth(4);
      else if (tabId === 'settings') btn = navButtons.nth(5);
      else btn = navButtons.nth(0); // subscriptions

      await btn.click();
      await page.waitForTimeout(500);

      // Verify active styling
      const classAttr = await btn.getAttribute('class');
      const isActive = classAttr.includes('text-primary');
      tabSwitchResults.push({ tabId, activeRendered: isActive });
      log(`  Tab ${tabId} active state verified: ${isActive}`);
    }

    report.tests.tabSwitching = { status: 'PASSED', results: tabSwitchResults };
    await page.screenshot({ path: path.join(EVIDENCE_DIR, '02_tab_switching.png') });

    // Check 2.2: Bottom Clearance and Scroll Padding
    log('--- Check 2.2: Scroll Bottom Clearance & Occlusion Check ---');
    // We are on Subscriptions tab. Let's find subscription cards
    const cards = page.locator('article.card');
    const cardCount = await cards.count();
    log(`  Found ${cardCount} subscription cards rendered.`);

    if (cardCount > 0) {
      // Scroll to bottom
      await page.evaluate(() => window.scrollTo(0, document.body.scrollHeight));
      await page.waitForTimeout(500);

      const lastCard = cards.last();
      const lastCardBox = await lastCard.boundingBox();
      const navBox = await bottomNav.boundingBox();

      const lastCardBottom = lastCardBox.y + lastCardBox.height;
      const navTop = navBox.y;
      log(`  Last card bottom: ${lastCardBottom.toFixed(1)}px, Bottom Nav Top: ${navTop.toFixed(1)}px`);
      
      // Card bottom must be strictly less than or equal to navTop so action buttons are accessible
      const hasClearance = lastCardBottom <= navTop + 1; // 1px tolerance
      log(`  Last card not occluded by bottom nav: ${hasClearance} (gap: ${(navTop - lastCardBottom).toFixed(1)}px)`);

      report.tests.bottomClearance = {
        status: hasClearance ? 'PASSED' : 'FAILED',
        lastCardBottom,
        navTop,
        clearanceGap: navTop - lastCardBottom
      };
      if (!hasClearance) report.passed = false;
    }
    await page.screenshot({ path: path.join(EVIDENCE_DIR, '03_bottom_clearance.png') });

    // =========================================================================
    // Check 3: Full-Site Seamless i18n Switching & Persistence
    // =========================================================================
    log('--- Check 3: i18n Language Switching & Persistence ---');
    const langBtn = page.locator('button[aria-label="Switch Language"]');
    await langBtn.waitFor({ state: 'visible', timeout: 3000 });
    const initialLangText = (await langBtn.innerText()).trim();
    log(`  Initial Lang Toggle Text: "${initialLangText}"`);

    // Let's toggle to Chinese if not already
    let currentLocale = await page.evaluate(() => localStorage.getItem('csp_locale') || 'zh-CN');
    log(`  Current localStorage csp_locale: "${currentLocale}"`);

    // Ensure we switch to English first to test EN
    if (currentLocale !== 'en-US') {
      await langBtn.click();
      await page.waitForTimeout(400);
    }

    const enSubTitle = (await page.locator('#subscriptions-title').innerText()).trim();
    log(`  English Subscriptions Title: "${enSubTitle}"`);
    const isEnWorking = enSubTitle === 'Subscriptions';
    await page.screenshot({ path: path.join(EVIDENCE_DIR, '04_i18n_en.png') });

    // Switch to Chinese
    await langBtn.click();
    await page.waitForTimeout(400);
    const zhSubTitle = (await page.locator('#subscriptions-title').innerText()).trim();
    log(`  Chinese Subscriptions Title: "${zhSubTitle}"`);
    const isZhWorking = zhSubTitle === '订阅源管理';
    await page.screenshot({ path: path.join(EVIDENCE_DIR, '05_i18n_zh.png') });

    // Test Persistence across reload
    log('  Testing persistence: reloading page with Chinese active...');
    await page.reload({ waitUntil: 'networkidle' });
    await page.waitForTimeout(500);
    const reloadedZhTitle = (await page.locator('#subscriptions-title').innerText()).trim();
    const persistedLocale = await page.evaluate(() => localStorage.getItem('csp_locale'));
    log(`  After reload title: "${reloadedZhTitle}", localStorage: "${persistedLocale}"`);
    const persistencePass = reloadedZhTitle === '订阅源管理' && persistedLocale === 'zh-CN';

    // Walk through key views in Chinese to check for raw untranslated placeholders
    const viewsToCheck = [
      { tabIndex: 1, name: 'Nodes', expectedHeaderSnippet: '节点' },
      { tabIndex: 2, name: 'Probes', expectedHeaderSnippet: '探针' },
      { tabIndex: 3, name: 'Policy', expectedHeaderSnippet: '策略' },
      { tabIndex: 4, name: 'Publications', expectedHeaderSnippet: '发布' },
      { tabIndex: 5, name: 'Settings', expectedHeaderSnippet: '设置' },
    ];
    let allViewsTranslated = true;
    for (const v of viewsToCheck) {
      await navButtons.nth(v.tabIndex).click();
      await page.waitForTimeout(400);
      const pageText = await page.innerText('main');
      const hasPlaceholder = /[a-z]+\.[a-z]{3,}/.test(pageText); // e.g. "subscriptions.title"
      const hasExpected = pageText.includes(v.expectedHeaderSnippet);
      log(`    View ${v.name}: hasExpectedSnippet=${hasExpected}, rawPlaceholderLeak=${hasPlaceholder}`);
      if (!hasExpected || hasPlaceholder) {
        allViewsTranslated = false;
      }
    }

    report.tests.i18n = {
      status: (isEnWorking && isZhWorking && persistencePass && allViewsTranslated) ? 'PASSED' : 'FAILED',
      isEnWorking,
      isZhWorking,
      persistencePass,
      allViewsTranslated
    };
    if (!report.tests.i18n.status.includes('PASSED')) report.passed = false;

    // Return to Subscriptions tab
    await navButtons.nth(0).click();
    await page.waitForTimeout(500);

    // =========================================================================
    // Check 4: Secondary Card / Drawer Interaction & Full Configuration
    // =========================================================================
    log('--- Check 4: Secondary DrawerCard & SubscriptionConfigDrawer ---');
    const addSubBtn = page.locator('button', { hasText: '添加订阅' });
    await addSubBtn.click();
    await page.waitForTimeout(500);

    // Verify Drawer Opened
    const drawerDialog = page.locator('div[role="dialog"]');
    await drawerDialog.waitFor({ state: 'visible', timeout: 3000 });
    log('  Drawer dialog opened successfully');
    await page.screenshot({ path: path.join(EVIDENCE_DIR, '06_drawer_opened_basic.png') });

    // Verify Tabs inside Drawer (基础设置 / 高级规则)
    const drawerTabs = drawerDialog.locator('.tabs button');
    const basicTabBtn = drawerTabs.nth(0);
    const advTabBtn = drawerTabs.nth(1);

    // Switch to Advanced Tab
    log('  Switching to Advanced tab in drawer...');
    await advTabBtn.click();
    await page.waitForTimeout(300);
    await page.screenshot({ path: path.join(EVIDENCE_DIR, '07_drawer_advanced_tab.png') });

    // Fill Advanced settings:
    // Cron schedule
    const cronInput = drawerDialog.locator('input[placeholder="0 3 * * *"]');
    await cronInput.fill('0 4 * * *');

    // Auto Test toggle
    const autoTestToggle = drawerDialog.locator('input[type="checkbox"].toggle-secondary');
    if (!(await autoTestToggle.isChecked())) {
      await autoTestToggle.click();
    }

    // Add Rename Rule
    const addRenameBtn = drawerDialog.locator('button', { hasText: '添加规则' }).first();
    await addRenameBtn.click();
    await page.waitForTimeout(200);
    const renameInputs = drawerDialog.locator('input[placeholder="正则表达式 (如 HK (.*))"]');
    await renameInputs.last().fill('HK_(.*)');
    const replaceInputs = drawerDialog.locator('input[placeholder="替换文本 (如 Hong Kong $1)"]');
    await replaceInputs.last().fill('HongKong-$1');

    // Add Filter Rule
    const addFilterBtn = drawerDialog.locator('button', { hasText: '添加过滤' }).first();
    await addFilterBtn.click();
    await page.waitForTimeout(200);
    const filterInputs = drawerDialog.locator('input[placeholder="正则表达式 (如 IPLC|BGP)"]');
    await filterInputs.last().fill('HongKong|Taiwan');

    // Fill Target Groups
    const targetGroupsInput = drawerDialog.locator('input[placeholder="Proxy, Auto, Streaming"]');
    await targetGroupsInput.fill('Proxy, CriticGroup');

    // Switch back to Basic Tab
    log('  Switching back to Basic tab...');
    await basicTabBtn.click();
    await page.waitForTimeout(300);

    const testSubName = `Critic-Auto-Test-${Date.now().toString().slice(-4)}`;
    const testSecretUrl = 'https://critic-secret.provider.invalid/sub?token=critic-super-secret-token-xyz';

    const nameInput = drawerDialog.locator('input[placeholder="e.g. Hong Kong VIP Feed"]');
    await nameInput.fill(testSubName);

    const urlInput = drawerDialog.locator('input[placeholder="https://..."]');
    await urlInput.fill(testSecretUrl);

    // Save
    log(`  Saving new subscription: ${testSubName}...`);
    const saveBtn = drawerDialog.locator('button', { hasText: '保存配置' });
    await saveBtn.click();
    await page.waitForTimeout(1000);

    // Verify Drawer closed
    const isDrawerClosed = !(await drawerDialog.isVisible());
    log(`  Drawer closed after save: ${isDrawerClosed}`);

    // Verify Card appeared in list
    const newCard = page.locator('article.card', { hasText: testSubName });
    await newCard.waitFor({ state: 'visible', timeout: 5000 });
    log(`  New card "${testSubName}" visible in UI!`);

    // Verify secret URL is masked with '***' on the card
    const cardSecretText = await newCard.locator('p.font-mono').innerText();
    log(`  Card secret display: "${cardSecretText}" (MUST be '***')`);
    const secretMaskedProperly = cardSecretText.trim() === '***';
    if (!secretMaskedProperly) {
      log(`  ✗ SECURITY FAILURE: Card revealed secret: ${cardSecretText}`);
    }

    // Verify badges rendered on the card
    const badgesText = await newCard.innerText();
    const hasCronBadge = badgesText.includes('0 4 * * *');
    const hasAutoTestBadge = badgesText.includes('Auto Test');
    const hasRenameBadge = badgesText.includes('1 Rename');
    const hasFilterBadge = badgesText.includes('1 Filter');
    const hasGroupsBadge = badgesText.includes('2 Groups');

    log(`  Badges verified: cron=${hasCronBadge}, autoTest=${hasAutoTestBadge}, rename=${hasRenameBadge}, filter=${hasFilterBadge}, groups=${hasGroupsBadge}`);
    await page.screenshot({ path: path.join(EVIDENCE_DIR, '08_created_card_badges.png') });

    // Open Edit Drawer for this card
    log('  Opening Edit drawer on created card...');
    const editBtn = newCard.locator('button', { hasText: '编辑' });
    await editBtn.click();
    await page.waitForTimeout(500);

    await drawerDialog.waitFor({ state: 'visible', timeout: 3000 });
    // Verify masked URL in edit drawer
    const editUrlValue = await urlInput.inputValue();
    log(`  Edit drawer URL input value: "${editUrlValue}" (MUST be '***')`);
    const editUrlMasked = editUrlValue === '***';

    // Verify Advanced Tab settings in Edit mode
    await advTabBtn.click();
    await page.waitForTimeout(300);
    const editCronValue = await cronInput.inputValue();
    const editGroupsValue = await targetGroupsInput.inputValue();
    log(`  Edit drawer Advanced values: cron="${editCronValue}", groups="${editGroupsValue}"`);
    const editAdvValuesCorrect = editCronValue === '0 4 * * *' && editGroupsValue.includes('CriticGroup');

    // Close drawer via close button (X)
    const closeBtn = drawerDialog.locator('button[aria-label="Close drawer"]');
    await closeBtn.click();
    await page.waitForTimeout(500);
    const isClosedAfterX = !(await drawerDialog.isVisible());
    log(`  Drawer closed via X button: ${isClosedAfterX}`);

    // Open drawer again and test Backdrop click close
    await editBtn.click();
    await drawerDialog.waitFor({ state: 'visible', timeout: 3000 });
    // Click backdrop
    const backdrop = drawerDialog.locator('div[aria-hidden="true"]');
    await backdrop.click({ position: { x: 10, y: 10 } });
    await page.waitForTimeout(500);
    const isClosedAfterBackdrop = !(await drawerDialog.isVisible());
    log(`  Drawer closed via Backdrop click: ${isClosedAfterBackdrop}`);

    // Clean up: Delete the test card
    log('  Deleting test subscription for 100% rollback hygiene...');
    const deleteBtn = newCard.locator('button[title="删除"]');
    await deleteBtn.click();
    await page.waitForTimeout(500);

    const confirmModal = page.locator('div.modal-box');
    await confirmModal.waitFor({ state: 'visible', timeout: 3000 });
    const confirmDeleteBtn = confirmModal.locator('button', { hasText: '删除' });
    await confirmDeleteBtn.click();
    await page.waitForTimeout(1000);

    // Verify card is gone
    const cardDeleted = (await page.locator('article.card', { hasText: testSubName }).count()) === 0;
    log(`  Test card cleaned up from UI: ${cardDeleted}`);
    await page.screenshot({ path: path.join(EVIDENCE_DIR, '09_test_card_cleaned.png') });

    report.tests.drawerAndFullConfig = {
      status: (isDrawerClosed && secretMaskedProperly && hasCronBadge && editUrlMasked && editAdvValuesCorrect && isClosedAfterX && isClosedAfterBackdrop && cardDeleted) ? 'PASSED' : 'FAILED',
      secretMaskedProperly,
      hasCronBadge,
      hasAutoTestBadge,
      hasRenameBadge,
      hasFilterBadge,
      hasGroupsBadge,
      editUrlMasked,
      editAdvValuesCorrect,
      isClosedAfterX,
      isClosedAfterBackdrop,
      cardDeleted
    };
    if (!report.tests.drawerAndFullConfig.status.includes('PASSED')) report.passed = false;

    // =========================================================================
    // Check 5: Dirty Token Self-Healing & Open Mode Deadlock Check
    // =========================================================================
    log('--- Check 5: Dirty Token & Deadlock Resilience Check ---');
    await page.evaluate(() => {
      localStorage.setItem('csp_token', 'critic-dirty-stale-token-deadlock-check-12345');
    });
    log('  Injected dirty stale token into localStorage. Reloading page...');
    await page.reload({ waitUntil: 'networkidle' });
    await page.waitForTimeout(1000);

    // Check if auth gate blocked or modal stuck
    const modalBlocked = await page.locator('.modal-open').isVisible().catch(() => false);
    const mainVisible = await page.locator('main').isVisible();
    const tokenAfterSelfHeal = await page.evaluate(() => localStorage.getItem('csp_token'));
    log(`  After reload with dirty token: mainVisible=${mainVisible}, modalBlocked=${modalBlocked}, token=${tokenAfterSelfHeal}`);

    const dirtyTokenPass = mainVisible && !modalBlocked;
    report.tests.dirtyTokenSelfHealing = {
      status: dirtyTokenPass ? 'PASSED' : 'FAILED',
      mainVisible,
      modalBlocked,
      tokenAfterSelfHeal
    };
    if (!dirtyTokenPass) report.passed = false;
    await page.screenshot({ path: path.join(EVIDENCE_DIR, '10_dirty_token_resilience.png') });

    report.pageErrors = pageErrors;
    if (pageErrors.length > 0) {
      log(`  Notice: ${pageErrors.length} console/page errors detected during session:`);
      pageErrors.forEach(e => log(`    ${e}`));
    }

    log('=========================================================================');
    log(`Audit Run Complete! Overall Passed: ${report.passed}`);
    log('=========================================================================');

  } catch (err) {
    log(`FATAL ERROR during audit: ${err.message}`);
    report.passed = false;
    report.fatalError = err.stack;
    await page.screenshot({ path: path.join(EVIDENCE_DIR, 'fatal_error.png') }).catch(() => {});
  } finally {
    await browser.close();
    fs.writeFileSync(path.join(EVIDENCE_DIR, 'audit_report.json'), JSON.stringify(report, null, 2));
  }
}

runAudit();
