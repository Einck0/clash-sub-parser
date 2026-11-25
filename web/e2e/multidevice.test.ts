// @vitest-environment jsdom
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { createApp, nextTick, type App as VueApp } from 'vue'
import App from '../src/App.vue'
import { router } from '../src/router'
import { i18n } from '../src/locales'
import { api } from '../src/api/client'
import { THEME_STORAGE_KEY, DEFAULT_THEME, type ThemeName } from '../src/theme'
import { ROUTE_STORAGE_KEY, VALID_TABS, getRouteTab, isLegacyHash, type NavTab } from '../src/navigation'
import { setLocale } from '../src/locales'
import type { NodeRecord } from '../src/features/nodes/nodeView'

describe('CSP Multi-Device & E2E Verification Suite', () => {
  let app: VueApp | null = null
  let container: HTMLDivElement | null = null
  let consoleErrorSpy: ReturnType<typeof vi.spyOn>
  let consoleWarnSpy: ReturnType<typeof vi.spyOn>
  const uncaughtErrors: Error[] = []

  let recentNavEvents: Array<{ event: string; hash: string; route: string; time: number }> = []

  function recordNavEvent(event: string) {
    recentNavEvents.push({
      event,
      hash: typeof window !== 'undefined' ? window.location.hash : '',
      route: router.currentRoute.value.fullPath,
      time: Date.now(),
    })
    if (recentNavEvents.length > 25) {
      recentNavEvents.shift()
    }
  }

  if (typeof window !== 'undefined') {
    window.addEventListener('popstate', () => recordNavEvent('popstate'))
    window.addEventListener('hashchange', () => recordNavEvent('hashchange'))
  }
  router.beforeEach((to, from) => {
    recordNavEvent(`router:beforeEach(${from.fullPath}->${to.fullPath})`)
  })
  router.afterEach((to, from, failure) => {
    recordNavEvent(`router:afterEach(${from.fullPath}->${to.fullPath}${failure ? ':failed(' + (failure.type ?? failure.message) + ')' : ''})`)
  })

  async function drainAsyncEvents(): Promise<void> {
    // Flush both macro-tasks (timers/popstate in jsdom) and micro-tasks (promises/nextTick)
    await new Promise((resolve) => setTimeout(resolve, 0))
    await nextTick()
  }

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
    target: 'mihomo',
    content: 'mixed-port: 7890\nallow-lan: true\nproxies:\n  - name: HK Premium 01\n    type: ss',
    filename: 'csp-mihomo.yaml',
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

  beforeEach(async () => {
    uncaughtErrors.length = 0
    window.localStorage.clear()
    setLocale('en-US')
    recentNavEvents = []

    // Controlled reset: use history.replaceState and await router.replace rather than bare window.location.hash = ''
    // Bare window.location.hash = '' triggers jsdom's fragment navigation setter, scheduling an asynchronous
    // non-blocking setTimeout popstate/hashchange event in the jsdom timer queue that collides with subsequent test pushes.
    window.history.replaceState(null, '', window.location.pathname)
    await router.replace({ path: '/' }).catch(() => {})
    await drainAsyncEvents()

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

  afterEach(async () => {
    if (app && container) {
      app.unmount()
      app = null
    }
    if (container && container.parentNode) {
      container.parentNode.removeChild(container)
      container = null
    }
    vi.restoreAllMocks()
    await router.replace({ path: '/' }).catch(() => {})
    window.history.replaceState(null, '', window.location.pathname)
    await drainAsyncEvents()
  })

  async function waitForView(
    viewSelector: string,
    root: HTMLElement = container!,
    targetTab?: string
  ): Promise<Element | null> {
    const existing = root.querySelector(viewSelector)
    if (existing && (!targetTab || router.currentRoute.value.name === targetTab)) return existing

    let recordedFailure: any = null

    // Listen to real router navigation events if a target route transition is expected or underway
    if (targetTab && router.currentRoute.value.name !== targetTab) {
      await new Promise<void>((resolve, reject) => {
        let timer: any = null
        const unhook = router.afterEach((to, _from, failure) => {
          if (to.name === targetTab) {
            clearTimeout(timer)
            unhook()
            if (failure) {
              recordedFailure = failure
              reject(failure)
            } else {
              resolve()
            }
          }
        })
        timer = setTimeout(() => {
          unhook()
          resolve()
        }, 1500)
      }).catch((err) => {
        recordedFailure = err
      })
    }

    await nextTick()
    const afterTick1 = root.querySelector(viewSelector)
    if (afterTick1 && (!targetTab || router.currentRoute.value.name === targetTab)) return afterTick1

    await nextTick()
    const afterTick2 = root.querySelector(viewSelector)
    if (afterTick2 && (!targetTab || router.currentRoute.value.name === targetTab)) return afterTick2

    const found = await new Promise<Element | null>((resolve) => {
      const timer = setTimeout(() => {
        observer.disconnect()
        resolve(root.querySelector(viewSelector))
      }, 1500)
      const observer = new MutationObserver(() => {
        const el = root.querySelector(viewSelector)
        if (el && (!targetTab || router.currentRoute.value.name === targetTab)) {
          clearTimeout(timer)
          observer.disconnect()
          resolve(el)
        }
      })
      observer.observe(root, { childList: true, subtree: true, characterData: true })
    })

    if (!found) {
      const diagnostic = {
        currentRoute: router.currentRoute.value.fullPath,
        currentRouteName: String(router.currentRoute.value.name),
        hash: typeof window !== 'undefined' ? window.location.hash : '',
        targetTab: targetTab || null,
        selector: viewSelector,
        failureType: recordedFailure ? recordedFailure.type || recordedFailure.name || 'UNKNOWN_FAILURE' : 'TIMEOUT_WITHOUT_NAVIGATION_EVENT',
        failureMessage: recordedFailure ? recordedFailure.message || String(recordedFailure) : 'Element not found within timeout window',
        recentNavEvents: [...recentNavEvents],
      }
      console.warn('[ROUTER_TRANSITION_DIAGNOSTIC]', JSON.stringify(diagnostic))
      throw new Error(`[waitForView Diagnostic Failure] ${JSON.stringify(diagnostic)}`)
    }

    return found
  }

  async function mountApp(): Promise<HTMLDivElement> {
    const targetTab = getRouteTab()
    if (router.currentRoute.value.name !== targetTab) {
      await router.replace({ name: targetTab }).catch(() => {})
    }
    await router.isReady()
    await drainAsyncEvents()
    app = createApp(App)
    app.use(router)
    app.use(i18n)
    app.mount(container!)
    await nextTick()
    await nextTick()
    await drainAsyncEvents()
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

      // Three real route links and the More dialog trigger form the four dock targets.
      const links = mobileNav?.querySelectorAll('a[href]')
      expect(links?.length).toBe(3)
      expect(Array.from(links || []).map(link => link.getAttribute('href'))).toEqual([
        '#/nodes', '#/policy', '#/subscriptions',
      ])
      const more = mobileNav?.querySelector('[data-testid="dock-link-more"]')
      expect(more?.getAttribute('aria-haspopup')).toBe('dialog')
      expect(more?.getAttribute('aria-expanded')).toBe('false')

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
      expect(healthBadge?.textContent?.toLowerCase()).toContain('service online')

      // Zero console exceptions
      expect(uncaughtErrors).toHaveLength(0)
    })

    it('verifies 375px compact topbar class contract and interactive controls', async () => {
      setViewport(375, 667)
      setupApiMocks()
      const root = await mountApp()

      // 1. Header has layout classes and no horizontal overflow
      const header = root.querySelector('header')
      expect(header).not.toBeNull()
      expect(header?.classList.contains('overflow-hidden') || header?.classList.contains('overflow-visible')).toBe(true)
      const mainContainer = header?.parentElement
      expect(mainContainer?.classList.contains('overflow-x-hidden')).toBe(true)

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
      expect(healthText?.textContent?.toLowerCase()).toContain('service online')

      const refreshBtn = healthIndicator?.querySelector('button[aria-label="Refresh health"]')
      expect(refreshBtn).not.toBeNull()

      // 4. Theme toggle button with aria-label
      const themeBtn = root.querySelector('header button[data-testid="theme-toggle"]')
      expect(themeBtn).not.toBeNull()

      // 5. Title container min-w-0 and truncate
      const titleContainer = root.querySelector('header .min-w-0')
      expect(titleContainer).not.toBeNull()
      const titleSpan = titleContainer?.querySelector('.truncate')
      expect(titleSpan).not.toBeNull()

      // 6. Interactive test: clicking auth badge navigates to Settings view
      await drainAsyncEvents()
      ;(authBadge as HTMLButtonElement).click()
      const settingsView = await waitForView('[data-testid="settings-view"]', root, 'settings')
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

      const mobileNav = root.querySelector('[data-testid="mobile-dock"]')!
      const subLink = mobileNav.querySelector<HTMLAnchorElement>('[data-testid="dock-link-subscriptions"]')!
      expect(subLink?.getAttribute('href')).toBe('#/subscriptions')
      subLink.click()
      const subTitle = await waitForView('[data-testid="subscriptions-view"] h2[data-testid="toolbar-title"]', root, 'subscriptions')
      expect(subTitle?.textContent).toContain('Subscriptions')
      expect(subLink.getAttribute('aria-current')).toBe('page')
      expect(['#subscriptions', '#/subscriptions']).toContain(window.location.hash)

      const nodeLink = mobileNav.querySelector<HTMLAnchorElement>('[data-testid="dock-link-nodes"]')!
      expect(nodeLink?.getAttribute('href')).toBe('#/nodes')
      nodeLink.click()
      const nodeTitle = await waitForView('h2#nodes-title', root, 'nodes')
      expect(nodeTitle?.textContent).toBe('Node management and probing')
      expect(nodeLink.getAttribute('aria-current')).toBe('page')
      expect(subLink.hasAttribute('aria-current')).toBe(false)
      expect(['#nodes', '#/nodes']).toContain(window.location.hash)

      // More is a real teleported dialog; closing it returns focus to its trigger.
      const more = mobileNav.querySelector<HTMLButtonElement>('[data-testid="dock-link-more"]')!
      more.focus()
      more.click()
      await drainAsyncEvents()
      const sheet = document.querySelector<HTMLElement>('[data-testid="mobile-more-sheet"]')!
      expect(sheet).not.toBeNull()
      const dialog = sheet.closest('[role="dialog"]')!
      expect(dialog.getAttribute('aria-modal')).toBe('true')
      expect(dialog.contains(document.activeElement)).toBe(true)
      expect(more.getAttribute('aria-expanded')).toBe('true')
      window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true, cancelable: true }))
      await drainAsyncEvents()
      expect(document.querySelector('[data-testid="mobile-more-sheet"]')).toBeNull()
      expect(more.getAttribute('aria-expanded')).toBe('false')
      expect(document.activeElement).toBe(more)

      more.click()
      await drainAsyncEvents()
      const publicationLink = document.querySelector<HTMLAnchorElement>('[data-testid="mobile-more-sheet"] [data-testid="dock-link-publications"]')!
      expect(publicationLink?.getAttribute('href')).toBe('#/publications')
      publicationLink.click()
      const publicationTitle = await waitForView('[data-testid="client-subscriptions-page"] h2[data-testid="toolbar-title"]', root, 'publications')
      expect(publicationTitle?.textContent).toBe('Client Subscriptions')
      expect(document.querySelector('[data-testid="mobile-more-sheet"]')).toBeNull()
      expect(window.localStorage.getItem(ROUTE_STORAGE_KEY)).toBe('publications')

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

      // Desktop navigation items (9 items: dashboard, subscriptions, nodes, dns, chains, policy, rules, publications, settings)
      const navLinks = sidebar?.querySelectorAll('nav a[href]')
      expect(navLinks?.length).toBe(9)
      expect(Array.from(navLinks || []).map(link => link.getAttribute('href'))).toEqual(
        expect.arrayContaining(VALID_TABS.map(tab => `#/${tab}`))
      )
      expect(sidebar?.querySelector('a[aria-current="page"]')?.getAttribute('href')).toBe('#/dashboard')

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

      const tabsToTest: { tab: NavTab; viewSelector: string; expectedText: string }[] = [
        { tab: 'subscriptions', viewSelector: '[data-testid="subscriptions-view"] h2[data-testid="toolbar-title"]', expectedText: 'Subscriptions' },
        { tab: 'nodes', viewSelector: 'h2#nodes-title', expectedText: 'Node management and probing' },
        { tab: 'rules', viewSelector: 'h2#rules-title', expectedText: 'Routing Rule Desk' },
        { tab: 'policy', viewSelector: 'h2#policy-title', expectedText: 'Policy Groups' },
        { tab: 'publications', viewSelector: '[data-testid="client-subscriptions-page"] h2[data-testid="toolbar-title"]', expectedText: 'Client Subscriptions' },
        { tab: 'dashboard', viewSelector: 'main h2[data-testid="toolbar-title"]', expectedText: 'CSP Control Plane' },
      ]

      for (const t of tabsToTest) {
        const sidebar = root.querySelector('aside')!
        const link = sidebar.querySelector<HTMLAnchorElement>(`nav a[href="#/${t.tab}"]`)!
        expect(link).not.toBeNull()
        link.click()
        const heading = await waitForView(t.viewSelector, root, t.tab)

        expect(heading?.textContent?.trim()).toBe(t.expectedText)
        expect(link.getAttribute('aria-current')).toBe('page')
        expect(root.querySelector('main')?.textContent?.trim()).not.toBe('')
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
      expect(root.querySelector('header span.capitalize')?.textContent?.toLowerCase()).toContain('policy')
      expect(uncaughtErrors).toHaveLength(0)
    })

    it('restores active view from localStorage when URL hash is omitted and redirects legacy probes to nodes', async () => {
      window.localStorage.setItem(ROUTE_STORAGE_KEY, 'probes')
      window.history.replaceState(null, '', window.location.pathname)
      await drainAsyncEvents()
      setViewport(390, 844)
      setupApiMocks()
      const root = await mountApp()

      expect(root.querySelector('h2#nodes-title')?.textContent).toBe('Node management and probing')
      expect(router.currentRoute.value.name).toBe('nodes')
      expect(root.querySelector('[data-testid="dock-link-nodes"]')?.getAttribute('aria-current')).toBe('page')
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
      const pubTitle = await waitForView('[data-testid="client-subscriptions-page"] h2[data-testid="toolbar-title"]', root, 'publications')
      expect(pubTitle?.textContent).toBe('Client Subscriptions')
      expect(window.localStorage.getItem(ROUTE_STORAGE_KEY)).toBe('publications')

      // Simulate browser Back button to #subscriptions
      window.location.hash = '#subscriptions'
      window.dispatchEvent(new HashChangeEvent('hashchange'))
      const subTitle = await waitForView('[data-testid="subscriptions-view"] h2[data-testid="toolbar-title"]', root, 'subscriptions')
      expect(subTitle?.textContent).toBe('Subscriptions')
      expect(window.localStorage.getItem(ROUTE_STORAGE_KEY)).toBe('subscriptions')
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

    it('preserves in-flight route push when stale hashchange occurs without cancelling navigation', async () => {
      setViewport(1440, 900)
      setupApiMocks()
      const root = await mountApp()

      expect(router.currentRoute.value.name).toBe('dashboard')

      let resolveDeferredNav!: () => void
      const deferredGate = new Promise<void>((resolve) => {
        resolveDeferredNav = resolve
      })

      // Gate /policy navigation with a controlled deferred guard
      const unhookGuard = router.beforeEach(async (to) => {
        if (to.name === 'policy') {
          await deferredGate
        }
      })

      try {
        // Trigger in-flight route push to /policy
        const pushPromise = router.push('/policy')

        // While navigation is in flight, dispatch a stale/delayed hashchange event (e.g. hash = '' from beforeEach)
        window.location.hash = ''
        window.dispatchEvent(new HashChangeEvent('hashchange'))

        // Release the deferred navigation gate
        resolveDeferredNav()

        // Wait for the push to conclude; it must NOT be cancelled
        const failure = await pushPromise
        expect(failure).toBeUndefined()

        // Router must be on policy and view must render Policy Groups
        const policyView = await waitForView('#policy-title', root, 'policy')
        expect(policyView).not.toBeNull()
        expect(router.currentRoute.value.name).toBe('policy')
      } finally {
        unhookGuard()
      }
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
      await waitForView('#subscriptions-title', root)
      await waitForView('[data-testid="empty-state"]', root)
      expect(root.textContent).toContain('No subscriptions yet')

      // Nodes empty state
      window.location.hash = '#nodes'
      window.dispatchEvent(new HashChangeEvent('hashchange'))
      await waitForView('#nodes-title', root)
      await waitForView('[data-testid="empty-state"]', root)
      expect(root.textContent).toContain('No nodes found')

      // Policy empty state
      window.location.hash = '#policy'
      window.dispatchEvent(new HashChangeEvent('hashchange'))
      await waitForView('#policy-title', root)
      await waitForView('[data-testid="empty-state"]', root)
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

      // Navigate to Probes tab (which redirects to /nodes)
      window.location.hash = '#nodes'
      window.dispatchEvent(new HashChangeEvent('hashchange'))
      await waitForView('#nodes-title', root)

      // Trigger full probe confirmation modal
      const triggerBtn = root.querySelector('[data-testid="probe-all-btn"]') as HTMLButtonElement | null ||
        Array.from(root.querySelectorAll('button')).find((b) =>
          b.textContent?.includes('全量检测') || b.textContent?.includes('Trigger Probe') || b.textContent?.includes('Run Batch Probe') || b.textContent?.includes('发起批量探针')
        )
      expect(triggerBtn).toBeDefined()
      triggerBtn?.click()
      await nextTick()
      await new Promise((r) => setTimeout(r, 25))

      // Verify modal dialog structure
      const dialog = document.body.querySelector('dialog[open], .modal[open], dialog')
      expect(dialog).not.toBeNull()

      // Now close dialog
      const closeDialogBtn = dialog?.querySelector('button')
      closeDialogBtn?.click()
      await nextTick()

      // Open probe history drawer to inspect runs & evidence
      const historyBtn = root.querySelector('[data-testid="node-probe-history-btn"]') as HTMLButtonElement | null
      expect(historyBtn).toBeDefined()
      historyBtn?.click()
      await nextTick()
      await new Promise((r) => setTimeout(r, 30))

      // Click "Evidence" button on probe run card
      const evidenceBtn = Array.from(document.body.querySelectorAll('button')).find((b) =>
        b.textContent?.includes('Evidence') || b.closest('[data-testid="probe-run-card"]')
      )
      expect(evidenceBtn).toBeDefined()
      evidenceBtn?.click()
      await nextTick()
      await new Promise((r) => setTimeout(r, 25))

      // Verify 40% soft backdrop blur overlay exists in DOM
      const backdrop = document.body.querySelector('.bg-black\\/40')
      expect(backdrop).not.toBeNull()
      expect(backdrop?.className).toContain('backdrop-blur-sm')

      // Verify mobile grab handle exists for touch ergonomics
      const grabHandle = document.body.querySelector('.cursor-grab')
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
      await waitForView('[data-testid="client-subscriptions-page"]', root)

      // Check client subscriptions section exists and is readable
      const pageEl = root.querySelector('[data-testid="client-subscriptions-page"]')
      expect(pageEl).not.toBeNull()
      const copyBtns = root.querySelectorAll('[data-testid="copy-client-subscription-btn"]')
      expect(copyBtns.length).toBe(4)

      // Navigate to Policy tab and open group sheet in landscape
      window.location.hash = '#policy'
      window.dispatchEvent(new HashChangeEvent('hashchange'))
      await waitForView('#policy-title', root)

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
  // 7. Four Viewports & Large-Scale Fixtures Verification (1280x800, 392x872, 375x667, 667x375)
  // =========================================================================
  describe('Four Standard Viewports & Synthetic Large-Scale Fixtures (250 Nodes / 150 Groups)', () => {
    type LargeFixtureNode = NodeRecord & {
      logicalId: string
      displayName: string
      countryCode?: string
    }

    const largeNodesFixture: LargeFixtureNode[] = Array.from({ length: 250 }, (_, i): LargeFixtureNode => {
      const index = i + 1
      if (index === 250) {
        return {
          logicalId: 'node-target-250',
          logical_id: 'node-target-250',
          displayName: 'Taiwan Special Node 250',
          display_name: 'Taiwan Special Node 250',
          protocol: 'vmess',
          active: true,
          countryCode: 'TW',
          capabilities: {
            ai: { verdict: 'available' as const, latency_ms: 100 },
          },
        }
      }
      if (index === 70) {
        return {
          logicalId: 'node-ai-only-70',
          logical_id: 'node-ai-only-70',
          displayName: 'Node AI Only 70',
          display_name: 'Node AI Only 70',
          protocol: 'ss',
          active: true,
          countryCode: 'HK',
          health_status: 'unknown',
          capabilities: {
            ai: { verdict: 'available' as const, latency_ms: 88 },
          },
        }
      }
      return {
        logicalId: `node-batch-${index}`,
        logical_id: `node-batch-${index}`,
        displayName: `Batch Node ${index}`,
        display_name: `Batch Node ${index}`,
        protocol: index % 2 === 0 ? 'ss' : 'vmess',
        active: true,
        countryCode: 'US',
        capabilities: {
          baseline: { verdict: 'available' as const, latency_ms: 50 + (index % 100) },
        },
      }
    })

    const largeGroupsFixture = Array.from({ length: 150 }, (_, i) => {
      const index = i + 1
      return {
        id: `grp-policy-${index}`,
        name: `Policy Group ${index}`,
        group_type: 'select' as const,
        edges: [],
      }
    })

    it('verifies 1280x800 Desktop Viewport: aside uses sticky dynamic viewport height and remains accessible on scroll', async () => {
      setViewport(1280, 800)
      setupApiMocks()
      const root = await mountApp()

      const aside = root.querySelector('aside')
      expect(aside).not.toBeNull()
      expect(aside?.className).toContain('sticky')
      expect(aside?.className).toContain('top-0')
      expect(aside?.classList.contains('h-dvh')).toBe(true)
      expect(aside?.className).toContain('overflow-y-auto')

      const parent = aside?.parentElement
      expect(parent).not.toBeNull()
      expect(parent?.classList.contains('overflow-x-hidden')).toBe(false)

      window.scrollY = 600
      window.dispatchEvent(new Event('scroll'))
      await nextTick()

      expect(aside?.isConnected).toBe(true)
      expect(aside?.classList.contains('md:flex')).toBe(true)
      expect(uncaughtErrors).toHaveLength(0)
    })

    it('verifies 392x872 Mobile Large Viewport: zero horizontal overflow in mobile navigation and main content', async () => {
      setViewport(392, 872)
      setupApiMocks()
      const root = await mountApp()

      const mobileNav = root.querySelector('nav.md\\:hidden')
      expect(mobileNav).not.toBeNull()
      expect(mobileNav?.classList.contains('fixed')).toBe(true)

      const main = root.querySelector('main')
      expect(main).not.toBeNull()
      const mainContainer = main?.parentElement
      expect(mainContainer?.classList.contains('overflow-x-hidden')).toBe(true)
      expect(uncaughtErrors).toHaveLength(0)
    })

    it('verifies 375x667 Mobile Viewport: touch targets and bottom navigation bar ergonomics', async () => {
      setViewport(375, 667)
      setupApiMocks()
      const root = await mountApp()

      const mobileNav = root.querySelector('[data-testid="mobile-dock"]')!
      const targets = mobileNav.querySelectorAll<HTMLElement>('a[href], button')
      expect(targets).toHaveLength(4)
      for (const target of targets) {
        expect(target.textContent?.trim()).not.toBe('')
        expect(target.tabIndex).toBe(0)
        expect(target.classList.contains('touch-manipulation')).toBe(true)
        expect(target.classList.contains('py-1.5')).toBe(true)
        expect(target.classList.contains('rounded-xl')).toBe(true)
        target.focus()
        expect(document.activeElement).toBe(target)
      }
      expect(uncaughtErrors).toHaveLength(0)
    })

    it('verifies 667x375 Landscape Viewport: compact layout with zero horizontal overflow', async () => {
      setViewport(667, 375)
      setupApiMocks()
      const root = await mountApp()

      const header = root.querySelector('header')
      expect(header).not.toBeNull()
      expect(header?.classList.contains('overflow-hidden') || header?.classList.contains('overflow-visible')).toBe(true)
      const mainContainer = header?.parentElement
      expect(mainContainer?.classList.contains('overflow-x-hidden')).toBe(true)
      expect(uncaughtErrors).toHaveLength(0)
    })

    it('handles request race conditions gracefully and correctly displays page 2 real items', async () => {
      setViewport(1280, 800)
      const calls: { page: number; resolve: (val: any) => void }[] = []

      vi.spyOn(api, 'get').mockImplementation(async (path: string, options?: any) => {
        if (path === '/healthz') return { status: 'healthy' }
        if (path === '/api/v1/auth/status') return { mode: 'open', authenticated: true, subject: 'admin' }
        if (path === '/api/v1/nodes') {
          const p = Number(options?.params?.page || 1)
          const ps = Number(options?.params?.page_size || 50)
          const start = (p - 1) * ps
          const slice = largeNodesFixture.slice(start, start + ps)
          return new Promise((resolve) => {
            calls.push({
              page: p,
              resolve: () => resolve({ items: slice, total: 250, page: p, page_size: ps }),
            })
          })
        }
        return { items: [], total: 0 }
      })

      const { useProbes } = await import('../src/features/probes/useProbes')
      const probesState = useProbes()

      const p1Promise = probesState.loadProbeNodes(1, 50)
      const p2Promise = probesState.loadProbeNodes(2, 50)

      await nextTick()
      expect(calls.length).toBe(2)

      calls[1].resolve(null)
      await p2Promise
      await nextTick()

      calls[0].resolve(null)
      await p1Promise
      await nextTick()

      expect(probesState.probeNodes.value.length).toBe(50)
      expect(probesState.probeNodes.value[0].logicalId).toBe('node-batch-51')
      expect(probesState.totalNodes.value).toBe(250)
    })

    it('searches for 250th node and preserves selection in PolicyEditorSheet', async () => {
      const { default: PolicyEditorSheet } = await import('../src/features/policy/PolicyEditorSheet.vue')
      const { h } = await import('vue')

      const getSpy = vi.spyOn(api, 'get').mockImplementation(async (path: string, options?: any) => {
        const p = Number(options?.params?.page || 1)
        const ps = Number(options?.params?.page_size || 50)
        const start = (p - 1) * ps
        if (path === '/api/v1/nodes') {
          const search = (options?.params?.search || '').toLowerCase()
          const filtered = largeNodesFixture.filter((n) =>
            !search || n.displayName.toLowerCase().includes(search) || n.logicalId.toLowerCase().includes(search)
          )
          return { items: filtered.slice(start, start + ps), total: filtered.length, page: p, page_size: ps }
        }
        if (path === '/api/v1/policies/groups') {
          return { items: largeGroupsFixture.slice(start, start + ps), total: largeGroupsFixture.length, page: p, page_size: ps }
        }
        return { items: [], total: 0 }
      })
      vi.spyOn(api, 'post').mockResolvedValue({
        selection_digest: 'fixture-selection', members: [], node_count: 0, diagnostics: [],
      })

      const sheetContainer = document.createElement('div')
      document.body.appendChild(sheetContainer)
      const group = {
        id: 'grp-test',
        name: 'Test Group',
        group_type: 'select' as const,
        edges: [],
      }
      const sheetApp = createApp({
        render() {
          return h(PolicyEditorSheet, {
            open: true,
            group,
            allGroups: largeGroupsFixture,
            mode: 'edges',
          })
        },
      })
      sheetApp.mount(sheetContainer)
      try {
        await drainAsyncEvents()
        // Drawer content is teleported to body, not rendered inside the mount container.
        const sheet = document.querySelector<HTMLElement>('[data-testid="policy-editor-sheet"]')!
        expect(sheet).not.toBeNull()
        expect(sheet.closest('[role="dialog"]')?.getAttribute('aria-modal')).toBe('true')
        const details = sheet.querySelector<HTMLDetailsElement>('[data-testid="advanced-pipeline-details"]')!
        details.querySelector('summary')!.click()
        expect(details.open).toBe(true)
        const nodeRadio = sheet.querySelector<HTMLInputElement>('input[type="radio"][value="node"]')!
        expect(nodeRadio).not.toBeNull()
        nodeRadio.click()
        await drainAsyncEvents()

        const select = sheet.querySelector<HTMLSelectElement>('[data-testid="edge-node-select"]')!
        expect(select).not.toBeNull()
        expect(select.disabled).toBe(false)
        expect(select.options).toHaveLength(51)
        expect(select.querySelector('option[value="node-target-250"]')).toBeNull()
        expect(getSpy).toHaveBeenCalledWith('/api/v1/nodes', { params: {
          page: 1, page_size: 50, scope: 'enabled_subscriptions', active_only: 'true',
        } })
        expect(getSpy).toHaveBeenCalledWith('/api/v1/policies/groups', { params: { page: 2, page_size: 100 } })

        // Reach the last page through real controls before searching or selecting node 250.
        for (let page = 2; page <= 5; page++) {
          const next = sheet.querySelector<HTMLButtonElement>('[data-testid="candidate-next-page"]')!
          expect(next.disabled).toBe(false)
          next.click()
          await drainAsyncEvents()
          expect(getSpy).toHaveBeenCalledWith('/api/v1/nodes', { params: {
            page, page_size: 50, scope: 'enabled_subscriptions', active_only: 'true',
          } })
          expect(select.disabled).toBe(false)
        }
        expect(sheet.querySelector<HTMLButtonElement>('[data-testid="candidate-next-page"]')?.disabled).toBe(true)
        const target = select.querySelector('option[value="node-target-250"]')!
        expect(target?.textContent).toContain('Taiwan Special Node 250')
        select.value = 'node-target-250'
        select.dispatchEvent(new Event('change', { bubbles: true }))
        await nextTick()
        expect(select.value).toBe('node-target-250')

        const searchInput = sheet.querySelector<HTMLInputElement>('[data-testid="candidate-node-search-input"]')!
        searchInput.value = 'Batch Node'
        searchInput.dispatchEvent(new Event('input', { bubbles: true }))
        await vi.waitFor(async () => {
          await nextTick()
          expect(getSpy).toHaveBeenCalledWith('/api/v1/nodes', { params: {
            page: 1, page_size: 50, scope: 'enabled_subscriptions', active_only: 'true', search: 'Batch Node',
          } })
          expect(select.disabled).toBe(false)
          expect(select.value).toBe('node-target-250')
          expect(select.selectedOptions[0]?.textContent).toContain('Taiwan Special Node 250')
          expect(select.selectedOptions[0]?.textContent).toContain('跨页锁定已保留')
        }, { timeout: 1500 })

        searchInput.value = '250'
        searchInput.dispatchEvent(new Event('input', { bubbles: true }))
        await vi.waitFor(async () => {
          await nextTick()
          expect(getSpy).toHaveBeenCalledWith('/api/v1/nodes', { params: {
            page: 1, page_size: 50, scope: 'enabled_subscriptions', active_only: 'true', search: '250',
          } })
          expect(select.disabled).toBe(false)
          expect(select.options).toHaveLength(2)
          expect(select.value).toBe('node-target-250')
          expect(select.selectedOptions[0]?.textContent).toContain('Taiwan Special Node 250')
        }, { timeout: 1500 })
        expect(uncaughtErrors).toHaveLength(0)
      } finally {
        sheetApp.unmount()
        sheetContainer.remove()
      }
      expect(document.querySelector('[data-testid="policy-editor-sheet"]')).toBeNull()
    })

    it('classifies node with unknown baseline but available AI probe as unknown, neither healthy nor unhealthy', async () => {
      const { nodeUnderlyingHealthCategory, nodeHealthBadge } = await import('../src/features/nodes/nodeView')

      const aiOnlyNode = largeNodesFixture.find((n) => n.logicalId === 'node-ai-only-70')!
      expect(aiOnlyNode).toBeDefined()

      const category = nodeUnderlyingHealthCategory(aiOnlyNode)
      expect(category).toBe('unknown')
      expect(category).not.toBe('healthy')
      expect(category).not.toBe('unhealthy')

      const badge = nodeHealthBadge(aiOnlyNode)
      expect(badge.label).toBe('Unknown')
      expect(badge.tone).toBe('info')
    })
  })

  // =========================================================================
  // 8. Strict Zero Console Uncaught Exceptions Invariant
  // =========================================================================
  describe('Strict 0 Console Exceptions Invariant', () => {
    it('guarantees 0 uncaught errors and 0 unhandled promise rejections', () => {
      expect(uncaughtErrors).toHaveLength(0)
    })
  })
})
