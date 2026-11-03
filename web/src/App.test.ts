// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { createApp, nextTick, type App as VueApp } from 'vue'
import App from './App.vue'
import { router } from './router'
import { i18n, setLocale } from './locales'
import { api } from './api/client'

describe('App.vue Navigation Integration with Proxy Chains', () => {
  let app: VueApp | null = null
  let container: HTMLDivElement | null = null

  beforeEach(() => {
    container = document.createElement('div')
    document.body.appendChild(container)
    setLocale('zh-CN')

    vi.spyOn(api, 'get').mockImplementation(async (path: string) => {
      if (path === '/healthz') return { status: 'healthy' }
      if (path === '/api/v1/auth/status') return { mode: 'open', authenticated: true, subject: 'admin' }
      if (path === '/api/v1/proxy-chains') return { bindings: [], version: 'empty-v1' }
      if (path === '/api/v1/proxy-chains/options') return { nodes: [], groups: [], subscriptions: [] }
      if (path === '/api/v1/proxy-chains/effects') return { version: 'empty-v1', effects: [], nodes: [] }
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
    setLocale('zh-CN')
  })

  async function mountApp(initialPath = '/chains') {
    router.push(initialPath)
    await router.isReady()

    app = createApp(App)
    app.use(router)
    app.use(i18n)
    app.mount(container!)
    await nextTick()
    await Promise.resolve()
    await nextTick()
  }

  it('renders chains navigation in desktop sidebar under Rules & DNS group', async () => {
    await mountApp('/chains')

    const sidebar = container!.querySelector('aside')
    expect(sidebar).toBeTruthy()

    const chainsBtn = sidebar!.querySelector('button[href="#/chains"], button[href="/chains"]')
    expect(chainsBtn).toBeTruthy()
    expect(chainsBtn?.textContent).toContain('链式代理')
    expect(chainsBtn?.getAttribute('aria-current')).toBe('page')
  })

  it('renders chains entry inside mobile More navigation sheet', async () => {
    await mountApp('/dashboard')

    const moreBtn = container!.querySelector('[data-testid="dock-link-more"]') as HTMLButtonElement
    expect(moreBtn).toBeTruthy()

    moreBtn.click()
    await nextTick()

    const moreSheet = document.body.querySelector('[data-testid="mobile-more-sheet"]')
    expect(moreSheet).toBeTruthy()

    const chainsLink = moreSheet!.querySelector('[data-testid="dock-link-chains"]')
    expect(chainsLink).toBeTruthy()
    expect(chainsLink?.textContent).toContain('链式代理')
  })

  it('updates navigation label when switching locale to en-US', async () => {
    setLocale('en-US')
    await mountApp('/chains')

    const sidebar = container!.querySelector('aside')
    const chainsBtn = sidebar!.querySelector('button[href="#/chains"], button[href="/chains"]')
    expect(chainsBtn?.textContent).toContain('Proxy Chains')
  })

  it('updates aria-current when navigating between routes', async () => {
    await mountApp('/dashboard')

    const sidebar = container!.querySelector('aside')
    const chainsBtn = sidebar!.querySelector('button[href="#/chains"], button[href="/chains"]')
    expect(chainsBtn?.getAttribute('aria-current')).toBeNull()

    const dashboardBtn = sidebar!.querySelector('button[href="#/dashboard"], button[href="/dashboard"]')
    expect(dashboardBtn?.getAttribute('aria-current')).toBe('page')
  })
})
