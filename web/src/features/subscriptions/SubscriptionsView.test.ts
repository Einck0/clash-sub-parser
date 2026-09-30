// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { createApp, h, nextTick } from 'vue'
import SubscriptionsView from './SubscriptionsView.vue'
import { api } from '../../api/client'
import { toastStore } from '../../ui/toast'
import { setLocale } from '../../locales'
import {
  formatRefreshTime,
  formatFullDateTime,
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
