// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { setImmediate } from 'node:timers'
import { createApp, h, nextTick } from 'vue'
import SubscriptionsView from './SubscriptionsView.vue'
import { api, ApiError } from '../../api/client'
import { toastStore, type Toast } from '../../ui/toast'
import { setLocale } from '../../locales'
import {
  formatRefreshTime,
  formatFullDateTime,
  useSubscriptions,
  extractSecondLevelDomainName,
  type SubscriptionRecord,
} from './useSubscriptions'

describe('SubscriptionsView - Last Refreshed Display & Refresh Flow', () => {
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

  function mountView() {
    app = createApp({
      render() {
        return h(SubscriptionsView)
      },
    })
    app.mount(container)
  }

  const sampleSubscriptions: SubscriptionRecord[] = [
    {
      id: 'sub-never',
      name: 'Sub Never Refreshed',
      source_url_secret_ref: 'https://example.com/never',
      enabled: true,
      refresh_policy: {
        interval_seconds: 3600,
        user_agent_policy: 'default',
        timeout_seconds: 30,
        max_response_bytes: 10485760,
      },
      revision: 'rev-never',
      created_at: '2026-09-29T00:00:00Z',
      updated_at: '2026-09-29T00:00:00Z',
      last_refreshed_at: null,
      last_refresh_outcome: null,
    },
    {
      id: 'sub-success',
      name: 'Sub Success Refreshed',
      source_url_secret_ref: 'https://example.com/success',
      enabled: true,
      refresh_policy: {
        interval_seconds: 86400,
        user_agent_policy: 'default',
        timeout_seconds: 30,
        max_response_bytes: 10485760,
      },
      revision: 'rev-success',
      created_at: '2026-09-29T00:00:00Z',
      updated_at: '2026-09-29T00:00:00Z',
      last_refreshed_at: '2026-09-29T02:00:00Z',
      last_refresh_outcome: 'success',
    },
    {
      id: 'sub-failed',
      name: 'Sub Failed Refreshed',
      source_url_secret_ref: 'https://example.com/failed',
      enabled: true,
      refresh_policy: {
        interval_seconds: 86400,
        user_agent_policy: 'default',
        timeout_seconds: 30,
        max_response_bytes: 10485760,
      },
      revision: 'rev-failed',
      created_at: '2026-09-29T00:00:00Z',
      updated_at: '2026-09-29T00:00:00Z',
      last_refreshed_at: '2026-09-29T03:00:00Z',
      last_refresh_outcome: 'failed',
    },
    {
      id: 'sub-partial',
      name: 'Sub Partial Refreshed',
      source_url_secret_ref: 'https://example.com/partial',
      enabled: true,
      refresh_policy: {
        interval_seconds: 86400,
        user_agent_policy: 'default',
        timeout_seconds: 30,
        max_response_bytes: 10485760,
      },
      revision: 'rev-partial',
      created_at: '2026-09-29T00:00:00Z',
      updated_at: '2026-09-29T00:00:00Z',
      last_refreshed_at: '2026-09-29T04:00:00Z',
      last_refresh_outcome: 'partial',
    },
  ]

  it.each([['zh-CN', '正在刷新'], ['en-US', 'Refreshing']] as const)(
    'keeps all triggers pending and clears a disabled source refresh in %s', async (locale, pendingText) => {
      setLocale(locale)
      const disabled = { ...sampleSubscriptions[0]!, enabled: false }
      vi.spyOn(api, 'get').mockResolvedValue({ items: [disabled], page: 1, page_size: 50, total: 1 })
      let resolve!: (value: unknown) => void
      const post = vi.spyOn(api, 'post').mockImplementation(() => new Promise((done) => { resolve = done }) as any)
      mountView()
      await nextTick()
      await nextTick()
      const direct = container.querySelector<HTMLButtonElement>('[data-testid="subscription-card-refresh-btn"]')!
      expect(direct.disabled).toBe(false)
      direct.click()
      await nextTick()
      expect(direct.disabled).toBe(true)
      expect(direct.textContent).toContain(pendingText)
      expect(direct.querySelector('.animate-spin')).not.toBeNull()
      expect(container.querySelector('[data-testid="sub-refresh-loading"]')?.textContent).toContain(pendingText)
      container.querySelector<HTMLButtonElement>('[data-testid="subscription-actions-trigger"]')!.click()
      await nextTick()
      const menu = document.querySelector<HTMLButtonElement>('[data-testid="sub-action-refresh"]')!
      expect(menu.disabled).toBe(true)
      expect(menu.textContent).toContain(pendingText)
      expect(menu.querySelector('.animate-spin')).not.toBeNull()
      direct.click()
      menu.click()
      expect(post).toHaveBeenCalledTimes(1)
      expect(post.mock.calls[0]?.[0]).toBe('/api/v1/subscriptions/sub-never/refresh')
      resolve({ status: 'completed' })
      // Flush the resolved API and list reload before Vue's render flush (no wall-clock delay).
      await new Promise<void>((done) => { setImmediate(done) })
      await nextTick()
      expect(container.querySelector('[data-testid="sub-refresh-loading"]')).toBeNull()
      expect(container.querySelector<HTMLButtonElement>('[data-testid="subscription-card-refresh-btn"]')?.disabled).toBe(false)
      expect(toastStore.items.value.at(-1)?.message).toBe(locale === 'zh-CN' ? '订阅刷新已完成' : 'Subscription refresh completed')
    }
  )

  it('restores card and menu controls when a disabled manual refresh fails', async () => {
    const disabled = { ...sampleSubscriptions[0]!, enabled: false }
    vi.spyOn(api, 'get').mockResolvedValue({ items: [disabled], page: 1, page_size: 50, total: 1 })
    let reject!: (reason: Error) => void
    vi.spyOn(api, 'post').mockImplementation(() => new Promise((_resolve, fail) => { reject = fail }) as any)
    mountView()
    await nextTick()
    await nextTick()
    container.querySelector<HTMLButtonElement>('[data-testid="subscription-actions-trigger"]')!.click()
    await nextTick()
    document.querySelector<HTMLButtonElement>('[data-testid="sub-action-refresh"]')!.click()
    await nextTick()
    expect(container.querySelector('[data-testid="sub-refresh-loading"]')).not.toBeNull()
    reject(new Error('fixture unavailable'))
    // Drain the rejected API and subsequent list reload without a wall-clock delay.
    await new Promise<void>((done) => { setImmediate(done) })
    await nextTick()
    expect(container.querySelector('[data-testid="sub-refresh-loading"]')).toBeNull()
    expect(container.querySelector<HTMLButtonElement>('[data-testid="subscription-card-refresh-btn"]')!.disabled).toBe(false)
    container.querySelector<HTMLButtonElement>('[data-testid="subscription-actions-trigger"]')!.click()
    await nextTick()
    expect(document.querySelector<HTMLButtonElement>('[data-testid="sub-action-refresh"]')!.disabled).toBe(false)
    expect(toastStore.items.value.at(-1)).toMatchObject({ message: 'fixture unavailable', tone: 'error' })
  })

  it('rejects duplicate programmatic refreshes and always cleans up after rejection', async () => {
    let reject!: (reason: Error) => void
    const post = vi.spyOn(api, 'post').mockImplementation(() => new Promise((_resolve, fail) => { reject = fail }) as any)
    vi.spyOn(api, 'get').mockResolvedValue({ items: [], total: 0 })
    const state = useSubscriptions()
    const disabled = { ...sampleSubscriptions[0]!, enabled: false }
    const pending = state.refresh(disabled)
    await state.refresh(disabled)
    expect(post).toHaveBeenCalledTimes(1)
    expect(state.refreshingIDs.value.has(disabled.id)).toBe(true)
    reject(new Error('fixture refresh failed'))
    await pending
    expect(state.refreshingIDs.value.size).toBe(0)
    expect(toastStore.items.value.at(-1)).toMatchObject({ message: 'fixture refresh failed', tone: 'error' })
  })

  it('renders "从未刷新" for subscription without refresh records', async () => {
    vi.spyOn(api, 'get').mockResolvedValue({
      items: [sampleSubscriptions[0]],
      page: 1,
      page_size: 50,
      total: 1,
    })

    mountView()
    await nextTick()
    await new Promise((r) => setTimeout(r, 20))

    const neverEl = container.querySelector('[data-testid="sub-never-refreshed"]')
    expect(neverEl).not.toBeNull()
    expect(neverEl?.textContent?.trim()).toBe('从未刷新')
  })

  it('renders "刷新成功" badge and formatted time with full datetime hover tooltip', async () => {
    vi.spyOn(api, 'get').mockResolvedValue({
      items: [sampleSubscriptions[1]],
      page: 1,
      page_size: 50,
      total: 1,
    })

    mountView()
    await nextTick()
    await new Promise((r) => setTimeout(r, 20))

    const successEl = container.querySelector('[data-testid="sub-refresh-success"]')
    expect(successEl).not.toBeNull()
    expect(successEl?.textContent).toContain('刷新成功')

    const timeEl = successEl?.querySelector('[data-testid="sub-refresh-time"]')
    expect(timeEl).not.toBeNull()
    const title = timeEl?.getAttribute('title')
    expect(title).toBeTruthy()
    // title contains full datetime
    expect(title?.length).toBeGreaterThan(5)
  })

  it('renders prominent "刷新失败" badge and error-styled time when outcome is failed', async () => {
    vi.spyOn(api, 'get').mockResolvedValue({
      items: [sampleSubscriptions[2]],
      page: 1,
      page_size: 50,
      total: 1,
    })

    mountView()
    await nextTick()
    await new Promise((r) => setTimeout(r, 20))

    const failedEl = container.querySelector('[data-testid="sub-refresh-failed"]')
    expect(failedEl).not.toBeNull()
    expect(failedEl?.textContent).toContain('刷新失败')

    // Badge has badge-error class
    const badge = failedEl?.querySelector('.badge-error')
    expect(badge).not.toBeNull()

    // Time text has text-error styling, not implying fresh data
    const timeEl = failedEl?.querySelector('[data-testid="sub-refresh-time"]')
    expect(timeEl?.className).toContain('text-error')
  })

  it('renders "部分成功" badge when outcome is partial', async () => {
    vi.spyOn(api, 'get').mockResolvedValue({
      items: [sampleSubscriptions[3]],
      page: 1,
      page_size: 50,
      total: 1,
    })

    mountView()
    await nextTick()
    await new Promise((r) => setTimeout(r, 20))

    const partialEl = container.querySelector('[data-testid="sub-refresh-partial"]')
    expect(partialEl).not.toBeNull()
    expect(partialEl?.textContent).toContain('部分成功')

    const badge = partialEl?.querySelector('.badge-warning')
    expect(badge).not.toBeNull()
  })

  it('renders english translations when locale is switched to en-US', async () => {
    setLocale('en-US')
    vi.spyOn(api, 'get').mockResolvedValue({
      items: sampleSubscriptions,
      page: 1,
      page_size: 50,
      total: 4,
    })

    mountView()
    await nextTick()
    await new Promise((r) => setTimeout(r, 20))

    const cards = container.querySelectorAll('[data-testid="subscription-card"]')
    expect(cards.length).toBe(4)

    // Check header dt label
    const lastRefreshHeaders = container.querySelectorAll('[data-testid="subscription-last-refresh"] dt')
    expect(lastRefreshHeaders.length).toBe(4)
    expect(lastRefreshHeaders[0]?.textContent?.trim()).toBe('Last Refreshed')

    // Never refreshed
    const neverEl = container.querySelector('[data-testid="sub-never-refreshed"]')
    expect(neverEl?.textContent?.trim()).toBe('Never refreshed')

    // Success
    const successEl = container.querySelector('[data-testid="sub-refresh-success"]')
    expect(successEl?.textContent).toContain('Success')

    // Failed
    const failedEl = container.querySelector('[data-testid="sub-refresh-failed"]')
    expect(failedEl?.textContent).toContain('Refresh Failed')

    // Partial
    const partialEl = container.querySelector('[data-testid="sub-refresh-partial"]')
    expect(partialEl?.textContent).toContain('Partial')
  })

  it('guarantees dedicated row (col-span-2) and adaptive non-wrapping badges to prevent visual clipping', async () => {
    setLocale('en-US')
    vi.spyOn(api, 'get').mockResolvedValue({
      items: sampleSubscriptions,
      page: 1,
      page_size: 50,
      total: 4,
    })

    mountView()
    await nextTick()
    await new Promise((r) => setTimeout(r, 20))

    // Verify all last-refresh blocks occupy dedicated row (col-span-2)
    const blocks = container.querySelectorAll('[data-testid="subscription-last-refresh"]')
    expect(blocks.length).toBe(4)
    blocks.forEach((block) => {
      expect(block.className).toContain('col-span-2')
    })

    // Verify failed badge styling: badge-sm, h-auto, whitespace-nowrap, text-error-content
    const failedBlock = container.querySelector('[data-testid="sub-refresh-failed"]')
    const failedBadge = failedBlock?.querySelector('.badge-error') as HTMLElement
    expect(failedBadge).not.toBeNull()
    expect(failedBadge.className).toContain('badge-sm')
    expect(failedBadge.className).toContain('h-auto')
    expect(failedBadge.className).toContain('whitespace-nowrap')
    expect(failedBadge.className).toContain('text-error-content')
    expect(failedBadge.textContent?.trim()).toBe('Refresh Failed')

    // Verify partial badge styling: badge-sm, h-auto, whitespace-nowrap, text-warning-content
    const partialBlock = container.querySelector('[data-testid="sub-refresh-partial"]')
    const partialBadge = partialBlock?.querySelector('.badge-warning') as HTMLElement
    expect(partialBadge).not.toBeNull()
    expect(partialBadge.className).toContain('badge-sm')
    expect(partialBadge.className).toContain('h-auto')
    expect(partialBadge.className).toContain('whitespace-nowrap')
    expect(partialBadge.className).toContain('text-warning-content')
    expect(partialBadge.textContent?.trim()).toBe('Partial')

    // Verify success badge styling: badge-sm, h-auto, whitespace-nowrap
    const successBlock = container.querySelector('[data-testid="sub-refresh-success"]')
    const successBadge = successBlock?.querySelector('.badge-success') as HTMLElement
    expect(successBadge).not.toBeNull()
    expect(successBadge.className).toContain('badge-sm')
    expect(successBadge.className).toContain('h-auto')
    expect(successBadge.className).toContain('whitespace-nowrap')
    expect(successBadge.textContent?.trim()).toBe('Success')

    // Verify timestamps retain full title tooltips in all non-never cards
    const timeEls = container.querySelectorAll('[data-testid="sub-refresh-time"]')
    expect(timeEls.length).toBe(3)
    timeEls.forEach((el) => {
      const title = el.getAttribute('title')
      expect(title).toBeTruthy()
      expect(title).toMatch(/\d{4}/) // contains 4-digit year
    })
  })

  it('reloads subscription list after refreshing a subscription', async () => {
    let getCalls = 0
    vi.spyOn(api, 'get').mockImplementation(async () => {
      getCalls++
      return {
        items: [sampleSubscriptions[0]],
        page: 1,
        page_size: 50,
        total: 1,
      }
    })
    const postSpy = vi.spyOn(api, 'post').mockResolvedValue({
      outcome: 'success',
      refreshed_at: '2026-09-29T05:00:00Z',
    })

    mountView()
    await nextTick()
    await new Promise((r) => setTimeout(r, 20))
    expect(getCalls).toBe(1)

    // Open popover actions
    const trigger = container.querySelector('[data-testid="subscription-actions-trigger"]') as HTMLButtonElement
    expect(trigger).not.toBeNull()
    trigger.click()
    await nextTick()

    // Click refresh button
    const refreshBtn = document.body.querySelector('[data-testid="sub-action-refresh"]') as HTMLButtonElement
    expect(refreshBtn).not.toBeNull()
    refreshBtn.click()
    await nextTick()
    await new Promise((r) => setTimeout(r, 20))

    // Verify api.post was called with sub-never id
    expect(postSpy).toHaveBeenCalledWith(
      '/api/v1/subscriptions/sub-never/refresh',
      undefined,
      expect.objectContaining({ headers: expect.any(Object) })
    )

    // Verify api.get was called again to reload the list immediately
    expect(getCalls).toBe(2)
  })

  it('renders primary badge and userinfo traffic stats when configured', async () => {
    const primarySub: SubscriptionRecord = {
      id: 'sub-primary',
      name: 'Primary Airport',
      source_url_secret_ref: 'https://example.com/sub',
      enabled: true,
      is_primary: true,
      userinfo: 'upload=1073741824; download=4294967296; total=107374182400; expire=1893456000',
      refresh_policy: {
        interval_seconds: 86400,
        user_agent_policy: 'default',
        timeout_seconds: 30,
        max_response_bytes: 10485760,
      },
      revision: 'rev-primary',
      created_at: '2026-10-17T00:00:00Z',
      updated_at: '2026-10-17T00:00:00Z',
    }

    vi.spyOn(api, 'get').mockResolvedValueOnce({
      items: [primarySub],
      page: 1,
      page_size: 50,
      total: 1,
    })

    mountView()
    await nextTick()
    await new Promise((resolve) => setImmediate(resolve))
    await nextTick()

    // 1. Verify primary badge
    const badge = container.querySelector('[data-testid="primary-sub-badge"]')
    expect(badge).not.toBeNull()
    expect(badge?.textContent).toContain('主订阅')

    // 2. Verify userinfo panel
    const userinfoPanel = container.querySelector('[data-testid="subscription-userinfo"]')
    expect(userinfoPanel).not.toBeNull()
    expect(userinfoPanel?.textContent).toContain('流量与周期')
    expect(userinfoPanel?.textContent).toContain('已用:')
    expect(userinfoPanel?.textContent).toContain('剩余:')
  })
})

describe('formatRefreshTime and formatFullDateTime helpers', () => {
  it('returns empty string for null, undefined, or empty input', () => {
    expect(formatRefreshTime(null)).toBe('')
    expect(formatRefreshTime(undefined)).toBe('')
    expect(formatRefreshTime('')).toBe('')
    expect(formatFullDateTime(null)).toBe('')
    expect(formatFullDateTime(undefined)).toBe('')
  })

  it('formats recent timestamps as 刚刚 / just now', () => {
    const recentIso = new Date(Date.now() - 10_000).toISOString()
    expect(formatRefreshTime(recentIso, 'zh-CN')).toBe('刚刚')
    expect(formatRefreshTime(recentIso, 'en-US')).toBe('just now')
  })

  it('formats minutes ago', () => {
    const minsIso = new Date(Date.now() - 15 * 60_000).toISOString()
    expect(formatRefreshTime(minsIso, 'zh-CN')).toBe('15 分钟前')
    expect(formatRefreshTime(minsIso, 'en-US')).toBe('15 mins ago')
  })

  it('formats hours ago', () => {
    const hrsIso = new Date(Date.now() - 3 * 3600_000).toISOString()
    expect(formatRefreshTime(hrsIso, 'zh-CN')).toBe('3 小时前')
    expect(formatRefreshTime(hrsIso, 'en-US')).toBe('3 hrs ago')
  })

  it('formats days ago', () => {
    const daysIso = new Date(Date.now() - 2 * 86400_000).toISOString()
    expect(formatRefreshTime(daysIso, 'zh-CN')).toBe('2 天前')
    expect(formatRefreshTime(daysIso, 'en-US')).toBe('2 days ago')
  })

  it('formats full date and time string without throw', () => {
    const iso = '2026-09-29T12:34:56Z'
    const fullZh = formatFullDateTime(iso, 'zh-CN')
    const fullEn = formatFullDateTime(iso, 'en-US')
    expect(fullZh.length).toBeGreaterThan(5)
    expect(fullEn.length).toBeGreaterThan(5)
  })
})

describe('SubscriptionConfigDrawer validation guards', () => {
  it('loads, edits and clears only the authoritative top-level prefix', async () => {
    const { default: SubscriptionConfigDrawer } = await import('./SubscriptionConfigDrawer.vue')
    const mountEl = document.createElement('div')
    document.body.appendChild(mountEl)
    const saved = vi.fn()
    const source: SubscriptionRecord = {
      id: 'prefix-source', name: 'Prefix source', source_url_secret_ref: 'https://example.com/feed',
      enabled: false, prefix: 'A ', refresh_policy: { interval_seconds: 3600, timeout_seconds: 30, user_agent_policy: 'default', max_response_bytes: 1024 },
      revision: 'rev', created_at: '', updated_at: '',
      // A legacy config value must not override an intentionally cleared top-level value.
      config: { auto_test: false, ...({ prefix: 'legacy' } as object) },
    }
    const app = createApp({ render: () => h(SubscriptionConfigDrawer, { modelValue: true, subscription: source, onSave: saved }) })
    app.mount(mountEl)
    try {
      await nextTick()
      const input = document.querySelector<HTMLInputElement>('[data-testid="subscription-prefix-input"]')!
      expect(input.value).toBe('A ')
      for (const prefix of ['B ', '']) {
        input.value = prefix
        input.dispatchEvent(new Event('input'))
        document.querySelector<HTMLButtonElement>('[data-testid="subscription-drawer-save-btn"]')!.click()
        await nextTick()
        const payload = saved.mock.calls.at(-1)?.[0]
        expect(payload.prefix).toBe(prefix)
        expect(payload.config).not.toHaveProperty('prefix')
      }
    } finally {
      app.unmount()
      mountEl.remove()
    }
    // Clearing persists as an empty top-level field, never a fallback to old config.
    const { subscriptionPatchPayload } = await import('./useSubscriptions')
    expect(subscriptionPatchPayload(saved.mock.calls.at(-1)?.[0]).prefix).toBe('')
  })
  it('blocks save and forces basic tab when saving with blank name or source URL', async () => {
    const { default: SubscriptionConfigDrawer } = await import('./SubscriptionConfigDrawer.vue')
    const { createApp, h, nextTick } = await import('vue')

    const saveSpy = vi.fn()
    const mountEl = document.createElement('div')
    document.body.appendChild(mountEl)

    const app = createApp({
      render() {
        return h(SubscriptionConfigDrawer, {
          modelValue: true,
          onSave: saveSpy,
        })
      },
    })
    app.mount(mountEl)
    await nextTick()

    // Switch to advanced tab (Drawer teleports to document.body)
    const tabs = document.body.querySelectorAll('.tab')
    expect(tabs.length).toBe(2)
    ;(tabs[1] as HTMLElement).click()
    await nextTick()

    // Click footer save button
    const saveBtn = document.body.querySelector('[data-testid="subscription-drawer-save-btn"]') as HTMLButtonElement | null
    expect(saveBtn).not.toBeNull()
    saveBtn?.click()
    await nextTick()

    // Verification 1: save must NOT be emitted
    expect(saveSpy).not.toHaveBeenCalled()

    // Verification 2: active tab must be switched back to basic
    expect(tabs[0].className).toContain('tab-active')

    // Verification 3: validation error messages must be shown
    const nameError = document.body.querySelector('[data-testid="subscription-name-error"]')
    const urlError = document.body.querySelector('[data-testid="subscription-url-error"]')
    expect(nameError?.textContent).toContain('订阅源名称不能为空')
    expect(urlError?.textContent).toContain('订阅地址不能为空')

    // Now fill the required fields and verify save succeeds
    const nameInput = document.body.querySelector('[data-testid="subscription-name-input"]') as HTMLInputElement | null
    const urlInput = document.body.querySelector('[data-testid="subscription-url-input"]') as HTMLInputElement | null
    if (nameInput && urlInput) {
      nameInput.value = 'Valid HK Sub'
      nameInput.dispatchEvent(new Event('input'))
      urlInput.value = 'https://example.com/sub.yaml'
      urlInput.dispatchEvent(new Event('input'))
    }
    await nextTick()

    saveBtn?.click()
    await nextTick()

    expect(saveSpy).toHaveBeenCalledTimes(1)
    expect(saveSpy).toHaveBeenCalledWith(
      expect.objectContaining({
        name: 'Valid HK Sub',
        source_url_secret_ref: 'https://example.com/sub.yaml',
      })
    )

    app.unmount()
    mountEl.remove()
  })
})

describe('Auto-naming & Domain Extraction Logic', () => {
  it('extracts capitalized second-level domain name according to project contract', () => {
    expect(extractSecondLevelDomainName('https://z.7li7li.com/sub')).toBe('7li7li')
    expect(extractSecondLevelDomainName('https://msub.example.com/api')).toBe('Example')
    expect(extractSecondLevelDomainName('https://www.example.com')).toBe('Example')
    expect(extractSecondLevelDomainName('https://example.com')).toBe('Example')
    expect(extractSecondLevelDomainName('http://localhost:8080/feed')).toBe('Localhost')
    expect(extractSecondLevelDomainName('http://192.168.1.1/feed')).toBe('192.168.1.1')
    expect(extractSecondLevelDomainName('')).toBe('')
  })

  it('auto-fills name from URL unless user has manually modified the name', async () => {
    const { default: SubscriptionConfigDrawer } = await import('./SubscriptionConfigDrawer.vue')
    const mountEl = document.createElement('div')
    document.body.appendChild(mountEl)
    const app = createApp({
      render: () => h(SubscriptionConfigDrawer, { modelValue: true }),
    })
    app.mount(mountEl)

    try {
      await nextTick()
      const nameInput = document.querySelector<HTMLInputElement>('[data-testid="subscription-name-input"]')!
      const urlInput = document.querySelector<HTMLInputElement>('[data-testid="subscription-url-input"]')!

      // 1. Entering URL auto-populates name
      urlInput.value = 'https://msub.example.com/feed'
      urlInput.dispatchEvent(new Event('input'))
      await nextTick()
      expect(nameInput.value).toBe('Example')

      // 2. Changing URL updates auto-name when user hasn't manually edited
      urlInput.value = 'https://z.7li7li.com/sub'
      urlInput.dispatchEvent(new Event('input'))
      await nextTick()
      expect(nameInput.value).toBe('7li7li')

      // 3. User manually modifies name
      nameInput.value = 'My Custom Airport'
      nameInput.dispatchEvent(new Event('input'))
      await nextTick()
      expect(nameInput.value).toBe('My Custom Airport')

      // 4. Changing URL subsequently must NOT overwrite user's manual name
      urlInput.value = 'https://another.provider.com/sub'
      urlInput.dispatchEvent(new Event('input'))
      await nextTick()
      expect(nameInput.value).toBe('My Custom Airport')
    } finally {
      app.unmount()
      mountEl.remove()
    }
  })

  it('does not overwrite existing subscription name when editing URL', async () => {
    const { default: SubscriptionConfigDrawer } = await import('./SubscriptionConfigDrawer.vue')
    const mountEl = document.createElement('div')
    document.body.appendChild(mountEl)
    const existing: SubscriptionRecord = {
      id: 'sub-edit-1',
      name: 'Existing Handcrafted Name',
      source_url_secret_ref: 'https://old.provider.com/sub',
      enabled: true,
      refresh_policy: { interval_seconds: 3600, timeout_seconds: 30, user_agent_policy: 'default', max_response_bytes: 1024 },
      revision: 'rev-1',
      created_at: '',
      updated_at: '',
    }
    const app = createApp({
      render: () => h(SubscriptionConfigDrawer, { modelValue: true, subscription: existing }),
    })
    app.mount(mountEl)

    try {
      await nextTick()
      const nameInput = document.querySelector<HTMLInputElement>('[data-testid="subscription-name-input"]')!
      const urlInput = document.querySelector<HTMLInputElement>('[data-testid="subscription-url-input"]')!
      expect(nameInput.value).toBe('Existing Handcrafted Name')

      urlInput.value = 'https://new.provider.com/sub'
      urlInput.dispatchEvent(new Event('input'))
      await nextTick()

      // Must retain existing name
      expect(nameInput.value).toBe('Existing Handcrafted Name')
    } finally {
      app.unmount()
      mountEl.remove()
    }
  })
})

describe('Unsaved Changes Guard & Progressive Disclosure Fidelity', () => {
  it('allows clean drawer to close immediately without confirm prompt', async () => {
    const { default: SubscriptionConfigDrawer } = await import('./SubscriptionConfigDrawer.vue')
    const mountEl = document.createElement('div')
    document.body.appendChild(mountEl)
    const confirmSpy = vi.spyOn(window, 'confirm')
    const updateModelSpy = vi.fn()

    const app = createApp({
      render: () => h(SubscriptionConfigDrawer, { modelValue: true, 'onUpdate:modelValue': updateModelSpy }),
    })
    app.mount(mountEl)

    try {
      await nextTick()
      const cancelBtn = Array.from(document.body.querySelectorAll('button')).find((b) =>
        b.textContent?.includes('取消') || b.textContent?.includes('Cancel')
      )
      expect(cancelBtn).toBeTruthy()
      cancelBtn?.click()
      await nextTick()

      expect(confirmSpy).not.toHaveBeenCalled()
      expect(updateModelSpy).toHaveBeenCalledWith(false)
    } finally {
      app.unmount()
      mountEl.remove()
      confirmSpy.mockRestore()
    }
  })

  it('prompts user and preserves dirty draft when discard is cancelled', async () => {
    const { default: SubscriptionConfigDrawer } = await import('./SubscriptionConfigDrawer.vue')
    const mountEl = document.createElement('div')
    document.body.appendChild(mountEl)
    // User cancels discard prompt
    const confirmSpy = vi.spyOn(window, 'confirm').mockReturnValue(false)
    const updateModelSpy = vi.fn()

    const app = createApp({
      render: () => h(SubscriptionConfigDrawer, { modelValue: true, 'onUpdate:modelValue': updateModelSpy }),
    })
    app.mount(mountEl)

    try {
      await nextTick()
      const nameInput = document.querySelector<HTMLInputElement>('[data-testid="subscription-name-input"]')!
      nameInput.value = 'Unsaved Dirty Name'
      nameInput.dispatchEvent(new Event('input'))
      await nextTick()

      const cancelBtn = Array.from(document.body.querySelectorAll('button')).find((b) =>
        b.textContent?.includes('取消') || b.textContent?.includes('Cancel')
      )
      cancelBtn?.click()
      await nextTick()

      // Prompt was shown, but since user cancelled, update:modelValue(false) was NOT emitted!
      expect(confirmSpy).toHaveBeenCalled()
      expect(updateModelSpy).not.toHaveBeenCalledWith(false)

      // When user confirms discard:
      confirmSpy.mockReturnValue(true)
      cancelBtn?.click()
      await nextTick()
      expect(updateModelSpy).toHaveBeenCalledWith(false)
    } finally {
      app.unmount()
      mountEl.remove()
      confirmSpy.mockRestore()
    }
  })

  it('preserves advanced tab configuration in the same draft across tab switches', async () => {
    const { default: SubscriptionConfigDrawer } = await import('./SubscriptionConfigDrawer.vue')
    const mountEl = document.createElement('div')
    document.body.appendChild(mountEl)
    const saved = vi.fn()

    const app = createApp({
      render: () => h(SubscriptionConfigDrawer, { modelValue: true, onSave: saved }),
    })
    app.mount(mountEl)

    try {
      await nextTick()
      // Fill basic fields
      const nameInput = document.querySelector<HTMLInputElement>('[data-testid="subscription-name-input"]')!
      const urlInput = document.querySelector<HTMLInputElement>('[data-testid="subscription-url-input"]')!
      nameInput.value = 'Full Fidelity Sub'
      nameInput.dispatchEvent(new Event('input'))
      urlInput.value = 'https://example.com/fidelity.yaml'
      urlInput.dispatchEvent(new Event('input'))

      // Switch to advanced tab
      const tabs = document.body.querySelectorAll('.tab')
      ;(tabs[1] as HTMLElement).click()
      await nextTick()

      // Edit probe policy mode to disabled
      const disabledRadio = document.querySelector<HTMLInputElement>('[data-testid="probe-policy-mode-disabled"]')!
      disabledRadio.click()
      await nextTick()

      // Switch back to basic tab
      ;(tabs[0] as HTMLElement).click()
      await nextTick()

      // Save from basic tab
      const saveBtn = document.querySelector<HTMLButtonElement>('[data-testid="subscription-drawer-save-btn"]')!
      saveBtn.click()
      await nextTick()

      expect(saved).toHaveBeenCalledTimes(1)
      const payload = saved.mock.calls[0][0]
      expect(payload.name).toBe('Full Fidelity Sub')
      expect(payload.source_url_secret_ref).toBe('https://example.com/fidelity.yaml')
      // Advanced configuration was preserved in the same draft
      expect(payload.config.probe_policy).toEqual({ mode: 'disabled' })
    } finally {
      app.unmount()
      mountEl.remove()
    }
  })
})

describe('Per-Row State, Concurrency & Layout Invariants', () => {
  it('isolates refresh failure and row error to the specific subscription', async () => {
    const subA: SubscriptionRecord = {
      id: 'sub-A',
      name: 'Sub Alpha',
      source_url_secret_ref: 'https://example.com/a',
      enabled: true,
      refresh_policy: { interval_seconds: 3600, timeout_seconds: 30, user_agent_policy: 'default', max_response_bytes: 1024 },
      revision: 'rev-a',
      created_at: '',
      updated_at: '',
    }
    const subB: SubscriptionRecord = {
      id: 'sub-B',
      name: 'Sub Beta',
      source_url_secret_ref: 'https://example.com/b',
      enabled: true,
      refresh_policy: { interval_seconds: 3600, timeout_seconds: 30, user_agent_policy: 'default', max_response_bytes: 1024 },
      revision: 'rev-b',
      created_at: '',
      updated_at: '',
    }

    vi.spyOn(api, 'get').mockResolvedValue({ items: [subA, subB], page: 1, page_size: 50, total: 2 })
    vi.spyOn(api, 'post').mockImplementation((url: string) => {
      if (url.includes('sub-A')) {
        return Promise.reject(new Error('Sub A timeout failure'))
      }
      return Promise.resolve({ status: 'completed' })
    })

    const hook = useSubscriptions()
    await hook.load()
    expect(hook.items.value).toHaveLength(2)

    // Refresh Sub A
    await hook.refresh(subA).catch(() => {})
    expect(hook.rowErrors.value['sub-A']).toBe('Sub A timeout failure')
    expect(hook.rowErrors.value['sub-B']).toBeUndefined()

    // Dismiss Sub A row error
    hook.clearRowError('sub-A')
    expect(hook.rowErrors.value['sub-A']).toBeUndefined()
  })

  it('prevents out-of-order reload race conditions from overwriting newer state', async () => {
    let resolveFirst!: (value: any) => void
    let resolveSecond!: (value: any) => void

    const firstPromise = new Promise((res) => { resolveFirst = res })
    const secondPromise = new Promise((res) => { resolveSecond = res })

    let callCount = 0
    vi.spyOn(api, 'get').mockImplementation(() => {
      callCount++
      if (callCount === 1) return firstPromise as any
      return secondPromise as any
    })

    const hook = useSubscriptions()
    const p1 = hook.load() // sequence 1
    const p2 = hook.load() // sequence 2

    // Resolve second request first (newer response)
    resolveSecond({
      items: [{ id: 'sub-newer', name: 'Newer Subscription', source_url_secret_ref: 'https://new.com', enabled: true, refresh_policy: { interval_seconds: 3600, timeout_seconds: 30, user_agent_policy: 'default', max_response_bytes: 1024 }, revision: 'r2', created_at: '', updated_at: '' }],
      page: 1,
      page_size: 50,
      total: 1,
    })
    await p2
    expect(hook.items.value[0]?.id).toBe('sub-newer')

    // Now resolve first request later (stale response)
    resolveFirst({
      items: [{ id: 'sub-stale', name: 'Stale Subscription', source_url_secret_ref: 'https://old.com', enabled: true, refresh_policy: { interval_seconds: 3600, timeout_seconds: 30, user_agent_policy: 'default', max_response_bytes: 1024 }, revision: 'r1', created_at: '', updated_at: '' }],
      page: 1,
      page_size: 50,
      total: 1,
    })
    await p1

    // State must NOT be overwritten by stale out-of-order response
    expect(hook.items.value[0]?.id).toBe('sub-newer')
  })

  it('does not display fake 0 sources on initial loading or error', async () => {
    vi.spyOn(api, 'get').mockImplementation(() => new Promise(() => {})) // never resolves (simulating loading)

    const mountEl = document.createElement('div')
    document.body.appendChild(mountEl)
    const app = createApp({ render: () => h(SubscriptionsView) })
    app.mount(mountEl)

    try {
      await nextTick()
      // On initial loading, total count footer must NOT be rendered with fake 0
      expect(document.querySelector('[data-testid="subscription-total-count"]')).toBeNull()
      // Skeletons must be present
      expect(document.querySelectorAll('.skeleton').length).toBeGreaterThan(0)
    } finally {
      app.unmount()
      mountEl.remove()
    }
  })

  it('renders PageToolbar without duplicate visible h1 headings', async () => {
    vi.spyOn(api, 'get').mockResolvedValue({ items: [], page: 1, page_size: 50, total: 0 })

    const mountEl = document.createElement('div')
    document.body.appendChild(mountEl)
    const app = createApp({ render: () => h(SubscriptionsView) })
    app.mount(mountEl)

    try {
      await nextTick()
      await new Promise((r) => setTimeout(r, 10))
      await nextTick()

      // PageToolbar should exist
      const toolbar = document.querySelector('[data-testid="page-toolbar"]')
      expect(toolbar).not.toBeNull()

      // No visible duplicate h1 elements
      const h1List = Array.from(document.querySelectorAll('h1'))
      const visibleH1 = h1List.filter((h1) => !h1.classList.contains('sr-only'))
      expect(visibleH1.length).toBe(0)
    } finally {
      app.unmount()
      mountEl.remove()
    }
  })

  describe('FR-RM6 Subscription Delete Retry & Mutation vs Reload Isolation', () => {
    const testSub: SubscriptionRecord = {
      id: 'sub-to-delete',
      name: 'Delete Target Subscription',
      source_url_secret_ref: 'https://example.com/target',
      enabled: true,
      node_count: 5,
      source_node_count: 5,
      counts_scope: 'enabled_subscriptions',
      refresh_policy: {
        interval_seconds: 3600,
        user_agent_policy: 'default',
        timeout_seconds: 30,
        max_response_bytes: 1048576,
      },
      revision: 'rev-del-1',
      created_at: '2026-10-01T00:00:00Z',
      updated_at: '2026-10-01T00:00:00Z',
    }

    it('retains confirmation modal and pending object on DELETE failure, unlocking button for retry', async () => {
      vi.spyOn(api, 'get').mockResolvedValue([testSub])
      vi.spyOn(api, 'delete').mockRejectedValueOnce(
        new ApiError(500, 'server_error', 'Database write lock timeout')
      )

      const mountEl = document.createElement('div')
      document.body.appendChild(mountEl)
      const app = createApp({ render: () => h(SubscriptionsView) })
      app.mount(mountEl)

      try {
        await nextTick()
        await new Promise((r) => setTimeout(r, 20))
        await nextTick()

        // Open confirm delete modal
        const deleteActionBtn = document.querySelector('[data-testid="sub-action-delete"]') as HTMLButtonElement
        expect(deleteActionBtn).not.toBeNull()
        deleteActionBtn.click()
        await nextTick()

        const modalMsg = document.querySelector('[data-testid="confirm-modal-message"]')
        expect(modalMsg?.textContent).toContain('Delete Target Subscription')

        const confirmBtn = document.querySelector('[data-testid="confirm-modal-confirm"]') as HTMLButtonElement
        expect(confirmBtn).not.toBeNull()
        confirmBtn.click()

        // Wait for delete mutation to finish failing
        await nextTick()
        await new Promise((r) => setTimeout(r, 20))
        await nextTick()

        // 1. Error toast was pushed
        expect(toastStore.items.value.length).toBeGreaterThan(0)
        expect(toastStore.items.value.some((t: Toast) => t.tone === 'error' && t.message.includes('Database write lock timeout'))).toBe(true)

        // 2. Modal REMAINS OPEN with pending object message intact
        const dialogEl = document.querySelector('dialog') as HTMLDialogElement | null
        expect(dialogEl).not.toBeNull()
        expect(document.querySelector('[data-testid="confirm-modal-message"]')?.textContent).toContain('Delete Target Subscription')

        // 3. Confirm button is unlocked (loading = false, disabled = false) so user can retry
        expect(confirmBtn.disabled).toBe(false)
        expect(confirmBtn.classList.contains('loading')).toBe(false)
      } finally {
        app.unmount()
        mountEl.remove()
      }
    })

    it('closes modal on DELETE success and separates subsequent reload failure from mutation', async () => {
      let getCallCount = 0
      vi.spyOn(api, 'get').mockImplementation(async () => {
        getCallCount++
        if (getCallCount === 1) {
          return [testSub]
        }
        // Subsequent reload after delete fails
        throw new ApiError(500, 'server_error', 'Failed to reload subscription list')
      })
      vi.spyOn(api, 'delete').mockResolvedValueOnce(undefined)

      const mountEl = document.createElement('div')
      document.body.appendChild(mountEl)
      const app = createApp({ render: () => h(SubscriptionsView) })
      app.mount(mountEl)

      try {
        await nextTick()
        await new Promise((r) => setTimeout(r, 20))
        await nextTick()

        const deleteActionBtn = document.querySelector('[data-testid="sub-action-delete"]') as HTMLButtonElement
        deleteActionBtn.click()
        await nextTick()

        const confirmBtn = document.querySelector('[data-testid="confirm-modal-confirm"]') as HTMLButtonElement
        confirmBtn.click()

        await nextTick()
        await new Promise((r) => setTimeout(r, 20))
        await nextTick()

        // 1. Success toast was pushed for delete mutation
        expect(toastStore.items.value.some((t: Toast) => t.tone === 'success' && t.message.includes('订阅源已删除'))).toBe(true)

        // 2. Warning toast pushed for subsequent reload failure (not calling DELETE failed)
        expect(toastStore.items.value.some((t: Toast) => t.tone === 'warning' && t.message.includes('刷新列表失败'))).toBe(true)

        // 3. Modal is CLOSED and does NOT induce repeated DELETE
        const modalMsg = document.querySelector('[data-testid="confirm-modal-message"]')
        // Either message is empty or dialog is closed
        expect(modalMsg?.textContent || '').not.toContain('Delete Target Subscription')
      } finally {
        app.unmount()
        mountEl.remove()
      }
    })

    it('useSubscriptions.remove removes deleted item immediately, adjusts total, and emits warning when reload fails', async () => {
      let getCalls = 0
      vi.spyOn(api, 'get').mockImplementation(async () => {
        getCalls++
        if (getCalls === 1) {
          return { items: [testSub], total: 1 }
        }
        throw new ApiError(500, 'server_error', 'Database connection refused')
      })
      vi.spyOn(api, 'delete').mockResolvedValueOnce(undefined)

      const hook = useSubscriptions()
      const initialLoadOk = await hook.load()
      expect(initialLoadOk).toBe(true)
      expect(hook.items.value).toHaveLength(1)
      expect(hook.total.value).toBe(1)
      expect(hook.items.value[0].id).toBe('sub-to-delete')

      // Perform remove
      const res = await hook.remove(testSub)
      expect(res.ok).toBe(true)

      // 1. In-memory items and total are immediately adjusted to remove the deleted entry
      expect(hook.items.value).toHaveLength(0)
      expect(hook.total.value).toBe(0)

      // 2. Load failure error is reflected in hook.error
      expect(hook.error.value).toBe('Database connection refused')

      // 3. Toasts: both success (for deletion) and warning (for reload failure) were published
      expect(toastStore.items.value.some((t: Toast) => t.tone === 'success' && t.message.includes('订阅源已删除'))).toBe(true)
      expect(toastStore.items.value.some((t: Toast) => t.tone === 'warning' && t.message.includes('订阅源已删除，但刷新列表失败: Database connection refused'))).toBe(true)
    })
  })
})
