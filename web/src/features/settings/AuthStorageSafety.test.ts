// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { createApp, h, nextTick } from 'vue'
import { createMemoryHistory, createRouter, RouterView } from 'vue-router'
import { api, ApiClient } from '../../api/client'
import { setLocale } from '../../locales'
import { toastStore } from '../../ui/toast'
import { useAuthSettings } from './useAuthSettings'
import AuthSecuritySettings from './AuthSecuritySettings.vue'
import SettingsView from './SettingsView.vue'
import { getDirectSubscriptionUrl } from '../publications/usePublications'

const confirmed = {
  admin_auth_enabled: true, export_auth_enabled: true, token_configured: true,
  admin_mode: 'protected' as const, export_mode: 'protected' as const,
}
const cleared = {
  admin_auth_enabled: false, export_auth_enabled: false, token_configured: false,
  admin_mode: 'open' as const, export_mode: 'open' as const,
}
const flush = async () => { for (let i = 0; i < 8; i++) await Promise.resolve(); await nextTick() }
function denyStorage(failure: 'getter' | 'getItem' | 'setItem' | 'removeItem') {
  const fail = () => { throw new DOMException('Storage denied', 'SecurityError') }
  return failure === 'getter'
    ? vi.spyOn(window, 'localStorage', 'get').mockImplementation(fail)
    : vi.spyOn(Storage.prototype, failure).mockImplementation(fail)
}

describe('authentication settings storage consumer safety', () => {
  let app: ReturnType<typeof createApp> | undefined
  let root: HTMLDivElement
  beforeEach(() => {
    vi.restoreAllMocks()
    window.localStorage.clear()
    window.sessionStorage.clear()
    setLocale('zh-CN')
    toastStore.clear()
    // Keep native ApiClient storage handling and fresh in-memory credentials per case.
    const client = new ApiClient({ getAuthToken: () => window.localStorage.getItem('csp_token') })
    vi.spyOn(api, 'getAuthToken').mockImplementation(() => client.getAuthToken())
    vi.spyOn(api, 'setAuthToken').mockImplementation(token => client.setAuthToken(token))
    vi.spyOn(api, 'getCsrfToken').mockImplementation(() => client.getCsrfToken())
    vi.spyOn(api, 'setCsrfToken').mockImplementation(token => client.setCsrfToken(token))
    vi.spyOn(api, 'get').mockImplementation(async (path: string) => path === '/api/v1/settings/auth' ? { ...confirmed } : { items: [], total: 0 })
    vi.spyOn(window, 'confirm').mockReturnValue(true)
    root = document.createElement('div')
    document.body.appendChild(root)
  })
  afterEach(() => {
    app?.unmount()
    app = undefined
    root.remove()
    vi.restoreAllMocks()
    window.localStorage.clear()
    window.sessionStorage.clear()
    toastStore.clear()
  })

  it.each(['getter', 'getItem'] as const)('does not abort real Settings initialization or server-mode probing after %s failure', async failure => {
    denyStorage(failure)
    const router = createRouter({ history: createMemoryHistory(), routes: [{ path: '/settings', component: SettingsView }] })
    await router.push('/settings?section=security')
    await router.isReady()
    app = createApp({ render: () => h(RouterView) })
    app.use(router)
    const renderError = vi.fn()
    app.config.errorHandler = renderError
    app.mount(root)
    await flush()
    expect(renderError).not.toHaveBeenCalled()
    expect(api.get).toHaveBeenCalledWith('/api/v1/settings/auth')
    expect(root.querySelector('[data-testid="auth-mode-badge"]')?.textContent).toContain('受保护')
    expect(root.querySelector('[data-testid="settings-token-input"]')).not.toBeNull()
  })

  it.each(['getter', 'getItem', 'setItem'] as const)('accepts a successful token rotation despite %s failure and uses the new token without stale cache fallback', async failure => {
    window.localStorage.setItem('csp_token', 'previous-synthetic-token')
    const auth = useAuthSettings()
    auth.syncStoredToken()
    await auth.probeAuthMode()
    app = createApp({ render: () => h(AuthSecuritySettings, { auth }) })
    app.mount(root)
    const put = vi.spyOn(api, 'put').mockResolvedValue({ ...confirmed })
    const reconnect = vi.fn()
    window.addEventListener('csp:auth-success', reconnect)
    try {
      denyStorage(failure)
      auth.inputToken.value = 'replacement-synthetic-token'
      await auth.saveToken()
      await flush()
      expect(put).toHaveBeenCalledTimes(1)
      expect(auth.settingsError.value).toBe('')
      expect(auth.authSettings.value).toEqual(confirmed)
      expect(auth.authMode.value).toBe('token')
      expect(auth.inputToken.value).toBe('')
      expect(auth.isDirty.value).toBe(false)
      expect(api.getAuthToken()).toBe('replacement-synthetic-token')
      expect(api.getCsrfToken()).toBeNull()
      auth.syncStoredToken()
      expect(auth.storedToken.value).toBe('replacement-synthetic-token')
      expect(reconnect).toHaveBeenCalledOnce()
      expect(toastStore.items.value.some(item => item.tone === 'error')).toBe(false)
      expect(toastStore.items.value.some(item => item.tone === 'success')).toBe(true)
      expect(getDirectSubscriptionUrl('mihomo', { mode: 'protected' })).toContain('token=replacement-synthetic-token')
      expect(root.innerHTML).not.toContain('previous-synthetic-token')
      expect(root.textContent).not.toContain('replacement-synthetic-token')
    } finally { window.removeEventListener('csp:auth-success', reconnect) }
  })

  it.each(['getter', 'removeItem'] as const)('accepts server token clearing despite %s failure without re-exposing a stale stored token', async failure => {
    window.localStorage.setItem('csp_token', 'obsolete-synthetic-token')
    const auth = useAuthSettings()
    auth.syncStoredToken()
    await auth.probeAuthMode()
    const put = vi.spyOn(api, 'put').mockResolvedValue({ ...cleared })
    const reconnect = vi.fn()
    window.addEventListener('csp:auth-success', reconnect)
    try {
      denyStorage(failure)
      auth.adminEnabled.value = false
      auth.exportEnabled.value = false
      await auth.clearToken()
      expect(put).toHaveBeenCalledWith('/api/v1/settings/auth', {
        admin_auth_enabled: false, export_auth_enabled: false, clear_token: true,
      })
      expect(auth.settingsError.value).toBe('')
      expect(auth.authSettings.value).toEqual(cleared)
      expect(auth.isDirty.value).toBe(false)
      expect(api.getAuthToken()).toBeNull()
      auth.syncStoredToken()
      expect(auth.storedToken.value).toBe('')
      expect(auth.displayToken.value).not.toContain('obsolete-synthetic-token')
      expect(reconnect).toHaveBeenCalledOnce()
      expect(getDirectSubscriptionUrl('mihomo', { mode: 'protected' })).toBe('')
      expect(getDirectSubscriptionUrl('mihomo', { mode: 'open' })).not.toContain('token=')
      expect(toastStore.items.value.some(item => item.tone === 'error')).toBe(false)
    } finally { window.removeEventListener('csp:auth-success', reconnect) }
  })

  it('retains the token draft and prior credential on an actual server rejection', async () => {
    api.setAuthToken('previous-synthetic-token')
    const auth = useAuthSettings()
    await auth.probeAuthMode()
    const reconnect = vi.fn()
    window.addEventListener('csp:auth-success', reconnect)
    try {
      vi.spyOn(api, 'put').mockRejectedValue(new Error('Server rejected update'))
      denyStorage('getter')
      auth.inputToken.value = 'unsaved-synthetic-token'
      await auth.saveToken()
      expect(auth.settingsError.value).toBe('Server rejected update')
      expect(auth.inputToken.value).toBe('unsaved-synthetic-token')
      expect(auth.isDirty.value).toBe(true)
      expect(api.getAuthToken()).toBe('previous-synthetic-token')
      expect(reconnect).not.toHaveBeenCalled()
    } finally { window.removeEventListener('csp:auth-success', reconnect) }
  })
})
