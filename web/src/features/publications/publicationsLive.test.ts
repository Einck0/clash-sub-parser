// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { createApp, h, nextTick } from 'vue'
import PublicationsView from './PublicationsView.vue'
import { usePublications, getDirectSubscriptionUrl, getTargetFilename } from './usePublications'
import { api, ApiError } from '../../api/client'
import { toastStore } from '../../ui/toast'
import { setLocale } from '../../locales'

describe('Clean Minimal Client Subscriptions Specification (Zero-Overengineering)', () => {
  let container: HTMLDivElement
  let app: ReturnType<typeof createApp> | null = null

  beforeEach(() => {
    container = document.createElement('div')
    document.body.appendChild(container)
    localStorage.clear()
    sessionStorage.clear()
    toastStore.clear()
    setLocale('zh-CN')
    vi.restoreAllMocks()
    vi.spyOn(api, 'get').mockImplementation(async (path: string) => path === '/api/v1/settings/auth'
      ? { export_mode: api.getAuthToken() ? 'protected' : 'open' }
      : [])

    if (!HTMLDialogElement.prototype.showModal) {
      HTMLDialogElement.prototype.showModal = function (this: HTMLDialogElement) {
        this.setAttribute('open', '')
        Object.defineProperty(this, 'open', { value: true, writable: true, configurable: true })
      }
    }
    if (!HTMLDialogElement.prototype.close) {
      HTMLDialogElement.prototype.close = function (this: HTMLDialogElement) {
        this.removeAttribute('open')
        Object.defineProperty(this, 'open', { value: false, writable: true, configurable: true })
        this.dispatchEvent(new Event('close'))
      }
    }
  })

  afterEach(() => {
    if (app) {
      app.unmount()
      app = null
    }
    container.remove()
    document.body.innerHTML = ''
    localStorage.clear()
    sessionStorage.clear()
    toastStore.clear()
    setLocale('zh-CN')
    vi.restoreAllMocks()
  })

  it('SC-MIN-01 & SC-MIN-05: Renders 4 modern targets in a clean list without UUIDs, state machines, or reset buttons', async () => {
    app = createApp({ render: () => h(PublicationsView) })
    app.mount(container)

    await nextTick()

    // Verifies 4 client subscription rows exist
    const rows = container.querySelectorAll('[data-testid^="client-subscription-row-"]')
    expect(rows).toHaveLength(4)

    expect(container.querySelector('[data-testid="client-subscription-row-mihomo"]')).not.toBeNull()
    expect(container.querySelector('[data-testid="client-subscription-row-singbox"]')).not.toBeNull()
    expect(container.querySelector('[data-testid="client-subscription-row-surge"]')).not.toBeNull()
    expect(container.querySelector('[data-testid="client-subscription-row-qx"]')).not.toBeNull()

    // Verifies no over-engineered elements exist
    expect(container.querySelector('[data-testid="reset-client-subscription-btn"]')).toBeNull()
    expect(container.querySelector('[data-testid="generation-warning-banner"]')).toBeNull()
    expect(container.querySelector('[data-testid="subscription-link-error"]')).toBeNull()
  })

  it('SC-MIN-02 & SC-MIN-03: Open Mode displays and copies clean direct URLs without token query parameter', async () => {
    localStorage.removeItem('csp_token')

    const writeText = vi.fn().mockResolvedValue(undefined)
    Object.assign(navigator, { clipboard: { writeText } })

    app = createApp({ render: () => h(PublicationsView) })
    app.mount(container)

    await Promise.resolve()
    await nextTick()

    // Mihomo input displays /clash
    const mihomoInput = container.querySelector('[data-testid="direct-url-input-mihomo"]') as HTMLInputElement
    expect(mihomoInput.value).toBe(`${window.location.origin}/clash`)

    // Singbox input displays /singbox
    const singboxInput = container.querySelector('[data-testid="direct-url-input-singbox"]') as HTMLInputElement
    expect(singboxInput.value).toBe(`${window.location.origin}/singbox`)

    // Click copy button on Mihomo
    const copyMihomoBtn = container.querySelector('[data-testid-target="copy-client-subscription-mihomo"]') as HTMLButtonElement
    copyMihomoBtn.click()
    await nextTick()
    await new Promise((r) => setTimeout(r, 20))

    expect(writeText).toHaveBeenCalledWith(`${window.location.origin}/clash`)
    expect(toastStore.items.value.length).toBeGreaterThan(0)
    expect(toastStore.items.value[0].tone).toBe('success')
  })

  it('SC-MIN-02: Protected Mode automatically extracts token from localStorage and appends ?token=', async () => {
    localStorage.setItem('csp_token', 'my-admin-settings-secret')

    const writeText = vi.fn().mockResolvedValue(undefined)
    Object.assign(navigator, { clipboard: { writeText } })

    app = createApp({ render: () => h(PublicationsView) })
    app.mount(container)

    await Promise.resolve()
    await nextTick()

    // Input displays direct route with ?token=...
    const surgeInput = container.querySelector('[data-testid="direct-url-input-surge"]') as HTMLInputElement
    expect(surgeInput.value).toBe(`${window.location.origin}/surge?token=my-admin-settings-secret`)

    const copySurgeBtn = container.querySelector('[data-testid-target="copy-client-subscription-surge"]') as HTMLButtonElement
    copySurgeBtn.click()
    await nextTick()

    expect(writeText).toHaveBeenCalledWith(`${window.location.origin}/surge?token=my-admin-settings-secret`)
  })

  it('SC-MIN-04: Opens manual copy fallback modal when clipboard write fails', async () => {
    localStorage.setItem('csp_token', 'fallback-token-456')

    Object.assign(navigator, {
      clipboard: {
        writeText: vi.fn().mockRejectedValue(new Error('Clipboard permission denied')),
      },
    })

    app = createApp({ render: () => h(PublicationsView) })
    app.mount(container)

    await Promise.resolve()
    await nextTick()

    const copyQxBtn = container.querySelector('[data-testid-target="copy-client-subscription-qx"]') as HTMLButtonElement
    copyQxBtn.click()
    await nextTick()
    await new Promise((r) => setTimeout(r, 20))

    const textarea = container.querySelector('[data-testid="manual-copy-textarea"]') as HTMLTextAreaElement
    expect(textarea).not.toBeNull()
    expect(textarea.value).toBe(`${window.location.origin}/qx?token=fallback-token-456`)
  })

  it('Download configuration button triggers download of correct target file via fetchBlob', async () => {
    const pub = usePublications()
    const appendSpy = vi.spyOn(document.body, 'appendChild')
    const originalClick = HTMLAnchorElement.prototype.click
    HTMLAnchorElement.prototype.click = vi.fn()

    const mockBlob = new Blob(['singbox-config-json'])
    const fetchSpy = vi.spyOn(globalThis, 'fetch').mockResolvedValue({
      ok: true,
      status: 200,
      headers: new Headers(),
      blob: async () => mockBlob,
      text: async () => 'singbox-config-json',
    } as any)

    try {
      const res = await pub.downloadDirectConfig('singbox', { mode: 'open' })
      expect(res).toBe(true)
      expect(fetchSpy).toHaveBeenCalledWith(
        expect.stringContaining('/singbox'),
        expect.objectContaining({ method: 'GET' })
      )
      expect(appendSpy).toHaveBeenCalled()

      const createdAnchor = appendSpy.mock.calls[0][0] as HTMLAnchorElement
      expect(createdAnchor.tagName).toBe('A')
      expect(createdAnchor.download).toBe('sing-box.json')
    } finally {
      HTMLAnchorElement.prototype.click = originalClick
      appendSpy.mockRestore()
    }
  })

  it('Compiler 422 error is not treated as a successful download and does not download error file', async () => {
    const pub = usePublications()
    const appendSpy = vi.spyOn(document.body, 'appendChild')

    vi.spyOn(globalThis, 'fetch').mockResolvedValue({
      ok: false,
      status: 422,
      headers: new Headers(),
      text: async () => JSON.stringify({
        error: {
          code: 'empty_group',
          message: 'Route group [Streaming] contains no reachable candidates',
        },
      }),
    } as any)

    await expect(pub.downloadDirectConfig('mihomo', { mode: 'open' })).rejects.toThrow(
      'Route group [Streaming] contains no reachable candidates'
    )

    // Essential contract: anchor must NEVER be created or clicked for 422 compiler error
    expect(appendSpy).not.toHaveBeenCalled()
  })

  it.each([
    {
      name: 'flat envelope',
      payload: { code: 'unsupported_target_capability', message: 'Unsupported transport', request_id: 'flat-request' },
      code: 'unsupported_target_capability', requestId: 'flat-request', message: 'Unsupported transport',
    },
    {
      name: 'nested envelope',
      payload: { error: { code: 'required_nonempty', message: 'Empty group', request_id: 'nested-request' } },
      code: 'required_nonempty', requestId: 'nested-request', message: 'Empty group',
    },
    {
      name: 'malformed message uses HTTP fallback',
      payload: { code: 'format_error', message: { token: 'do-not-render-object' } },
      code: 'format_error', requestId: 'header-request', message: 'HTTP 422',
    },
    {
      name: 'null envelope uses HTTP fallback',
      payload: null,
      code: 'download_compiler_error', requestId: 'header-request', message: 'HTTP 422',
    },
  ])('preserves direct download error identity: $name', async ({ payload, code, requestId, message }) => {
    const pub = usePublications()
    const appendSpy = vi.spyOn(document.body, 'appendChild')
    const blob = vi.fn()
    vi.spyOn(globalThis, 'fetch').mockResolvedValue({
      ok: false,
      status: 422,
      headers: new Headers({ 'X-Request-ID': 'header-request' }),
      text: async () => JSON.stringify(payload),
      blob,
    } as any)

    const err = await pub.downloadDirectConfig('surge', { mode: 'open' }).catch(error => error)
    expect(err).toBeInstanceOf(ApiError)
    expect(err).toMatchObject({ status: 422, code, requestId, message })
    expect(pub.downloadingTargets.value.surge).toBe(false)
    expect(appendSpy).not.toHaveBeenCalled()
    expect(blob).not.toHaveBeenCalled()
  })

  it('Redacts sensitive tokens from compiler download error messages without leaking secret', async () => {
    const pub = usePublications()

    vi.spyOn(globalThis, 'fetch').mockResolvedValue({
      ok: false,
      status: 422,
      headers: new Headers(),
      text: async () => 'Failed compilation for token=my-secret-credential-value in query',
    } as any)

    try {
      await pub.downloadDirectConfig('surge', { mode: 'open' })
      expect.fail('should have thrown error')
    } catch (err: any) {
      expect(err.message).not.toContain('my-secret-credential-value')
      expect(err.message).toContain('token=***')
    }
  })

  it('PublicationsView disables downloads without a verified preview and never fetches a live export', async () => {
    const fetchSpy = vi.spyOn(globalThis, 'fetch')
    const anchorClickSpy = vi.spyOn(HTMLAnchorElement.prototype, 'click').mockImplementation(() => {})

    app = createApp({ render: () => h(PublicationsView) })
    app.mount(container)

    await nextTick()

    const downloadBtn = container.querySelector('[data-testid="download-preview-btn"]')
    expect(downloadBtn).toBeNull()
    expect(container.querySelector('[data-testid="preview-actions"]')).toBeNull()

    await nextTick()

    expect(fetchSpy).not.toHaveBeenCalled()
    expect(anchorClickSpy).not.toHaveBeenCalled()
    expect(toastStore.items.value).toHaveLength(0)
  })

  it('Zero backend CRUD calls: completely eliminates /api/v1/subscription-links', async () => {
    const getSpy = vi.spyOn(api, 'get')
    const postSpy = vi.spyOn(api, 'post')

    app = createApp({ render: () => h(PublicationsView) })
    app.mount(container)

    await nextTick()

    expect(getSpy).not.toHaveBeenCalledWith('/api/v1/subscription-links')
    expect(postSpy).not.toHaveBeenCalledWith('/api/v1/subscription-links', expect.anything())
  })
})
