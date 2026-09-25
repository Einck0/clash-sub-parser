// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { createApp, h, nextTick } from 'vue'
import SettingsView from './SettingsView.vue'
import App from '../../App.vue'
import { api } from '../../api/client'
import { toastStore } from '../../ui/toast'
import { ROUTE_STORAGE_KEY } from '../../navigation'
import { setLocale } from '../../locales'
import { router } from '../../router'
import { i18n } from '../../locales'

describe('SettingsView Component', () => {
  let container: HTMLDivElement
  let app: ReturnType<typeof createApp> | null = null

  beforeEach(() => {
    container = document.createElement('div')
    document.body.appendChild(container)
    localStorage.clear()
    toastStore.clear()
    setLocale('en-US')
    vi.restoreAllMocks()
  })

  afterEach(() => {
    if (app) {
      app.unmount()
      app = null
    }
    container.remove()
    localStorage.clear()
    toastStore.clear()
    vi.restoreAllMocks()
  })

  function mountSettings() {
    app = createApp({
      render() {
        return h(SettingsView)
      },
    })
    app.mount(container)

    return {
      getAuthModeBadge: () => container.querySelector('[data-testid="auth-mode-badge"]'),
      getStoredTokenDisplay: () => container.querySelector('[data-testid="stored-token-display"]'),
      getToggleVisibilityBtn: () => container.querySelector('[data-testid="toggle-token-visibility-btn"]') as HTMLButtonElement,
      getTokenInput: () => container.querySelector('input[data-testid="settings-token-input"]') as HTMLInputElement,
      getSaveTokenBtn: () => container.querySelector('button[data-testid="save-token-btn"]') as HTMLButtonElement,
      getTestConnectionBtn: () => container.querySelector('button[data-testid="test-connection-btn"]') as HTMLButtonElement,
      getClearTokenBtn: () => container.querySelector('button[data-testid="clear-token-btn"]') as HTMLButtonElement,
      getRuntimeInfo: () => container.querySelector('[data-testid="runtime-info"]'),
      getConnectionResult: () => container.querySelector('[data-testid="connection-result"]'),
    }
  }

  it('probes auth mode on mount and renders Open Mode with gentle warning style', async () => {
    vi.spyOn(api, 'get').mockImplementation(async (path: string) => {
      if (path === '/api/v1/auth/status') {
        return { mode: 'open', authenticated: true, subject: 'admin' }
      }
      return {}
    })

    const { getAuthModeBadge } = mountSettings()
    await nextTick()
    await new Promise((r) => setTimeout(r, 10))

    const badge = getAuthModeBadge()
    expect(badge).toBeTruthy()
    expect(badge?.textContent).toContain('Open Mode')
  })

  it('probes auth mode on mount and renders Protected with green/success style', async () => {
    vi.spyOn(api, 'get').mockImplementation(async (path: string) => {
      if (path === '/api/v1/auth/status') {
        return { mode: 'protected', authenticated: true, subject: 'admin' }
      }
      return {}
    })

    const { getAuthModeBadge } = mountSettings()
    await nextTick()
    await new Promise((r) => setTimeout(r, 10))

    const badge = getAuthModeBadge()
    expect(badge).toBeTruthy()
    expect(badge?.textContent).toContain('Protected')
  })

  it('displays stored token masked by default and toggles visibility on click', async () => {
    localStorage.setItem('csp_token', 'my-super-secret-admin-token-12345')
    vi.spyOn(api, 'get').mockResolvedValue({ mode: 'protected', authenticated: true, subject: 'admin' })

    const { getStoredTokenDisplay, getToggleVisibilityBtn } = mountSettings()
    await nextTick()
    await new Promise((r) => setTimeout(r, 10))

    const display = getStoredTokenDisplay()
    const toggleBtn = getToggleVisibilityBtn()

    expect(display).toBeTruthy()
    // Default: masked, should not reveal full secret plaintext
    expect(display?.textContent).not.toBe('my-super-secret-admin-token-12345')
    expect(display?.textContent).toContain('••••')

    // Click toggle to show
    toggleBtn.click()
    await nextTick()
    expect(display?.textContent).toBe('my-super-secret-admin-token-12345')

    // Click toggle to mask again
    toggleBtn.click()
    await nextTick()
    expect(display?.textContent).toContain('••••')
  })

  it('displays empty/unconfigured state when no token is stored in localStorage', async () => {
    vi.spyOn(api, 'get').mockResolvedValue({ mode: 'open', authenticated: true, subject: 'admin' })

    const { getStoredTokenDisplay } = mountSettings()
    await nextTick()
    await new Promise((r) => setTimeout(r, 10))

    const display = getStoredTokenDisplay()
    expect(display?.textContent?.toLowerCase()).toMatch(/none|未配置|no token/)
  })

  it('saves new token to localStorage, updates API client and pushes success toast', async () => {
    vi.spyOn(api, 'get').mockResolvedValue({ mode: 'protected', authenticated: true, subject: 'admin' })
    vi.spyOn(api, 'post').mockResolvedValue({ status: 'ok' })
    const setAuthTokenSpy = vi.fn()
    ;(api as any).setAuthToken = setAuthTokenSpy

    const { getTokenInput, getSaveTokenBtn, getStoredTokenDisplay } = mountSettings()
    await nextTick()

    const input = getTokenInput()
    const saveBtn = getSaveTokenBtn()

    input.value = 'newly-created-token-888'
    input.dispatchEvent(new Event('input'))
    await nextTick()

    saveBtn.click()
    await nextTick()
    await new Promise((r) => setTimeout(r, 10))
    await nextTick()

    expect(localStorage.getItem('csp_token')).toBe('newly-created-token-888')
    expect(setAuthTokenSpy).toHaveBeenCalledWith('newly-created-token-888')
    expect(toastStore.items.value.some((t) => t.tone === 'success')).toBe(true)
    expect(getStoredTokenDisplay()?.textContent).toContain('••••')
  })

  it('tests connectivity to backend and shows success state when reachable', async () => {
    const getSpy = vi.spyOn(api, 'get').mockImplementation(async (path: string) => {
      if (path === '/api/v1/auth/status') {
        return { mode: 'protected', authenticated: true, subject: 'admin' }
      }
      return {}
    })

    const { getTestConnectionBtn, getConnectionResult } = mountSettings()
    await nextTick()
    await new Promise((r) => setTimeout(r, 10))

    const testBtn = getTestConnectionBtn()
    testBtn.click()
    await nextTick()
    await new Promise((r) => setTimeout(r, 10))

    expect(getSpy).toHaveBeenCalledWith('/api/v1/auth/status')
    expect(toastStore.items.value.some((t) => t.tone === 'success')).toBe(true)
    expect(getConnectionResult()?.textContent?.toLowerCase()).toMatch(/connected|reachable|正常|success/)
  })

  it('tests connectivity to backend and shows error state when request fails', async () => {
    vi.spyOn(api, 'get').mockImplementation(async (path: string) => {
      if (path === '/api/v1/auth/status') {
        throw new Error('Connection refused')
      }
      return {}
    })

    const { getTestConnectionBtn, getConnectionResult } = mountSettings()
    await nextTick()
    await new Promise((r) => setTimeout(r, 10))

    const testBtn = getTestConnectionBtn()
    testBtn.click()
    await nextTick()
    await new Promise((r) => setTimeout(r, 10))

    expect(toastStore.items.value.some((t) => t.tone === 'error')).toBe(true)
    expect(getConnectionResult()?.textContent?.toLowerCase()).toMatch(/error|failed|refused|失败/)
  })

  it('clears stored token from localStorage, updates API client and pushes info toast', async () => {
    localStorage.setItem('csp_token', 'token-to-clear')
    vi.spyOn(api, 'get').mockResolvedValue({ mode: 'open', authenticated: true, subject: 'admin' })
    vi.spyOn(api, 'post').mockResolvedValue({ status: 'ok' })
    const setAuthTokenSpy = vi.fn()
    ;(api as any).setAuthToken = setAuthTokenSpy

    const { getClearTokenBtn, getStoredTokenDisplay } = mountSettings()
    await nextTick()
    await new Promise((r) => setTimeout(r, 10))

    const clearBtn = getClearTokenBtn()
    clearBtn.click()
    await nextTick()
    await new Promise((r) => setTimeout(r, 10))
    await nextTick()

    expect(localStorage.getItem('csp_token')).toBeNull()
    expect(setAuthTokenSpy).toHaveBeenCalledWith(null)
    expect(getStoredTokenDisplay()?.textContent?.toLowerCase()).toMatch(/none|未配置|no token/)
    expect(toastStore.items.value.some((t) => t.tone === 'info' || t.tone === 'success')).toBe(true)
  })

  it('renders runtime environment info cards', async () => {
    vi.spyOn(api, 'get').mockResolvedValue({ mode: 'open', authenticated: true, subject: 'admin' })

    const { getRuntimeInfo } = mountSettings()
    await nextTick()

    const runtimeInfo = getRuntimeInfo()
    expect(runtimeInfo).toBeTruthy()
    expect(runtimeInfo?.textContent).toContain('Single-Binary Go 1.22+')
    expect(runtimeInfo?.textContent).toContain('internal/webassets')
  })

  it('displays single accurate token status badge without duplication in en and zh', async () => {
    localStorage.setItem('csp_token', 'token-active-123')
    vi.spyOn(api, 'get').mockResolvedValue({ mode: 'protected', authenticated: true, subject: 'admin' })

    mountSettings()
    await nextTick()
    await new Promise((r) => setTimeout(r, 10))

    const statusBadge = container.querySelector('[data-testid="token-status-badge"]')
    expect(statusBadge).toBeTruthy()
    expect(statusBadge?.textContent?.trim()).toBe('Active')
    expect(statusBadge?.textContent).not.toContain('Active Active')

    setLocale('zh-CN')
    await nextTick()
    expect(statusBadge?.textContent?.trim()).toBe('活跃')
    expect(statusBadge?.textContent).not.toContain('Active Active')
  })
})

describe('App.vue Integration: Settings View & Topbar Auth Badge', () => {
  let container: HTMLDivElement
  let app: ReturnType<typeof createApp> | null = null

  beforeEach(() => {
    container = document.createElement('div')
    document.body.appendChild(container)
    localStorage.clear()
    toastStore.clear()
    vi.restoreAllMocks()
  })

  afterEach(() => {
    if (app) {
      app.unmount()
      app = null
    }
    container.remove()
    localStorage.clear()
    toastStore.clear()
    setLocale('en-US')
    vi.restoreAllMocks()
  })

  function mountAppWithMocks(authMode: 'open' | 'token' = 'open') {
    vi.spyOn(api, 'get').mockImplementation(async (path: string) => {
      if (path === '/healthz') return { status: 'healthy' }
      if (path === '/api/v1/auth/status') {
        return {
          mode: authMode === 'open' ? 'open' : 'protected',
          authenticated: true,
          subject: 'admin',
        }
      }
      return { items: [], total: 0 }
    })

    app = createApp(App)
    app.use(router)
    app.use(i18n)
    app.mount(container)

    return {
      getHeaderAuthBadge: () => container.querySelector('[data-testid="header-auth-badge"]') as HTMLElement,
      getSidebarButtons: () => Array.from(container.querySelectorAll('aside nav button')),
      getMainContent: () => container.querySelector('main'),
    }
  }

  it('renders Open Mode badge in topbar header when auth status is open', async () => {
    const { getHeaderAuthBadge } = mountAppWithMocks('open')
    await nextTick()
    await new Promise((r) => setTimeout(r, 20))

    const badge = getHeaderAuthBadge()
    expect(badge).toBeTruthy()
    expect(badge.textContent).toContain('Open Mode')
  })

  it('renders Protected badge in topbar header when auth status is protected', async () => {
    const { getHeaderAuthBadge } = mountAppWithMocks('token')
    await nextTick()
    await new Promise((r) => setTimeout(r, 20))

    const badge = getHeaderAuthBadge()
    expect(badge).toBeTruthy()
    expect(badge.textContent).toContain('Protected')
  })

  it('clicking topbar auth badge navigates to Settings view', async () => {
    const { getHeaderAuthBadge, getMainContent } = mountAppWithMocks('token')
    await nextTick()
    await new Promise((r) => setTimeout(r, 20))

    const badge = getHeaderAuthBadge()
    badge.click()
    await nextTick()
    await new Promise((r) => setTimeout(r, 20))

    const main = getMainContent()
    expect(main?.textContent).toContain('Security & Credentials')
    expect(['#settings', '#/settings']).toContain(window.location.hash)
  })

  it('switches to Settings view when clicking Settings in navigation, replacing Phase 6.1 card', async () => {
    const { getSidebarButtons, getMainContent } = mountAppWithMocks('open')
    await nextTick()
    await new Promise((r) => setTimeout(r, 20))

    const buttons = getSidebarButtons()
    const settingsBtn = buttons.find((b): b is HTMLButtonElement => b instanceof HTMLButtonElement && b.textContent?.includes('Settings') === true)
    expect(settingsBtn).toBeTruthy()

    settingsBtn?.click()
    await nextTick()
    await new Promise((r) => setTimeout(r, 25))

    const main = getMainContent()
    expect(main?.textContent).toContain('Security & Credentials')
    expect(main?.textContent).not.toContain('Phase 6.1')
    expect(['#settings', '#/settings']).toContain(window.location.hash)
  })
})
