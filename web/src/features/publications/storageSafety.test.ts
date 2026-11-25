// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { createApp, effectScope, h, nextTick, type EffectScope } from 'vue'
import { api, ApiClient } from '../../api/client'
import PublicationsView from './PublicationsView.vue'
import { setLocale } from '../../locales'
import {
  clearPublicationCapabilities,
  clearStoredPublication,
  getDirectSubscriptionUrl,
  getStoredPublication,
  getStoredTarget,
  publicationSession,
  PUBLICATION_LATEST_KEY,
  PUBLICATION_STORAGE_PREFIX,
  setStoredPublication,
  setStoredTarget,
  usePublications,
} from './usePublications'
import type { PublicationDetail } from './publicationTypes'

const publication: PublicationDetail = {
  id: 'synthetic-publication', target: 'mihomo', state: 'active',
  snapshot_digest: 'synthetic-digest', created_at: '2026-11-25T00:00:00Z',
  export_url: '/publish/v1/synthetic-publication?token=synthetic-capability',
}

function denyStorage(kind: 'localStorage' | 'sessionStorage') {
  return vi.spyOn(window, kind, 'get').mockImplementation(() => {
    throw new DOMException('Storage access denied', 'SecurityError')
  })
}

describe('publication storage property access safety', () => {
  let scope: EffectScope
  let app: ReturnType<typeof createApp> | undefined
  let root: HTMLDivElement
  let clipboardDescriptor: PropertyDescriptor | undefined
  const flush = async () => { for (let i = 0; i < 8; i++) await Promise.resolve(); await nextTick() }
  beforeEach(() => {
    vi.restoreAllMocks()
    clipboardDescriptor = Object.getOwnPropertyDescriptor(navigator, 'clipboard')
    clearStoredPublication()
    window.localStorage.clear()
    window.sessionStorage.clear()
    scope = effectScope()
    setLocale('zh-CN')
    // Fresh native client state per case; do not fake its protected storage behavior.
    const client = new ApiClient({ getAuthToken: () => window.localStorage.getItem('csp_token') })
    vi.spyOn(api, 'getAuthToken').mockImplementation(() => client.getAuthToken())
    vi.spyOn(api, 'setAuthToken').mockImplementation(token => client.setAuthToken(token))
    root = document.createElement('div')
    document.body.appendChild(root)
    HTMLDialogElement.prototype.showModal = function () { this.open = true }
    HTMLDialogElement.prototype.close = function () { this.open = false }
  })
  afterEach(() => {
    app?.unmount()
    app = undefined
    root.remove()
    if (clipboardDescriptor) Object.defineProperty(navigator, 'clipboard', clipboardDescriptor)
    else Reflect.deleteProperty(navigator, 'clipboard')
    scope.stop()
    vi.restoreAllMocks()
    clearStoredPublication()
    window.localStorage.clear()
    window.sessionStorage.clear()
  })

  it('creates the hook and switches target when the sessionStorage getter throws', () => {
    const getter = denyStorage('sessionStorage')
    const state = scope.run(() => usePublications())!
    expect(getter).toHaveBeenCalled()
    expect(state.selectedTarget.value).toBe('mihomo')
    expect(state.activePublication.value).toBeNull()
    expect(() => { state.selectedTarget.value = 'surge' }).not.toThrow()
    expect(state.activePublication.value).toBeNull()
  })

  it('keeps publication reads, writes and clears safe at the getter boundary', () => {
    const getter = denyStorage('sessionStorage')
    expect(getStoredPublication('mihomo')).toBeNull()
    expect(getStoredPublication()).toBeNull()
    expect(setStoredPublication(publication)).toBe(false)
    expect(() => clearStoredPublication('mihomo')).not.toThrow()
    expect(() => clearStoredPublication()).not.toThrow()
    expect(getter).toHaveBeenCalled()

    getter.mockRestore()
    expect(setStoredPublication(publication)).toBe(true)
    expect(getStoredPublication('mihomo')).toEqual(publication)
  })

  it('keeps a successful publication usable in memory when persistence is unavailable', async () => {
    denyStorage('sessionStorage')
    vi.spyOn(api, 'post').mockResolvedValue({ publication, export_url: publication.export_url })
    const state = scope.run(() => usePublications('mihomo'))!
    expect((await state.publish('mihomo')).id).toBe(publication.id)
    expect(state.activePublication.value?.id).toBe(publication.id)
    clearPublicationCapabilities()
    expect(state.activePublication.value).toBeNull()
  })

  it('invalidates memory and late requests on logout even while storage is inaccessible, then removes old capabilities before restoration', async () => {
    const storage = window.sessionStorage
    expect(setStoredPublication(publication)).toBe(true)
    expect(setStoredPublication({ ...publication, id: 'synthetic-surge', target: 'surge' })).toBe(true)
    storage.setItem('unrelated-key', 'keep')
    const state = scope.run(() => usePublications('mihomo'))!
    expect(state.activePublication.value?.id).toBe(publication.id)

    let finish!: (value: unknown) => void
    vi.spyOn(api, 'post').mockImplementationOnce(() => new Promise(resolve => { finish = resolve }))
    const pending = state.publish('mihomo')
    const getter = denyStorage('sessionStorage')
    const previousSession = publicationSession.value
    expect(() => clearPublicationCapabilities()).not.toThrow()
    expect(publicationSession.value).toBe(previousSession + 1)
    expect(state.activePublication.value).toBeNull()
    finish({ publication, export_url: publication.export_url })
    await expect(pending).rejects.toThrow('publication session changed')
    expect(state.activePublication.value).toBeNull()

    // Inaccessible browser storage could not be physically erased yet.
    expect(storage.getItem(PUBLICATION_LATEST_KEY)).not.toBeNull()
    getter.mockRestore()
    const restored = scope.run(() => usePublications('surge'))!
    expect(restored.activePublication.value).toBeNull()
    expect(storage.getItem(PUBLICATION_LATEST_KEY)).toBeNull()
    expect(storage.getItem(`${PUBLICATION_STORAGE_PREFIX}mihomo`)).toBeNull()
    expect(storage.getItem(`${PUBLICATION_STORAGE_PREFIX}surge`)).toBeNull()
    expect(storage.getItem('unrelated-key')).toBe('keep')
  })

  it('does not rehydrate stale capabilities if deferred cache removal still fails', () => {
    const storage = window.sessionStorage
    setStoredPublication(publication)
    const remove = vi.spyOn(Storage.prototype, 'removeItem').mockImplementation(() => {
      throw new DOMException('Storage removal denied', 'SecurityError')
    })
    const previousSession = publicationSession.value
    expect(() => clearPublicationCapabilities()).not.toThrow()
    expect(publicationSession.value).toBe(previousSession + 1)
    expect(getStoredPublication('mihomo')).toBeNull()
    expect(setStoredPublication(publication)).toBe(false)
    remove.mockRestore()
    expect(getStoredPublication('mihomo')).toBeNull()
    expect(storage.getItem(PUBLICATION_LATEST_KEY)).toBeNull()
  })

  it.each(['getter', 'getItem'] as const)('renders the actual protected-link page without a fabricated open URL when localStorage %s fails', async failure => {
    window.localStorage.setItem('csp_token', 'stale-storage-secret')
    const denied = failure === 'getter' ? denyStorage('localStorage')
      : vi.spyOn(Storage.prototype, 'getItem').mockImplementation(() => { throw new DOMException('Read denied', 'SecurityError') })
    const clipboard = vi.fn().mockResolvedValue(undefined)
    Object.defineProperty(navigator, 'clipboard', { configurable: true, value: { writeText: clipboard } })
    vi.spyOn(api, 'get').mockResolvedValue({ export_mode: 'protected' })
    app = createApp({ render: () => h(PublicationsView) })
    const renderError = vi.fn()
    app.config.errorHandler = renderError
    app.mount(root)
    await flush()
    expect(denied).toHaveBeenCalled()
    expect(renderError).not.toHaveBeenCalled()
    expect(root.querySelectorAll('[data-testid^="client-subscription-row-"]')).toHaveLength(4)
    expect((root.querySelector('[data-testid="direct-url-input-mihomo"]') as HTMLInputElement).value).toBe('')
    const copy = root.querySelector('[data-testid-target="copy-client-subscription-mihomo"]') as HTMLButtonElement
    expect(copy.disabled).toBe(true)
    copy.click()
    expect(clipboard).not.toHaveBeenCalled()
    expect(root.querySelector('[data-testid="live-link-unavailable"]')?.textContent).toContain('需要当前会话令牌')
    expect(root.innerHTML).not.toContain('stale-storage-secret')

    // Only a server-confirmed public export mode can enable a tokenless link.
    vi.mocked(api.get).mockResolvedValue({ export_mode: 'open' })
    ;(root.querySelector('[data-testid="live-link-unavailable"] button') as HTMLButtonElement).click()
    await flush()
    expect((root.querySelector('[data-testid="direct-url-input-mihomo"]') as HTMLInputElement).value).toBe(`${location.origin}/clash`)
    expect(copy.disabled).toBe(false)
  })

  it.each(['getter', 'getItem', 'setItem'] as const)('renders and copies the native in-memory token when localStorage %s is restricted', async failure => {
    window.localStorage.setItem('csp_token', 'obsolete-cache-token')
    if (failure === 'getter') denyStorage('localStorage')
    else vi.spyOn(Storage.prototype, failure).mockImplementation(() => { throw new DOMException('Storage denied', 'SecurityError') })
    api.setAuthToken('current-memory-token')
    const clipboard = vi.fn().mockResolvedValue(undefined)
    Object.defineProperty(navigator, 'clipboard', { configurable: true, value: { writeText: clipboard } })
    vi.spyOn(api, 'get').mockResolvedValue({ export_mode: 'protected' })
    app = createApp({ render: () => h(PublicationsView) })
    app.mount(root)
    await flush()
    const expected = `${location.origin}/clash?token=current-memory-token`
    expect((root.querySelector('[data-testid="direct-url-input-mihomo"]') as HTMLInputElement).value).toBe(expected)
    ;(root.querySelector('[data-testid-target="copy-client-subscription-mihomo"]') as HTMLButtonElement).click()
    await flush()
    expect(clipboard).toHaveBeenCalledWith(expected)
    expect(root.innerHTML).not.toContain('obsolete-cache-token')
  })

  it('does not reread an old token after denied removal and clears manual-copy capability on logout', async () => {
    window.localStorage.setItem('csp_token', 'obsolete-cache-token')
    api.setAuthToken('current-memory-token')
    Object.defineProperty(navigator, 'clipboard', { configurable: true, value: { writeText: vi.fn().mockRejectedValue(new Error('Clipboard denied')) } })
    const state = scope.run(() => usePublications())!
    await state.copyDirectUrl('mihomo', { mode: 'protected' })
    expect(state.manualCopyUrl.value).toContain('current-memory-token')
    vi.spyOn(Storage.prototype, 'removeItem').mockImplementation(() => { throw new DOMException('Remove denied', 'SecurityError') })
    api.setAuthToken(null)
    clearPublicationCapabilities()
    expect(getDirectSubscriptionUrl('mihomo')).toBe('')
    expect(getDirectSubscriptionUrl('mihomo', { mode: 'protected' })).toBe('')
    expect(state.manualCopyUrl.value).toBe('')
    expect(state.manualCopyModalOpen.value).toBe(false)
    expect(await state.copyDirectUrl('mihomo')).toEqual({ url: '', copied: false })
    expect(state.manualCopyModalOpen.value).toBe(false)
  })

  it('does not reopen a late manual-copy capability after logout', async () => {
    api.setAuthToken('current-memory-token')
    let reject!: (error: Error) => void
    Object.defineProperty(navigator, 'clipboard', { configurable: true, value: { writeText: vi.fn(() => new Promise<void>((_resolve, fail) => { reject = fail })) } })
    const state = scope.run(() => usePublications())!
    const pending = state.copyDirectUrl('mihomo')
    api.setAuthToken(null)
    clearPublicationCapabilities()
    reject(new Error('Clipboard denied'))
    expect(await pending).toEqual({ url: '', copied: false })
    expect(state.manualCopyUrl.value).toBe('')
    expect(state.manualCopyModalOpen.value).toBe(false)
  })

  it('uses the same in-memory token for direct download URL and Authorization despite storage errors', async () => {
    denyStorage('localStorage')
    api.setAuthToken('download-memory-token')
    const fetcher = vi.spyOn(globalThis, 'fetch').mockResolvedValue({
      ok: false, status: 422, headers: new Headers(), text: async () => 'Unsupported target',
    } as Response)
    const state = scope.run(() => usePublications())!
    await expect(state.downloadDirectConfig('surge')).rejects.toThrow('Unsupported target')
    expect(fetcher).toHaveBeenCalledWith(`${location.origin}/surge?token=download-memory-token`, {
      method: 'GET', headers: { Accept: '*/*', Authorization: 'Bearer download-memory-token' },
    })
  })

  it('handles the localStorage getter on the same hook initialization path', () => {
    const getter = denyStorage('localStorage')
    const state = scope.run(() => usePublications())!
    expect(state.selectedTarget.value).toBe('mihomo')
    expect(getStoredTarget()).toBe('mihomo')
    expect(setStoredTarget('qx')).toBe(false)
    expect(() => { state.selectedTarget.value = 'qx' }).not.toThrow()
    expect(getter).toHaveBeenCalled()
  })
})
