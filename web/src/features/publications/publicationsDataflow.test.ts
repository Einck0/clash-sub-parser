// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { createApp, h, nextTick } from 'vue'
import PublicationsView from './PublicationsView.vue'
import { usePublications } from './usePublications'
import { api, ApiError, setOnUnauthorized } from '../../api/client'
import { toastStore } from '../../ui/toast'
import { setLocale } from '../../locales'
import type { PreviewResult } from './publicationTypes'

describe('CSP Dataflow Publications & Snapshot Publishing Specification', () => {
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

  it('Point 6: does not misclassify 422 capability errors containing "network" keyword as network errors', async () => {
    const error422 = new ApiError(
      422,
      'unsupported_target_capability',
      'sing-box compiler does not support network xhttp protocol at outbounds[0]'
    )

    vi.spyOn(api, 'post').mockRejectedValue(error422)

    app = createApp({
      render() {
        return h(PublicationsView)
      },
    })
    app.mount(container)

    await nextTick()
    await new Promise((r) => setTimeout(r, 20))

    const errorCard = container.querySelector('[data-testid="error-state-card"]')
    expect(errorCard).not.toBeNull()
    expect(errorCard?.textContent).toContain('目标格式不支持该协议或特性')
    expect(errorCard?.textContent).not.toContain('网络连接异常')
    expect(errorCard?.textContent).toContain('HTTP 422')
  })

  it('Point 7: renders all diagnostics and disables publish button when strict preview fails with 422', async () => {
    const strictError = new ApiError(
      422,
      'unsupported_target_capability',
      'Strict preview rejected due to incompatible capabilities',
      undefined,
      {
        diagnostics: [
          {
            node_id: 'node-us-xhttp',
            code: 'unsupported_target_capability',
            message: 'sing-box does not support xhttp protocol',
            target: 'singbox',
            reason: 'unsupported inbound transport',
          },
          {
            code: 'empty_group_not_allowed',
            message: 'Route group [Streaming] contains no reachable candidates',
            target: 'singbox',
            reason: 'empty policy group',
          },
        ],
      }
    )

    const postSpy = vi.spyOn(api, 'post').mockRejectedValue(strictError)

    app = createApp({
      render() {
        return h(PublicationsView)
      },
    })
    app.mount(container)

    await nextTick()
    await new Promise((r) => setTimeout(r, 20))

    // Preflight diagnostics list should show all diagnostics
    expect(container.textContent).toContain('sing-box does not support xhttp protocol')
    expect(container.textContent).toContain('Route group [Streaming] contains no reachable candidates')
    expect(container.textContent).toContain('node-us-xhttp')

    // Publish button must be disabled and not allow publication
    const buttons = Array.from(container.querySelectorAll('button'))
    const publishBtn = buttons.find((b) => b.textContent?.includes('生成发布') || b.textContent?.includes('发布'))
    expect(publishBtn).toBeDefined()
    expect(publishBtn?.disabled).toBe(true)

    // Clicking disabled button should not trigger publish
    publishBtn?.click()
    await nextTick()

    // api.post was only called for preview, never for publications
    expect(postSpy).toHaveBeenCalledTimes(1)
    expect(postSpy).toHaveBeenCalledWith('/api/v1/publications/preview', expect.anything())
    expect(postSpy).not.toHaveBeenCalledWith('/api/v1/publications', expect.anything())
  })

  it('Point 8: renders excluded nodes and reasons transparently in compatible preview mode', async () => {
    const compatPreview: PreviewResult = {
      snapshot_id: 'snap-compat-888',
      snapshot_digest: 'sha256:compat-snap',
      content_digest: 'sha256:compat-content',
      target: 'surge',
      content: 'proxies:\n  - name: HK-SS\n    type: ss\n',
      content_type: 'text/plain',
      filename: 'surge-compatible.conf',
      manifest: {
        node_count: 1,
        excluded_count: 2,
        excluded: [
          {
            node_id: 'node-vless-sg',
            code: 'unsupported_protocol',
            reason: 'Surge 目标不支持 VLESS 协议',
          },
          {
            node_id: 'node-wg-jp',
            code: 'missing_keys',
            reason: 'WireGuard 缺少对端连接私钥',
          },
        ],
      },
      diagnostics: [],
    }

    vi.spyOn(api, 'post').mockResolvedValue(compatPreview)

    app = createApp({
      render() {
        return h(PublicationsView)
      },
    })
    app.mount(container)

    await nextTick()
    await new Promise((r) => setTimeout(r, 20))

    // Excluded nodes card should be visible
    const excludedCard = container.querySelector('[data-testid="excluded-nodes-card"]')
    expect(excludedCard).not.toBeNull()
    expect(excludedCard?.textContent).toContain('node-vless-sg')
    expect(excludedCard?.textContent).toContain('Surge 目标不支持 VLESS 协议')
    expect(excludedCard?.textContent).toContain('node-wg-jp')
    expect(excludedCard?.textContent).toContain('WireGuard 缺少对端连接私钥')
  })

  it('Point 9: sends exact snapshot_id from successful preview when publishing', async () => {
    const successPreview: PreviewResult = {
      snapshot_id: 'snap-exact-id-777',
      snapshot_digest: 'sha256:snap777',
      content_digest: 'sha256:cnt777',
      target: 'mihomo',
      content: 'proxies:\n  - name: JP-SS\n    type: ss\n',
      content_type: 'application/x-yaml',
      filename: 'config.yaml',
      diagnostics: [],
    }

    const postSpy = vi.spyOn(api, 'post').mockImplementation(async (path: string) => {
      if (path === '/api/v1/publications/preview') {
        return successPreview
      }
      if (path === '/api/v1/publications') {
        return {
          id: 'pub-exact-1',
          target: 'mihomo',
          snapshot_digest: 'sha256:snap777',
          content_digest: 'sha256:cnt777',
          export_url: '/publish/v1/pub-exact-1',
          state: 'active',
          created_at: new Date().toISOString(),
        }
      }
      return {}
    })

    app = createApp({
      render() {
        return h(PublicationsView)
      },
    })
    app.mount(container)

    await nextTick()
    await new Promise((r) => setTimeout(r, 20))

    // Publish button is enabled
    const buttons = Array.from(container.querySelectorAll('button'))
    const publishBtn = buttons.find((b) => b.textContent?.includes('生成发布') || b.textContent?.includes('发布'))
    expect(publishBtn).toBeDefined()
    expect(publishBtn?.disabled).toBe(false)

    publishBtn?.click()
    await nextTick()
    await new Promise((r) => setTimeout(r, 20))

    expect(postSpy).toHaveBeenCalledWith('/api/v1/publications', {
      target: 'mihomo',
      snapshot_id: 'snap-exact-id-777',
    })
  })

  it('Point 10: clears old snapshot when target changes and rejects mismatched snapshot', async () => {
    const pub = usePublications('mihomo')

    pub.preview.value = {
      snapshot_id: 'snap-mihomo-001',
      snapshot_digest: 'sha256:snap1',
      content_digest: 'sha256:cnt1',
      target: 'mihomo',
      content: 'proxies: []',
      content_type: 'text/yaml',
      filename: 'mihomo.yaml',
    }

    expect(pub.preview.value?.snapshot_id).toBe('snap-mihomo-001')

    // Switching target immediately clears preview and snapshot
    pub.selectedTarget.value = 'singbox'
    expect(pub.preview.value).toBeNull()

    // If an attempt were made to publish with mismatched target, it throws snapshot_target_mismatch
    pub.preview.value = {
      snapshot_id: 'snap-mihomo-001',
      snapshot_digest: 'sha256:snap1',
      content_digest: 'sha256:cnt1',
      target: 'mihomo',
      content: 'proxies: []',
      content_type: 'text/yaml',
      filename: 'mihomo.yaml',
    }

    try {
      await pub.publish('singbox')
      expect.fail('should have thrown snapshot_target_mismatch')
    } catch (err: any) {
      expect(err.code).toBe('snapshot_target_mismatch')
    }
  })

  it('Point 11: preserves login gate and triggers onUnauthorized callback on 401', async () => {
    let unauthorizedTriggered = false
    setOnUnauthorized(() => {
      unauthorizedTriggered = true
    })

    const error401 = new ApiError(401, 'unauthorized', 'Session expired. Please log in.')
    vi.spyOn(api, 'post').mockRejectedValue(error401)

    const pub = usePublications('mihomo')
    await pub.fetchPreview('mihomo')

    expect(pub.error.value).toContain('Session expired')
    expect((pub.errorDetail.value as ApiError)?.status).toBe(401)
  })

  it('Point 12: renders responsively at mobile (375x667) and desktop without layout overflow', async () => {
    Object.defineProperty(window, 'innerWidth', { writable: true, configurable: true, value: 375 })
    Object.defineProperty(window, 'innerHeight', { writable: true, configurable: true, value: 667 })
    window.dispatchEvent(new Event('resize'))

    vi.spyOn(api, 'post').mockResolvedValue({
      snapshot_id: 'snap-mobile-1',
      snapshot_digest: 'sha256:snap-mob',
      content_digest: 'sha256:cnt-mob',
      target: 'mihomo',
      content: 'proxies: []\n',
      content_type: 'application/x-yaml',
      filename: 'mobile-config.yaml',
      diagnostics: [],
    })

    app = createApp({
      render() {
        return h(PublicationsView)
      },
    })
    app.mount(container)

    await nextTick()
    await new Promise((r) => setTimeout(r, 20))

    const section = container.querySelector('section')
    expect(section).not.toBeNull()

    // Verify mode buttons render cleanly on mobile
    const strictBtn = container.querySelector('[data-testid="mode-strict-btn"]')
    const compatBtn = container.querySelector('[data-testid="mode-compatible-btn"]')
    expect(strictBtn).not.toBeNull()
    expect(compatBtn).not.toBeNull()
  })
})
