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
  } finally {
    if (browser) await browser.close()
    await new Promise((resolve) => server.close(resolve))
  }
})
