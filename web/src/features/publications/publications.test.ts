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
    expect(mihomo.protocols).toEqual(['ss', 'vmess', 'vless', 'trojan', 'hysteria2', 'wireguard', 'tuic'])
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
    vi.restoreAllMocks()
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

    expect(postSpy).toHaveBeenCalledWith('/api/v1/publications/preview', { target: 'mihomo' })
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

    const capRes = await fetchPreview('surge')
    expect(capRes).toBeNull()
    expect((errorDetail.value as ApiError)?.code).toBe('unsupported_target_capability')
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
})

describe('PublicationsView component rendering', () => {
  beforeEach(() => {
    localStorage.clear()
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

  it('renders PublicationsView with real plaintext WG/TUIC configuration in DOM preview matching copied content', async () => {
    const { default: PublicationsView } = await import('./PublicationsView.vue')
    const { createApp, h, nextTick } = await import('vue')

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

    const mountEl = document.createElement('div')
    document.body.appendChild(mountEl)
    const testApp = createApp({
      render() {
        return h(PublicationsView)
      },
    })
    testApp.mount(mountEl)
    await nextTick()
    await new Promise((r) => setTimeout(r, 20))

    const preEl = mountEl.querySelector('pre')
    expect(preEl).not.toBeNull()
    expect(preEl?.className).toContain('adaptive-preview-box')
    expect(preEl?.textContent).toBe(rawConfig)
    expect(preEl?.textContent).toContain('wg-super-secret-privkey')
    expect(preEl?.textContent).toContain('wg-super-secret-psk')
    expect(preEl?.textContent).toContain('tuic-super-secret-password')
    expect(preEl?.textContent).not.toContain('***')

    // Capability boundary banner is present and no retired card exists
    const capCard = mountEl.querySelector('[data-testid="target-capability-boundary"]')
    expect(capCard).not.toBeNull()
    expect(capCard?.textContent).toContain('wireguard')
    expect(capCard?.textContent).toContain('tuic')
    expect(mountEl.querySelector('[data-testid="retired-target-card"]')).toBeNull()

    // Verify Copy Full Config exports the exact same real admin configuration
    const writeText = vi.fn().mockResolvedValue(undefined)
    Object.assign(navigator, { clipboard: { writeText } })
    const copyBtns = Array.from(mountEl.querySelectorAll('button')).filter(
      (b) => b.textContent?.includes('复制配置') || b.textContent?.includes('复制完整配置') || b.textContent?.includes('Copy')
    )
    expect(copyBtns.length).toBeGreaterThan(0)
    copyBtns[0].click()
    await nextTick()
    expect(writeText).toHaveBeenCalled()
    const copiedText = writeText.mock.calls[0][0] as string
    expect(copiedText).toBe(rawConfig)

    testApp.unmount()
    mountEl.remove()
  })

  it('renders only 4 modern target pills defaulting to Mihomo and updates capability boundary on target switch', async () => {
    const { default: PublicationsView } = await import('./PublicationsView.vue')
    const { createApp, h, nextTick } = await import('vue')

    const mockPreview: PreviewResult = {
      target: 'mihomo',
      snapshot_digest: 'sha256:snap-modern',
      content_digest: 'sha256:content-modern',
      content: 'proxies: []\n',
      content_type: 'application/x-yaml',
      filename: 'mihomo.yaml',
      diagnostics: [],
    }
    const postSpy = vi.spyOn(api, 'post').mockResolvedValue(mockPreview)

    const mountEl = document.createElement('div')
    document.body.appendChild(mountEl)
    const testApp = createApp({
      render() {
        return h(PublicationsView)
      },
    })
    testApp.mount(mountEl)
    await nextTick()
    await new Promise((r) => setTimeout(r, 20))

    const tablist = mountEl.querySelector('[role="tablist"]')
    expect(tablist).not.toBeNull()

    const targetButtons = Array.from(tablist!.querySelectorAll('button'))
    expect(targetButtons.length).toBe(4)

    const targets = targetButtons.map((b) => b.textContent?.trim())
    expect(targets.some((t) => t?.includes('Mihomo'))).toBe(true)
    expect(targets.some((t) => t?.includes('sing-box'))).toBe(true)
    expect(targets.some((t) => t?.includes('Surge'))).toBe(true)
    expect(targets.some((t) => t?.includes('Quantumult X'))).toBe(true)

    expect(postSpy).toHaveBeenCalledWith('/api/v1/publications/preview', { target: 'mihomo' })

    // Click Surge (the 3rd target) and verify capability boundary reflects Surge subset
    const surgeBtn = targetButtons[2]
    surgeBtn.click()
    await nextTick()
    await new Promise((r) => setTimeout(r, 20))

    expect(postSpy).toHaveBeenCalledWith('/api/v1/publications/preview', { target: 'surge' })
    const capCard = mountEl.querySelector('[data-testid="target-capability-boundary"]')
    expect(capCard?.textContent).toContain('Surge')
    expect(capCard?.textContent).toContain('仅导出 Surge 节点格式')
    expect(capCard?.textContent).toContain('忽略策略组与规则')
    expect(capCard?.textContent).toContain('拒绝 VLESS')

    testApp.unmount()
    mountEl.remove()
  })

  it('renders recoverable guidance card and disables export buttons on 409 no_active_revision', async () => {
    const { default: PublicationsView } = await import('./PublicationsView.vue')
    const { createApp, h, nextTick } = await import('vue')

    const apiError = new ApiError(409, 'no_active_revision', 'cannot resolve policy without an active configuration revision')
    const postSpy = vi.spyOn(api, 'post').mockRejectedValue(apiError)

    const mountEl = document.createElement('div')
    document.body.appendChild(mountEl)
    const testApp = createApp({
      render() {
        return h(PublicationsView)
      },
    })
    testApp.mount(mountEl)
    await nextTick()
    await new Promise((r) => setTimeout(r, 20))

    const card = mountEl.querySelector('[data-testid="no-active-revision-card"]')
    expect(card).not.toBeNull()
    expect(card?.textContent).toContain('no_active_revision')

    const buttons = Array.from(mountEl.querySelectorAll('button'))
    const downloadBtn = buttons.find((b) => b.textContent?.includes('下载') || b.textContent?.includes('Download'))
    const createPubBtn = buttons.find((b) => b.textContent?.includes('发布') || b.textContent?.includes('Create Publication'))
    expect(downloadBtn?.disabled).toBe(true)
    expect(createPubBtn?.disabled).toBe(true)

    postSpy.mockClear()
    const mockPreview: PreviewResult = {
      target: 'mihomo',
      snapshot_digest: 'sha256:snap-ok',
      content_digest: 'sha256:content-ok',
      content: 'proxies: []\n',
      content_type: 'application/x-yaml',
      filename: 'mihomo.yaml',
      diagnostics: [],
    }
    postSpy.mockResolvedValueOnce(mockPreview)

    const retryBtn = card?.querySelector('button')
    retryBtn?.click()
    await nextTick()
    await new Promise((r) => setTimeout(r, 20))

    expect(postSpy).toHaveBeenCalledWith('/api/v1/publications/preview', { target: 'mihomo' })
    expect(mountEl.querySelector('[data-testid="no-active-revision-card"]')).toBeNull()
    expect(downloadBtn?.disabled).toBe(false)
    expect(createPubBtn?.disabled).toBe(false)

    testApp.unmount()
    mountEl.remove()
  })

  it('renders generic ErrorStateCard for 422 or 500 errors', async () => {
    const { default: PublicationsView } = await import('./PublicationsView.vue')
    const { createApp, h, nextTick } = await import('vue')

    const apiError = new ApiError(422, 'unsupported_target_capability', 'wireguard protocol unsupported')
    vi.spyOn(api, 'post').mockRejectedValue(apiError)

    const mountEl = document.createElement('div')
    document.body.appendChild(mountEl)
    const testApp = createApp({
      render() {
        return h(PublicationsView)
      },
    })
    testApp.mount(mountEl)
    await nextTick()
    await new Promise((r) => setTimeout(r, 20))

    const errCard = mountEl.querySelector('[data-testid="error-state-card"]')
    expect(errCard).not.toBeNull()
    expect(errCard?.textContent).toContain('422')
    expect(errCard?.textContent).toContain('unsupported_target_capability')

    testApp.unmount()
    mountEl.remove()
  })
})
