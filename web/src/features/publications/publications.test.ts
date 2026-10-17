// @vitest-environment jsdom
import { describe, expect, it, vi, beforeEach } from 'vitest'
import {
  COMPILER_TARGETS,
  DEFAULT_COMPILER_TARGET,
  auditEventLabel,
  getTargetMetadata,
  groupTypeLabel,
  isValidCompilerTarget,
  preflightCheckLabel,
  publicationStateLabel,
  targetLabel,
  targetFileExt,
  formatDigest,
  type PreviewResult,
} from './publicationTypes'
import {
  usePublications,
  TARGET_STORAGE_KEY,
  getStoredTarget,
  setStoredTarget,
  clearPublicationCapabilities,
  buildValidatedSubscriptionUrl,
  publicationSession,
} from './usePublications'
import { api, ApiError } from '../../api/client'

describe('publication types and capability boundaries', () => {
  it('defines only the four modern compiler targets defaulting to mihomo', () => {
    const targets = COMPILER_TARGETS.map((t) => t.target)
    expect(targets).toEqual(['mihomo', 'singbox', 'surge', 'qx'])
    expect(DEFAULT_COMPILER_TARGET).toBe('mihomo')
  })

  it('declares truthful capability boundaries per target without claiming all 4 support everything', () => {
    const mihomo = getTargetMetadata('mihomo')
    expect(mihomo.protocols).toEqual(['ss', 'vmess', 'vless', 'trojan', 'hysteria2', 'wireguard', 'tuic', 'http', 'socks5', 'anytls'])
    expect(mihomo.groupTypes).toEqual(['select', 'urltest', 'fallback', 'loadbalance'])
    expect(mihomo.ruleKinds).toContain('GEOSITE')
    expect(mihomo.ruleKinds).toContain('RULE-SET')
    expect(mihomo.desc).toContain('导出完整 Mihomo 配置')

    const singbox = getTargetMetadata('singbox')
    expect(singbox.protocols).toEqual(['ss', 'vmess', 'vless', 'trojan', 'hysteria2', 'wireguard', 'tuic'])
    expect(singbox.groupTypes).toEqual([])
    expect(singbox.ruleKinds).toEqual([])
    expect(singbox.desc).toContain('仅导出 sing-box 节点格式')
    expect(singbox.desc).toContain('忽略策略组与规则')

    const surge = getTargetMetadata('surge')
    expect(surge.protocols).toEqual(['ss', 'vmess', 'trojan', 'hysteria2', 'tuic', 'wireguard'])
    expect(surge.protocols).not.toContain('vless')
    expect(surge.groupTypes).toEqual([])
    expect(surge.ruleKinds).toEqual([])
    expect(surge.desc).toContain('仅导出 Surge 节点格式')
    expect(surge.desc).toContain('忽略策略组与规则')

    const qx = getTargetMetadata('qx')
    expect(qx.protocols).toEqual(['ss', 'vmess', 'trojan'])
    expect(qx.groupTypes).toEqual([])
    expect(qx.ruleKinds).toEqual([])
    expect(qx.desc).toContain('仅导出 Quantumult X 节点格式')
    expect(qx.desc).toContain('忽略策略组与规则')

    expect(groupTypeLabel('select')).toContain('手动选择')
    expect(publicationStateLabel('active')).toBe('已生效')
    expect(publicationStateLabel('revoked')).toBe('已撤销')
    expect(preflightCheckLabel('empty_routed_group')).toBe('空路由策略组检查')
    expect(auditEventLabel('publication.create')).toBe('创建订阅发布')
  })

  it('formats target labels, extensions, and digests accurately', () => {
    expect(targetLabel('mihomo')).toBe('Mihomo')
    expect(targetLabel('singbox')).toBe('sing-box')
    expect(targetLabel('surge')).toBe('Surge')
    expect(targetLabel('qx')).toBe('Quantumult X')

    expect(targetFileExt('mihomo')).toBe('yaml')
    expect(targetFileExt('singbox')).toBe('json')
    expect(targetFileExt('surge')).toBe('conf')
    expect(targetFileExt('qx')).toBe('conf')

    const longDigest = 'a1b2c3d4e5f67890123456789abcdef0123456789abcdef0123456789abcdef0'
    expect(formatDigest(longDigest)).toBe('a1b2c3d4...cdef0')
    expect(formatDigest('short')).toBe('short')
    expect(formatDigest('')).toBe('--')
  })

  it('validates compiler targets strictly against the 4 modern targets', () => {
    expect(isValidCompilerTarget('mihomo')).toBe(true)
    expect(isValidCompilerTarget('singbox')).toBe(true)
    expect(isValidCompilerTarget('surge')).toBe(true)
    expect(isValidCompilerTarget('qx')).toBe(true)

    expect(isValidCompilerTarget('')).toBe(false)
    expect(isValidCompilerTarget(null)).toBe(false)
    expect(isValidCompilerTarget('unknown')).toBe(false)
  })
})

describe('target persistence', () => {
  beforeEach(() => {
    localStorage.clear()
    vi.restoreAllMocks()
  })

  it('loads default modern target mihomo when storage is empty or unrecognized', () => {
    expect(getStoredTarget()).toBe('mihomo')
    localStorage.setItem(TARGET_STORAGE_KEY, 'unrecognized-target')
    expect(getStoredTarget()).toBe('mihomo')
  })

  it('loads and saves valid modern targets', () => {
    expect(setStoredTarget('singbox')).toBe(true)
    expect(localStorage.getItem(TARGET_STORAGE_KEY)).toBe('singbox')
    expect(getStoredTarget()).toBe('singbox')

    expect(setStoredTarget('surge')).toBe(true)
    expect(getStoredTarget()).toBe('surge')

    expect(setStoredTarget('invalid' as any)).toBe(false)
    expect(getStoredTarget()).toBe('surge')
  })
})

describe('usePublications composable', () => {
  beforeEach(() => {
    localStorage.clear()
    sessionStorage.clear()
    vi.restoreAllMocks()
  })

  it('clears only the revoked target capability, preserving other target and unrelated session data', async () => {
    const { setStoredPublication, clearStoredPublication, getStoredPublication } = await import('./usePublications')
    const mihomo = { id: 'pub-mihomo', target: 'mihomo', export_url: '/publish/v1/one?token=one' } as any
    const singbox = { id: 'pub-singbox', target: 'singbox', export_url: '/publish/v1/two?token=two' } as any
    setStoredPublication(mihomo)
    setStoredPublication(singbox)
    sessionStorage.setItem('unrelated_key', 'keep-me')
    clearStoredPublication('mihomo')
    expect(getStoredPublication('mihomo')).toBeNull()
    // Revoking the latest must not delete a different target's slot.
    setStoredPublication(mihomo)
    clearStoredPublication('singbox')
    expect(getStoredPublication('mihomo')?.id).toBe('pub-mihomo')
    expect(getStoredPublication('singbox')).toBeNull()
    expect(sessionStorage.getItem('unrelated_key')).toBe('keep-me')
  })

  it('does not restore a late publication response after the auth session ends', async () => {
    const { clearPublicationCapabilities } = await import('./usePublications')
    let finish!: (value: unknown) => void
    vi.spyOn(api, 'post').mockImplementationOnce(() => new Promise(resolve => { finish = resolve }))
    const publication = usePublications('mihomo')
    const pending = publication.publish('mihomo')
    clearPublicationCapabilities()
    finish({ publication: { id: 'late-pub', target: 'mihomo' }, export_url: '/publish/v1/late?token=late' })
    await expect(pending).rejects.toThrow('publication session changed')
    expect(publication.activePublication.value).toBeNull()
    expect(sessionStorage.getItem('csp_publication_latest')).toBeNull()
  })

  it('initializes with default target mihomo', () => {
    const { selectedTarget } = usePublications()
    expect(selectedTarget.value).toBe('mihomo')
  })

  it('fetches rendered preview via POST /api/v1/publications/preview for mihomo with WireGuard, TUIC, Hysteria2, and SS', async () => {
    const mockPreview: PreviewResult = {
      target: 'mihomo',
      snapshot_digest: 'sha256:snap123',
      content_digest: 'sha256:content456',
      content:
        'proxies:\n  - name: JP WireGuard\n    type: wireguard\n    server: wg.jp.example.com\n    port: 51820\n    ip: 10.0.0.2/32\n    public-key: wg-pub-1\n  - name: SG TUIC\n    type: tuic\n    server: tuic.sg.example.com\n    port: 8443\n    uuid: 00000000-0000-4000-8000-000000000001\n',
      content_type: 'application/x-yaml',
      filename: 'mihomo-config.yaml',
      diagnostics: [],
    }

    const postSpy = vi.spyOn(api, 'post').mockResolvedValueOnce(mockPreview)

    const { preview, loadingPreview, fetchPreview, selectedTarget } = usePublications()
    await fetchPreview('mihomo')

    expect(postSpy).toHaveBeenCalledWith('/api/v1/publications/preview', { target: 'mihomo', compat_mode: 'strict' })
    expect(loadingPreview.value).toBe(false)
    expect(selectedTarget.value).toBe('mihomo')
    expect(preview.value).toEqual(mockPreview)
    expect(preview.value?.content).toContain('JP WireGuard')
    expect(preview.value?.content).toContain('SG TUIC')
  })

  it('creates an immutable publication via POST /api/v1/publications and revokes it via POST /api/v1/publications/{id}/revoke', async () => {
    const mockPub = {
      publication: {
        id: 'pub-uuid-1',
        target: 'mihomo',
        snapshot_digest: 'sha256:snap1',
        state: 'active',
        created_at: '2026-09-16T10:00:00Z',
      },
      export_url: '/publish/v1/pub-uuid-1?token=export_sec_token',
      content_digest: 'sha256:cnt1',
    }

    const postSpy = vi.spyOn(api, 'post').mockResolvedValueOnce(mockPub)

    const { activePublication, publishing, revoking, publish, revoke } = usePublications()
    const res = await publish('mihomo')

    expect(postSpy).toHaveBeenCalledWith('/api/v1/publications', { target: 'mihomo' })
    expect(publishing.value).toBe(false)
    expect(activePublication.value?.id).toBe('pub-uuid-1')
    expect(activePublication.value?.target).toBe('mihomo')
    expect(res.export_url).toContain('export_sec_token')

    postSpy.mockResolvedValueOnce({
      id: 'pub-uuid-1',
      state: 'revoked',
    })
    await revoke('pub-uuid-1')
    expect(postSpy).toHaveBeenCalledWith('/api/v1/publications/pub-uuid-1/revoke')
    expect(revoking.value).toBe(false)
    expect(activePublication.value?.state).toBe('revoked')
    expect(activePublication.value?.revoked_at).toBeTruthy()
  })

  it('handles 409 no_active_revision and 422 unsupported_target_capability errors cleanly', async () => {
    const apiError = new ApiError(409, 'no_active_revision', 'cannot resolve policy without an active configuration revision')
    vi.spyOn(api, 'post').mockRejectedValueOnce(apiError)

    const { preview, error, errorDetail, isNoActiveRevision, fetchPreview } = usePublications()
    const result = await fetchPreview('mihomo')

    expect(result).toBeNull()
    expect(preview.value).toBeNull()
    expect(error.value).toBe('cannot resolve policy without an active configuration revision')
    expect(errorDetail.value).toBe(apiError)
    expect(isNoActiveRevision.value).toBe(true)

    const capError = new ApiError(422, 'unsupported_target_capability', 'surge target does not support vless protocol at nodes[0]')
    vi.spyOn(api, 'post').mockRejectedValueOnce(capError)

    const capState = usePublications('surge')
    const capRes = await capState.fetchPreview('surge')
    expect(capRes).toBeNull()
    expect((capState.errorDetail.value as ApiError)?.code).toBe('unsupported_target_capability')
  })

  it('keeps sing-box selected while delayed Mihomo publication resolves and restores Mihomo when selected again', async () => {
    let finish!: (value: unknown) => void
    vi.spyOn(api, 'post').mockImplementationOnce(() => new Promise(resolve => { finish = resolve }))
    const state = usePublications('mihomo')
    const pending = state.publish('mihomo')
    state.selectedTarget.value = 'singbox'
    expect(state.activePublication.value).toBeNull()
    finish({ publication: { id: 'delayed-mihomo', target: 'mihomo' }, export_url: '/publish/v1/delayed-mihomo?token=secret' })
    await pending
    expect(state.selectedTarget.value).toBe('singbox')
    expect(getStoredTarget()).toBe('singbox')
    expect(state.activePublication.value).toBeNull()
    expect(state.getFullExportUrl()).toBe('')
    state.selectedTarget.value = 'mihomo'
    expect(state.activePublication.value?.id).toBe('delayed-mihomo')
    expect(state.getFullExportUrl()).toContain('token=secret')
  })

  it('ignores delayed preview/error from a previously selected target', async () => {
    let finish!: (value: unknown) => void
    vi.spyOn(api, 'post').mockImplementationOnce(() => new Promise(resolve => { finish = resolve }))
    const state = usePublications('mihomo')
    const pending = state.fetchPreview('mihomo')
    state.selectedTarget.value = 'singbox'
    finish({ target: 'mihomo', content: 'old' })
    await pending
    expect(state.selectedTarget.value).toBe('singbox')
    expect(state.preview.value).toBeNull()
  })

  it('handles clipboard copying and tracks copied state', async () => {
    const writeText = vi.fn().mockResolvedValue(undefined)
    Object.assign(navigator, {
      clipboard: {
        writeText,
      },
    })

    const { copyToClipboard, copied } = usePublications()
    const success = await copyToClipboard('test config content')

    expect(success).toBe(true)
    expect(writeText).toHaveBeenCalledWith('test config content')
    expect(copied.value).toBe(true)
  })

  it('persists active publication capability token in sessionStorage and restores it on reload', async () => {
    sessionStorage.clear()
    const mockPub = {
      publication: {
        id: 'pub-reload-1',
        target: 'mihomo',
        snapshot_digest: 'sha256:snap-reload',
        state: 'active',
        created_at: '2026-09-29T10:00:00Z',
      },
      export_url: '/publish/v1/pub-reload-1?token=secret_capability_token_xyz',
      content_digest: 'sha256:cnt-reload',
    }

    vi.spyOn(api, 'post').mockResolvedValueOnce(mockPub)

    const instance1 = usePublications('mihomo')
    await instance1.publish('mihomo')

    expect(instance1.activePublication.value?.export_url).toBe(
      '/publish/v1/pub-reload-1?token=secret_capability_token_xyz'
    )
    expect(sessionStorage.getItem('csp_publication_latest')).toContain('secret_capability_token_xyz')

    // Simulate page reload by creating a fresh composable instance
    const instance2 = usePublications('mihomo')
    expect(instance2.activePublication.value).not.toBeNull()
    expect(instance2.activePublication.value?.id).toBe('pub-reload-1')
    expect(instance2.activePublication.value?.export_url).toBe(
      '/publish/v1/pub-reload-1?token=secret_capability_token_xyz'
    )

    const fullUrl = instance2.getFullExportUrl()
    expect(fullUrl).toContain('token=secret_capability_token_xyz')
  })
})

describe('PublicationsView component rendering', () => {
  beforeEach(() => {
    localStorage.clear()
    sessionStorage.clear()
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

  it('fetches real plaintext WG/TUIC configuration via usePublications and renders client subscriptions without exposing tokens', async () => {
    const rawConfig = [
      'proxies:',
      '  - name: WG-Edge',
      '    type: wireguard',
      '    server: 198.51.100.10',
      '    port: 51820',
      '    ip: 10.0.0.2/32',
      '    public-key: wg-public-key-visible',
      '    private-key: wg-super-secret-privkey',
      '    pre-shared-key: wg-super-secret-psk',
      '  - name: TUIC-Edge',
      '    type: tuic',
      '    server: 198.51.100.11',
      '    port: 8443',
      '    uuid: 00000000-0000-4000-8000-000000000099',
      '    password: tuic-super-secret-password',
      '    congestion-controller: bbr',
    ].join('\n')

    vi.spyOn(api, 'post').mockResolvedValue({
      target: 'mihomo',
      snapshot_digest: 'sha256:test',
      content_digest: 'sha256:test',
      content: rawConfig,
      content_type: 'application/x-yaml',
      filename: 'mihomo.yaml',
      diagnostics: [],
    })

    const pub = usePublications('mihomo')
    const preview = await pub.fetchPreview('mihomo')
    expect(preview?.content).toBe(rawConfig)
    expect(preview?.content).toContain('wg-super-secret-privkey')
    expect(preview?.content).toContain('tuic-super-secret-password')

    const { default: PublicationsView } = await import('./PublicationsView.vue')
    const { createApp, h, nextTick } = await import('vue')

    vi.spyOn(api, 'get').mockResolvedValue([
      {
        id: 'link-mihomo',
        target: 'mihomo',
        token: 'sec_plain_token',
        token_masked: 'sec_1234****',
        state: 'active',
        is_default: true,
        created_at: new Date().toISOString(),
        updated_at: new Date().toISOString(),
      },
    ])

    const mountEl = document.createElement('div')
    document.body.appendChild(mountEl)
    const testApp = createApp({ render: () => h(PublicationsView) })
    testApp.mount(mountEl)
    await nextTick()
    await new Promise((r) => setTimeout(r, 20))

    expect(mountEl.textContent).not.toContain('sec_plain_token')
    testApp.unmount()
    mountEl.remove()
  })

  it('renders only 4 modern target rows defaulting to Mihomo with accurate capability boundaries', async () => {
    const { default: PublicationsView } = await import('./PublicationsView.vue')
    const { createApp, h, nextTick } = await import('vue')

    vi.spyOn(api, 'get').mockResolvedValue([])

    const mountEl = document.createElement('div')
    document.body.appendChild(mountEl)
    const testApp = createApp({ render: () => h(PublicationsView) })
    testApp.mount(mountEl)
    await nextTick()
    await new Promise((r) => setTimeout(r, 20))

    const rows = Array.from(mountEl.querySelectorAll('[data-testid^="client-subscription-row-"]'))
    expect(rows.length).toBe(4)

    const rowTexts = rows.map((r) => r.textContent || '')
    expect(rowTexts.some((t) => t.includes('Mihomo') && t.includes('完整配置'))).toBe(true)
    expect(rowTexts.some((t) => t.includes('sing-box') && t.includes('节点订阅'))).toBe(true)
    expect(rowTexts.some((t) => t.includes('Surge') && t.includes('节点订阅'))).toBe(true)
    expect(rowTexts.some((t) => t.includes('Quantumult X') && t.includes('节点订阅'))).toBe(true)

    testApp.unmount()
    mountEl.remove()
  })

  it('isolates target requests so delayed responses do not corrupt other targets', async () => {
    let resolveMihomo!: (val: any) => void
    const slowMihomo = new Promise((resolve) => { resolveMihomo = resolve })

    const pub = usePublications('mihomo')
    vi.spyOn(api, 'post').mockImplementation(async (path: string) => {
      if (path.includes('link-m/copy')) return slowMihomo
      if (path.includes('link-s/copy')) return { id: 'link-s', target: 'singbox', url: '/subscribe/v1/link-s?token=s', public_base_url: '' }
      return {}
    })

    pub.subscriptionLinks.value = [
      { id: 'link-m', label: 'Mihomo', target: 'mihomo', state: 'active', is_default: true, created_at: '', updated_at: '' },
      { id: 'link-s', label: 'SingBox', target: 'singbox', state: 'active', is_default: true, created_at: '', updated_at: '' },
    ]

    const p1 = pub.copyDefaultClientSubscription('mihomo')
    const p2 = pub.copyDefaultClientSubscription('singbox')

    resolveMihomo({ id: 'link-m', target: 'mihomo', url: '/subscribe/v1/link-m?token=m', public_base_url: '' })
    const [res1, res2] = await Promise.all([p1, p2])

    expect(res1.url).toContain('link-m')
    expect(res2.url).toContain('link-s')
  })

  it('renders recoverable guidance card and disables export buttons on 409 no_active_revision in usePublications', async () => {
    const apiError = new ApiError(409, 'no_active_revision', 'cannot resolve policy without an active configuration revision')
    vi.spyOn(api, 'post').mockRejectedValue(apiError)

    const pub = usePublications('mihomo')
    await pub.fetchPreview('mihomo')

    expect(pub.isNoActiveRevision.value).toBe(true)
    expect((pub.errorDetail.value as ApiError)?.code).toBe('no_active_revision')
  })

  it('renders error alert in PublicationsView when linkError occurs', async () => {
    const { default: PublicationsView } = await import('./PublicationsView.vue')
    const { createApp, h, nextTick } = await import('vue')

    vi.spyOn(api, 'get').mockResolvedValue([])
    vi.spyOn(api, 'post').mockRejectedValue(new Error('Network gateway timeout'))

    const mountEl = document.createElement('div')
    document.body.appendChild(mountEl)
    const testApp = createApp({ render: () => h(PublicationsView) })
    testApp.mount(mountEl)
    await nextTick()
    await new Promise((r) => setTimeout(r, 20))

    const copyBtn = mountEl.querySelector('[data-testid="copy-client-subscription-btn"]') as HTMLButtonElement
    copyBtn.click()
    await nextTick()
    await new Promise((r) => setTimeout(r, 30))

    const errAlert = mountEl.querySelector('[data-testid="subscription-link-error"]')
    expect(errAlert).not.toBeNull()
    expect(errAlert?.textContent).toContain('Network gateway timeout')

    testApp.unmount()
    mountEl.remove()
  })

  it('captures filtered_nodes_empty diagnostics on rejected preview in usePublications', async () => {
    const response = {
      code: 'publication_preflight_rejected',
      diagnostics: [{ code: 'filtered_nodes_empty', severity: 'error', message: 'Configured node filters left no exportable nodes' }],
    }
    vi.spyOn(api, 'post').mockRejectedValue(new ApiError(409, response.code, 'Publication preflight rejected', undefined, response))

    const pub = usePublications('mihomo')
    await pub.fetchPreview('mihomo')

    expect(pub.preflightDiagnostics.value).toHaveLength(1)
    expect(pub.preflightDiagnostics.value[0].code).toBe('filtered_nodes_empty')
  })

  it('switches between different stored target links without exposing the previous target', async () => {
    const { default: PublicationsView } = await import('./PublicationsView.vue')
    const { createApp, h, nextTick } = await import('vue')

    const links = [
      { id: 'link-m', label: 'Mihomo', target: 'mihomo', state: 'active', is_default: true, created_at: '', updated_at: '' },
      { id: 'link-s', label: 'SingBox', target: 'singbox', state: 'active', is_default: true, created_at: '', updated_at: '' },
    ]
    vi.spyOn(api, 'get').mockResolvedValue(links)
    vi.spyOn(api, 'post').mockImplementation(async (path: string) => {
      if (path.includes('link-s/copy')) {
        return { id: 'link-s', target: 'singbox', path: '/subscribe/v1/link-s?token=tok-s', url: '/subscribe/v1/link-s?token=tok-s', public_base_url: '' }
      }
      return {}
    })

    const writeText = vi.fn().mockResolvedValue(undefined)
    Object.assign(navigator, { clipboard: { writeText } })

    const mountEl = document.createElement('div')
    document.body.appendChild(mountEl)
    const app = createApp({ render: () => h(PublicationsView) })
    app.mount(mountEl)
    await nextTick()
    await new Promise(r => setTimeout(r, 20))

    const singboxCopyBtn = mountEl.querySelector('[data-testid-target="copy-client-subscription-singbox"]') as HTMLButtonElement
    expect(singboxCopyBtn).not.toBeNull()
    singboxCopyBtn.click()
    await nextTick()
    await new Promise(r => setTimeout(r, 30))

    expect(writeText).toHaveBeenCalledWith(expect.stringContaining('/subscribe/v1/link-s?token=tok-s'))
    expect(writeText).not.toHaveBeenCalledWith(expect.stringContaining('/subscribe/v1/link-m'))

    app.unmount()
    mountEl.remove()
  })

  it('does not offer or copy the previous target subscription after switching to an unpublished target', async () => {
    const pub = usePublications('mihomo')
    pub.subscriptionLinks.value = [
      { id: 'link-m', label: 'Mihomo', target: 'mihomo', state: 'active', is_default: true, created_at: '', updated_at: '' },
    ]

    const postSpy = vi.spyOn(api, 'post').mockImplementation(async (path: string) => {
      if (path === '/api/v1/subscription-links') {
        return { id: 'link-new-surge', label: 'Surge', target: 'surge', state: 'active', is_default: true, created_at: '', updated_at: '' }
      }
      if (path.includes('link-new-surge/copy')) {
        return { id: 'link-new-surge', target: 'surge', path: '/subscribe/v1/link-new-surge?token=tok-surge', url: '/subscribe/v1/link-new-surge?token=tok-surge', public_base_url: '' }
      }
      return {}
    })

    const writeText = vi.fn().mockResolvedValue(undefined)
    Object.assign(navigator, { clipboard: { writeText } })

    const res = await pub.copyDefaultClientSubscription('surge')
    expect(postSpy).toHaveBeenCalledWith('/api/v1/subscription-links', { target: 'surge', is_default: true })
    expect(res.url).toContain('link-new-surge')
    expect(res.url).not.toContain('link-m')
  })

  it('restores active publication and copies full subscription URL with token after page reload', async () => {
    const { default: PublicationsView } = await import('./PublicationsView.vue')
    const { createApp, h, nextTick } = await import('vue')

    const serverLinks = [
      {
        id: 'link-reload-1',
        label: 'Mihomo 默认订阅',
        target: 'mihomo',
        token_masked: 'sec_1234****',
        state: 'active',
        is_default: true,
        created_at: new Date().toISOString(),
        updated_at: new Date().toISOString(),
      },
    ]

    vi.spyOn(api, 'get').mockResolvedValue(serverLinks)
    vi.spyOn(api, 'post').mockImplementation(async (path: string) => {
      if (path.includes('/copy')) {
        return {
          id: 'link-reload-1',
          target: 'mihomo',
          path: '/subscribe/v1/link-reload-1?token=persisted_token_999',
          url: '/subscribe/v1/link-reload-1?token=persisted_token_999',
          public_base_url: '',
        }
      }
      return {}
    })

    const writeText = vi.fn().mockResolvedValue(undefined)
    Object.assign(navigator, { clipboard: { writeText } })

    const mountEl = document.createElement('div')
    document.body.appendChild(mountEl)
    const testApp = createApp({ render: () => h(PublicationsView) })
    testApp.mount(mountEl)
    await nextTick()
    await new Promise((r) => setTimeout(r, 20))

    const copyBtn = mountEl.querySelector('[data-testid-target="copy-client-subscription-mihomo"]') as HTMLButtonElement
    expect(copyBtn).not.toBeNull()
    copyBtn.click()
    await nextTick()
    await new Promise((r) => setTimeout(r, 30))

    expect(writeText).toHaveBeenCalledTimes(1)
    const copiedUrl = writeText.mock.calls[0][0] as string
    expect(copiedUrl).toContain('/subscribe/v1/link-reload-1?token=persisted_token_999')

    testApp.unmount()
    mountEl.remove()
  })

  it('switches copied links between configured public base URL and origin resolution', async () => {
    const pub = usePublications('mihomo')
    pub.subscriptionLinks.value = [
      { id: 'link-pub-1', label: 'Mihomo', target: 'mihomo', state: 'active', is_default: true, created_at: '', updated_at: '' },
    ]

    const writeText = vi.fn().mockResolvedValue(undefined)
    Object.assign(navigator, { clipboard: { writeText } })

    // 1. With public base url
    vi.spyOn(api, 'post').mockResolvedValueOnce({
      id: 'link-pub-1',
      target: 'mihomo',
      path: '/subscribe/v1/link-pub-1?token=tok-1',
      url: 'https://sub.mycorp.org/subscribe/v1/link-pub-1?token=tok-1',
      public_base_url: 'https://sub.mycorp.org',
    })

    const res1 = await pub.copySubscriptionLink('link-pub-1', 'mihomo')
    expect(res1.url).toBe('https://sub.mycorp.org/subscribe/v1/link-pub-1?token=tok-1')

    // 2. Without public base url (empty) -> uses origin
    vi.spyOn(api, 'post').mockResolvedValueOnce({
      id: 'link-pub-1',
      target: 'mihomo',
      path: '/subscribe/v1/link-pub-1?token=tok-1',
      url: '/subscribe/v1/link-pub-1?token=tok-1',
      public_base_url: '',
    })

    const res2 = await pub.copySubscriptionLink('link-pub-1', 'mihomo')
    expect(res2.url).toBe(`${window.location.origin}/subscribe/v1/link-pub-1?token=tok-1`)
  })

  it('management copy API generates independent export tokens, avoiding leaking admin credentials', async () => {
    const pub = usePublications('mihomo')
    pub.subscriptionLinks.value = [
      { id: 'link-auth-1', label: 'Mihomo', target: 'mihomo', state: 'active', is_default: true, created_at: '', updated_at: '' },
    ]

    const postSpy = vi.spyOn(api, 'post').mockResolvedValue({
      id: 'link-auth-1',
      target: 'mihomo',
      path: '/subscribe/v1/link-auth-1?token=sec_independent_export_token',
      url: '/subscribe/v1/link-auth-1?token=sec_independent_export_token',
      token: 'sec_independent_export_token',
      public_base_url: '',
    })

    const res = await pub.copySubscriptionLink('link-auth-1', 'mihomo')
    expect(postSpy).toHaveBeenCalledWith('/api/v1/subscription-links/link-auth-1/copy')
    expect(res.url).toContain('token=sec_independent_export_token')
    expect(res.url).not.toContain('admin-secret-token')
  })

  describe('S309 Remediation Behavior & Safety Tests', () => {
    it('Point 1: copySubscriptionLink cancels side effects if session is invalidated during /copy network call', async () => {
      let resolveCopy!: (val: any) => void
      const copyPromise = new Promise((resolve) => { resolveCopy = resolve })

      vi.spyOn(api, 'post').mockImplementation(async (path: string) => {
        if (path.includes('/copy')) return copyPromise
        return {}
      })

      const writeText = vi.fn().mockResolvedValue(undefined)
      Object.assign(navigator, { clipboard: { writeText } })

      const pub = usePublications('mihomo')
      const copyPending = pub.copySubscriptionLink('link-session-1', 'mihomo')

      // User logs out or session changes while copy is in flight
      clearPublicationCapabilities()

      // Network call returns sensitive token
      resolveCopy({
        id: 'link-session-1',
        target: 'mihomo',
        path: '/subscribe/v1/link-session-1?token=leaked_secret_token',
        token: 'leaked_secret_token',
      })

      const result = await copyPending

      // Must cancel stale operation: no clipboard write, no modal opened
      expect(writeText).not.toHaveBeenCalled()
      expect(pub.manualCopyModalOpen.value).toBe(false)
      expect(pub.manualCopyUrl.value).toBe('')
      expect(result.copied).toBe(false)
      expect(result.url).toBe('')
    })

    it('Point 1: copyDefaultClientSubscription cancels and suppresses clipboard if session invalidated during ensureDefault', async () => {
      let resolveEnsure!: (val: any) => void
      const ensurePromise = new Promise((resolve) => { resolveEnsure = resolve })

      vi.spyOn(api, 'post').mockImplementation(async (path: string) => {
        if (path === '/api/v1/subscription-links') return ensurePromise
        return {}
      })

      const writeText = vi.fn().mockResolvedValue(undefined)
      Object.assign(navigator, { clipboard: { writeText } })

      const pub = usePublications('mihomo')
      const copyPending = pub.copyDefaultClientSubscription('mihomo')

      clearPublicationCapabilities()

      resolveEnsure({
        id: 'link-created-post-logout',
        target: 'mihomo',
        state: 'active',
        is_default: true,
      })

      const result = await copyPending
      expect(writeText).not.toHaveBeenCalled()
      expect(result.copied).toBe(false)
      expect(result.url).toBe('')
    })

    it('Point 2: isolates copyingTargets and prevents crosstalk; A completing does not clear B', async () => {
      let resolveA!: (val: any) => void
      let resolveB!: (val: any) => void
      const promiseA = new Promise((resolve) => { resolveA = resolve })
      const promiseB = new Promise((resolve) => { resolveB = resolve })

      vi.spyOn(api, 'post').mockImplementation(async (path: string) => {
        if (path.includes('link-a/copy')) return promiseA
        if (path.includes('link-b/copy')) return promiseB
        return { allowed: true }
      })

      const pub = usePublications('mihomo')
      pub.subscriptionLinks.value = [
        { id: 'link-a', label: 'Mihomo', target: 'mihomo', state: 'active', is_default: true, created_at: '', updated_at: '' },
        { id: 'link-b', label: 'SingBox', target: 'singbox', state: 'active', is_default: true, created_at: '', updated_at: '' },
      ]

      const copyA = pub.copyDefaultClientSubscription('mihomo')
      const copyB = pub.copyDefaultClientSubscription('singbox')

      // Both targets are loading concurrently
      expect(pub.copyingTargets.value.mihomo).toBe(true)
      expect(pub.copyingTargets.value.singbox).toBe(true)

      // Target A finishes first
      resolveA({ id: 'link-a', target: 'mihomo', path: '/subscribe/v1/link-a?token=tok-a', token: 'tok-a' })
      await copyA

      // Target A is no longer loading, but Target B MUST still be loading!
      expect(pub.copyingTargets.value.mihomo).toBe(false)
      expect(pub.copyingTargets.value.singbox).toBe(true)

      // Target B finishes
      resolveB({ id: 'link-b', target: 'singbox', path: '/subscribe/v1/link-b?token=tok-b', token: 'tok-b' })
      await copyB

      expect(pub.copyingTargets.value.singbox).toBe(false)
    })

    it('Point 2: target errors are isolated without overwriting errors of other targets', async () => {
      const pub = usePublications('mihomo')
      pub.subscriptionLinks.value = [
        { id: 'link-err', label: 'Mihomo', target: 'mihomo', state: 'active', is_default: true, created_at: '', updated_at: '' },
      ]

      vi.spyOn(api, 'post').mockImplementation(async (path: string) => {
        if (path.includes('link-err/copy')) {
          throw new Error('Mihomo copy failed with gateway 502')
        }
        return {}
      })

      await expect(pub.copySubscriptionLink('link-err', 'mihomo')).rejects.toThrow('Mihomo copy failed with gateway 502')

      expect(pub.targetLinkErrors.value.mihomo).toContain('Mihomo copy failed with gateway 502')
      expect(pub.targetLinkErrors.value.singbox).toBe('')
      expect(pub.targetLinkErrors.value.surge).toBe('')
      expect(pub.targetLinkErrors.value.qx).toBe('')
    })

    it('Point 3: never renders fake 订阅已就绪 in PublicationsView even if active link exists', async () => {
      const { default: PublicationsView } = await import('./PublicationsView.vue')
      const { createApp, h, nextTick } = await import('vue')

      const serverLinks = [
        { id: 'link-1', label: 'Mihomo', target: 'mihomo', state: 'active', is_default: true, created_at: '', updated_at: '' },
        { id: 'link-2', label: 'Surge', target: 'surge', state: 'active', is_default: true, created_at: '', updated_at: '' },
      ]
      vi.spyOn(api, 'get').mockResolvedValue(serverLinks)

      const mountEl = document.createElement('div')
      document.body.appendChild(mountEl)
      const testApp = createApp({ render: () => h(PublicationsView) })
      testApp.mount(mountEl)
      await nextTick()
      await new Promise((r) => setTimeout(r, 20))

      // Crucial: "订阅已就绪" must not exist
      expect(mountEl.textContent).not.toContain('订阅已就绪')

      testApp.unmount()
      mountEl.remove()
    })

    it('Point 3: warning banner gives truthful message by type without promising client caching behavior', async () => {
      const pub = usePublications('singbox')
      pub.subscriptionLinks.value = [
        { id: 'link-sb', label: 'SingBox', target: 'singbox', state: 'active', is_default: true, created_at: '', updated_at: '' },
      ]

      vi.spyOn(api, 'post').mockImplementation(async (path: string) => {
        if (path.includes('/copy')) {
          return { id: 'link-sb', target: 'singbox', path: '/subscribe/v1/link-sb?token=sb-tok', token: 'sb-tok' }
        }
        if (path.includes('/check')) {
          return {
            id: 'link-sb',
            target: 'singbox',
            allowed: false,
            message: 'No nodes passed filter criteria',
          }
        }
        return {}
      })

      const res = await pub.copySubscriptionLink('link-sb', 'singbox')
      expect(res.allowed).toBe(false)
      expect(pub.targetGenerationWarnings.value.singbox).toContain('节点列表')
      expect(pub.targetGenerationWarnings.value.singbox).not.toContain('策略树')
      expect(pub.targetGenerationWarnings.value.singbox).toContain('No nodes passed filter criteria')
    })

    it('Point 4: reset confirmation modal dynamically injects specific target name (e.g. Surge / Mihomo)', async () => {
      HTMLDialogElement.prototype.showModal = function () { this.setAttribute('open', '') }
      HTMLDialogElement.prototype.close = function () { this.removeAttribute('open'); this.dispatchEvent(new Event('close')) }

      const { default: PublicationsView } = await import('./PublicationsView.vue')
      const { createApp, h, nextTick } = await import('vue')

      const serverLinks = [
        { id: 'link-surge-1', label: 'Surge', target: 'surge', state: 'active', is_default: true, created_at: '', updated_at: '' },
      ]
      vi.spyOn(api, 'get').mockResolvedValue(serverLinks)

      const mountEl = document.createElement('div')
      document.body.appendChild(mountEl)
      const testApp = createApp({ render: () => h(PublicationsView) })
      testApp.mount(mountEl)
      await nextTick()
      await new Promise((r) => setTimeout(r, 20))

      const surgeResetBtn = mountEl.querySelector('[data-testid-target="reset-client-subscription-surge"]') as HTMLButtonElement
      expect(surgeResetBtn).not.toBeNull()
      surgeResetBtn.click()
      await nextTick()
      await new Promise((r) => setTimeout(r, 20))

      // The modal title must explicitly include Surge
      const dialog = mountEl.querySelector('dialog')
      expect(dialog).not.toBeNull()
      expect(dialog?.textContent).toContain('Surge')

      // Cancel button closes without calling rotate
      const cancelBtn = Array.from(mountEl.querySelectorAll('button')).find((b) => b.textContent?.includes('取消'))
      cancelBtn?.click()
      await nextTick()

      testApp.unmount()
      mountEl.remove()
    })

    it('Point 4: copy and reset buttons on same row are mutually exclusive', async () => {
      const pub = usePublications('mihomo')
      pub.subscriptionLinks.value = [
        { id: 'link-m', label: 'Mihomo', target: 'mihomo', state: 'active', is_default: true, created_at: '', updated_at: '' },
      ]

      // While copying is active, resetDefaultSubscriptionLink throws
      pub.copyingTargets.value.mihomo = true
      await expect(pub.resetDefaultSubscriptionLink('mihomo')).rejects.toThrow('Target mihomo is currently busy')

      // Other target is NOT busy
      pub.copyingTargets.value.mihomo = false
      pub.resettingTargets.value.mihomo = true
      const copyRes = await pub.copyDefaultClientSubscription('mihomo')
      expect(copyRes.copied).toBe(false)
      expect(copyRes.url).toBe('')
    })

    describe('Point 5: buildValidatedSubscriptionUrl path security & format validation', () => {
      it('validates standard UUID relative path with origin fallback', () => {
        const copyRes = {
          id: '01925b3a-96e0-7e12-8e6f-7f7f98e6c4ba',
          target: 'mihomo' as const,
          path: '/subscribe/v1/01925b3a-96e0-7e12-8e6f-7f7f98e6c4ba?token=valid-tok-123',
          token: 'valid-tok-123',
          public_base_url: '',
        }
        const url = buildValidatedSubscriptionUrl(copyRes, '01925b3a-96e0-7e12-8e6f-7f7f98e6c4ba', 'https://demo.local')
        expect(url).toBe('https://demo.local/subscribe/v1/01925b3a-96e0-7e12-8e6f-7f7f98e6c4ba?token=valid-tok-123')
      })

      it('respects valid public_base_url when configured', () => {
        const copyRes = {
          id: 'link-abc',
          target: 'surge' as const,
          path: '/subscribe/v1/link-abc?token=tok-abc',
          token: 'tok-abc',
          public_base_url: 'https://cdn.example.org:8443/custom-prefix/',
        }
        const url = buildValidatedSubscriptionUrl(copyRes, 'link-abc')
        expect(url).toBe('https://cdn.example.org:8443/custom-prefix/subscribe/v1/link-abc?token=tok-abc')
      })

      it('rejects scheme or authority in relative path', () => {
        const copyRes = {
          id: 'link-1',
          target: 'mihomo' as const,
          path: 'https://evil.attacker.com/subscribe/v1/link-1?token=tok',
          token: 'tok',
          public_base_url: '',
        }
        expect(() => buildValidatedSubscriptionUrl(copyRes, 'link-1')).toThrow('scheme and authority are forbidden')
      })

      it('rejects protocol-relative path //', () => {
        const copyRes = {
          id: 'link-1',
          target: 'mihomo' as const,
          path: '//evil.com/subscribe/v1/link-1?token=tok',
          token: 'tok',
          public_base_url: '',
        }
        expect(() => buildValidatedSubscriptionUrl(copyRes, 'link-1')).toThrow('scheme and authority are forbidden')
      })

      it('rejects path with hash fragment', () => {
        const copyRes = {
          id: 'link-1',
          target: 'mihomo' as const,
          path: '/subscribe/v1/link-1?token=tok#injected-hash',
          token: 'tok',
          public_base_url: '',
        }
        expect(() => buildValidatedSubscriptionUrl(copyRes, 'link-1')).toThrow('hash fragment')
      })

      it('rejects unexpected query parameters', () => {
        const copyRes = {
          id: 'link-1',
          target: 'mihomo' as const,
          path: '/subscribe/v1/link-1?token=tok&role=admin',
          token: 'tok',
          public_base_url: '',
        }
        expect(() => buildValidatedSubscriptionUrl(copyRes, 'link-1')).toThrow('Unexpected query parameter role')
      })

      it('rejects path mismatch for different link ID', () => {
        const copyRes = {
          id: 'link-1',
          target: 'mihomo' as const,
          path: '/subscribe/v1/link-other?token=tok',
          token: 'tok',
          public_base_url: '',
        }
        expect(() => buildValidatedSubscriptionUrl(copyRes, 'link-1')).toThrow('Subscription link path mismatch')
      })
    })
  })
})
