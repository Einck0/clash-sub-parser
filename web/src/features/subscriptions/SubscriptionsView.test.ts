// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { setImmediate } from 'node:timers'
import { createApp, h, nextTick } from 'vue'
import SubscriptionsView from './SubscriptionsView.vue'
import { api } from '../../api/client'
import { toastStore } from '../../ui/toast'
import { setLocale } from '../../locales'
import {
  formatRefreshTime,
  formatFullDateTime,
  useSubscriptions,
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
