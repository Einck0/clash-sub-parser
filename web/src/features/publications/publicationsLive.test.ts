// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { createApp, h, nextTick } from 'vue'
import PublicationsView from './PublicationsView.vue'
import { usePublications, getDirectSubscriptionUrl, getTargetFilename } from './usePublications'
import { api } from '../../api/client'
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

    await nextTick()

    const copyQxBtn = container.querySelector('[data-testid-target="copy-client-subscription-qx"]') as HTMLButtonElement
    copyQxBtn.click()
    await nextTick()
    await new Promise((r) => setTimeout(r, 20))

    const textarea = container.querySelector('[data-testid="manual-copy-textarea"]') as HTMLTextAreaElement
    expect(textarea).not.toBeNull()
    expect(textarea.value).toBe(`${window.location.origin}/qx?token=fallback-token-456`)
  })

  it('Download configuration button triggers download of correct target file', async () => {
    const pub = usePublications()
    const appendSpy = vi.spyOn(document.body, 'appendChild')
    const originalClick = HTMLAnchorElement.prototype.click
    HTMLAnchorElement.prototype.click = vi.fn()

    try {
      pub.downloadDirectConfig('singbox')
      expect(appendSpy).toHaveBeenCalled()

      const createdAnchor = appendSpy.mock.calls[0][0] as HTMLAnchorElement
      expect(createdAnchor.tagName).toBe('A')
      expect(createdAnchor.download).toBe('sing-box.json')
      expect(createdAnchor.href).toContain('/singbox')
    } finally {
      HTMLAnchorElement.prototype.click = originalClick
      appendSpy.mockRestore()
    }
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
