import assert from 'node:assert/strict'
import test from 'node:test'
import http from 'node:http'
import fs from 'node:fs'
import path from 'node:path'
import { chromium } from '@playwright/test'

const distDir = path.resolve(import.meta.dirname, '../dist')

// Simple static file server serving Vite production build
function createStaticServer() {
  const mimeTypes = {
    '.html': 'text/html',
    '.js': 'application/javascript',
    '.css': 'text/css',
    '.json': 'application/json',
    '.svg': 'image/svg+xml',
    '.png': 'image/png',
    '.ico': 'image/x-icon'
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
    } catch (e) {
      res.writeHead(404)
      res.end('Not found')
    }
  })

  return server
}

test('Real Multi-Viewport DOM Geometry Gate (375px, 768px, 1024px, 1440px)', async (t) => {
  // Ensure dist exists
  assert.ok(fs.existsSync(distDir), 'frontend/dist must exist before running DOM geometry tests')

  const server = createStaticServer()
  await new Promise((resolve) => server.listen(0, '127.0.0.1', resolve))
  const port = server.address().port
  const baseUrl = `http://127.0.0.1:${port}`

  const defaultBrowserPath = process.env.HOME ? path.join(process.env.HOME, '.cache/ms-playwright') : undefined
  process.env.PLAYWRIGHT_BROWSERS_PATH = process.env.PLAYWRIGHT_BROWSERS_PATH || defaultBrowserPath

  let browser
  try {
    browser = await chromium.launch({
      headless: true
    })
  } catch (err) {
    server.close()
    throw new Error(`Failed to launch Chromium: ${err.message}`)
  }

  const viewports = [
    { name: 'mobile (375x812)', width: 375, height: 812 },
    { name: 'tablet (768x1024)', width: 768, height: 1024 },
    { name: 'desktop (1024x768)', width: 1024, height: 768 },
    { name: 'wide (1440x900)', width: 1440, height: 900 }
  ]

  const routes = [
    '/',
    '/nodes',
    '/probe',
    '/subscriptions',
    '/node-groups',
    '/proxy-chains',
    '/rules',
    '/generate',
    '/settings',
    '/ledger',
    '/dns'
  ]

  try {
    for (const vp of viewports) {
      await t.test(`Viewport ${vp.name}: zero document horizontal overflow across all routes`, async () => {
        const page = await browser.newPage({
          viewport: { width: vp.width, height: vp.height }
        })

        for (const route of routes) {
          await page.goto(`${baseUrl}${route}`, { waitUntil: 'domcontentloaded' })
          await page.waitForTimeout(100)

          const overflow = await page.evaluate(() => {
            const docEl = document.documentElement
            const body = document.body
            const scrollWidth = Math.max(docEl.scrollWidth, body.scrollWidth)
            const clientWidth = docEl.clientWidth
            return {
              scrollWidth,
              clientWidth,
              hasOverflow: scrollWidth > clientWidth + 1 // 1px tolerance for subpixel antialiasing
            }
          })

          assert.equal(
            overflow.hasOverflow,
            false,
            `Route ${route} at viewport ${vp.name} has document horizontal overflow: scrollWidth=${overflow.scrollWidth} > clientWidth=${overflow.clientWidth}`
          )
        }

        await page.close()
      })
    }

    await t.test('AppModal DOM geometry & accessibility lifecycle on 1024px & 375px', async () => {
      // Test at desktop (1024px)
      const page = await browser.newPage({
        viewport: { width: 1024, height: 768 }
      })
      await page.goto(baseUrl, { waitUntil: 'domcontentloaded' })

      // Open QuickExportModal (size="md" = 640px)
      const quickExportBtn = page.locator('button[aria-label="快速导出订阅"], button:has-text("快速导出")').first()
      if (await quickExportBtn.isVisible()) {
        await quickExportBtn.click()
        await page.waitForTimeout(200)

        const modalDialog = page.locator('[role="dialog"]').first()
        await modalDialog.waitFor({ state: 'visible', timeout: 5000 })

        const box = await modalDialog.boundingBox()
        assert.ok(box, 'QuickExportModal dialog bounding box must exist')
        assert.ok(box.width <= 640 + 2, `Modal width ${box.width}px should not exceed md scale (640px)`)
        assert.ok(box.x >= 0, `Modal left ${box.x}px should be within viewport`)
        assert.ok(box.x + box.width <= 1024, `Modal right ${box.x + box.width}px should be within viewport`)

        // A11y attributes
        const ariaModal = await modalDialog.getAttribute('aria-modal')
        assert.equal(ariaModal, 'true', 'Modal must have aria-modal="true"')

        // Escape lifecycle
        await page.keyboard.press('Escape')
        await page.waitForTimeout(200)
        const isClosed = await modalDialog.isHidden()
        assert.equal(isClosed, true, 'Pressing Escape must close AppModal')
      }

      await page.close()

      // Test at mobile (375px)
      const mobilePage = await browser.newPage({
        viewport: { width: 375, height: 667 }
      })
      await mobilePage.goto(baseUrl, { waitUntil: 'domcontentloaded' })

      const mobileExportBtn = mobilePage.locator('button[aria-label="快速导出订阅"], button:has-text("快速导出")').first()
      if (await mobileExportBtn.isVisible()) {
        await mobileExportBtn.click()
        await mobilePage.waitForTimeout(200)

        const modalDialog = mobilePage.locator('[role="dialog"]').first()
        await modalDialog.waitFor({ state: 'visible', timeout: 5000 })

        const box = await modalDialog.boundingBox()
        assert.ok(box, 'QuickExportModal dialog bounding box must exist on mobile')
        assert.ok(box.width <= 375 - 16, `Modal width ${box.width}px must respect mobile gutter (<= 359px)`)
        assert.ok(box.x >= 0 && box.x + box.width <= 375, `Modal must be within 375px viewport: x=${box.x}, w=${box.width}`)

        await mobilePage.keyboard.press('Escape')
        await mobilePage.waitForTimeout(200)
        assert.equal(await modalDialog.isHidden(), true, 'Escape closes modal on mobile')
      }

      await mobilePage.close()
    })

    await t.test('AppDrawer / BaseDrawer DOM geometry (320px nav & 375px bottom sheet)', async () => {
      // Mobile 375px navigation drawer
      const mobilePage = await browser.newPage({
        viewport: { width: 375, height: 667 }
      })
      await mobilePage.goto(baseUrl, { waitUntil: 'domcontentloaded' })

      const hamburger = mobilePage.locator('button[aria-label="打开导航菜单"]').first()
      if (await hamburger.isVisible()) {
        await hamburger.click()
        await mobilePage.waitForTimeout(200)

        const navDrawer = mobilePage.locator('[role="dialog"]').first()
        await navDrawer.waitFor({ state: 'visible', timeout: 5000 })

        const box = await navDrawer.boundingBox()
        assert.ok(box, 'Navigation drawer bounding box must exist')
        assert.ok(box.width <= 320 + 2, `Left navigation drawer width ${box.width}px should not exceed 320px`)
        assert.ok(box.x <= 1, `Left navigation drawer should start at left edge: x=${box.x}`)

        await mobilePage.keyboard.press('Escape')
        await mobilePage.waitForTimeout(200)
        assert.equal(await navDrawer.isHidden(), true, 'Escape closes mobile navigation drawer')
      }

      await mobilePage.close()
    })

    await t.test('Route recovery view geometry and return action on /non-existent-recovery-route', async () => {
      const page = await browser.newPage({
        viewport: { width: 1024, height: 768 }
      })
      await page.goto(`${baseUrl}/non-existent-recovery-route`, { waitUntil: 'domcontentloaded' })
      await page.waitForTimeout(200)

      const recoveryView = page.locator('[data-testid="not-found-view"], .not-found-view')
        .or(page.getByText('404'))
        .or(page.getByText('未找到'))
        .first()
      assert.ok(await recoveryView.isVisible(), 'Recovery view must be visible for unmatched route')

      const returnAction = page.locator('a[href*="/nodes"], a[href*="/probe"]')
        .or(page.getByRole('link', { name: /返回/ }))
        .or(page.getByRole('button', { name: /返回/ }))
        .first()
      assert.ok(await returnAction.isVisible(), 'Return action to Node Ledger must be visible in recovery view')

      await page.close()
    })

    await t.test('Generate route primary export action visible in initial 1440x900 viewport', async () => {
      const page = await browser.newPage({
        viewport: { width: 1440, height: 900 }
      })
      await page.goto(`${baseUrl}/generate`, { waitUntil: 'domcontentloaded' })
      await page.waitForTimeout(200)

      const exportInputOrBtn = page.locator('button:has-text("复制链接"), button:has-text("复制"), input[readonly]').first()
      assert.ok(await exportInputOrBtn.isVisible(), 'Export URL copy action must be visible')

      const box = await exportInputOrBtn.boundingBox()
      assert.ok(box, 'Export URL copy action bounding box must exist')
      assert.ok(box.y + box.height <= 900, `Primary export URL action must be in initial 900px viewport: y=${box.y}, h=${box.height}`)

      await page.close()
    })
  } finally {
    if (browser) await browser.close()
    await new Promise((resolve) => server.close(resolve))
  }
})
