// @vitest-environment jsdom
import { describe, expect, it, vi, beforeEach } from 'vitest'
import {
  COMPILER_TARGETS,
  targetLabel,
  targetFileExt,
  formatDigest,
  type CompilerTarget,
  type PreviewResult,
  type PublicationDetail,
} from './publicationTypes'
import { usePublications } from './usePublications'
import { api, ApiError } from '../../api/client'

describe('publication types and helpers', () => {
  it('covers all five standard compiler targets', () => {
    const targets = COMPILER_TARGETS.map((t) => t.target)
    expect(targets).toEqual(['clash', 'mihomo', 'singbox', 'surge', 'qx'])
  })

  it('formats target labels and extensions accurately', () => {
    expect(targetLabel('clash')).toBe('Clash')
    expect(targetLabel('mihomo')).toBe('Mihomo')
    expect(targetLabel('singbox')).toBe('sing-box')
    expect(targetLabel('surge')).toBe('Surge')
    expect(targetLabel('qx')).toBe('Quantumult X')

    expect(targetFileExt('clash')).toBe('yaml')
    expect(targetFileExt('mihomo')).toBe('yaml')
    expect(targetFileExt('singbox')).toBe('json')
    expect(targetFileExt('surge')).toBe('conf')
    expect(targetFileExt('qx')).toBe('conf')
  })

  it('formats digests cleanly for display', () => {
    const longDigest = 'a1b2c3d4e5f67890123456789abcdef0123456789abcdef0123456789abcdef0'
    expect(formatDigest(longDigest)).toBe('a1b2c3d4...cdef0')
    expect(formatDigest('short')).toBe('short')
  })
})

describe('usePublications composable', () => {
  beforeEach(() => {
    vi.restoreAllMocks()
  })

  it('fetches rendered preview via POST /api/v1/publications/preview', async () => {
    const mockPreview: PreviewResult = {
      target: 'clash',
      snapshot_digest: 'sha256:snap123',
      content_digest: 'sha256:content456',
      content: 'proxies:\n  - name: Tokyo 01\n    type: ss\n',
      content_type: 'application/x-yaml',
      filename: 'clash-config.yaml',
      diagnostics: [],
    }

    const postSpy = vi.spyOn(api, 'post').mockResolvedValueOnce(mockPreview)

    const { preview, loadingPreview, fetchPreview } = usePublications()
    await fetchPreview('clash')

    expect(postSpy).toHaveBeenCalledWith('/api/v1/publications/preview', { target: 'clash' })
    expect(loadingPreview.value).toBe(false)
    expect(preview.value).toEqual(mockPreview)
    expect(preview.value?.content).toContain('Tokyo 01')
  })

  it('creates an immutable publication via POST /api/v1/publications', async () => {
    const mockPub: PublicationDetail = {
      id: 'pub-uuid-1',
      target: 'singbox',
      snapshot_digest: 'sha256:snap1',
      content_digest: 'sha256:cnt1',
      export_url: '/publish/v1/pub-uuid-1?token=export_sec_token',
      created_at: '2026-09-16T10:00:00Z',
    }

    const postSpy = vi.spyOn(api, 'post').mockResolvedValueOnce(mockPub)

    const { activePublication, publishing, publish } = usePublications()
    const res = await publish('singbox')

    expect(postSpy).toHaveBeenCalledWith('/api/v1/publications', { target: 'singbox' })
    expect(publishing.value).toBe(false)
    expect(activePublication.value).toEqual(mockPub)
    expect(res.export_url).toContain('export_sec_token')
  })

  it('revokes an immutable publication via POST /api/v1/publications/{id}/revoke', async () => {
    const postSpy = vi.spyOn(api, 'post').mockResolvedValueOnce({
      id: 'pub-uuid-1',
      status: 'revoked',
    })

    const { activePublication, revoking, revoke } = usePublications()
    activePublication.value = {
      id: 'pub-uuid-1',
      target: 'singbox',
      snapshot_digest: 'sha256:snap1',
      created_at: '2026-09-16T10:00:00Z',
    }

    await revoke('pub-uuid-1')
    expect(postSpy).toHaveBeenCalledWith('/api/v1/publications/pub-uuid-1/revoke')
    expect(revoking.value).toBe(false)
    expect(activePublication.value?.revoked_at).toBeTruthy()
  })

  it('handles 409 no_active_revision error preserving ApiError structure and code', async () => {
    const apiError = new ApiError(409, 'no_active_revision', 'cannot resolve policy without an active configuration revision')
    vi.spyOn(api, 'post').mockRejectedValueOnce(apiError)

    const { preview, error, errorDetail, isNoActiveRevision, fetchPreview } = usePublications()
    const result = await fetchPreview('clash')

    expect(result).toBeNull()
    expect(preview.value).toBeNull()
    expect(error.value).toBe('cannot resolve policy without an active configuration revision')
    expect(errorDetail.value).toBe(apiError)
    expect(isNoActiveRevision.value).toBe(true)
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

  it('renders PublicationsView with dynamic fluid max-height preview pre container', async () => {
    const { default: PublicationsView } = await import('./PublicationsView.vue')
    const { createApp, h, nextTick } = await import('vue')
    
    vi.spyOn(api, 'get').mockResolvedValue({ items: [], total: 0 })
    vi.spyOn(api, 'post').mockResolvedValue({
      target: 'clash',
      snapshot_digest: 'sha256:test',
      content_digest: 'sha256:test',
      content: 'proxies:\n  - name: test\n',
      content_type: 'application/x-yaml',
      filename: 'clash.yaml',
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

    // 409 specific alert card should be present
    const card = mountEl.querySelector('[data-testid="no-active-revision-card"]')
    expect(card).not.toBeNull()
    expect(card?.textContent).toContain('no_active_revision')

    // Download button and create publication button should be disabled
    const buttons = Array.from(mountEl.querySelectorAll('button'))
    const downloadBtn = buttons.find((b) => b.textContent?.includes('下载') || b.textContent?.includes('Download'))
    const createPubBtn = buttons.find((b) => b.textContent?.includes('发布') || b.textContent?.includes('Create Publication'))
    expect(downloadBtn?.disabled).toBe(true)
    expect(createPubBtn?.disabled).toBe(true)

    // Retry should trigger fetchPreview again
    postSpy.mockClear()
    const mockPreview: PreviewResult = {
      target: 'clash',
      snapshot_digest: 'sha256:snap-ok',
      content_digest: 'sha256:content-ok',
      content: 'proxies: []\n',
      content_type: 'application/x-yaml',
      filename: 'clash.yaml',
      diagnostics: [],
    }
    postSpy.mockResolvedValueOnce(mockPreview)

    const retryBtn = card?.querySelector('button')
    retryBtn?.click()
    await nextTick()
    await new Promise((r) => setTimeout(r, 20))

    expect(postSpy).toHaveBeenCalledWith('/api/v1/publications/preview', { target: 'clash' })
    expect(mountEl.querySelector('[data-testid="no-active-revision-card"]')).toBeNull()
    expect(downloadBtn?.disabled).toBe(false)
    expect(createPubBtn?.disabled).toBe(false)

    testApp.unmount()
    mountEl.remove()
  })

  it('renders generic ErrorStateCard for 500 server errors or 401 unauthorized', async () => {
    const { default: PublicationsView } = await import('./PublicationsView.vue')
    const { createApp, h, nextTick } = await import('vue')

    const apiError = new ApiError(500, 'internal_error', 'Internal server exploded')
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

    // 409 card should NOT be present
    expect(mountEl.querySelector('[data-testid="no-active-revision-card"]')).toBeNull()

    // Generic ErrorStateCard should be present
    const errCard = mountEl.querySelector('[data-testid="error-state-card"]')
    expect(errCard).not.toBeNull()
    expect(errCard?.textContent).toContain('500')

    testApp.unmount()
    mountEl.remove()
  })

  it('renders all 5 target pills with flex-wrap ensuring Quantumult X and all targets are accessible', async () => {
    const { default: PublicationsView } = await import('./PublicationsView.vue')
    const { createApp, h, nextTick } = await import('vue')

    const mockPreview: PreviewResult = {
      target: 'clash',
      snapshot_digest: 'sha256:snap-all',
      content_digest: 'sha256:content-all',
      content: 'proxies: []\n',
      content_type: 'application/x-yaml',
      filename: 'clash.yaml',
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
    expect(tablist?.className).toContain('flex-wrap')

    const targetButtons = Array.from(tablist!.querySelectorAll('button'))
    expect(targetButtons.length).toBe(5)

    const targets = targetButtons.map((b) => b.textContent?.trim())
    expect(targets.some((t) => t?.includes('Clash'))).toBe(true)
    expect(targets.some((t) => t?.includes('Mihomo'))).toBe(true)
    expect(targets.some((t) => t?.includes('sing-box'))).toBe(true)
    expect(targets.some((t) => t?.includes('Surge'))).toBe(true)
    expect(targets.some((t) => t?.includes('Quantumult X'))).toBe(true)

    // Click Quantumult X (the 5th target)
    const qxBtn = targetButtons[4]
    expect(qxBtn.textContent).toContain('Quantumult X')
    qxBtn.click()
    await nextTick()
    await new Promise((r) => setTimeout(r, 20))

    expect(postSpy).toHaveBeenCalledWith('/api/v1/publications/preview', { target: 'qx' })

    testApp.unmount()
    mountEl.remove()
  })
})
