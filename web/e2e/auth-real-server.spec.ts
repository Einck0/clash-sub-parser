// @ts-nocheck
import { test, expect } from '@playwright/test'
import { spawn, execSync } from 'node:child_process'
import http from 'node:http'
import net from 'node:net'
import fs from 'node:fs'
import path from 'node:path'
import os from 'node:os'
import { fileURLToPath } from 'node:url'

const __filename = fileURLToPath(import.meta.url)
const __dirname = path.dirname(__filename)
const repoRoot = path.resolve(__dirname, '../../')

/**
 * Dynamically allocate an available ephemeral TCP port on 127.0.0.1
 */
function getFreePort(): Promise<number> {
  return new Promise((resolve, reject) => {
    const srv = net.createServer()
    srv.listen(0, '127.0.0.1', () => {
      const addr = srv.address()
      if (addr && typeof addr === 'object') {
        const port = addr.port
        srv.close(() => resolve(port))
      } else {
        srv.close(() => reject(new Error('Failed to obtain free port')))
      }
    })
    srv.on('error', reject)
  })
}

/**
 * Ensure the single-binary Go executable 'csp' is compiled and ready.
 */
function ensureCspBinary(): string {
  const binaryPath = path.join(os.tmpdir(), 'csp-e2e-test-bin')
  execSync(`go build -o "${binaryPath}" ./cmd/csp`, { cwd: repoRoot, stdio: 'pipe' })
  return binaryPath
}

/**
 * Poll the /healthz endpoint until the server is ready to handle requests.
 */
async function waitForHealthy(baseUrl: string, timeoutMs = 15_000): Promise<void> {
  const start = Date.now()
  while (Date.now() - start < timeoutMs) {
    try {
      const ok = await new Promise<boolean>((resolve) => {
        const req = http.get(`${baseUrl}/healthz`, (res) => {
          resolve(res.statusCode === 200)
        })
        req.on('error', () => resolve(false))
        req.setTimeout(1000, () => {
          req.destroy()
          resolve(false)
        })
      })
      if (ok) return
    } catch {
      // retry
    }
    await new Promise((r) => setTimeout(r, 100))
  }
  throw new Error(`Timeout waiting for CSP server at ${baseUrl} to become healthy`)
}

interface RunningServer {
  url: string
  port: number
  adminToken?: string
  stop: () => Promise<void>
}

/**
 * Start a real Go CSP server instance on a dynamic port with an isolated temporary SQLite database.
 * No mocks: all HTTP requests (static assets & /api/v1/*) are served directly by Go.
 */
async function startServer(options: { adminToken?: string } = {}): Promise<RunningServer> {
  const binaryPath = ensureCspBinary()
  const port = await getFreePort()
  const tmpDir = fs.mkdtempSync(path.join(os.tmpdir(), 'csp-e2e-'))
  const dbPath = path.join(tmpDir, 'test.db')
  const args = ['serve', '-addr', `127.0.0.1:${port}`, '-db', dbPath]
  if (options.adminToken) {
    args.push('-admin-token', options.adminToken)
  }

  const child = spawn(binaryPath, args, {
    cwd: repoRoot,
    env: {
      ...process.env,
      CSP_ADMIN_TOKEN: options.adminToken || '',
    },
    stdio: ['ignore', 'pipe', 'pipe'],
  })

  const url = `http://127.0.0.1:${port}`
  try {
    await waitForHealthy(url, 15_000)
  } catch (err) {
    child.kill('SIGTERM')
    try { fs.rmSync(tmpDir, { recursive: true, force: true }) } catch {}
    throw err
  }

  return {
    url,
    port,
    adminToken: options.adminToken,
    stop: async () => {
      return new Promise<void>((resolve) => {
        let closed = false
        child.on('close', () => {
          closed = true
          try { fs.rmSync(tmpDir, { recursive: true, force: true }) } catch {}
          resolve()
        })
        child.kill('SIGTERM')
        setTimeout(() => {
          if (!closed) {
            try { child.kill('SIGKILL') } catch {}
            try { fs.rmSync(tmpDir, { recursive: true, force: true }) } catch {}
            resolve()
          }
        }, 2000)
      })
    },
  }
}

const TEST_ADMIN_TOKEN = 'test-admin-token-2026-playwright'

let openServer: RunningServer | null = null
let openServerUrl = ''

let tokenServer: RunningServer | null = null
let tokenServerUrl = ''

test.beforeAll(async () => {
  if (process.env.CSP_OPEN_SERVER_URL) {
    openServerUrl = process.env.CSP_OPEN_SERVER_URL
  } else {
    openServer = await startServer({})
    openServerUrl = openServer.url
  }

  if (process.env.CSP_TOKEN_SERVER_URL) {
    tokenServerUrl = process.env.CSP_TOKEN_SERVER_URL
  } else {
    tokenServer = await startServer({ adminToken: TEST_ADMIN_TOKEN })
    tokenServerUrl = tokenServer.url
  }
})

test.afterAll(async () => {
  if (openServer) {
    await openServer.stop()
    openServer = null
  }
  if (tokenServer) {
    await tokenServer.stop()
    tokenServer = null
  }
})

// ============================================================================
// 对抗用例 1（脏缓存自愈断言）：注入脏 localStorage Token 刷新，断言自动净化且全网零 401
// ============================================================================
test.describe('Real Go Server E2E: Adversarial Case 1 - Dirty Cache Self-Healing in Open Mode', () => {
  test('injecting dirty localStorage token auto-purges token, produces zero 401s, and renders open mode cleanly', async ({ page, context }) => {
    const consoleErrors: string[] = []
    const http401Urls: string[] = []

    page.on('console', (msg) => {
      if (msg.type() === 'error') {
        consoleErrors.push(msg.text())
      }
    })
    page.on('pageerror', (err) => {
      consoleErrors.push(err.message)
    })
    page.on('response', (res) => {
      if (res.url().includes('/api/v1/') && res.status() === 401) {
        http401Urls.push(res.url())
      }
    })

    // Setup: Inject dirty legacy token before loading application
    await context.clearCookies()
    await page.goto(openServerUrl)
    await page.evaluate(() => {
      localStorage.setItem('csp_token', 'legacy-corrupted-dirty-bearer-token-xyz-12345')
    })
    // Reload page to simulate real user revisiting SPA with dirty cache
    await page.reload()

    // 1. Wait for SPA to finish probing and transition to Open Mode
    const authBadge = page.locator('[data-testid="header-auth-badge"]')
    await expect(authBadge).toBeVisible()
    await expect(authBadge).toContainText('Open Mode')

    // 2. Assert zero 401 HTTP responses occurred during initialization
    expect(http401Urls).toHaveLength(0)

    // 3. Assert dirty token was auto-purged from localStorage
    const storedToken = await page.evaluate(() => localStorage.getItem('csp_token'))
    expect(storedToken).toBeNull()

    // 4. Assert zero auth gate blocking the UI
    await expect(page.locator('[data-testid="auth-gate"]')).not.toBeVisible()

    // 5. Navigate to business tabs (Subscriptions, Nodes, Settings) with zero 401s
    await page.goto(`${openServerUrl}/#subscriptions`)
    await expect(page.locator('#subscriptions-title')).toBeVisible()
    await expect(page.getByRole('heading', { name: 'Subscriptions', exact: true })).toBeVisible()

    await page.goto(`${openServerUrl}/#nodes`)
    await expect(page.locator('#nodes-title')).toBeVisible()
    await expect(page.getByRole('heading', { name: 'Node ledger' })).toBeVisible()

    await page.goto(`${openServerUrl}/#settings`)
    await expect(page.locator('[data-testid="settings-view"]')).toBeVisible()
    await expect(page.locator('[data-testid="auth-mode-badge"]')).toContainText('Open Mode')

    // 6. Assert DOM has no auth errors and console is clean
    const pageText = await page.textContent('body')
    expect(pageText).not.toContain('Authentication credentials required')
    expect(pageText).not.toContain('无法连接控制面认证服务')
    expect(http401Urls).toHaveLength(0)
    expect(consoleErrors).toHaveLength(0)
  })
})

// ============================================================================
// 对抗用例 2（Open Mode 基线）：无 Token 正常访问、主题切换与响应式视口
// ============================================================================
test.describe('Real Go Server E2E: Adversarial Case 2 - Open Mode Clean Baseline & Responsive Viewports', () => {
  test('anonymous user accesses dashboard, Subscriptions and Node Ledger with zero 401s and clean rendering', async ({ page, context }) => {
    const consoleErrors: string[] = []
    const http401Urls: string[] = []

    page.on('console', (msg) => {
      if (msg.type() === 'error') {
        consoleErrors.push(msg.text())
      }
    })
    page.on('pageerror', (err) => {
      consoleErrors.push(err.message)
    })
    page.on('response', (res) => {
      if (res.url().includes('/api/v1/') && res.status() === 401) {
        http401Urls.push(res.url())
      }
    })

    // Clean session: no token in localStorage
    await context.clearCookies()
    await page.goto(openServerUrl)
    await page.evaluate(() => localStorage.clear())
    await page.reload()

    // 1. Assert header and Open Mode badge
    await expect(page.locator('header')).toBeVisible()
    const authBadge = page.locator('[data-testid="header-auth-badge"]')
    await expect(authBadge).toBeVisible()
    await expect(authBadge).toContainText('Open Mode')

    // 2. Navigate to Subscriptions view
    await page.goto(`${openServerUrl}/#subscriptions`)
    await expect(page.locator('#subscriptions-title')).toBeVisible()
    await expect(page.getByRole('heading', { name: 'Subscriptions', exact: true })).toBeVisible()
    await expect(page.locator('text=No subscriptions yet')).toBeVisible()

    // 3. Navigate to Node Ledger view
    await page.goto(`${openServerUrl}/#nodes`)
    await expect(page.locator('#nodes-title')).toBeVisible()
    await expect(page.getByRole('heading', { name: 'Node ledger' })).toBeVisible()

    // 4. Navigate to Settings view
    await page.goto(`${openServerUrl}/#settings`)
    await expect(page.locator('[data-testid="settings-view"]')).toBeVisible()
    await expect(page.getByRole('heading', { name: 'Security & Credentials' })).toBeVisible()
    await expect(page.locator('[data-testid="auth-mode-badge"]')).toContainText('Open Mode')

    // 5. Assert ZERO 401 responses occurred across all /api/v1/ endpoints
    expect(http401Urls).toHaveLength(0)

    // 6. Assert DOM does not contain any auth error
    const pageText = await page.textContent('body')
    expect(pageText).not.toContain('Authentication credentials required')
    expect(consoleErrors).toHaveLength(0)
  })

  test('switches theme and persists data-theme on real server', async ({ page, context }) => {
    await context.clearCookies()
    await page.goto(openServerUrl)

    const themeBtn = page.locator('header [aria-label="Toggle Theme"]')
    if (await themeBtn.isVisible()) {
      await themeBtn.click()
      const cyberpunkOption = page.locator('.dropdown-content button:has-text("cyberpunk")')
      if (await cyberpunkOption.isVisible()) {
        await cyberpunkOption.click()
        await expect(page.locator('html')).toHaveAttribute('data-theme', 'cyberpunk')
        await page.reload()
        await expect(page.locator('html')).toHaveAttribute('data-theme', 'cyberpunk')
      }
    }
  })

  test('375px viewport has zero horizontal overflow and preserves interactive header controls', async ({ page, context }) => {
    await context.clearCookies()
    await page.setViewportSize({ width: 375, height: 667 })
    await page.goto(openServerUrl)
    await page.waitForLoadState('domcontentloaded')

    // 1. Verify three interactive entrances exist and are visible
    const authBadge = page.locator('[data-testid="header-auth-badge"]')
    await expect(authBadge).toBeVisible()

    const healthRefreshBtn = page.locator('header button[aria-label="Refresh health"]')
    await expect(healthRefreshBtn).toBeVisible()

    const themeBtn = page.locator('header [aria-label="Toggle Theme"]')
    await expect(themeBtn).toBeVisible()

    // 2. In modern architecture, clicking authBadge navigates to settings view (no popup modal)
    await authBadge.click()
    await expect(page.locator('[data-testid="settings-view"]')).toBeVisible()

    // 3. Assert documentElement.scrollWidth <= documentElement.clientWidth across all core views
    const testViews = ['', '#subscriptions', '#nodes', '#probes', '#policy', '#publications', '#settings']
    for (const view of testViews) {
      if (view) {
        await page.goto(`${openServerUrl}/${view}`)
      }
      await page.waitForTimeout(50)

      const metrics = await page.evaluate(() => {
        const doc = document.documentElement
        const body = document.body
        const header = document.querySelector('header')
        return {
          scrollWidth: doc.scrollWidth,
          clientWidth: doc.clientWidth,
          bodyScrollWidth: body.scrollWidth,
          bodyClientWidth: body.clientWidth,
          headerScrollWidth: header?.scrollWidth ?? 0,
          headerClientWidth: header?.clientWidth ?? 0,
        }
      })

      expect(metrics.scrollWidth).toBeLessThanOrEqual(metrics.clientWidth)
      expect(metrics.bodyScrollWidth).toBeLessThanOrEqual(metrics.bodyClientWidth)
      expect(metrics.headerScrollWidth).toBeLessThanOrEqual(metrics.headerClientWidth)
    }

    // 4. Capture screenshot artifact for downstream evidence handoff (in isolated tmpdir unless overridden)
    const screenshotDir = process.env.SCREENSHOTS_DIR || path.join(os.tmpdir(), 'csp-e2e-screenshots')
    fs.mkdirSync(screenshotDir, { recursive: true })
    const screenshotPath = path.resolve(screenshotDir, 'mobile-375-header-no-overflow.png')
    await page.screenshot({ path: screenshotPath, fullPage: false })
  })
})

// ============================================================================
// 对抗用例 3（Protected Mode）：受控门禁阻断与正确登录
// ============================================================================
test.describe('Real Go Server E2E: Adversarial Case 3 - Protected Mode (Controlled Gate & Authentication)', () => {
  test('anonymous access displays AuthGate, inputting valid token self-heals view to 200 OK', async ({ page, context }) => {
    const http401Urls: string[] = []
    page.on('response', (res) => {
      // Monitor if any business API was blindly requested and triggered 401
      if (res.url().includes('/api/v1/subscriptions') || res.url().includes('/api/v1/nodes')) {
        if (res.status() === 401) {
          http401Urls.push(res.url())
        }
      }
    })

    // Clean session: no token in localStorage
    await context.clearCookies()
    await page.goto(tokenServerUrl)
    await page.evaluate(() => localStorage.clear())
    await page.reload()

    // 1. Assert header displays Protected badge
    const authBadge = page.locator('[data-testid="header-auth-badge"]')
    await expect(authBadge).toBeVisible()
    await expect(authBadge).toContainText('Protected')

    // 2. AuthGate must be visible and blocking business views
    const gate = page.locator('[data-testid="auth-gate"]')
    await expect(gate).toBeVisible()
    await expect(gate).toContainText('控制面受限访问门禁')

    // Assert zero blind business API calls triggered 401
    expect(http401Urls).toHaveLength(0)

    // 3. Test empty input validation
    const tokenInput = page.locator('[data-testid="auth-gate-token-input"]')
    await expect(tokenInput).toBeVisible()
    await tokenInput.fill('')
    const submitBtn = page.locator('[data-testid="auth-gate-submit"]')
    await submitBtn.click()

    const gateError = page.locator('[data-testid="auth-gate-error"]')
    await expect(gateError).toBeVisible()
    await expect(gateError).toContainText('令牌不能为空')

    // 4. Test invalid token failure
    await tokenInput.fill('invalid-token-guess-123')
    await submitBtn.click()
    await expect(gateError).toBeVisible()
    await expect(gateError).toContainText('令牌无效或已过期，请检查后重试')

    // 5. Test password visibility toggle
    const toggleBtn = page.locator('[data-testid="auth-gate-toggle-visibility"]')
    await expect(tokenInput).toHaveAttribute('type', 'password')
    await toggleBtn.click()
    await expect(tokenInput).toHaveAttribute('type', 'text')
    await toggleBtn.click()
    await expect(tokenInput).toHaveAttribute('type', 'password')

    // 6. Enter valid token and submit
    await tokenInput.fill(TEST_ADMIN_TOKEN)
    await submitBtn.click()

    // 7. Assert AuthGate is unmounted
    await expect(gate).not.toBeVisible()

    // 8. Assert token is persisted in localStorage['csp_token']
    const savedToken = await page.evaluate(() => localStorage.getItem('csp_token'))
    expect(savedToken).toBe(TEST_ADMIN_TOKEN)

    // 9. Navigate to Subscriptions view: loads with 200 OK
    await page.goto(`${tokenServerUrl}/#subscriptions`)
    await expect(page.locator('#subscriptions-title')).toBeVisible()
    await expect(page.locator('text=No subscriptions yet')).toBeVisible()

    // 10. Subsequent navigation to Node Ledger succeeds without re-prompting
    await page.goto(`${tokenServerUrl}/#nodes`)
    await expect(page.locator('#nodes-title')).toBeVisible()
    await expect(page.getByRole('heading', { name: /node ledger/i })).toBeVisible()
    await expect(gate).not.toBeVisible()
  })
})

// ============================================================================
// 对抗用例 4（逃生通道）：点击清空凭据立即自愈
// ============================================================================
test.describe('Real Go Server E2E: Adversarial Case 4 - Escape Hatch (Emergency Local Storage Reset)', () => {
  test('clicking escape hatch in AuthGate resets stored credentials and restores healthy gate ready state', async ({ page, context }) => {
    await context.clearCookies()
    await page.goto(tokenServerUrl)
    // Inject a stale or corrupted token into localStorage
    await page.evaluate(() => {
      localStorage.setItem('csp_token', 'stale-corrupted-token-locking-browser')
    })
    await page.reload()

    // AuthGate should be visible
    const gate = page.locator('[data-testid="auth-gate"]')
    await expect(gate).toBeVisible()

    const tokenInput = page.locator('[data-testid="auth-gate-token-input"]')
    await tokenInput.fill('some-staged-input')

    // Click escape hatch reset button
    const resetBtn = page.locator('[data-testid="auth-gate-reset"]')
    await expect(resetBtn).toBeVisible()
    await resetBtn.click()

    // Assert token in localStorage is immediately cleared
    const storedToken = await page.evaluate(() => localStorage.getItem('csp_token'))
    expect(storedToken).toBeNull()

    // Assert input field was cleared
    await expect(tokenInput).toHaveValue('')

    // System is clean, can now authenticate with proper credentials
    await tokenInput.fill(TEST_ADMIN_TOKEN)
    await page.locator('[data-testid="auth-gate-submit"]').click()
    await expect(gate).not.toBeVisible()
  })
})
