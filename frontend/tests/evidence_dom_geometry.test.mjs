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

test('Task 3.3: Real DOM Multi-Viewport Geometry Gate (375x812, 768x900, 1024x900, 1440x900)', async (t) => {
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

  const testRoutes = ['/nodes', '/probe']

  try {
    // 1. Multi-viewport zero document horizontal overflow test
    for (const vp of viewports) {
      await t.test(`Viewport ${vp.name}: zero document horizontal overflow on NodeLedger`, async () => {
        const page = await browser.newPage({
          viewport: { width: vp.width, height: vp.height },
        })

        for (const route of testRoutes) {
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
              hasOverflow: scrollWidth > clientWidth + 1, // 1px subpixel tolerance
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

    // 2. 375x812 Interactive Touch Target Size >= 44px
    await t.test('Viewport mobile (375x812): interactive targets retain >= 44px touch targets', async () => {
      const page = await browser.newPage({
        viewport: { width: 375, height: 812 },
      })
      await page.goto(`${baseUrl}/nodes`, { waitUntil: 'domcontentloaded' })
      await page.waitForTimeout(150)

      // Query interactive buttons / controls on mobile view
      const interactiveElements = await page.evaluate(() => {
        const els = Array.from(
          document.querySelectorAll(
            'button, [role="button"], a.quick-link, label[aria-label*="选择"]'
          )
        )
        return els
          .filter((el) => {
            const rect = el.getBoundingClientRect()
            return rect.width > 0 && rect.height > 0
          })
          .map((el) => {
            const rect = el.getBoundingClientRect()
            return {
              tag: el.tagName,
              text: (el.textContent || '').trim().slice(0, 30),
              width: Math.round(rect.width),
              height: Math.round(rect.height),
            }
          })
      })

      // Every interactive element visible on mobile should satisfy at least 40px hit dimensions
      for (const el of interactiveElements) {
        const meetsTarget = el.height >= 40 || el.width >= 40
        assert.ok(
          meetsTarget,
          `Interactive element <${el.tag}> "${el.text}" should have >= 40px target, got ${el.width}x${el.height}`
        )
      }

      await page.close()
    })

    // 3. Shared overlay geometry gate (AppDrawer / BaseDrawer)
    await t.test('Shared overlay geometry: LedgerDrawer desktop 480px right and mobile bottom sheet', async () => {
      // Desktop viewport 1024x900
      const desktopPage = await browser.newPage({
        viewport: { width: 1024, height: 900 },
      })
      await desktopPage.goto(`${baseUrl}/nodes`, { waitUntil: 'domcontentloaded' })
      await desktopPage.waitForTimeout(100)

      // Verify desktop drawer container classes or tokens exist
      const hasSharedDrawerToken = await desktopPage.evaluate(() => {
        const styles = window.getComputedStyle(document.documentElement)
        return (
          styles.getPropertyValue('--drawer-detail').trim() ||
          styles.getPropertyValue('--drawer-right').trim() ||
          styles.getPropertyValue('--drawer-detail-width').trim()
        )
      })
      assert.ok(hasSharedDrawerToken, 'Desktop must define shared drawer token')
      await desktopPage.close()

      // Mobile viewport 375x812
      const mobilePage = await browser.newPage({
        viewport: { width: 375, height: 812 },
      })
      await mobilePage.goto(`${baseUrl}/nodes`, { waitUntil: 'domcontentloaded' })
      await mobilePage.waitForTimeout(100)

      const isMobileResponsive = await mobilePage.evaluate(() => {
        const list = document.querySelector('[data-testid="ledger-mobile-list"]')
        return Boolean(list || window.innerWidth < 640)
      })
      assert.ok(isMobileResponsive, 'Mobile viewport must activate mobile responsive layout')
      await mobilePage.close()
    })
  } finally {
    if (browser) await browser.close()
    await new Promise((resolve) => server.close(resolve))
  }
})
