// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { createApp, h, nextTick } from 'vue'
import PublicationsView from './PublicationsView.vue'
import { usePublications } from './usePublications'
import { api, ApiError } from '../../api/client'
import { toastStore } from '../../ui/toast'
import { setLocale } from '../../locales'
import type { SubscriptionLink, SubscriptionLinkCheckResult } from './publicationTypes'

describe('T107/T108 Client Subscription UI Specification', () => {
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

  it('FR-PUB-LIVE-01: One-click copy creates default client subscription link without preview or publication prerequisite', async () => {
    const mockCreatedLink: SubscriptionLink = {
      id: 'sub-link-001',
      label: 'Mihomo 默认订阅',
      target: 'mihomo',
      token_masked: 'sec_1234****',
      state: 'active',
      is_default: true,
      created_at: '2026-10-16T10:00:00Z',
      updated_at: '2026-10-16T10:00:00Z',
    }

    const mockCopyResult = {
      id: 'sub-link-001',
      target: 'mihomo',
      url: 'https://sub.example.com/subscribe/v1/sub-link-001?token=sec_live_token_abc',
      token: 'sec_live_token_abc',
      public_base_url: 'https://sub.example.com',
    }

    const mockCheckResult: SubscriptionLinkCheckResult = {
      id: 'sub-link-001',
      target: 'mihomo',
      allowed: true,
      as_of: '2026-10-16T10:00:00Z',
      node_count: 5,
    }

    vi.spyOn(api, 'get').mockImplementation(async (path: string) => {
      if (path === '/api/v1/subscription-links') return []
      if (path === '/api/v1/settings/public-base-url') return { public_base_url: 'https://sub.example.com' }
      if (path === '/api/v1/settings/auth') return { admin_auth_enabled: true, export_auth_enabled: true, admin_mode: 'protected', export_mode: 'protected', token_configured: true }
      return {}
    })

    const postSpy = vi.spyOn(api, 'post').mockImplementation(async (path: string, body: any) => {
      if (path === '/api/v1/subscription-links') return mockCreatedLink
      if (path === '/api/v1/subscription-links/sub-link-001/copy') return mockCopyResult
      if (path === '/api/v1/subscription-links/sub-link-001/check') return mockCheckResult
      if (path === '/api/v1/publications/preview') return { target: 'mihomo', content: 'test', snapshot_digest: 's1', content_digest: 'c1' }
      return {}
    })

    const writeText = vi.fn().mockResolvedValue(undefined)
    Object.assign(navigator, { clipboard: { writeText } })

    app = createApp({ render: () => h(PublicationsView) })
    app.mount(container)

    await nextTick()
    await new Promise((r) => setTimeout(r, 30))

    const copyBtn = container.querySelector('[data-testid="copy-client-subscription-btn"]') as HTMLButtonElement
    expect(copyBtn).not.toBeNull()

    copyBtn.click()
    await nextTick()
    await new Promise((r) => setTimeout(r, 40))

    // Verified: Idempotent create + copy
    expect(postSpy).toHaveBeenCalledWith('/api/v1/subscription-links', { target: 'mihomo', is_default: true })
    expect(postSpy).toHaveBeenCalledWith('/api/v1/subscription-links/sub-link-001/copy')
    expect(writeText).toHaveBeenCalledWith('https://sub.example.com/subscribe/v1/sub-link-001?token=sec_live_token_abc')

    // Verified: Never required /api/v1/publications create
    expect(postSpy).not.toHaveBeenCalledWith('/api/v1/publications', expect.anything())
  })

  it('FR-PUB-LIVE-07 & SC-S01: Automatically resolves relative path against window.location.origin when public_base_url is empty, without requiring public base URL setup', async () => {
    vi.spyOn(api, 'get').mockImplementation(async (path: string) => {
      if (path === '/api/v1/subscription-links') return []
      if (path === '/api/v1/settings/public-base-url') return { public_base_url: '' }
      return {}
    })

    vi.spyOn(api, 'post').mockImplementation(async (path: string) => {
      if (path === '/api/v1/subscription-links') {
        return {
          id: 'link-unconf',
          label: 'Default',
          target: 'mihomo',
          state: 'active',
          is_default: true,
          created_at: '2026-10-16T10:00:00Z',
          updated_at: '2026-10-16T10:00:00Z',
        }
      }
      if (path.includes('/copy')) {
        return {
          id: 'link-unconf',
          target: 'mihomo',
          path: '/subscribe/v1/link-unconf?token=sec_live_tok',
          url: '/subscribe/v1/link-unconf?token=sec_live_tok',
          public_base_url: '',
        }
      }
      return { allowed: true }
    })

    const writeText = vi.fn().mockResolvedValue(undefined)
    Object.assign(navigator, { clipboard: { writeText } })

    app = createApp({ render: () => h(PublicationsView) })
    app.mount(container)

    await nextTick()
    await new Promise((r) => setTimeout(r, 30))

    const copyBtn = container.querySelector('[data-testid="copy-client-subscription-btn"]') as HTMLButtonElement
    copyBtn.click()
    await nextTick()
    await new Promise((r) => setTimeout(r, 40))

    // Verified: No public base URL setup form or warning appears
    expect(container.querySelector('[data-testid="public-base-url-guide"]')).toBeNull()

    // Verified: Full URL is automatically constructed using window.location.origin
    const expectedUrl = `${window.location.origin}/subscribe/v1/link-unconf?token=sec_live_tok`
    expect(writeText).toHaveBeenCalledWith(expectedUrl)
  })

  it('FR-PUB-LIVE-01: Opens manual copy fallback modal when clipboard API write fails', async () => {
    const mockLink: SubscriptionLink = {
      id: 'sub-link-manual',
      label: 'Default',
      target: 'mihomo',
      state: 'active',
      is_default: true,
      created_at: '2026-10-16T10:00:00Z',
      updated_at: '2026-10-16T10:00:00Z',
    }

    vi.spyOn(api, 'get').mockResolvedValue([])
    vi.spyOn(api, 'post').mockImplementation(async (path: string) => {
      if (path === '/api/v1/subscription-links') return mockLink
      if (path.includes('/copy')) {
        return {
          id: 'sub-link-manual',
          target: 'mihomo',
          url: 'https://sub.example.com/subscribe/v1/sub-link-manual?token=tok_manual',
          public_base_url: 'https://sub.example.com',
        }
      }
      return { allowed: true }
    })

    // Simulate clipboard permission failure
    Object.assign(navigator, {
      clipboard: {
        writeText: vi.fn().mockRejectedValue(new Error('Permission denied')),
      },
    })

    app = createApp({ render: () => h(PublicationsView) })
    app.mount(container)

    await nextTick()
    await new Promise((r) => setTimeout(r, 30))

    const copyBtn = container.querySelector('[data-testid="copy-client-subscription-btn"]') as HTMLButtonElement
    copyBtn.click()
    await nextTick()
    await new Promise((r) => setTimeout(r, 40))

    // Fallback modal opens with textarea containing full URL
    const textarea = container.querySelector('[data-testid="manual-copy-textarea"]') as HTMLTextAreaElement
    expect(textarea).not.toBeNull()
    expect(textarea.value).toBe('https://sub.example.com/subscribe/v1/sub-link-manual?token=tok_manual')
  })

  it('FR-PUB-LIVE-06: Warns "链接已创建，当前不可更新" when generation check fails, without falsely declaring health', async () => {
    const mockLink: SubscriptionLink = {
      id: 'sub-link-blocked',
      label: 'Default',
      target: 'mihomo',
      state: 'active',
      is_default: true,
      created_at: '2026-10-16T10:00:00Z',
      updated_at: '2026-10-16T10:00:00Z',
    }

    vi.spyOn(api, 'get').mockResolvedValue([])
    vi.spyOn(api, 'post').mockImplementation(async (path: string) => {
      if (path === '/api/v1/subscription-links') return mockLink
      if (path.includes('/copy')) {
        return {
          id: 'sub-link-blocked',
          target: 'mihomo',
          url: 'https://sub.example.com/subscribe/v1/sub-link-blocked?token=tok_blocked',
          public_base_url: 'https://sub.example.com',
        }
      }
      if (path.includes('/check')) {
        return {
          id: 'sub-link-blocked',
          target: 'mihomo',
          allowed: false,
          as_of: '2026-10-16T10:00:00Z',
          node_count: 0,
          message: 'No active revision available',
        }
      }
      return {}
    })

    const writeText = vi.fn().mockResolvedValue(undefined)
    Object.assign(navigator, { clipboard: { writeText } })

    app = createApp({ render: () => h(PublicationsView) })
    app.mount(container)

    await nextTick()
    await new Promise((r) => setTimeout(r, 30))

    const copyBtn = container.querySelector('[data-testid="copy-client-subscription-btn"]') as HTMLButtonElement
    copyBtn.click()
    await nextTick()
    await new Promise((r) => setTimeout(r, 40))

    // Copy succeeds and full URL was copied
    expect(writeText).toHaveBeenCalledWith('https://sub.example.com/subscribe/v1/sub-link-blocked?token=tok_blocked')

    // Truthful warning banner rendered
    const warningBanner = container.querySelector('[data-testid="generation-warning-banner"]')
    expect(warningBanner).not.toBeNull()
    expect(warningBanner?.textContent).toContain('链接已创建，当前不可更新')
  })

  it('FR-PUB-LIVE-05 & SC-S02: Restores server subscription links across browsers and allows rotate with confirmation dialog, cancel has no side effects', async () => {
    const serverLinks: SubscriptionLink[] = [
      {
        id: 'link-corp-1',
        label: 'Mihomo 默认订阅',
        target: 'mihomo',
        token_masked: 'sec_1234****',
        state: 'active',
        is_default: true,
        created_at: '2026-10-16T08:00:00Z',
        updated_at: '2026-10-16T08:00:00Z',
      },
      {
        id: 'link-corp-2',
        label: 'Surge 默认订阅',
        target: 'surge',
        token_masked: 'sec_5678****',
        state: 'active',
        is_default: true,
        created_at: '2026-10-16T09:00:00Z',
        updated_at: '2026-10-16T09:00:00Z',
      },
    ]

    vi.spyOn(api, 'get').mockImplementation(async (path: string) => {
      if (path === '/api/v1/subscription-links') return serverLinks
      return {}
    })

    const postSpy = vi.spyOn(api, 'post').mockImplementation(async (path: string) => {
      if (path === '/api/v1/subscription-links/link-corp-1/rotate-token') {
        return {
          ...serverLinks[0],
          token_masked: 'sec_9999****',
          updated_at: '2026-10-16T12:00:00Z',
        }
      }
      return {}
    })

    app = createApp({ render: () => h(PublicationsView) })
    app.mount(container)

    await nextTick()
    await new Promise((r) => setTimeout(r, 30))

    // Verify both target rows render in the simplified list
    expect(container.querySelector('[data-testid="client-subscription-row-mihomo"]')).not.toBeNull()
    expect(container.querySelector('[data-testid="client-subscription-row-surge"]')).not.toBeNull()

    // Test Token Reset: first cancel confirmation (SC-S02: cancel has no side effects)
    const resetBtn = container.querySelector('[data-testid-target="reset-client-subscription-mihomo"]') as HTMLButtonElement
    expect(resetBtn).not.toBeNull()
    resetBtn.click()
    await nextTick()

    const cancelButtons = Array.from(document.querySelectorAll('button')).filter((b) => b.textContent?.includes('取消'))
    expect(cancelButtons.length).toBeGreaterThan(0)
    cancelButtons[0].click()
    await nextTick()
    expect(postSpy).not.toHaveBeenCalledWith('/api/v1/subscription-links/link-corp-1/rotate-token')

    // Now re-open and confirm reset
    resetBtn.click()
    await nextTick()

    const confirmButtons = Array.from(document.querySelectorAll('button')).filter((b) => b.textContent?.includes('确认重置'))
    expect(confirmButtons.length).toBeGreaterThan(0)
    confirmButtons[0].click()
    await nextTick()
    await new Promise((r) => setTimeout(r, 20))

    expect(postSpy).toHaveBeenCalledWith('/api/v1/subscription-links/link-corp-1/rotate-token')
  })

  it('FR-PUB-LIVE-07: Isolates asynchronous responses when switching targets rapidly', async () => {
    let finishMihomoCopy!: (value: any) => void
    const slowMihomoCopyPromise = new Promise((resolve) => {
      finishMihomoCopy = resolve
    })

    const composable = usePublications('mihomo')
    composable.subscriptionLinks.value = [
      {
        id: 'link-m',
        label: 'Mihomo',
        target: 'mihomo',
        state: 'active',
        is_default: true,
        created_at: '',
        updated_at: '',
      },
      {
        id: 'link-s',
        label: 'SingBox',
        target: 'singbox',
        state: 'active',
        is_default: true,
        created_at: '',
        updated_at: '',
      },
    ]

    vi.spyOn(api, 'post').mockImplementation(async (path: string) => {
      if (path === '/api/v1/subscription-links/link-m/copy') {
        return slowMihomoCopyPromise
      }
      if (path === '/api/v1/subscription-links/link-s/copy') {
        return {
          id: 'link-s',
          target: 'singbox',
          url: 'https://sub.example.com/subscribe/v1/link-s?token=tok-s',
          public_base_url: 'https://sub.example.com',
        }
      }
      return { allowed: true }
    })

    const pendingMihomo = composable.copyDefaultClientSubscription('mihomo')
    // User immediately switches target to singbox
    composable.selectedTarget.value = 'singbox'

    // Slow mihomo resolves later
    finishMihomoCopy({
      id: 'link-m',
      target: 'mihomo',
      url: 'https://sub.example.com/subscribe/v1/link-m?token=tok-m',
      public_base_url: 'https://sub.example.com',
    })

    await pendingMihomo

    // Selected target remains singbox and mihomo response did not corrupt active target
    expect(composable.selectedTarget.value).toBe('singbox')
  })

  it('SC-S01: Renders four client rows with full config vs node-only badges, without publication form or link management table', async () => {
    vi.spyOn(api, 'get').mockResolvedValue([])

    app = createApp({ render: () => h(PublicationsView) })
    app.mount(container)

    await nextTick()
    await new Promise((r) => setTimeout(r, 30))

    // Verify 4 target rows exist
    const rows = container.querySelectorAll('[data-testid^="client-subscription-row-"]')
    expect(rows.length).toBe(4)

    // Verify Mihomo is marked as full config
    const mihomoRow = container.querySelector('[data-testid="client-subscription-row-mihomo"]')
    expect(mihomoRow?.textContent).toContain('完整配置')
    expect(mihomoRow?.textContent).toContain('Mihomo')

    // Verify other three are marked as node subscriptions
    const singboxRow = container.querySelector('[data-testid="client-subscription-row-singbox"]')
    expect(singboxRow?.textContent).toContain('节点订阅')

    const surgeRow = container.querySelector('[data-testid="client-subscription-row-surge"]')
    expect(surgeRow?.textContent).toContain('节点订阅')

    const qxRow = container.querySelector('[data-testid="client-subscription-row-qx"]')
    expect(qxRow?.textContent).toContain('节点订阅')

    // Verified: No obsolete publication or link asset management elements
    expect(container.querySelector('[data-testid="server-subscription-links-section"]')).toBeNull()
    expect(container.querySelector('[data-testid="target-capability-boundary"]')).toBeNull()
    expect(container.querySelector('table')).toBeNull()
  })
})
