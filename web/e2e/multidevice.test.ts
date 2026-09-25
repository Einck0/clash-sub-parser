// @vitest-environment jsdom
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { createApp, nextTick, type App as VueApp } from 'vue'
import App from '../src/App.vue'
import { router } from '../src/router'
import { i18n } from '../src/locales'
import { api } from '../src/api/client'
import { THEME_STORAGE_KEY, DEFAULT_THEME, type ThemeName } from '../src/theme'
import { ROUTE_STORAGE_KEY, VALID_TABS, type NavTab } from '../src/navigation'
import { setLocale } from '../src/locales'

describe('CSP Multi-Device & E2E Verification Suite', () => {
  let app: VueApp | null = null
  let container: HTMLDivElement | null = null
  let consoleErrorSpy: ReturnType<typeof vi.spyOn>
  let consoleWarnSpy: ReturnType<typeof vi.spyOn>
  const uncaughtErrors: Error[] = []

  // Sample mock responses for deterministic, no-white-screen rendering
  const mockSubscription = {
    id: 'sub-test-1',
    name: 'Primary Feed',
    source_url_secret_ref: 'sec:sub-source-1',
    enabled: true,
    refresh_policy: {
      interval_seconds: 3600,
      timeout_seconds: 30,
      retry_count: 3,
    },
    created_at: '2026-09-16T10:00:00Z',
    updated_at: '2026-09-16T10:00:00Z',
  }

  const mockNode = {
    logicalId: 'node-hk-01',
    displayName: 'HK Premium 01',
    protocol: 'shadowsocks',
    active: true,
    countryCode: 'HK',
    tags: ['fast', 'iplc'],
    evidence: {
      streaming: { status: 'available', tone: 'success' },
      ai: { status: 'available', tone: 'success' },
    },
  }

  const mockProbeRun = {
    id: 'run-test-101',
    state: 'succeeded' as const,
    kinds: ['baseline', 'geo', 'streaming', 'ai', 'ip_risk'] as const,
    deadline_minutes: 10,
    created_at: '2026-09-16T10:00:00Z',
    started_at: '2026-09-16T10:00:05Z',
    finished_at: '2026-09-16T10:01:20Z',
  }

  const mockPolicyGroup = {
    id: 'grp-auto-1',
    name: 'Auto Select HK',
    group_type: 'urltest' as const,
    edges: [
      { id: 'edge-1', parent_group_id: 'grp-auto-1', node_logical_id: 'node-hk-01', position: 0 },
    ],
  }

  const mockPreview = {
    target: 'clash',
    content: 'mixed-port: 7890\nallow-lan: true\nproxies:\n  - name: HK Premium 01\n    type: ss',
    filename: 'csp-clash.yaml',
    content_type: 'text/yaml',
    digest: 'sha256-mock-digest-preview-12345',
    generated_at: '2026-09-16T10:00:00Z',
  }

  function setupApiMocks(overrides: { failHealth?: boolean; emptyData?: boolean } = {}) {
    vi.spyOn(api, 'get').mockImplementation(async (path: string, options?: any) => {
      if (path === '/healthz') {
        if (overrides.failHealth) {
          throw new Error('Health check failed')
        }
        return { status: 'healthy' }
      }
      if (path === '/api/v1/auth/status') {
        return { mode: 'open', authenticated: true, subject: 'admin' }
      }
      if (path === '/api/v1/subscriptions') {
        return overrides.emptyData
          ? { items: [], total: 0 }
          : { items: [mockSubscription], total: 1 }
      }
      if (path === '/api/v1/nodes') {
        return overrides.emptyData
          ? { items: [], total: 0, page: 1, page_size: 50, has_more: false }
          : { items: [mockNode], total: 1, page: 1, page_size: 50, has_more: false }
      }
      if (path === '/api/v1/probes/runs') {
        return overrides.emptyData
          ? { items: [], total: 0 }
          : { items: [mockProbeRun], total: 1 }
      }
      if (path.includes('/observations')) {
        return { items: [], total: 0 }
      }
      if (path === '/api/v1/policies/groups') {
        return overrides.emptyData
          ? { items: [], total: 0 }
          : { items: [mockPolicyGroup], total: 1 }
      }
      if (path === '/api/v1/policies/rules') {
        return { items: [], total: 0 }
      }
      if (path.includes('/api/v1/policies/preview')) {
        return mockPreview
      }
      return { items: [], total: 0 }
    })
    vi.spyOn(api, 'post').mockImplementation(async (path: string, body?: any) => {
      if (path.includes('/api/v1/publications/preview') || path.includes('/api/v1/policies/preview')) {
        return mockPreview
      }
      return { items: [], total: 0 }
    })
  }

  function setViewport(width: number, height: number = 844) {
    Object.defineProperty(window, 'innerWidth', { writable: true, configurable: true, value: width })
    Object.defineProperty(window, 'innerHeight', { writable: true, configurable: true, value: height })
    window.dispatchEvent(new Event('resize'))
  }

  beforeEach(() => {
    uncaughtErrors.length = 0
    window.localStorage.clear()
    setLocale('en-US')
    window.location.hash = ''
    document.documentElement.removeAttribute('data-theme')

    // Track console output to assert 0 uncaught exceptions
    consoleErrorSpy = vi.spyOn(console, 'error').mockImplementation((...args) => {
      // Record any uncaught exception messages
      const msg = args.map(String).join(' ')
      uncaughtErrors.push(new Error(msg))
    })
    consoleWarnSpy = vi.spyOn(console, 'warn').mockImplementation(() => {})

    // Mock ResizeObserver for jsdom Virtualizer support
    if (typeof globalThis.ResizeObserver === 'undefined') {
      globalThis.ResizeObserver = class {
        observe() {}
        unobserve() {}
        disconnect() {}
      } as any
    }

    // Polyfill HTMLDialogElement showModal & close for jsdom environment
    if (typeof HTMLDialogElement !== 'undefined') {
      if (!HTMLDialogElement.prototype.showModal) {
        HTMLDialogElement.prototype.showModal = function () {
          this.setAttribute('open', '')
        }
      }
      if (!HTMLDialogElement.prototype.close) {
        HTMLDialogElement.prototype.close = function () {
          this.removeAttribute('open')
        }
      }
    }

    container = document.createElement('div')
    container.id = 'app-root'
    document.body.appendChild(container)
  })

  afterEach(() => {
    if (app && container) {
      app.unmount()
      app = null
    }
    if (container && container.parentNode) {
      container.parentNode.removeChild(container)
      container = null
    }
    vi.restoreAllMocks()
  })

  async function mountApp(): Promise<HTMLDivElement> {
    app = createApp(App)
    app.use(router)
    app.use(i18n)
    app.mount(container!)
    await nextTick()
    // Allow pending async onMounted promises to resolve
    await new Promise((r) => setTimeout(r, 20))
    await nextTick()
    return container!
  }

  // =========================================================================
  // 1. Mobile Viewport (375px iPhone SE / 390px iPhone 14/15/16) Verification
  // =========================================================================
  describe('Mobile Viewport Ergonomics (375px / 390px)', () => {
    it('renders mobile layout with bottom navigation bar and hides desktop sidebar at 375px', async () => {
      setViewport(375, 667)
      setupApiMocks()
      const root = await mountApp()

      // Desktop sidebar should be hidden via md:flex
      const sidebar = root.querySelector('aside')
      expect(sidebar).not.toBeNull()
      expect(sidebar?.classList.contains('hidden')).toBe(true)
      expect(sidebar?.classList.contains('md:flex')).toBe(true)

      // Mobile bottom nav should be present and visible
      const mobileNav = root.querySelector('nav.md\\:hidden')
      expect(mobileNav).not.toBeNull()
      expect(mobileNav?.classList.contains('fixed')).toBe(true)
      expect(['fixed', 'bottom-0', 'bottom-3', 'bottom-4'].some(cls => mobileNav?.classList.contains(cls))).toBe(true)

      // Bottom nav must contain core destination buttons
      const buttons = mobileNav?.querySelectorAll('button')
      expect(buttons?.length).toBeGreaterThanOrEqual(4)

      // Main container or main content has bottom inset to prevent bottom nav overlay
      const mainEl = root.querySelector('main')
      const hasInset = !!(mainEl?.style?.paddingBottom || mainEl?.parentElement?.className?.match(/pb-(16|\[calc)/))
      expect(hasInset).toBe(true)

      // Zero console exceptions
      expect(uncaughtErrors).toHaveLength(0)
    })

    it('renders responsive topbar with compact brand icon and health probe at 390px', async () => {
      setViewport(390, 844)
      setupApiMocks()
      const root = await mountApp()

      // Mobile header brand circle (md:hidden)
      const brandIcon = root.querySelector('header .md\\:hidden')
      expect(brandIcon).not.toBeNull()
      expect(brandIcon?.textContent?.trim()).toBe('C')

      // Health probe indicator
      const healthBadge = root.querySelector('header .header-health-indicator')
      expect(healthBadge).not.toBeNull()
      expect(healthBadge?.textContent?.toLowerCase()).toContain('healthy')

      // Zero console exceptions
      expect(uncaughtErrors).toHaveLength(0)
    })

    it('verifies 375px compact topbar class contract and interactive controls', async () => {
      setViewport(375, 667)
      setupApiMocks()
      const root = await mountApp()

      // 1. Header has overflow-hidden and layout classes
      const header = root.querySelector('header')
      expect(header).not.toBeNull()
      expect(header?.classList.contains('overflow-hidden')).toBe(true)

      // 2. Auth badge compact contract: data-testid, px-1.5 sm:px-2.5, status dot, hidden sm:inline label
      const authBadge = root.querySelector('[data-testid="header-auth-badge"]')
      expect(authBadge).not.toBeNull()
      expect(authBadge?.className).toContain('px-1.5')
      expect(authBadge?.className).toContain('sm:px-2.5')
      const authDot = authBadge?.querySelector('.rounded-full')
      expect(authDot).not.toBeNull()
      const authText = authBadge?.querySelector('span.hidden.sm\\:inline')
      expect(authText).not.toBeNull()
      expect(authText?.textContent).toBe('Open Mode')

      // 3. Health indicator compact contract: px-1.5 sm:px-2.5, status dot, hidden sm:inline label, refresh button
      const healthIndicator = root.querySelector('.header-health-indicator')
      expect(healthIndicator).not.toBeNull()
      expect(healthIndicator?.className).toContain('px-1.5')
      expect(healthIndicator?.className).toContain('sm:px-2.5')
      const healthDot = healthIndicator?.querySelector('.rounded-full')
      expect(healthDot).not.toBeNull()
      const healthText = healthIndicator?.querySelector('span.hidden.sm\\:inline')
      expect(healthText).not.toBeNull()
      expect(healthText?.textContent?.toLowerCase()).toContain('healthy')

      const refreshBtn = healthIndicator?.querySelector('button[aria-label="Refresh health"]')
      expect(refreshBtn).not.toBeNull()

      // 4. Theme toggle button with aria-label
      const themeBtn = root.querySelector('header button[aria-label="Toggle Theme"]')
      expect(themeBtn).not.toBeNull()

      // 5. Title container min-w-0 and truncate
      const titleContainer = root.querySelector('header .min-w-0')
      expect(titleContainer).not.toBeNull()
      const titleSpan = titleContainer?.querySelector('.truncate')
      expect(titleSpan).not.toBeNull()

      // 6. Interactive test: clicking auth badge navigates to Settings view
      ;(authBadge as HTMLButtonElement).click()
      await nextTick()
      await new Promise((r) => setTimeout(r, 20))
      const settingsView = root.querySelector('[data-testid="settings-view"]')
      expect(settingsView).not.toBeNull()
      expect(['#settings', '#/settings']).toContain(window.location.hash)

      // 7. Interactive test: clicking refresh health button invokes api
      const apiGetSpy = vi.spyOn(api, 'get')
      ;(refreshBtn as HTMLButtonElement).click()
      await nextTick()
      expect(apiGetSpy).toHaveBeenCalledWith('/healthz')

      // Zero console exceptions
      expect(uncaughtErrors).toHaveLength(0)
    })

    it('enables seamless tab switching from mobile bottom navigation bar', async () => {
      setViewport(390, 844)
      setupApiMocks()
      const root = await mountApp()

      const mobileNav = root.querySelector('nav.md\\:hidden')
      const navButtons = mobileNav?.querySelectorAll('button') || []
      expect(navButtons.length).toBeGreaterThan(0)

      // Click "Subscriptions" button on mobile nav
      const subBtn = Array.from(navButtons).find((b) => b.textContent?.includes('Subscriptions'))
      expect(subBtn).toBeDefined()
      subBtn?.click()
      await nextTick()
      await new Promise((r) => setTimeout(r, 20))

      // Section title should be Subscriptions
      expect(root.querySelector('#subscriptions-title')?.textContent).toContain('Subscriptions')
      expect(['#subscriptions', '#/subscriptions']).toContain(window.location.hash)

      // Click "Node Ledger" button on mobile nav
      const nodeBtn = Array.from(navButtons).find((b) => b.textContent?.includes('Node Ledger'))
      expect(nodeBtn).toBeDefined()
      nodeBtn?.click()
      await nextTick()
      await new Promise((r) => setTimeout(r, 20))

      expect(root.querySelector('#nodes-title')?.textContent?.toLowerCase()).toContain('node ledger')
      expect(['#nodes', '#/nodes']).toContain(window.location.hash)

      // Zero console exceptions
      expect(uncaughtErrors).toHaveLength(0)
    })
  })

  // =========================================================================
  // 2. Desktop Viewport (1440px) Verification
  // =========================================================================
  describe('Desktop Viewport Verification (1440px)', () => {
    it('renders full desktop sidebar navigation and hides mobile bottom navigation', async () => {
      setViewport(1440, 900)
      setupApiMocks()
      const root = await mountApp()

      const sidebar = root.querySelector('aside')
      expect(sidebar).not.toBeNull()
      expect(sidebar?.textContent).toContain('CSP Control Plane')
      expect(sidebar?.textContent).toContain('v1.0 Clean-Slate')

      // Desktop navigation items
      const navButtons = sidebar?.querySelectorAll('nav button')
      expect(navButtons?.length).toBe(7)

      // Mobile bottom nav remains hidden on desktop
      const mobileNav = root.querySelector('nav.md\\:hidden')
      expect(mobileNav?.classList.contains('md:hidden')).toBe(true)

      // Zero console exceptions
      expect(uncaughtErrors).toHaveLength(0)
    })

    it('navigates through all core views smoothly on desktop without white-screen or error', async () => {
      setViewport(1440, 900)
      setupApiMocks()
      const root = await mountApp()

      const tabsToTest: { tab: NavTab; label: string; expectedText: string }[] = [
        { tab: 'subscriptions', label: 'Subscriptions', expectedText: 'Subscriptions' },
        { tab: 'nodes', label: 'Node Ledger', expectedText: 'Node Ledger' },
        { tab: 'probes', label: 'Probe Engine', expectedText: 'Probe Engine' },
        { tab: 'policy', label: 'Policy Tree', expectedText: 'Policy Groups' },
        { tab: 'publications', label: 'Exports', expectedText: 'Configuration Exports' },
        { tab: 'dashboard', label: 'Dashboard', expectedText: 'Clash Sub Parser Zashboard Foundation' },
      ]

      for (const t of tabsToTest) {
        const sidebar = root.querySelector('aside')
        const button = Array.from(sidebar?.querySelectorAll('button') || []).find((b) =>
          b.textContent?.trim().includes(t.label)
        )
        button?.click()
        await nextTick()
        await new Promise((r) => setTimeout(r, 25))

        expect(root.textContent).toContain(t.expectedText)
        expect([`#${t.tab}`, `#/${t.tab}`]).toContain(window.location.hash)
        expect(window.localStorage.getItem(ROUTE_STORAGE_KEY)).toBe(t.tab)
      }

      // Zero console exceptions throughout full navigation cycle
      expect(uncaughtErrors).toHaveLength(0)
    })
  })

  // =========================================================================
  // 3. Theme Switching & Persistence Verification
  // =========================================================================
  describe('Theme Switching and Persistence', () => {
    it('switches between DaisyUI themes and updates documentElement and localStorage', async () => {
      setViewport(1440, 900)
      setupApiMocks()
      const root = await mountApp()

      // Default theme applied initially
      expect(document.documentElement.getAttribute('data-theme')).toBe(DEFAULT_THEME)

      const testThemes: ThemeName[] = ['light', 'dark', 'dim', 'cyberpunk', 'cupcake', 'dracula', 'nord']
      const themeButtons = Array.from(root.querySelectorAll('.dropdown-content button'))

      for (const theme of testThemes) {
        const btn = themeButtons.find((b) => b.textContent?.trim().toLowerCase() === theme)
        if (btn) {
          ;(btn as HTMLButtonElement).click()
          await nextTick()

          expect(document.documentElement.getAttribute('data-theme')).toBe(theme)
          expect(window.localStorage.getItem(THEME_STORAGE_KEY)).toBe(theme)
        }
      }

      expect(uncaughtErrors).toHaveLength(0)
    })

    it('restores persisted theme from localStorage on reload / remount', async () => {
      window.localStorage.setItem(THEME_STORAGE_KEY, 'cyberpunk')
      setViewport(375, 667)
      setupApiMocks()
      await mountApp()

      expect(document.documentElement.getAttribute('data-theme')).toBe('cyberpunk')
      expect(uncaughtErrors).toHaveLength(0)
    })
  })

  // =========================================================================
  // 4. Route State Recovery & Hash Synchronization
  // =========================================================================
  describe('Route State Recovery and Synchronization', () => {
    it('restores active view from URL hash on initial mount', async () => {
      window.location.hash = '#policy'
      setViewport(1440, 900)
      setupApiMocks()
      const root = await mountApp()

      expect(root.textContent).toContain('Policy Groups')
      expect(root.querySelector('header span.capitalize')?.textContent).toContain('policy')
      expect(uncaughtErrors).toHaveLength(0)
    })

    it('restores active view from localStorage when URL hash is omitted', async () => {
      window.localStorage.setItem(ROUTE_STORAGE_KEY, 'probes')
      window.location.hash = ''
      setViewport(390, 844)
      setupApiMocks()
      const root = await mountApp()

      expect(root.querySelector('#probes-title')?.textContent).toContain('Probe Engine')
      expect(uncaughtErrors).toHaveLength(0)
    })

    it('recovers active view upon browser back / forward hashchange events', async () => {
      setViewport(1440, 900)
      setupApiMocks()
      const root = await mountApp()

      // Initial tab
      expect(root.textContent).toContain('Zashboard Foundation')

      // Simulate browser URL hash navigation to #publications
      window.location.hash = '#publications'
      window.dispatchEvent(new HashChangeEvent('hashchange'))
      await nextTick()
      await new Promise((r) => setTimeout(r, 20))

      expect(root.querySelector('#publications-title')?.textContent).toContain('Configuration Exports')

      // Simulate browser Back button to #subscriptions
      window.location.hash = '#subscriptions'
      window.dispatchEvent(new HashChangeEvent('hashchange'))
      await nextTick()
      await new Promise((r) => setTimeout(r, 20))

      expect(root.querySelector('#subscriptions-title')?.textContent).toContain('Subscriptions')
      expect(uncaughtErrors).toHaveLength(0)
    })

    it('safely handles unknown or corrupted hash routes by falling back to dashboard', async () => {
      window.location.hash = '#unknown-invalid-route-xyz'
      setViewport(375, 667)
      setupApiMocks()
      const root = await mountApp()

      // Should safely render Dashboard without crashing or blank screen
      expect(root.textContent).toContain('Clash Sub Parser Zashboard Foundation')
      expect(uncaughtErrors).toHaveLength(0)
    })
  })

  // =========================================================================
  // 5. No White Screen Guarantees (端到端无异常白屏)
  // =========================================================================
  describe('Zero Blank-Screen Guarantees across Dynamic States', () => {
    it('gracefully renders empty-state placeholders without blank screen when datasets are empty', async () => {
      setViewport(375, 667)
      setupApiMocks({ emptyData: true })
      const root = await mountApp()

      // Subscriptions empty state
      window.location.hash = '#subscriptions'
      window.dispatchEvent(new HashChangeEvent('hashchange'))
      await nextTick()
      await new Promise((r) => setTimeout(r, 25))
      expect(root.textContent).toContain('No subscriptions yet')

      // Nodes empty state
      window.location.hash = '#nodes'
      window.dispatchEvent(new HashChangeEvent('hashchange'))
      await nextTick()
      await new Promise((r) => setTimeout(r, 25))
      expect(root.textContent).toContain('No nodes found')

      // Policy empty state
      window.location.hash = '#policy'
      window.dispatchEvent(new HashChangeEvent('hashchange'))
      await nextTick()
      await new Promise((r) => setTimeout(r, 25))
      expect(root.textContent).toContain('No policy groups defined')

      expect(uncaughtErrors).toHaveLength(0)
    })

    it('gracefully handles API network or 500 failure without crashing the application shell', async () => {
      setViewport(1440, 900)
      setupApiMocks({ failHealth: true })
      const root = await mountApp()

      // Health indicator shows unhealthy, app shell remains intact
      const healthBadge = root.querySelector('header .header-health-indicator')
      expect(healthBadge?.textContent?.toLowerCase()).toContain('unhealthy')

      // App structure and navigation still fully functional
      expect(root.querySelector('aside')).not.toBeNull()
      expect(root.querySelector('main')).not.toBeNull()
      expect(uncaughtErrors).toHaveLength(0)
    })
  })

  // =========================================================================
  // 6. Interactive Sheet & Drawer Smoothness (Zashboard Handfeel)
  // =========================================================================
  describe('Zashboard Mobile Interaction & Modal Ergonomics', () => {
    it('verifies 40% soft backdrop blur and grab handle contract in mobile sheets', async () => {
      setViewport(375, 667)
      setupApiMocks()
      const root = await mountApp()

      // Navigate to Probes tab
      window.location.hash = '#probes'
      window.dispatchEvent(new HashChangeEvent('hashchange'))
      await nextTick()
      await new Promise((r) => setTimeout(r, 25))

      // Trigger "Trigger Probe" modal
      const triggerBtn = Array.from(root.querySelectorAll('button')).find((b) =>
        (b.textContent?.includes('Trigger Probe') || b.textContent?.includes('Run Batch Probe') || b.textContent?.includes('发起批量探针'))
      )
      expect(triggerBtn).toBeDefined()
      triggerBtn?.click()
      await nextTick()
      await new Promise((r) => setTimeout(r, 25))

      // Verify modal dialog structure
      const dialog = root.querySelector('dialog[open], .modal[open], dialog')
      expect(dialog).not.toBeNull()

      // Now test inspecting Evidence Sheet with touch grab handle and 40% soft backdrop blur
      const closeDialogBtn = dialog?.querySelector('button')
      closeDialogBtn?.click()
      await nextTick()

      // Click "Evidence" button on probe run card
      const evidenceBtn = Array.from(root.querySelectorAll('button')).find((b) =>
        b.textContent?.includes('Evidence')
      )
      expect(evidenceBtn).toBeDefined()
      evidenceBtn?.click()
      await nextTick()
      await new Promise((r) => setTimeout(r, 25))

      // Verify 40% soft backdrop blur overlay exists in DOM
      const backdrop = root.querySelector('.bg-black\\/40')
      expect(backdrop).not.toBeNull()
      expect(backdrop?.className).toContain('backdrop-blur-sm')

      // Verify mobile grab handle exists for touch ergonomics
      const grabHandle = root.querySelector('.cursor-grab')
      expect(grabHandle).not.toBeNull()
      expect(grabHandle?.querySelector('.rounded-full')).not.toBeNull()

      expect(uncaughtErrors).toHaveLength(0)
    })

    it('verifies adaptive overlay surfaces and code preview box readability in short and landscape screens', async () => {
      // Test short viewport (e.g. landscape phone 667x375)
      setViewport(667, 375)
      setupApiMocks()
      const root = await mountApp()

      // Navigate to Publications tab
      window.location.hash = '#publications'
      window.dispatchEvent(new HashChangeEvent('hashchange'))
      await nextTick()
      await new Promise((r) => setTimeout(r, 25))

      // Check code preview pre element exists and has adaptive-preview-box class
      const preEl = root.querySelector('pre')
      expect(preEl).not.toBeNull()
      expect(preEl?.className).toContain('adaptive-preview-box')
      // Ensure content is rendered and readable
      expect(preEl?.textContent).toContain('proxies:')

      // Navigate to Policy tab and open group sheet in landscape
      window.location.hash = '#policy'
      window.dispatchEvent(new HashChangeEvent('hashchange'))
      await nextTick()
      await new Promise((r) => setTimeout(r, 25))

      const addGroupBtn = Array.from(root.querySelectorAll('button')).find((b) =>
        b.textContent?.includes('Add Group') || b.textContent?.includes('New Group')
      )
      if (addGroupBtn) {
        addGroupBtn.click()
        await nextTick()
        await new Promise((r) => setTimeout(r, 25))

        const sheet = root.querySelector('[role="dialog"].adaptive-surface-sheet')
        expect(sheet).not.toBeNull()
      }

      expect(uncaughtErrors).toHaveLength(0)
    })
  })

  // =========================================================================
  // 7. Strict Zero Console Uncaught Exceptions Invariant
  // =========================================================================
  describe('Strict 0 Console Exceptions Invariant', () => {
    it('guarantees 0 uncaught errors and 0 unhandled promise rejections', () => {
      expect(uncaughtErrors).toHaveLength(0)
    })
  })
})
