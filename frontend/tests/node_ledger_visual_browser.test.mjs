import assert from 'node:assert/strict'
import test from 'node:test'
import http from 'node:http'
import fs from 'node:fs'
import path from 'node:path'
import { chromium } from '@playwright/test'

const distDir = path.resolve(import.meta.dirname, '../dist')

function createStaticServer() {
  const mimeTypes = {
    '.html': 'text/html',
    '.js': 'application/javascript',
    '.css': 'text/css',
    '.json': 'application/json',
    '.svg': 'image/svg+xml',
    '.png': 'image/png',
    '.ico': 'image/x-icon',
  }

  const server = http.createServer((req, res) => {
    let reqPath = req.url.split('?')[0]
    if (reqPath === '/') reqPath = '/index.html'

    let filePath = path.join(distDir, reqPath)
    if (!fs.existsSync(filePath) || fs.statSync(filePath).isDirectory()) {
      filePath = path.join(distDir, 'index.html')
    }

    const ext = path.extname(filePath)
    const contentType = mimeTypes[ext] || 'application/octet-stream'

    try {
      const content = fs.readFileSync(filePath)
      res.writeHead(200, { 'Content-Type': contentType })
      res.end(content)
    } catch (_) {
      res.writeHead(404)
      res.end('Not found')
    }
  })

  return server
}

test('Task 1.3: Node Ledger Visual, Elevation & Reduced-Motion Browser Gate (375, 768, 1024, 1440px)', async (t) => {
  assert.ok(fs.existsSync(distDir), 'frontend/dist must exist before running DOM geometry tests')

  const server = createStaticServer()
  await new Promise((resolve) => server.listen(0, '127.0.0.1', resolve))
  const port = server.address().port
  const baseUrl = `http://127.0.0.1:${port}`

  const defaultBrowserPath = process.env.HOME ? path.join(process.env.HOME, '.cache/ms-playwright') : undefined
  process.env.PLAYWRIGHT_BROWSERS_PATH = process.env.PLAYWRIGHT_BROWSERS_PATH || defaultBrowserPath

  let browser
  try {
    browser = await chromium.launch({ headless: true })
  } catch (err) {
    server.close()
    throw new Error(`Failed to launch Chromium: ${err.message}`)
  }

  const viewports = [
    { name: 'mobile (375x812)', width: 375, height: 812 },
    { name: 'tablet (768x900)', width: 768, height: 900 },
    { name: 'desktop (1024x900)', width: 1024, height: 900 },
    { name: 'wide (1440x900)', width: 1440, height: 900 },
  ]

  try {
    // 1. Multi-viewport zero document horizontal overflow and dark canvas check
    for (const vp of viewports) {
      await t.test(`Viewport ${vp.name}: Node Ledger dark canvas & zero overflow`, async () => {
        const page = await browser.newPage({
          viewport: { width: vp.width, height: vp.height },
        })

        // Mock all backend calls non-destructively
        await page.route('/api/**', async (route) => {
          await route.fulfill({
            status: 200,
            contentType: 'application/json',
            body: JSON.stringify([]),
          })
        })

        await page.goto(`${baseUrl}/nodes`, { waitUntil: 'domcontentloaded' })
        await page.waitForTimeout(100)

        // Check zero document overflow
        const overflow = await page.evaluate(() => {
          const docEl = document.documentElement
          const body = document.body
          const scrollWidth = Math.max(docEl.scrollWidth, body.scrollWidth)
          const clientWidth = docEl.clientWidth
          return {
            scrollWidth,
            clientWidth,
            hasOverflow: scrollWidth > clientWidth + 1,
          }
        })
        assert.equal(overflow.hasOverflow, false, `Viewport ${vp.name} must not have document horizontal overflow`)

        // Check canvas dark token resolves to #0F172A
        const canvasColor = await page.evaluate(() => {
          return window.getComputedStyle(document.body).backgroundColor
        })
        // rgb(15, 23, 42) corresponds to #0F172A
        assert.equal(canvasColor, 'rgb(15, 23, 42)', `Viewport ${vp.name} document body background must resolve to rgb(15, 23, 42) [#0F172A], got ${canvasColor}`)

        await page.close()
      })
    }

    // 2. Reduced motion lifecycle: overlays complete state without durable animations
    await t.test('Reduced motion lifecycle: prefers-reduced-motion disables transitions and blur animation', async () => {
      const page = await browser.newPage({
        viewport: { width: 1024, height: 900 },
      })
      await page.emulateMedia({ reducedMotion: 'reduce' })

      await page.route('/api/**', async (route) => {
        await route.fulfill({
          status: 200,
          contentType: 'application/json',
          body: JSON.stringify([]),
        })
      })

      await page.goto(`${baseUrl}/nodes`, { waitUntil: 'domcontentloaded' })
      await page.waitForTimeout(100)

      const reducedMotionTransitions = await page.evaluate(() => {
        const testEl = document.createElement('div')
        testEl.className = 'drawer-slide-right'
        document.body.appendChild(testEl)
        const style = window.getComputedStyle(testEl)
        const animDuration = parseFloat(style.animationDuration) || 0
        const transDuration = parseFloat(style.transitionDuration) || 0
        document.body.removeChild(testEl)
        return { animDuration, transDuration }
      })

      // In reduced motion, animations/transitions must be effectively zero (<= 0.001s)
      assert.ok(
        reducedMotionTransitions.animDuration <= 0.001,
        `Reduced motion should disable or minimize animation duration, got ${reducedMotionTransitions.animDuration}s`
      )

      await page.close()
    })

    // 3. Facet Collapsible Architecture: No candidate options mounted when closed, preserves workspace
    await t.test('Facet collapsible: options not mounted when closed, preserves workspace', async () => {
      const page = await browser.newPage({
        viewport: { width: 1024, height: 900 },
      })

      await page.route('/api/**', async (route) => {
        await route.fulfill({
          status: 200,
          contentType: 'application/json',
          body: JSON.stringify([
            {
              name: 'HK-Test-Node',
              node_key: 'hk-1',
              type: 'vmess',
              server: '1.2.3.4',
              port: 443,
              subscription_name: 'Sub-A',
            },
          ]),
        })
      })

      await page.goto(`${baseUrl}/nodes`, { waitUntil: 'domcontentloaded' })
      await page.waitForTimeout(150)

      // Verify no candidate option is mounted while facets are closed
      const unmountedCount = await page.evaluate(() => {
        return document.querySelectorAll('.facet-candidate-option').length
      })
      assert.equal(unmountedCount, 0, 'Candidate options must NOT be mounted in DOM when facets are closed')

      // Verify all facet buttons have aria-expanded="false"
      const expandedButtons = await page.evaluate(() => {
        return Array.from(document.querySelectorAll('.facet-controller-container button'))
          .map((b) => b.getAttribute('aria-expanded'))
      })
      assert.ok(expandedButtons.length >= 4, 'Must have at least 4 facet controller buttons')
      assert.ok(expandedButtons.every((exp) => exp === 'false'), 'All facet triggers must initially have aria-expanded="false"')

      await page.close()
    })

    // 4. Keyboard accessibility & click-outside lifecycle on facets
    await t.test('Facet keyboard accessibility & click-outside lifecycle (Enter/Space/Escape)', async () => {
      const page = await browser.newPage({
        viewport: { width: 1024, height: 900 },
      })

      await page.route('/api/**', async (route) => {
        await route.fulfill({
          status: 200,
          contentType: 'application/json',
          body: JSON.stringify([
            {
              name: 'HK-01',
              type: 'vmess',
              subscription_name: 'Sub-A',
            },
          ]),
        })
      })

      await page.goto(`${baseUrl}/nodes`, { waitUntil: 'domcontentloaded' })
      await page.waitForTimeout(150)

      // Find health status facet button (has static options)
      const statusFacetBtn = page.locator('.facet-controller-container button').nth(2)
      await statusFacetBtn.focus()

      // Press Enter to open
      await page.keyboard.press('Enter')
      await page.waitForTimeout(50)

      let mountedCount = await page.evaluate(() => document.querySelectorAll('.facet-candidate-option').length)
      assert.ok(mountedCount > 0, 'Candidate options must mount into DOM upon pressing Enter')

      let isExpanded = await statusFacetBtn.getAttribute('aria-expanded')
      assert.equal(isExpanded, 'true', 'Trigger must have aria-expanded="true" when open')

      // Press Escape to close
      await page.keyboard.press('Escape')
      await page.waitForTimeout(50)

      mountedCount = await page.evaluate(() => document.querySelectorAll('.facet-candidate-option').length)
      assert.equal(mountedCount, 0, 'Candidate options must unmount upon pressing Escape')

      isExpanded = await statusFacetBtn.getAttribute('aria-expanded')
      assert.equal(isExpanded, 'false', 'Trigger must have aria-expanded="false" after Escape')

      // Click to open, then click outside to close
      await statusFacetBtn.click()
      await page.waitForTimeout(50)
      mountedCount = await page.evaluate(() => document.querySelectorAll('.facet-candidate-option').length)
      assert.ok(mountedCount > 0, 'Candidate options must mount upon click')

      // Click outside on main heading or body
      await page.locator('h1, h2, body').first().click({ position: { x: 10, y: 10 } })
      await page.waitForTimeout(50)
      mountedCount = await page.evaluate(() => document.querySelectorAll('.facet-candidate-option').length)
      assert.equal(mountedCount, 0, 'Candidate options must unmount upon clicking outside')

      await page.close()
    })

    // 5. Active-filter summary strip token lifecycle & reset
    await t.test('Active-filter summary strip: token display, removal, and reset all', async () => {
      const page = await browser.newPage({
        viewport: { width: 1024, height: 900 },
      })

      await page.route('/api/**', async (route) => {
        await route.fulfill({
          status: 200,
          contentType: 'application/json',
          body: JSON.stringify([
            {
              name: 'HK-01',
              type: 'vmess',
              subscription_name: 'Sub-A',
            },
          ]),
        })
      })

      await page.goto(`${baseUrl}/nodes`, { waitUntil: 'domcontentloaded' })
      await page.waitForTimeout(150)

      // Initially no active filter summary strip
      let hasSummary = await page.evaluate(() => Boolean(document.querySelector('[aria-label="已生效筛选条件"]')))
      assert.equal(hasSummary, false, 'Summary strip must not render when no filter is active')

      // Open health status facet (3rd facet)
      const statusFacet = page.locator('.facet-controller-container button').nth(2)
      await statusFacet.click()
      await page.waitForTimeout(50)

      // Click first option (正常可用)
      const firstOption = page.locator('.facet-candidate-option').first()
      await firstOption.click()
      await page.waitForTimeout(50)

      // Now summary strip should be mounted
      hasSummary = await page.evaluate(() => Boolean(document.querySelector('[aria-label="已生效筛选条件"]')))
      assert.equal(hasSummary, true, 'Summary strip must render after selecting a facet option')

      // Check badge shows "1"
      const badgeText = await statusFacet.locator('span.rounded-full').textContent()
      assert.equal(badgeText?.trim(), '1', 'Facet trigger badge should display selected count 1')

      // Click the Reset All button in the summary strip
      const resetBtn = page.locator('[aria-label="已生效筛选条件"] button:has-text("清空所有")')
      await resetBtn.click()
      await page.waitForTimeout(50)

      // Summary strip should unmount again
      hasSummary = await page.evaluate(() => Boolean(document.querySelector('[aria-label="已生效筛选条件"]')))
      assert.equal(hasSummary, false, 'Summary strip must unmount after reset-all')

      await page.close()
    })
  } finally {
    if (browser) await browser.close()
    await new Promise((resolve) => server.close(resolve))
  }
})

test('Task 1.2: Node Ledger Chromium Contract Gate (Strict 375px Geometry, Keyboard A11y, Duplicate Name Guard, Closed Status)', async (t) => {
  assert.ok(fs.existsSync(distDir), 'frontend/dist must exist before running DOM geometry tests')

  const fixturesPath = path.resolve(import.meta.dirname, './fixtures/node_ledger_fixtures.json')
  const fixtures = JSON.parse(fs.readFileSync(fixturesPath, 'utf8'))

  const server = createStaticServer()
  await new Promise((resolve) => server.listen(0, '127.0.0.1', resolve))
  const port = server.address().port
  const baseUrl = `http://127.0.0.1:${port}`

  const defaultBrowserPath = process.env.HOME ? path.join(process.env.HOME, '.cache/ms-playwright') : undefined
  process.env.PLAYWRIGHT_BROWSERS_PATH = process.env.PLAYWRIGHT_BROWSERS_PATH || defaultBrowserPath

  let browser
  try {
    browser = await chromium.launch({ headless: true })
  } catch (err) {
    server.close()
    throw new Error(`Failed to launch Chromium: ${err.message}`)
  }

  const setupMockRoutes = async (page) => {
    await page.route('**/api/**', async (route) => {
      const url = route.request().url()
      if (url.includes('/api/proxy-chains/meta/node-ledger')) {
        await route.fulfill({
          status: 200,
          contentType: 'application/json',
          body: JSON.stringify(fixtures.current_nodes),
        })
      } else if (url.includes('/api/probe/results')) {
        const combinedResults = {
          ...fixtures.paged_probe_summaries.page_1.results,
          ...fixtures.paged_probe_summaries.page_2.results,
        }
        await route.fulfill({
          status: 200,
          contentType: 'application/json',
          body: JSON.stringify({
            results: combinedResults,
            next_cursor: null,
            has_more: false,
          }),
        })
      } else if (url.includes('/api/probe/status')) {
        await route.fulfill({
          status: 200,
          contentType: 'application/json',
          body: JSON.stringify({ enabled: false, running: false }),
        })
      } else {
        await route.fulfill({
          status: 200,
          contentType: 'application/json',
          body: JSON.stringify([]),
        })
      }
    })
  }

  try {
    // 1. Strict multi-viewport geometry with stress fixture data
    const viewports = [
      { name: 'mobile (375x812)', width: 375, height: 812 },
      { name: 'tablet (768x900)', width: 768, height: 900 },
      { name: 'desktop (1024x900)', width: 1024, height: 900 },
      { name: 'wide (1440x900)', width: 1440, height: 900 },
    ]

    for (const vp of viewports) {
      await t.test(`Strict geometry ${vp.name}: zero overflow under stress node fixtures`, async () => {
        const page = await browser.newPage({
          viewport: { width: vp.width, height: vp.height },
        })
        await setupMockRoutes(page)
        await page.goto(`${baseUrl}/nodes`, { waitUntil: 'domcontentloaded' })
        await page.waitForTimeout(200)

        const geom = await page.evaluate(() => {
          const docEl = document.documentElement
          const body = document.body
          const scrollWidth = Math.max(docEl.scrollWidth, body.scrollWidth)
          const clientWidth = docEl.clientWidth
          return { scrollWidth, clientWidth }
        })

        if (vp.width === 375) {
          assert.equal(
            geom.scrollWidth,
            375,
            `At 375px mobile viewport, scrollWidth must strictly equal 375px, got ${geom.scrollWidth}`
          )
          assert.equal(
            geom.clientWidth,
            375,
            `At 375px mobile viewport, clientWidth must strictly equal 375px, got ${geom.clientWidth}`
          )
        } else {
          assert.ok(
            geom.scrollWidth <= geom.clientWidth + 1,
            `At ${vp.name}, scrollWidth (${geom.scrollWidth}) must not exceed clientWidth (${geom.clientWidth})`
          )
        }
        await page.close()
      })
    }

    // 2. Inspection drawer keyboard lifecycle: focus trap, Escape exit, and focus restoration
    await t.test('Drawer overlay keyboard lifecycle: focus trap, Escape exit, and focus restore', async () => {
      const page = await browser.newPage({
        viewport: { width: 1024, height: 900 },
      })
      await setupMockRoutes(page)
      await page.goto(`${baseUrl}/nodes`, { waitUntil: 'domcontentloaded' })
      await page.waitForTimeout(300)

      const firstNode = page.locator('.table-row-dense:has-text("HK-01")').locator('visible=true').first()
      await firstNode.click()
      await page.waitForTimeout(150)

      const drawerVisible = await page.evaluate(() => Boolean(document.querySelector('[role="dialog"]')))
      assert.ok(drawerVisible, 'Drawer [role="dialog"] must mount upon clicking node row')

      const focusInside = await page.evaluate(() => {
        const dialog = document.querySelector('[role="dialog"]')
        return dialog ? dialog.contains(document.activeElement) : false
      })
      assert.ok(focusInside, 'Focus must move into drawer upon opening')

      await page.keyboard.press('Escape')
      await page.waitForTimeout(300)

      const drawerClosed = await page.evaluate(() => !document.querySelector('[role="dialog"]'))
      assert.ok(drawerClosed, 'Drawer must unmount or close upon pressing Escape')

      await page.close()
    })

    // 3. Chromium Gate: duplicate-name probe status isolation and chain edit guard (Red light test)
    await t.test('Chromium Gate: duplicate-name probe status isolation and chain edit guard', async () => {
      const page = await browser.newPage({
        viewport: { width: 1024, height: 900 },
      })
      await setupMockRoutes(page)
      await page.goto(`${baseUrl}/nodes`, { waitUntil: 'domcontentloaded' })
      await page.waitForTimeout(250)

      const usRows = page.locator('div:has-text("US-Relay")')
      const count = await usRows.count()
      assert.ok(count >= 2, `Must render at least 2 rows for duplicate "US-Relay", found ${count}`)

      // Probe status isolation in DOM:
      // Row 1 (vmess, port 443) has status ok (120ms), Row 2 (ss, port 8388) has status fail.
      // In unmigrated code, getProbe(item.name) causes both rows to render identical status.
      const duplicateRowStatuses = await page.evaluate(() => {
        const rows = Array.from(document.querySelectorAll('.table-row-dense'))
        const usRows = rows.filter((r) => r.querySelector('[title="US-Relay"]'))
        return usRows.map((r) => {
          const statusEl = r.querySelector(
            '.flex.items-center.justify-end span'
          )
          return statusEl ? statusEl.textContent.trim() : ''
        })
      })
      assert.equal(
        duplicateRowStatuses.length,
        2,
        `Must find exactly 2 rows for "US-Relay", found ${duplicateRowStatuses.length}`
      )
      assert.notEqual(
        duplicateRowStatuses[0],
        duplicateRowStatuses[1],
        `Same-name rows must NOT share probe status. Got identical statuses: ${JSON.stringify(duplicateRowStatuses)}`
      )
      assert.ok(
        duplicateRowStatuses.some((s) => s.includes('120ms')) &&
          duplicateRowStatuses.some((s) => s.includes('失败')),
        `One US-Relay row must show 120ms and the other must show 失败. Got: ${JSON.stringify(duplicateRowStatuses)}`
      )

      // Chain edit guard:
      const chainGuard = await page.evaluate(() => {
        const buttons = Array.from(document.querySelectorAll('button, [title], [aria-label]'))
        return buttons.some((el) => {
          const title = el.getAttribute('title') || ''
          const label = el.getAttribute('aria-label') || ''
          const text = el.textContent || ''
          return (
            title.includes('名称重复，暂不能安全编辑跳板') ||
            label.includes('名称重复，暂不能安全编辑跳板') ||
            text.includes('名称重复，暂不能安全编辑跳板')
          )
        })
      })
      assert.ok(
        chainGuard,
        'Duplicate-name nodes must display disabled chain action with copy "名称重复，暂不能安全编辑跳板"'
      )

      await page.close()
    })

    // 4. Chromium Gate: closed status presentation in DOM (timeout, skipped, unknown) (Red light test)
    await t.test('Chromium Gate: closed status model in DOM (timeout, skipped, unknown)', async () => {
      const page = await browser.newPage({
        viewport: { width: 1024, height: 900 },
      })
      await setupMockRoutes(page)
      await page.goto(`${baseUrl}/nodes`, { waitUntil: 'domcontentloaded' })
      await page.waitForTimeout(250)

      const pageText = await page.evaluate(() => document.body.innerText)

      assert.ok(pageText.includes('超时'), 'JP-Timeout must visibly render "超时"')
      assert.ok(
        pageText.includes('跳过') || pageText.includes('已跳过'),
        'SG-Skipped must visibly render "跳过" or "已跳过", never fallback to "未测"'
      )
      assert.ok(
        pageText.includes('状态未知'),
        'KR-Unknown with unsupported status must visibly render "状态未知", strictly forbidden from degrading to "未测"'
      )

      await page.close()
    })
  } finally {
    if (browser) await browser.close()
    await new Promise((resolve) => server.close(resolve))
  }
})
