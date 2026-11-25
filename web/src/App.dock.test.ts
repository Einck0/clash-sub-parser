// @vitest-environment jsdom
import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import { createApp, nextTick, type App as VueApp } from 'vue'
import App from './App.vue'
import { router } from './router'
import { i18n, setLocale } from './locales'
import { api } from './api/client'

// Browser geometry/contrast is verified against the same-SHA image; these cover interaction contracts.
describe('App navigation, theme and responsive lifecycle', () => {
  let app: VueApp | null = null
  let container: HTMLDivElement
  beforeEach(() => {
    container = document.createElement('div')
    document.body.appendChild(container)
    setLocale('zh-CN')
    localStorage.removeItem('csp_sidebar_collapsed')
    vi.spyOn(api, 'get').mockImplementation(async (path: string) => {
      if (path === '/healthz') return { status: 'healthy' }
      if (path === '/api/v1/auth/status') return { mode: 'open', authenticated: true, subject: '' }
      return { items: [], total: 0 }
    })
  })
  afterEach(() => {
    app?.unmount()
    app = null
    container.remove()
    vi.restoreAllMocks()
    vi.unstubAllGlobals()
    setLocale('zh-CN')
  })
  async function mount() {
    await router.push('/dashboard')
    await router.isReady()
    app = createApp(App)
    app.use(router)
    app.use(i18n)
    app.mount(container)
    await nextTick()
    await Promise.resolve()
    await nextTick()
  }
  const themeButton = () => container.querySelector<HTMLButtonElement>('[data-testid="theme-toggle"]')!
  const moreButton = () => container.querySelector<HTMLButtonElement>('[data-testid="dock-link-more"]')!

  it('keeps three primary routes and six secondary routes accessible without rigid dock heights', async () => {
    await mount()
    const dock = container.querySelector<HTMLElement>('[data-testid="mobile-dock"]')!
    expect(dock).toBeTruthy()
    expect(dock.classList.contains('h-14')).toBe(false)
    expect(dock.classList.contains('min-h-[48px]')).toBe(false)
    expect(dock.classList.contains('py-2')).toBe(true)
    expect(dock.querySelector('.grid-cols-4')).toBeTruthy()
    for (const route of ['nodes', 'policy', 'subscriptions']) {
      expect(dock.querySelector(`a[data-testid="dock-link-${route}"]`)?.getAttribute('href')).toBe(`#/${route}`)
    }
    expect(moreButton().getAttribute('aria-expanded')).toBe('false')
    moreButton().focus()
    moreButton().click()
    await nextTick()
    await nextTick()
    expect(moreButton().getAttribute('aria-expanded')).toBe('true')
    const sheet = document.querySelector('[data-testid="mobile-more-sheet"]')!
    expect(sheet.closest('[role="dialog"]')).toBeTruthy()
    for (const route of ['dashboard', 'dns', 'chains', 'rules', 'publications', 'settings']) {
      expect(sheet.querySelector(`a[data-testid="dock-link-${route}"]`)).toBeTruthy()
    }
    expect(document.body.style.overflow).toBe('hidden')
    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', cancelable: true }))
    await nextTick()
    await nextTick()
    expect(document.querySelector('[data-testid="mobile-more-sheet"]')).toBeNull()
    expect(document.activeElement).toBe(moreButton())
    expect(document.body.style.overflow).not.toBe('hidden')
  })

  it('closes the mobile sheet on desktop resize and does not leave scrolling locked', async () => {
    await mount()
    moreButton().click()
    await nextTick()
    vi.stubGlobal('innerWidth', 1280)
    window.dispatchEvent(new Event('resize'))
    await nextTick()
    expect(moreButton().getAttribute('aria-expanded')).toBe('false')
    expect(document.body.style.overflow).not.toBe('hidden')
  })

  it('provides every desktop route as a native link with a named collapsed state', async () => {
    await mount()
    const sidebar = container.querySelector('aside')!
    sidebar.querySelector<HTMLButtonElement>('[data-testid="sidebar-collapse-toggle"]')!.click()
    await nextTick()
    for (const route of ['dashboard', 'nodes', 'subscriptions', 'policy', 'rules', 'dns', 'chains', 'publications', 'settings']) {
      const link = sidebar.querySelector(`a[href="#/${route}"]`)!
      expect(link).toBeTruthy()
      expect(link.getAttribute('aria-label')).toBeTruthy()
    }
    expect(sidebar.querySelector('[data-testid="sidebar-collapse-toggle"]')?.getAttribute('aria-expanded')).toBe('false')
  })

  it('opens an accessible theme menu, supports arrow keys, persists selection and restores focus', async () => {
    await mount()
    const toggle = themeButton()
    expect(toggle.getAttribute('aria-expanded')).toBe('false')
    toggle.click()
    await nextTick()
    await nextTick()
    expect(toggle.getAttribute('aria-expanded')).toBe('true')
    const options = Array.from(container.querySelectorAll<HTMLButtonElement>('[role="menuitemradio"]'))
    expect(options).toHaveLength(7)
    expect(document.activeElement).toBe(options.find((item) => item.getAttribute('aria-checked') === 'true'))
    document.activeElement!.dispatchEvent(new KeyboardEvent('keydown', { key: 'Home', bubbles: true, cancelable: true }))
    expect(document.activeElement).toBe(options[0])
    document.activeElement!.dispatchEvent(new KeyboardEvent('keydown', { key: 'End', bubbles: true, cancelable: true }))
    expect(document.activeElement).toBe(options.at(-1))
    options.find((item) => item.textContent?.includes('赛博朋克') || item.textContent?.toLowerCase().includes('cyberpunk'))!.click()
    await nextTick()
    expect(document.documentElement.getAttribute('data-theme')).toBe('cyberpunk')
    expect(localStorage.getItem('csp_theme')).toBe('cyberpunk')
    expect(toggle.getAttribute('aria-expanded')).toBe('false')
    expect(document.activeElement).toBe(toggle)
  })

  it('closes theme on Escape or outside click without leaving a mobile scroll lock', async () => {
    await mount()
    themeButton().click()
    await nextTick()
    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true, cancelable: true }))
    await nextTick()
    expect(themeButton().getAttribute('aria-expanded')).toBe('false')
    themeButton().click()
    await nextTick()
    document.body.dispatchEvent(new MouseEvent('click', { bubbles: true }))
    await nextTick()
    expect(themeButton().getAttribute('aria-expanded')).toBe('false')
    expect(document.body.style.overflow).not.toBe('hidden')
  })

  it('keeps the header controls unclipped and updates accessible labels with locale', async () => {
    await mount()
    const header = container.querySelector('header')!
    expect(header.classList.contains('overflow-visible')).toBe(true)
    expect(header.querySelector('.min-w-0')).toBeTruthy()
    expect(header.children[1].classList.contains('shrink-0')).toBe(true)
    expect(themeButton().closest('.dropdown-end')).toBeTruthy()
    expect(themeButton().getAttribute('aria-label')).toBe('切换主题')
    setLocale('en-US')
    await nextTick()
    expect(themeButton().getAttribute('aria-label')).toBe('Choose theme')
    expect(document.documentElement.lang).toBe('en-US')
    expect(header.textContent).toContain('Service online')
  })
})
