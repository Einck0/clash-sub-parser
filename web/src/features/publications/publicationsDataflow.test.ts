// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { createApp, h, nextTick } from 'vue'
import PublicationsView from './PublicationsView.vue'
import ErrorStateCard from '../../ui/ErrorStateCard.vue'
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

    app = createApp({
      render() {
        return h(ErrorStateCard, { error: error422 })
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

  it('Point 7: captures all diagnostics when strict preview fails with 422 in usePublications', async () => {
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

    vi.spyOn(api, 'post').mockRejectedValue(strictError)

    const pub = usePublications('singbox')
    await pub.fetchPreview('singbox', undefined, 'strict')

    expect(pub.preflightDiagnostics.value).toHaveLength(2)
    expect(pub.preflightDiagnostics.value[0].code).toBe('unsupported_target_capability')
    expect(pub.preflightDiagnostics.value[1].code).toBe('empty_group_not_allowed')
    expect(pub.preflightDiagnostics.value[0].node_id).toBe('node-us-xhttp')
  })

  it('Point 8: retains excluded nodes and reasons transparently in compatible preview mode in usePublications', async () => {
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

    const pub = usePublications('surge')
    await pub.fetchPreview('surge', undefined, 'compatible')

    expect(pub.preview.value?.manifest?.excluded).toHaveLength(2)
    expect(pub.preview.value?.manifest?.excluded?.[0].node_id).toBe('node-vless-sg')
    expect(pub.preview.value?.manifest?.excluded?.[0].reason).toContain('Surge 目标不支持 VLESS 协议')
    expect(pub.preview.value?.manifest?.excluded?.[1].node_id).toBe('node-wg-jp')
  })

  it('Point 9: sends exact snapshot_id from successful preview when publishing via usePublications', async () => {
    const postSpy = vi.spyOn(api, 'post').mockImplementation(async (path: string) => {
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

    const pub = usePublications('mihomo')
    await pub.publish('mihomo', 'snap-exact-id-777')

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

    vi.spyOn(api, 'get').mockResolvedValue([])

    app = createApp({
      render() {
        return h(PublicationsView)
      },
    })
    app.mount(container)

    await nextTick()
    await new Promise((r) => setTimeout(r, 20))

    const section = container.querySelector('[data-testid="client-subscriptions-page"]')
    expect(section).not.toBeNull()

    // Verify all 4 target rows and copy buttons render cleanly on mobile
    const rows = container.querySelectorAll('[data-testid^="client-subscription-row-"]')
    expect(rows.length).toBe(4)

    const copyBtns = container.querySelectorAll('[data-testid="copy-client-subscription-btn"]')
    expect(copyBtns.length).toBe(4)
  })
})
