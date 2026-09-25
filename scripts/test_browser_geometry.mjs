// @ts-check
import { createRequire } from 'node:module'
const require = createRequire(import.meta.url)
const { chromium } = require('../web/node_modules/@playwright/test')
import http from 'node:http'
import fs from 'node:fs'
import path from 'node:path'
import { fileURLToPath } from 'node:url'

const __filename = fileURLToPath(import.meta.url)
const __dirname = path.dirname(__filename)
const distDir = process.env.DIST_DIR || '/tmp/safe-web-dist'
const screenshotsDir = process.env.SCREENSHOTS_DIR || path.resolve(__dirname, '../web/e2e/screenshots')

if (!fs.existsSync(screenshotsDir)) {
  fs.mkdirSync(screenshotsDir, { recursive: true })
}

// 1. Static file server for /tmp/safe-web-dist
function startServer(port = 5199) {
  const server = http.createServer((req, res) => {
    let reqPath = req.url?.split('?')[0] || '/'
    let filePath = path.join(distDir, reqPath === '/' ? 'index.html' : reqPath)
    if (!fs.existsSync(filePath) || fs.statSync(filePath).isDirectory()) {
      filePath = path.join(distDir, 'index.html')
    }
    const ext = path.extname(filePath)
    const mimeMap = {
      '.html': 'text/html',
      '.js': 'application/javascript',
      '.css': 'text/css',
      '.json': 'application/json',
      '.png': 'image/png',
      '.svg': 'image/svg+xml',
    }
    res.writeHead(200, { 'Content-Type': mimeMap[ext] || 'application/octet-stream' })
    fs.createReadStream(filePath).pipe(res)
  })

  return new Promise((resolve) => {
    server.listen(port, '127.0.0.1', () => {
      resolve({
        port,
        close: () => new Promise((cb) => server.close(cb)),
      })
    })
  })
}

// Mock API state
const mockData = {
  health: { status: 'healthy' },
  auth: { mode: 'protected', authenticated: true, subject: 'admin' },
  subscriptions: {
    items: [
      {
        id: 'sub-test-1',
        name: 'Primary HK & Global Subscription Feed',
        source_url_secret_ref: 'sec:very-long-secret-reference-name-that-could-cause-horizontal-overflow-if-unconstrained',
        enabled: true,
        refresh_policy: { interval_seconds: 3600, timeout_seconds: 30, retry_count: 3 },
        created_at: '2026-09-24T10:00:00Z',
      },
    ],
    total: 1,
  },
  nodes: {
    items: [
      { logicalId: 'node-hk-01', displayName: 'HK High Speed Premium Node', protocol: 'shadowsocks', active: true, countryCode: 'HK', tags: ['fast'] },
    ],
    total: 1,
  },
  probes: {
    items: [
      { id: 'run-101', state: 'succeeded', kinds: ['baseline', 'ai'], deadline_minutes: 10, created_at: '2026-09-24T10:00:00Z' },
    ],
    total: 1,
  },
  policyGroups: {
    items: [
      { id: 'grp-1', name: 'Auto Select HK', group_type: 'urltest', edges: [] },
    ],
    total: 1,
  },
  policyRules: {
    admission_rules: [
      {
        id: 'rule-admission-1',
        name: 'Reject High Risk IP & Filter Non-HK',
        expression: 'country in ["HK", "TW", "SG", "JP"] && speed >= 10000000 && ip_fraud_score <= 30 && protocol == "trojan"',
        action: 'allow',
        position: 1,
      },
    ],
    routing_rules: [],
  },
  publicationsPreview: {
    target: 'clash',
    snapshot_digest: 'sha256:7f83b1657ff1fc53b92dc18148a1d65dfc2d4b1fa3d677284addd200126d9069',
    content_digest: 'sha256:9b71d224bd62f3785d96d46ad3ea3d73319bfbc2890caadae2dff72519673ca72',
    content: 'mixed-port: 7890\nallow-lan: true\nmode: rule\nlog-level: info\nproxies:\n  - name: HK High Speed Premium Node\n    type: ss\n    server: 1.1.1.1\n    port: 8388\n',
    content_type: 'application/x-yaml',
    filename: 'config-clash-production-export.yaml',
    diagnostics: [],
  },
}

async function runGeometryAssertions() {
  const srv = await startServer(5199)
  const baseUrl = `http://127.0.0.1:${srv.port}`
  console.log(`Test server running at ${baseUrl}`)

  const browser = await chromium.launch({
    headless: true,
    args: ['--no-sandbox', '--disable-gpu', '--disable-dev-shm-usage'],
  })

  const viewports = [
    { name: 'Mobile-375x667', width: 375, height: 667, isMobile: true },
    { name: 'Mobile-392x872', width: 392, height: 872, isMobile: true },
    { name: 'Landscape-667x375', width: 667, height: 375, isMobile: true },
    { name: 'Desktop-1280x800', width: 1280, height: 800, isMobile: false },
  ]

  let allPassed = true
  const results = []

  for (const vp of viewports) {
    console.log(`\n========================================`)
    console.log(`Testing Viewport: ${vp.name} (${vp.width}x${vp.height})`)
    console.log(`========================================`)

    const context = await browser.newContext({
      viewport: { width: vp.width, height: vp.height },
      isMobile: vp.isMobile,
      hasTouch: vp.isMobile,
    })

    const page = await context.newPage()

    // Setup hermetic API route intercepts
    await page.route('**/healthz', (route) => route.fulfill({ json: mockData.health }))
    await page.route('**/api/v1/auth/status', (route) => route.fulfill({ json: mockData.auth }))
    await page.route('**/api/v1/subscriptions**', (route) => route.fulfill({ json: mockData.subscriptions }))
    await page.route('**/api/v1/nodes**', (route) => route.fulfill({ json: mockData.nodes }))
    await page.route('**/api/v1/probes/runs**', (route) => route.fulfill({ json: mockData.probes }))
    await page.route('**/api/v1/policies/groups**', (route) => route.fulfill({ json: mockData.policyGroups }))
    await page.route('**/api/v1/policies/rules**', (route) => route.fulfill({ json: mockData.policyRules }))
    await page.route('**/api/v1/publications/preview**', (route) => route.fulfill({ json: mockData.publicationsPreview }))

    // Helper: assert element within viewport horizontally
    async function assertInViewport(selector, label) {
      const el = page.locator(selector).first()
      const isVisible = await el.isVisible()
      if (!isVisible) throw new Error(`${label} (${selector}) is not visible!`)
      const box = await el.boundingBox()
      if (!box) throw new Error(`${label} (${selector}) has no bounding box!`)
      if (box.x < 0 || box.x + box.width > vp.width + 1) {
        throw new Error(`${label} bounds out of viewport: x=${box.x.toFixed(1)}, right=${(box.x + box.width).toFixed(1)}, viewportWidth=${vp.width}`)
      }
      return box
    }

    // Helper: assert main has no horizontal scroll
    async function assertNoMainHorizontalScroll(pageName) {
      const scrollInfo = await page.evaluate(() => {
        const main = document.querySelector('main')
        if (!main) return { ok: false, reason: 'main not found' }
        return {
          ok: main.scrollWidth <= main.clientWidth + 1,
          scrollWidth: main.scrollWidth,
          clientWidth: main.clientWidth,
        }
      })
      if (!scrollInfo.ok) {
        throw new Error(`[${vp.name}] ${pageName}: main horizontal overflow detected: scrollWidth=${scrollInfo.scrollWidth} > clientWidth=${scrollInfo.clientWidth}`)
      }
      console.log(`  ✓ ${pageName}: main.scrollWidth (${scrollInfo.scrollWidth}) <= clientWidth (${scrollInfo.clientWidth})`)
    }

    // -----------------------------------------------------------------
    // Page 1: Dashboard
    // -----------------------------------------------------------------
    console.log(`--- [${vp.name}] Checking Dashboard ---`)
    await page.goto(`${baseUrl}/#/dashboard`, { waitUntil: 'networkidle' })
    await page.waitForTimeout(500)
    await assertNoMainHorizontalScroll('Dashboard')

    // Telemetry Badges Assertion: badge.scrollHeight <= badge.clientHeight
    const badgeHeights = await page.evaluate(() => {
      const engine = document.querySelector('[data-testid="telemetry-badge-engine"]')
      const storage = document.querySelector('[data-testid="telemetry-badge-storage"]')
      if (!engine || !storage) return { error: 'Badges not found' }
      return {
        engine: { scrollHeight: engine.scrollHeight, clientHeight: engine.clientHeight },
        storage: { scrollHeight: storage.scrollHeight, clientHeight: storage.clientHeight },
      }
    })
    if (badgeHeights.error) throw new Error(badgeHeights.error)
    if (badgeHeights.engine.scrollHeight > badgeHeights.engine.clientHeight) {
      throw new Error(`Engine badge height pierces border: scrollHeight=${badgeHeights.engine.scrollHeight} > clientHeight=${badgeHeights.engine.clientHeight}`)
    }
    if (badgeHeights.storage.scrollHeight > badgeHeights.storage.clientHeight) {
      throw new Error(`Storage badge height pierces border: scrollHeight=${badgeHeights.storage.scrollHeight} > clientHeight=${badgeHeights.storage.clientHeight}`)
    }
    console.log(`  ✓ Dashboard Telemetry badges: engine (${badgeHeights.engine.scrollHeight} <= ${badgeHeights.engine.clientHeight}px), storage (${badgeHeights.storage.scrollHeight} <= ${badgeHeights.storage.clientHeight}px)`)
    await page.screenshot({ path: path.join(screenshotsDir, `${vp.name}_01_dashboard.png`) })

    // -----------------------------------------------------------------
    // Page 2: Publications
    // -----------------------------------------------------------------
    console.log(`--- [${vp.name}] Checking Publications ---`)
    await page.goto(`${baseUrl}/#/publications`, { waitUntil: 'networkidle' })
    await page.waitForTimeout(500)
    await assertNoMainHorizontalScroll('Publications')

    // Check Header action buttons within viewport
    const downloadBtnBox = await assertInViewport('button:has-text("Download"), button:has-text("下载")', 'Download button')
    const createPubBtnBox = await assertInViewport('button:has-text("Create Publication"), button:has-text("生成发布")', 'Create Publication button')
    console.log(`  ✓ Publications buttons in viewport: Download right=${(downloadBtnBox.x + downloadBtnBox.width).toFixed(1)}, CreatePub right=${(createPubBtnBox.x + createPubBtnBox.width).toFixed(1)} <= ${vp.width}`)

    // Check all 5 target format pills exist, wrap, and are fully visible and clickable
    const targetPills = page.locator('[role="tablist"] button')
    const pillCount = await targetPills.count()
    if (pillCount !== 5) throw new Error(`Expected 5 target pills, found ${pillCount}`)
    for (let i = 0; i < 5; i++) {
      const pill = targetPills.nth(i)
      const text = (await pill.innerText()).replace(/\n/g, ' ')
      const box = await pill.boundingBox()
      if (!box || box.x < 0 || box.x + box.width > vp.width + 1) {
        throw new Error(`Target pill ${i} "${text}" out of viewport: x=${box?.x}, right=${box ? box.x + box.width : 0}`)
      }
      console.log(`    ✓ Target pill [${i}] "${text}": in viewport (x=${box.x.toFixed(1)}..${(box.x + box.width).toFixed(1)})`)
    }
    // Click Quantumult X to verify interaction
    await targetPills.nth(4).click()
    await page.waitForTimeout(200)
    await assertNoMainHorizontalScroll('Publications after target switch')
    await page.screenshot({ path: path.join(screenshotsDir, `${vp.name}_02_publications.png`) })

    // Check 409 no_active_revision state recovery UI
    console.log(`--- [${vp.name}] Checking Publications 409 State ---`)
    await page.route('**/api/v1/publications/preview**', (route) =>
      route.fulfill({
        status: 409,
        contentType: 'application/json',
        json: { code: 'no_active_revision', message: 'No configuration revision active' },
      })
    )
    await page.locator('button[title="Refresh"], button[title="刷新"]').first().click()
    await page.waitForTimeout(500)
    await assertNoMainHorizontalScroll('Publications 409 Card')
    const card409 = page.locator('[data-testid="no-active-revision-card"]')
    const cardVisible = await card409.isVisible()
    if (!cardVisible) throw new Error('409 no-active-revision-card not visible!')
    const cardBox = await card409.boundingBox()
    if (cardBox.x < 0 || cardBox.x + cardBox.width > vp.width + 1) {
      throw new Error(`409 card overflows: right=${cardBox.x + cardBox.width} > ${vp.width}`)
    }
    console.log(`  ✓ Publications 409 card within viewport (right=${(cardBox.x + cardBox.width).toFixed(1)} <= ${vp.width})`)

    // Check bottom-most button clearance when scrolled
    if (vp.isMobile) {
      await page.evaluate(() => window.scrollTo(0, document.body.scrollHeight))
      await page.waitForTimeout(300)
      const lastBtn = card409.locator('button').last()
      const lastBtnBox = await lastBtn.boundingBox()
      const dockBox = await page.locator('[data-testid="mobile-dock"]').boundingBox()
      const padBottom = await page.evaluate(() => window.getComputedStyle(document.querySelector('main')).paddingBottom)
      console.log(`    [Debug Clearance] padBottom=${padBottom}, lastBtnBottom=${lastBtnBox.y + lastBtnBox.height}, dockTop=${dockBox.y}`)
      if (lastBtnBox && dockBox) {
        const gap = dockBox.y - (lastBtnBox.y + lastBtnBox.height)
        if (gap < 0) {
          throw new Error(`Dock covers last button in 409 card: gap is ${gap}px (< 0)!`)
        }
        console.log(`  ✓ Publications 409 card: last button has ${gap.toFixed(1)}px clearance above dock`)
      }
    }
    await page.screenshot({ path: path.join(screenshotsDir, `${vp.name}_03_publications_409.png`) })

    // Restore preview mock
    await page.route('**/api/v1/publications/preview**', (route) => route.fulfill({ json: mockData.publicationsPreview }))

    // -----------------------------------------------------------------
    // Page 3: Policy Admission Rules
    // -----------------------------------------------------------------
    console.log(`--- [${vp.name}] Checking Policy Admission Rules ---`)
    await page.goto(`${baseUrl}/#/policy`, { waitUntil: 'networkidle' })
    await page.waitForTimeout(500)
    // Click Rules tab
    const rulesTab = page.locator('button:has-text("Rules"), button:has-text("准入规则")').first()
    await rulesTab.click()
    await page.waitForTimeout(300)

    const overflowElements = await page.evaluate(() => {
      const list = []
      document.querySelectorAll('*').forEach(el => {
        const rect = el.getBoundingClientRect()
        if (el.scrollWidth > 376 || rect.right > 376) {
          list.push({
            tag: el.tagName,
            id: el.id,
            class: el.className ? String(el.className).slice(0, 100) : '',
            scrollWidth: el.scrollWidth,
            clientWidth: el.clientWidth,
            rectRight: rect.right,
            rectWidth: rect.width,
          })
        }
      })
      return list
    })
    console.log(`Policy overflow elements count: ${overflowElements.length}`)
    if (overflowElements.length > 0) {
      console.log('Sample overflow elements:', JSON.stringify(overflowElements.slice(0, 5), null, 2))
    }

    await assertNoMainHorizontalScroll('Policy Admission Rules')

    const ruleCard = page.locator('[data-testid="admission-rule-card"]').first()
    const ruleBox = await assertInViewport('[data-testid="admission-rule-card"]', 'Admission Rule Card')
    console.log(`  ✓ Admission Rule card in viewport: right=${(ruleBox.x + ruleBox.width).toFixed(1)} <= ${vp.width} (previously x=413 overflowed)`)
    await page.screenshot({ path: path.join(screenshotsDir, `${vp.name}_04_policy_admission.png`) })

    // -----------------------------------------------------------------
    // Page 4: Subscriptions & Actions Popover
    // -----------------------------------------------------------------
    console.log(`--- [${vp.name}] Checking Subscriptions ---`)
    await page.goto(`${baseUrl}/#/subscriptions`, { waitUntil: 'networkidle' })
    await page.waitForTimeout(500)
    await assertNoMainHorizontalScroll('Subscriptions')

    const subCardBox = await assertInViewport('[data-testid="subscription-card"]', 'Subscription Card')
    const menuTriggerBox = await assertInViewport('[data-testid="subscription-actions-trigger"]', 'Subscription Actions Trigger')
    console.log(`  ✓ Subscription card: width=${subCardBox.width.toFixed(1)}, right=${(subCardBox.x + subCardBox.width).toFixed(1)} <= ${vp.width}`)

    // Open Popover Menu
    await page.locator('[data-testid="subscription-actions-trigger"]').first().click()
    await page.waitForTimeout(300)
    const popoverPanel = page.locator('[data-testid="subscription-card"] [aria-orientation="vertical"]').first()
    const popoverBox = await popoverPanel.boundingBox()
    if (!popoverBox || popoverBox.x < 0 || popoverBox.x + popoverBox.width > vp.width + 1) {
      throw new Error(`Subscription menu popover overflows viewport: x=${popoverBox?.x}, right=${popoverBox ? popoverBox.x + popoverBox.width : 0}, viewportWidth=${vp.width}`)
    }
    console.log(`  ✓ Subscription menu popover in viewport: x=${popoverBox.x.toFixed(1)}..${(popoverBox.x + popoverBox.width).toFixed(1)} <= ${vp.width}`)
    await page.screenshot({ path: path.join(screenshotsDir, `${vp.name}_05_subscriptions_popover.png`) })

    // -----------------------------------------------------------------
    // Page 5: Settings & Token Status Badge
    // -----------------------------------------------------------------
    console.log(`--- [${vp.name}] Checking Settings ---`)
    // Set localStorage token to simulate active state
    await page.evaluate(() => localStorage.setItem('csp_token', 'test-token-active-12345'))
    await page.goto(`${baseUrl}/#/settings`, { waitUntil: 'networkidle' })
    await page.waitForTimeout(500)
    await assertNoMainHorizontalScroll('Settings')

    const tokenBadge = page.locator('[data-testid="token-status-badge"]')
    const badgeText = (await tokenBadge.innerText()).trim()
    if (badgeText.includes('Active Active')) {
      throw new Error(`Duplicate text "Active Active" found in Settings token badge! Actual: "${badgeText}"`)
    }
    console.log(`  ✓ Settings token status text: "${badgeText}" (no Active Active duplication)`)
    await page.screenshot({ path: path.join(screenshotsDir, `${vp.name}_06_settings.png`) })

    // -----------------------------------------------------------------
    // Mobile Dock (For mobile / tablet viewports)
    // -----------------------------------------------------------------
    if (vp.isMobile) {
      console.log(`--- [${vp.name}] Checking Mobile Dock (7 Routes & Gap) ---`)
      const dock = page.locator('[data-testid="mobile-dock"]')
      const dockVisible = await dock.isVisible()
      if (!dockVisible) throw new Error('Mobile dock is not visible on mobile viewport!')

      const expectedRoutes = ['dashboard', 'subscriptions', 'nodes', 'probes', 'policy', 'publications', 'settings']
      const dockButtons = []
      for (const routeId of expectedRoutes) {
        const btn = page.locator(`[data-testid="dock-link-${routeId}"]`)
        const isBtnVis = await btn.isVisible()
        if (!isBtnVis) throw new Error(`Dock button for ${routeId} is not visible!`)
        const bBox = await btn.boundingBox()
        dockButtons.push({ routeId, box: bBox })
      }

      // Check gaps between adjacent buttons on row 1 (items 0..3)
      for (let i = 0; i < 3; i++) {
        const leftBtn = dockButtons[i]
        const rightBtn = dockButtons[i + 1]
        const gap = rightBtn.box.x - (leftBtn.box.x + leftBtn.box.width)
        if (gap < 0.5) {
          throw new Error(`Dock zero gap sticking: between ${leftBtn.routeId} and ${rightBtn.routeId} gap is ${gap}px (< 0.5px)!`)
        }
      }
      console.log(`  ✓ Mobile Dock: row 1 adjacent gaps verified >= 1px (no 0-gap sticking)`)

      // Check gaps between adjacent buttons on row 2 (items 4..6)
      for (let i = 4; i < 6; i++) {
        const leftBtn = dockButtons[i]
        const rightBtn = dockButtons[i + 1]
        const gap = rightBtn.box.x - (leftBtn.box.x + leftBtn.box.width)
        if (gap < 0.5) {
          throw new Error(`Dock zero gap sticking: between ${leftBtn.routeId} and ${rightBtn.routeId} gap is ${gap}px (< 0.5px)!`)
        }
      }
      console.log(`  ✓ Mobile Dock: row 2 adjacent gaps verified >= 1px (no 0-gap sticking)`)

      // Test navigation to Dashboard via dock button
      await page.locator('[data-testid="dock-link-dashboard"]').click()
      await page.waitForTimeout(300)
      const currentUrl = page.url()
      if (!currentUrl.includes('/dashboard')) {
        throw new Error(`Clicking dock dashboard link did not navigate to dashboard! URL: ${currentUrl}`)
      }
      console.log(`  ✓ Dock navigation to Dashboard verified!`)
    }

    await context.close()
    results.push({ viewport: vp.name, status: 'PASSED' })
  }

  await browser.close()
  await srv.close()

  console.log(`\n========================================`)
  console.log(`All Browser Geometry Assertions Passed!`)
  console.log(JSON.stringify(results, null, 2))
  console.log(`Screenshots saved to: ${screenshotsDir}`)
  console.log(`========================================\n`)
}

runGeometryAssertions().catch((err) => {
  console.error('\n❌ Browser Geometry Assertions FAILED:', err)
  process.exit(1)
})
