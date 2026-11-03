// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { createApp, h, nextTick } from 'vue'
import AuthSecuritySettings from './AuthSecuritySettings.vue'
import { useAuthSettings } from './useAuthSettings'
import { api } from '../../api/client'
import { toastStore } from '../../ui/toast'
import { setLocale } from '../../locales'

describe('AuthSecuritySettings Component', () => {
  let container: HTMLDivElement
  let app: ReturnType<typeof createApp> | null = null

  beforeEach(() => {
    container = document.createElement('div')
    document.body.appendChild(container)
    localStorage.clear()
    toastStore.clear()
    setLocale('zh-CN')
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
    setLocale('zh-CN')
    vi.restoreAllMocks()
  })

  const settingsResponse = (admin = true, exportAuth = true, configured = true) => ({
    admin_auth_enabled: admin,
    export_auth_enabled: exportAuth,
    token_configured: configured,
    admin_mode: admin && configured ? 'protected' : 'open',
    export_mode: exportAuth ? 'protected' : 'open',
  })

  function mountAuthSettings() {
    const auth = useAuthSettings()
    app = createApp({
      render() {
        return h(AuthSecuritySettings, { auth })
      },
    })
    app.mount(container)

    return {
      auth,
      getStoredTokenDisplay: () => container.querySelector('[data-testid="stored-token-display"]'),
      getToggleVisibilityBtn: () => container.querySelector('[data-testid="toggle-token-visibility-btn"]') as HTMLButtonElement,
      getTokenInput: () => container.querySelector('input[data-testid="settings-token-input"]') as HTMLInputElement,
      getSaveTokenBtn: () => container.querySelector('button[data-testid="save-token-btn"]') as HTMLButtonElement,
      getClearTokenBtn: () => container.querySelector('button[data-testid="clear-token-btn"]') as HTMLButtonElement,
      getAdminSwitch: () => container.querySelector('input[data-testid="admin-auth-switch"]') as HTMLInputElement,
      getExportSwitch: () => container.querySelector('input[data-testid="export-auth-switch"]') as HTMLInputElement,
      getSaveSwitchesBtn: () => container.querySelector('button[data-testid="save-auth-switches-btn"]') as HTMLButtonElement,
      getExportModeBadge: () => container.querySelector('[data-testid="export-mode-badge"]'),
      getTestConnectionBtn: () => container.querySelector('button[data-testid="test-connection-btn"]') as HTMLButtonElement,
      getConnectionResult: () => container.querySelector('[data-testid="connection-result"]'),
      getTokenStatusBadge: () => container.querySelector('[data-testid="token-status-badge"]'),
    }
  }

  it('loads auth settings and renders independent switches', async () => {
    vi.spyOn(api, 'get').mockResolvedValue(settingsResponse(true, true, true))
    const { auth, getAdminSwitch, getExportSwitch, getExportModeBadge } = mountAuthSettings()

    auth.syncStoredToken()
    await auth.probeAuthMode()
    await nextTick()

    expect(getAdminSwitch().checked).toBe(true)
    expect(getExportSwitch().checked).toBe(true)
    expect(getExportModeBadge()?.textContent).toContain('受保护')
  })

  it('rejects invalid server response and sets error state instead of showing fake success', async () => {
    vi.spyOn(api, 'get').mockResolvedValue({ invalid: 'payload' })
    const { auth } = mountAuthSettings()

    await auth.probeAuthMode()
    await nextTick()

    expect(auth.authMode.value).toBe('error')
    expect(auth.authSettings.value).toBeNull()
    expect(auth.settingsError.value).toBeTruthy()
  })

  it('saves independent switches with expected payload', async () => {
    vi.spyOn(api, 'get').mockResolvedValue(settingsResponse(true, true, true))
    const putSpy = vi.spyOn(api, 'put').mockResolvedValue(settingsResponse(false, true, true))

    const { auth, getAdminSwitch, getSaveSwitchesBtn } = mountAuthSettings()
    await auth.probeAuthMode()
    await nextTick()

    getAdminSwitch().checked = false
    getAdminSwitch().dispatchEvent(new Event('change'))
    auth.adminEnabled.value = false
    await nextTick()

    getSaveSwitchesBtn().click()
    await nextTick()
    await new Promise((r) => setTimeout(r, 10))

    expect(putSpy).toHaveBeenCalledWith('/api/v1/settings/auth', {
      admin_auth_enabled: false,
      export_auth_enabled: true,
    })
    expect(toastStore.items.value.some((t) => t.tone === 'success')).toBe(true)
  })

  it('prompts confirmation when turning off export protection', async () => {
    vi.spyOn(api, 'get').mockResolvedValue(settingsResponse(true, true, true))
    const putSpy = vi.spyOn(api, 'put').mockResolvedValue(settingsResponse(true, false, true))
    const confirmSpy = vi.spyOn(window, 'confirm').mockReturnValue(false)

    const { auth, getExportSwitch, getSaveSwitchesBtn } = mountAuthSettings()
    await auth.probeAuthMode()
    await nextTick()

    getExportSwitch().checked = false
    auth.exportEnabled.value = false
    await nextTick()

    // When confirm is cancelled, PUT is not called
    getSaveSwitchesBtn().click()
    await nextTick()
    expect(confirmSpy).toHaveBeenCalled()
    expect(putSpy).not.toHaveBeenCalled()

    // When confirm is accepted, PUT is called
    confirmSpy.mockReturnValue(true)
    getSaveSwitchesBtn().click()
    await nextTick()
    await new Promise((r) => setTimeout(r, 10))
    expect(putSpy).toHaveBeenCalledWith('/api/v1/settings/auth', {
      admin_auth_enabled: true,
      export_auth_enabled: false,
    })
  })

  it('masks stored token by default and toggles visibility on button click', async () => {
    localStorage.setItem('csp_token', 'my-super-secret-admin-token-12345')
    vi.spyOn(api, 'get').mockResolvedValue(settingsResponse(true, true, true))

    const { auth, getStoredTokenDisplay, getToggleVisibilityBtn } = mountAuthSettings()
    auth.syncStoredToken()
    await auth.probeAuthMode()
    await nextTick()

    const display = getStoredTokenDisplay()
    const toggleBtn = getToggleVisibilityBtn()

    // Masked by default
    expect(display?.textContent).not.toBe('my-super-secret-admin-token-12345')
    expect(display?.textContent).toContain('••••')

    // Click toggle to reveal
    toggleBtn.click()
    await nextTick()
    expect(display?.textContent).toBe('my-super-secret-admin-token-12345')

    // Click toggle to mask again
    toggleBtn.click()
    await nextTick()
    expect(display?.textContent).toContain('••••')
  })

  it('rejects empty token without calling API', async () => {
    vi.spyOn(api, 'get').mockResolvedValue(settingsResponse(true, true, true))
    const putSpy = vi.spyOn(api, 'put').mockResolvedValue(settingsResponse(true, true, true))

    const { auth, getTokenInput, getSaveTokenBtn } = mountAuthSettings()
    await auth.probeAuthMode()
    await nextTick()

    getTokenInput().value = '   '
    auth.inputToken.value = '   '
    await nextTick()

    // Button should be disabled or action should warn
    expect(getSaveTokenBtn().disabled).toBe(true)
    await auth.saveToken()
    expect(putSpy).not.toHaveBeenCalled()
    expect(toastStore.items.value.some((t) => t.tone === 'warning')).toBe(true)
  })

  it('rotates token, invalidates cookies/CSRF, updates bearer header, and clears input draft', async () => {
    vi.spyOn(api, 'get').mockResolvedValue(settingsResponse(true, true, true))
    vi.spyOn(api, 'put').mockResolvedValue(settingsResponse(true, true, true))
    const setAuthTokenSpy = vi.fn()
    const setCsrfTokenSpy = vi.fn()
    ;(api as any).setAuthToken = setAuthTokenSpy
    ;(api as any).setCsrfToken = setCsrfTokenSpy

    const { auth, getTokenInput, getSaveTokenBtn, getStoredTokenDisplay } = mountAuthSettings()
    await auth.probeAuthMode()
    await nextTick()

    const input = getTokenInput()
    input.value = 'new-rotated-token-9999'
    input.dispatchEvent(new Event('input'))
    auth.inputToken.value = 'new-rotated-token-9999'
    await nextTick()

    getSaveTokenBtn().click()
    await nextTick()
    await new Promise((r) => setTimeout(r, 10))
    await nextTick()

    expect(api.put).toHaveBeenCalledWith('/api/v1/settings/auth', expect.objectContaining({
      token: 'new-rotated-token-9999',
      admin_auth_enabled: true,
      export_auth_enabled: true,
    }))
    expect(localStorage.getItem('csp_token')).toBe('new-rotated-token-9999')
    expect(setAuthTokenSpy).toHaveBeenCalledWith('new-rotated-token-9999')
    expect(setCsrfTokenSpy).toHaveBeenCalledWith(null)
    expect(auth.inputToken.value).toBe('')
    expect(getStoredTokenDisplay()?.textContent).toContain('••••')
    expect(toastStore.items.value.some((t) => t.tone === 'success')).toBe(true)
  })

  it('clears token with confirmation dialog, removing local bearer and resetting server state', async () => {
    localStorage.setItem('csp_token', 'token-to-be-cleared')
    vi.spyOn(api, 'get').mockResolvedValue(settingsResponse(true, true, true))
    vi.spyOn(api, 'put').mockResolvedValue(settingsResponse(true, true, false))
    const setAuthTokenSpy = vi.fn()
    ;(api as any).setAuthToken = setAuthTokenSpy
    vi.spyOn(window, 'confirm').mockReturnValue(true)

    const { auth, getClearTokenBtn, getStoredTokenDisplay } = mountAuthSettings()
    auth.syncStoredToken()
    await auth.probeAuthMode()
    await nextTick()

    getClearTokenBtn().click()
    await nextTick()
    await new Promise((r) => setTimeout(r, 10))
    await nextTick()

    expect(api.put).toHaveBeenCalledWith('/api/v1/settings/auth', expect.objectContaining({
      clear_token: true,
    }))
    expect(localStorage.getItem('csp_token')).toBeNull()
    expect(setAuthTokenSpy).toHaveBeenCalledWith(null)
    expect(getStoredTokenDisplay()?.textContent?.toLowerCase()).toMatch(/none|未配置|no token/)
    expect(toastStore.items.value.some((t) => t.tone === 'success')).toBe(true)
  })

  it('tests backend connectivity and reflects success or failure', async () => {
    const getSpy = vi.spyOn(api, 'get').mockImplementation(async (path: string) => {
      if (path === '/api/v1/auth/status') {
        return { mode: 'protected', authenticated: true, subject: 'admin' }
      }
      return settingsResponse()
    })

    const { auth, getTestConnectionBtn, getConnectionResult } = mountAuthSettings()
    await auth.probeAuthMode()
    await nextTick()

    getTestConnectionBtn().click()
    await nextTick()
    await new Promise((r) => setTimeout(r, 10))

    expect(getSpy).toHaveBeenCalledWith('/api/v1/auth/status')
    expect(toastStore.items.value.some((t) => t.tone === 'success')).toBe(true)
    expect(getConnectionResult()?.textContent?.toLowerCase()).toMatch(/connected|reachable|已连接|可达|正常|success/)

    // Failure case
    getSpy.mockRejectedValueOnce(new Error('Connection timed out'))
    getTestConnectionBtn().click()
    await nextTick()
    await new Promise((r) => setTimeout(r, 10))

    expect(toastStore.items.value.some((t) => t.tone === 'error')).toBe(true)
    expect(getConnectionResult()?.textContent?.toLowerCase()).toMatch(/error|failed|timed out|失败/)
  })

  it('session protection: late response does not overwrite active auth mode', async () => {
    let resolveFirstProbe!: (val: any) => void
    const firstProbePromise = new Promise((resolve) => {
      resolveFirstProbe = resolve
    })

    let probeCount = 0
    vi.spyOn(api, 'get').mockImplementation(async (path) => {
      if (path === '/api/v1/settings/auth') {
        probeCount++
        if (probeCount === 1) return firstProbePromise
        return settingsResponse(true, true, true)
      }
      return { mode: 'protected' }
    })

    const { auth } = mountAuthSettings()

    // Trigger initial slow probe
    const probe1 = auth.probeAuthMode()
    expect(auth.authMode.value).toBe('loading')

    // Trigger second probe which finishes immediately with protected mode
    const probe2 = auth.probeAuthMode()
    await probe2
    expect(auth.authMode.value).toBe('token')

    // First slow probe now finishes with open mode
    resolveFirstProbe(settingsResponse(false, false, false))
    await probe1
    await nextTick()

    // Session protection: late response does NOT overwrite the active auth mode
    expect(auth.authMode.value).toBe('token')
  })
})
