import http from 'node:http'
import fs from 'node:fs'
import path from 'node:path'
import { chromium } from '@playwright/test'

const distDir = path.resolve(import.meta.dirname, '../dist')
const outputDir = path.resolve(import.meta.dirname, '../evidence_screenshots')

if (!fs.existsSync(outputDir)) {
  fs.mkdirSync(outputDir, { recursive: true })
}

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

const mockNodes = [
  {
    name: 'JP-Tokyo-01',
    node_key: 'node:jp01',
    type: 'vmess',
    server: 'jp.example.com',
    port: 443,
    uuid: 'a1b2c3d4-e5f6-7890-abcd-ef1234567890',
    alterId: 0,
    cipher: 'auto',
    source: 'Tokyo Fast',
  },
  {
    name: 'US-SiliconValley-02',
    node_key: 'node:us02',
    type: 'ss',
    server: 'us.example.com',
    port: 8443,
    cipher: 'aes-256-gcm',
    source: 'US Premium',
  },
  {
    name: 'SG-Singapore-03',
    node_key: 'node:sg03',
    type: 'trojan',
    server: 'sg.example.com',
    port: 443,
    source: 'SG Direct',
  },
  {
    name: 'HK-Central-04',
    node_key: 'node:hk04',
    type: 'vless',
    server: 'hk.example.com',
    port: 443,
    source: 'HK Express',
  },
]

const mockProbeSummary = {
  results: {
    'node:jp01': {
      status: 'ok',
      latency_ms: 38,
      speed_mbps: 82.5,
      country: 'JP',
      ip: '203.0.113.195',
      media: { youtube: true, netflix: true, disney: false },
    },
    'node:us02': {
      status: 'ok',
      latency_ms: 142,
      speed_mbps: 45.2,
      country: 'US',
      ip: '198.51.100.44',
      media: { youtube: true, netflix: false, disney: false },
    },
    'node:sg03': {
      status: 'ok',
      latency_ms: 65,
      speed_mbps: 95.0,
      country: 'SG',
      ip: '203.0.113.88',
      media: { youtube: true, netflix: true, disney: true },
    },
    'node:hk04': {
      status: 'fail',
      latency_ms: null,
      speed_mbps: null,
      country: 'HK',
      ip: null,
      media: { youtube: false, netflix: false, disney: false },
    },
  },
  next_cursor: null,
  has_more: false,
}

const mockDetail = {
  status: 'ok',
  latency_ms: 38,
  speed_mbps: 82.5,
  country: 'JP',
  ip: '203.0.113.195',
  media: {
    chatgpt: {
      status: 'verified',
      verdict: 'full',
      region: 'JP',
      confidence: 'high',
      evidence_version: 'catalogue-2026-09-05',
      checked_at: 1788600000,
      evidence: { http_status: 200, signals: ['chat_model_ok', 'native_region'] },
    },
    gemini: {
      status: 'verified',
      verdict: 'full',
      region: 'JP',
      confidence: 'high',
      evidence_version: 'catalogue-2026-09-05',
      checked_at: 1788600000,
      evidence: { http_status: 200, signals: ['gemini_api_ok'] },
    },
    youtube: {
      status: 'verified',
      verdict: 'full',
      region: 'JP',
      confidence: 'high',
      evidence_version: 'catalogue-2026-09-05',
      checked_at: 1788600000,
      evidence: { http_status: 200, signals: ['playback_ok', 'premium_supported'] },
    },
    netflix: {
      status: 'verified',
      verdict: 'full',
      region: 'JP',
      confidence: 'high',
      evidence_version: 'catalogue-2026-09-05',
      checked_at: 1788600000,
      evidence: { http_status: 200, signals: ['licensed_titles_verified'] },
    },
    disney: {
      status: 'restricted',
      verdict: 'unsupported_region',
      region: '',
      confidence: 'high',
      evidence_version: 'catalogue-2026-09-05',
      checked_at: 1788600000,
      evidence: { http_status: 403, error_code: 'ERR_REGION_RESTRICTED' },
    },
  },
  identity_evidence: {
    consensus_ip: '203.0.113.195',
    consensus_country: 'JP',
    sources: [
      { provider: 'ipinfo', ip: '203.0.113.195', country: 'JP' },
      { provider: 'cloudflare', ip: '203.0.113.195', country: 'JP' },
    ],
  },
  diagnostics: [
    { phase: 'tcp_ping', latency_ms: 38, status: 'ok' },
    { phase: 'speedtest', speed_mbps: 82.5, status: 'ok' },
  ],
}

async function run() {
  const server = createStaticServer()
  await new Promise((resolve) => server.listen(0, '127.0.0.1', resolve))
  const port = server.address().port
  const baseUrl = `http://127.0.0.1:${port}`

  console.log(`Static server started at ${baseUrl}`)

  const defaultBrowserPath = process.env.HOME ? path.join(process.env.HOME, '.cache/ms-playwright') : undefined
  process.env.PLAYWRIGHT_BROWSERS_PATH = process.env.PLAYWRIGHT_BROWSERS_PATH || defaultBrowserPath

  const browser = await chromium.launch({ headless: true })

  const viewports = [
    { name: 'desktop_1440', width: 1440, height: 900 },
    { name: 'tablet_768', width: 768, height: 900 },
    { name: 'mobile_375', width: 375, height: 812 },
  ]

  const themes = ['dark', 'light']

  for (const theme of themes) {
    for (const vp of viewports) {
      console.log(`Capturing ${theme} mode on ${vp.name} (${vp.width}x${vp.height})...`)

      const page = await browser.newPage({
        viewport: { width: vp.width, height: vp.height },
      })

      page.on('console', (msg) => console.log(`[Browser Console ${msg.type()}]:`, msg.text()))
      page.on('pageerror', (err) => console.error(`[Browser PageError]:`, err))

      // Route mock API
      await page.route('**/api/**', async (route) => {
        const url = route.request().url()
        if (url.includes('/node-ledger') || url.includes('/api/nodes')) {
          await route.fulfill({
            status: 200,
            contentType: 'application/json',
            body: JSON.stringify(mockNodes),
          })
        } else if (url.includes('/api/probe/results/detail') || url.includes('/api/probe/results/node:')) {
          await route.fulfill({
            status: 200,
            contentType: 'application/json',
            body: JSON.stringify(mockDetail),
          })
        } else if (url.includes('/api/probe/results')) {
          await route.fulfill({
            status: 200,
            contentType: 'application/json',
            body: JSON.stringify(mockProbeSummary),
          })
        } else {
          await route.fulfill({
            status: 200,
            contentType: 'application/json',
            body: JSON.stringify([]),
          })
        }
      })

      // Set initial theme in local storage
      await page.addInitScript((th) => {
        localStorage.setItem('clash-sub-theme', th)
        if (document.documentElement) {
          document.documentElement.setAttribute('data-theme', th)
        }
      }, theme)

      await page.goto(`${baseUrl}/nodes`, { waitUntil: 'networkidle' })
      await page.waitForTimeout(300)

      // Ensure theme applied
      await page.evaluate((th) => {
        document.documentElement.setAttribute('data-theme', th)
      }, theme)
      await page.waitForTimeout(100)

      // 1. Overview screenshot
      const overviewPath = path.join(outputDir, `${theme}_${vp.name}_overview.png`)
      await page.screenshot({ path: overviewPath, fullPage: false })
      console.log(`Saved: ${overviewPath}`)

      // 2. Open drawer if desktop or mobile
      if (vp.width >= 1024) {
        // Click on first row to inspect node
        const firstRow = page.locator('.table-row-dense').first()
        if (await firstRow.isVisible()) {
          await firstRow.click()
          await page.waitForTimeout(600)
          const drawerPath = path.join(outputDir, `${theme}_${vp.name}_drawer.png`)
          await page.screenshot({ path: drawerPath, fullPage: false })
          console.log(`Saved: ${drawerPath}`)
        }
      } else if (vp.width === 375) {
        // Click on first mobile card
        const firstCard = page.locator('[data-testid="ledger-mobile-card"]').first()
        if (await firstCard.isVisible()) {
          await firstCard.click()
          await page.waitForTimeout(600)
          const drawerMobilePath = path.join(outputDir, `${theme}_${vp.name}_drawer_bottomsheet.png`)
          await page.screenshot({ path: drawerMobilePath, fullPage: false })
          console.log(`Saved: ${drawerMobilePath}`)
          const drawerPath = path.join(outputDir, `${theme}_${vp.name}_drawer.png`)
          await page.screenshot({ path: drawerPath, fullPage: false })
          console.log(`Saved: ${drawerPath}`)
        }
      }

      await page.close()
    }
  }

  await browser.close()
  server.close()
  console.log(`All screenshots captured successfully in ${outputDir}`)
}

run().catch((err) => {
  console.error('Failed to capture evidence:', err)
  process.exit(1)
})
