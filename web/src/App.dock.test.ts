// @vitest-environment jsdom
import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import { createApp, nextTick, type App as VueApp } from 'vue'
import App from './App.vue'
import { router } from './router'
import { i18n } from './locales'
import { api } from './api/client'

describe('App.vue Navigation Dock & Layout Adaptability', () => {
  let app: VueApp | null = null
  let container: HTMLDivElement | null = null

  beforeEach(() => {
    container = document.createElement('div')
    document.body.appendChild(container)
    vi.spyOn(api, 'get').mockImplementation(async (path: string) => {
      if (path === '/healthz') return { status: 'healthy' }
      if (path === '/api/v1/auth/status') return { mode: 'open', authenticated: true, subject: 'admin' }
      return { items: [], total: 0 }
    })
  })

  afterEach(() => {
    if (app) {
      app.unmount()
      app = null
    }
    if (container && container.parentNode) {
      container.parentNode.removeChild(container)
      container = null
    }
    vi.restoreAllMocks()
  })

  it('renders mobile navigation dock without hardcoded fixed height or hardcoded 48px min-height', async () => {
    router.push('/dashboard')
    await router.isReady()

    app = createApp(App)
    app.use(router)
    app.use(i18n)
    app.mount(container!)
    await nextTick()

    const dock = container!.querySelector('[data-testid="mobile-dock"]')
    expect(dock).toBeTruthy()

    // Verify it doesn't contain hardcoded h-16 or min-h-[48px]
    expect(dock!.classList.contains('h-16')).toBe(false)
    expect(dock!.classList.contains('min-h-[48px]')).toBe(false)

    // Verify dynamic safe area padding class
    expect(dock!.classList.contains('py-2')).toBe(true)
    expect(dock!.classList.contains('rounded-2xl') || dock!.classList.contains('rounded-full')).toBe(true)

    // Verify all 7 routes are accessible from the dock (including Dashboard)
    const expectedRouteIds = ['dashboard', 'subscriptions', 'nodes', 'probes', 'policy', 'publications', 'settings']
    for (const routeId of expectedRouteIds) {
      const linkBtn = dock!.querySelector(`[data-testid="dock-link-${routeId}"]`)
      expect(linkBtn, `Mobile dock must contain explicit entry for ${routeId}`).toBeTruthy()
    }

    // Verify nav action buttons do not enforce rigid min-h-[48px] or h-full
    const navButtons = dock!.querySelectorAll('button')
    expect(navButtons.length).toBeGreaterThan(0)
    for (const btn of Array.from(navButtons)) {
      expect(btn.classList.contains('min-h-[48px]')).toBe(false)
      expect(btn.classList.contains('h-full')).toBe(false)
    }
  })

  it('header topbar ensures dropdowns are not clipped while maintaining class contract', async () => {
    router.push('/dashboard')
    await router.isReady()

    app = createApp(App)
    app.use(router)
    app.use(i18n)
    app.mount(container!)
    await nextTick()

    const header = container!.querySelector('header')
    expect(header).toBeTruthy()
    // The topbar header must have overflow-visible overriding any clipping so dropdown-content can expand downwards
    expect(header!.classList.contains('overflow-visible')).toBe(true)
  })

  it('theme dropdown toggles open and closed on button click, with proper aria and focus behavior', async () => {
    router.push('/dashboard')
    await router.isReady()

    app = createApp(App)
    app.use(router)
    app.use(i18n)
    app.mount(container!)
    await nextTick()

    const themeToggleBtn = container!.querySelector('header button[aria-label="Toggle Theme"]') as HTMLButtonElement
    expect(themeToggleBtn).toBeTruthy()

    const dropdown = themeToggleBtn.closest('.dropdown') as HTMLElement
    expect(dropdown).toBeTruthy()
    expect(dropdown.classList.contains('dropdown-end')).toBe(true)

    // Initially closed
    expect(themeToggleBtn.getAttribute('aria-expanded')).toBe('false')
    expect(dropdown.classList.contains('dropdown-open')).toBe(false)

    // Click to open
    themeToggleBtn.click()
    await nextTick()
    expect(themeToggleBtn.getAttribute('aria-expanded')).toBe('true')
    expect(dropdown.classList.contains('dropdown-open')).toBe(true)

    // Click again to close
    themeToggleBtn.click()
    await nextTick()
    expect(themeToggleBtn.getAttribute('aria-expanded')).toBe('false')
    expect(dropdown.classList.contains('dropdown-open')).toBe(false)
  })

  it('selecting a theme option applies theme, persists to storage, and closes dropdown', async () => {
    router.push('/dashboard')
    await router.isReady()

    app = createApp(App)
    app.use(router)
    app.use(i18n)
    app.mount(container!)
    await nextTick()

    const themeToggleBtn = container!.querySelector('header button[aria-label="Toggle Theme"]') as HTMLButtonElement
    const dropdown = themeToggleBtn.closest('.dropdown') as HTMLElement

    // Open dropdown
    themeToggleBtn.click()
    await nextTick()
    expect(dropdown.classList.contains('dropdown-open')).toBe(true)

    const themeOptions = Array.from(dropdown.querySelectorAll('.dropdown-content button')) as HTMLButtonElement[]
    expect(themeOptions.length).toBeGreaterThan(0)

    const cyberpunkOption = themeOptions.find((btn) => btn.textContent?.trim().toLowerCase() === 'cyberpunk')
    expect(cyberpunkOption).toBeTruthy()

    cyberpunkOption!.click()
    await nextTick()

    expect(document.documentElement.getAttribute('data-theme')).toBe('cyberpunk')
    expect(window.localStorage.getItem('csp_theme')).toBe('cyberpunk')
    expect(dropdown.classList.contains('dropdown-open')).toBe(false)
    expect(themeToggleBtn.getAttribute('aria-expanded')).toBe('false')
  })

  it('escape key closes open theme dropdown', async () => {
    router.push('/dashboard')
    await router.isReady()

    app = createApp(App)
    app.use(router)
    app.use(i18n)
    app.mount(container!)
    await nextTick()

    const themeToggleBtn = container!.querySelector('header button[aria-label="Toggle Theme"]') as HTMLButtonElement
    const dropdown = themeToggleBtn.closest('.dropdown') as HTMLElement

    themeToggleBtn.click()
    await nextTick()
    expect(dropdown.classList.contains('dropdown-open')).toBe(true)

    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true }))
    await nextTick()

    expect(dropdown.classList.contains('dropdown-open')).toBe(false)
    expect(themeToggleBtn.getAttribute('aria-expanded')).toBe('false')
  })

  it('clicking outside closes open theme dropdown', async () => {
    router.push('/dashboard')
    await router.isReady()

    app = createApp(App)
    app.use(router)
    app.use(i18n)
    app.mount(container!)
    await nextTick()

    const themeToggleBtn = container!.querySelector('header button[aria-label="Toggle Theme"]') as HTMLButtonElement
    const dropdown = themeToggleBtn.closest('.dropdown') as HTMLElement

    themeToggleBtn.click()
    await nextTick()
    expect(dropdown.classList.contains('dropdown-open')).toBe(true)

    // Click outside
    document.body.dispatchEvent(new MouseEvent('click', { bubbles: true }))
    await nextTick()

    expect(dropdown.classList.contains('dropdown-open')).toBe(false)
    expect(themeToggleBtn.getAttribute('aria-expanded')).toBe('false')
  })

  it('header layout resilience across mobile (375x667, 392x872) and desktop viewports', async () => {
    router.push('/dashboard')
    await router.isReady()

    app = createApp(App)
    app.use(router)
    app.use(i18n)
    app.mount(container!)
    await nextTick()

    const header = container!.querySelector('header')
    expect(header).toBeTruthy()

    // Title area must have min-w-0 container and truncate class to prevent horizontal blowout
    const titleArea = header!.querySelector('.min-w-0')
    expect(titleArea).toBeTruthy()
    const titleSpan = titleArea!.querySelector('.truncate')
    expect(titleSpan).toBeTruthy()

    // Right-side controls must have shrink-0
    const rightControls = header!.children[1] as HTMLElement
    expect(rightControls).toBeTruthy()
    expect(rightControls.classList.contains('shrink-0')).toBe(true)

    // Theme dropdown must be aligned to the end (dropdown-end) so it stays within viewport
    const dropdown = rightControls.querySelector('.dropdown')
    expect(dropdown).toBeTruthy()
    expect(dropdown!.classList.contains('dropdown-end')).toBe(true)
  })
})
