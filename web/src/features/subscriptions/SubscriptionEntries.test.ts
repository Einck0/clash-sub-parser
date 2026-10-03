// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { createApp, h, nextTick } from 'vue'
import SubscriptionEntriesDrawer from './SubscriptionEntriesDrawer.vue'
import SubscriptionsView from './SubscriptionsView.vue'
import { api } from '../../api/client'
import { toastStore } from '../../ui/toast'
import { setLocale } from '../../locales'
import type { SubscriptionEntryDTO, SubscriptionRecord } from './useSubscriptions'

describe('Subscription Source Entries & Classification Override Flow', () => {
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

  const sampleSub: SubscriptionRecord = {
    id: 'sub-sample-1',
    name: 'Primary Upstream',
    source_url_secret_ref: 'https://example.com/sub',
    enabled: true,
    refresh_policy: {
      interval_seconds: 3600,
      user_agent_policy: 'default',
      timeout_seconds: 30,
      max_response_bytes: 10485760,
    },
    revision: 'rev-1',
    created_at: '2026-10-01T00:00:00Z',
    updated_at: '2026-10-01T00:00:00Z',
    last_refreshed_at: '2026-10-01T01:00:00Z',
    last_refresh_outcome: 'success',
  }

  it('preserves all 3 notice entries with identical node parameters as 3 metadata items (not deduped into 1)', async () => {
    const noticeEntries: SubscriptionEntryDTO[] = [
      {
        entry_id: 'entry-notice-1',
        subscription_id: 'sub-sample-1',
        payload_id: 'payload-notice-common',
        ordinal: 1,
        name: '官网地址: https://example.com',
        entry_kind: 'notice',
        user_kind_override: null,
        effective_kind: 'notice',
        classification_reason: '匹配公告正则规则',
      },
      {
        entry_id: 'entry-notice-2',
        subscription_id: 'sub-sample-1',
        payload_id: 'payload-notice-common',
        ordinal: 2,
        name: '剩余流量: 250 GB',
        entry_kind: 'notice',
        user_kind_override: null,
        effective_kind: 'notice',
        classification_reason: '匹配公告正则规则',
      },
      {
        entry_id: 'entry-notice-3',
        subscription_id: 'sub-sample-1',
        payload_id: 'payload-notice-common',
        ordinal: 3,
        name: '到期时间: 2027-01-01',
        entry_kind: 'notice',
        user_kind_override: null,
        effective_kind: 'notice',
        classification_reason: '匹配公告正则规则',
      },
    ]

    vi.spyOn(api, 'get').mockResolvedValueOnce({
      items: noticeEntries,
      total: 3,
    })

    app = createApp({
      render() {
        return h(SubscriptionEntriesDrawer, {
          modelValue: true,
          subscription: sampleSub,
        })
      },
    })
    app.mount(container)

    await nextTick()
    await new Promise((r) => setTimeout(r, 20))

    const noticeSection = document.body.querySelector('[data-testid="subscription-notices-section"]')
    expect(noticeSection).not.toBeNull()

    const noticeItems = document.body.querySelectorAll('[data-testid="subscription-notice-item"]')
    expect(noticeItems.length).toBe(3)
    expect(noticeSection?.textContent).toContain('官网地址')
    expect(noticeSection?.textContent).toContain('剩余流量: 250 GB')
    expect(noticeSection?.textContent).toContain('到期时间: 2027-01-01')
  })

  it('renders normal localhost proxy entries without filtering them out', async () => {
    const mixedEntries: SubscriptionEntryDTO[] = [
      {
        entry_id: 'entry-local-1',
        subscription_id: 'sub-sample-1',
        payload_id: 'payload-local-1',
        ordinal: 1,
        name: 'Localhost SOCKS5 127.0.0.1:1080',
        entry_kind: 'proxy',
        user_kind_override: null,
        effective_kind: 'proxy',
        node_logical_id: 'node-local-1',
        classification_reason: '正常识别为代理节点',
      },
    ]

    vi.spyOn(api, 'get').mockResolvedValueOnce({
      items: mixedEntries,
      total: 1,
    })

    app = createApp({
      render() {
        return h(SubscriptionEntriesDrawer, {
          modelValue: true,
          subscription: sampleSub,
        })
      },
    })
    app.mount(container)

    await nextTick()
    await new Promise((r) => setTimeout(r, 20))

    const entryItems = document.body.querySelectorAll('[data-testid="subscription-entry-item"]')
    expect(entryItems.length).toBe(1)
    expect(entryItems[0].textContent).toContain('Localhost SOCKS5 127.0.0.1:1080')
    expect(entryItems[0].textContent).toContain('生效: proxy')
  })

  it('allows correcting unknown entries to proxy with immediate inline override without nested confirmation modals', async () => {
    const unknownEntry: SubscriptionEntryDTO = {
      entry_id: 'entry-unknown-1',
      subscription_id: 'sub-sample-1',
      payload_id: 'payload-unknown-1',
      ordinal: 1,
      name: 'Custom Unrecognized Node',
      entry_kind: 'unknown',
      user_kind_override: null,
      effective_kind: 'unknown',
      classification_reason: '未匹配任何已知协议特征',
    }

    vi.spyOn(api, 'get').mockResolvedValueOnce({
      items: [unknownEntry],
      total: 1,
    })

    const updatedEntry: SubscriptionEntryDTO = {
      ...unknownEntry,
      user_kind_override: 'proxy',
      effective_kind: 'proxy',
      override_at: new Date().toISOString(),
      override_reason: '手动纠偏为代理节点',
    }

    const postSpy = vi.spyOn(api, 'post').mockResolvedValueOnce(updatedEntry)

    app = createApp({
      render() {
        return h(SubscriptionEntriesDrawer, {
          modelValue: true,
          subscription: sampleSub,
        })
      },
    })
    app.mount(container)

    await nextTick()
    await new Promise((r) => setTimeout(r, 20))

    const overrideProxyBtn = document.body.querySelector<HTMLButtonElement>('[data-testid="override-proxy-btn"]')
    expect(overrideProxyBtn).not.toBeNull()

    overrideProxyBtn!.click()
    await nextTick()
    await new Promise((r) => setTimeout(r, 20))

    expect(postSpy).toHaveBeenCalledWith(
      '/api/v1/subscriptions/sub-sample-1/entries/entry-unknown-1/override',
      { user_kind_override: 'proxy' }
    )

    const effectiveBadge = document.body.querySelector('[data-testid="entry-effective-kind"]')
    expect(effectiveBadge?.textContent).toContain('生效: proxy')
  })

  it('resets kind override to null when user selects auto, restoring server default effective kind', async () => {
    const overriddenEntry: SubscriptionEntryDTO = {
      entry_id: 'entry-custom-1',
      subscription_id: 'sub-sample-1',
      payload_id: 'payload-custom-1',
      ordinal: 1,
      name: 'Temporarily Overridden Node',
      entry_kind: 'proxy',
      user_kind_override: 'proxy',
      effective_kind: 'proxy',
      override_at: '2026-10-01T02:00:00Z',
    }

    vi.spyOn(api, 'get').mockResolvedValueOnce({
      items: [overriddenEntry],
      total: 1,
    })

    const resetEntry: SubscriptionEntryDTO = {
      ...overriddenEntry,
      user_kind_override: null,
      effective_kind: 'proxy',
      override_at: undefined,
    }

    const postSpy = vi.spyOn(api, 'post').mockResolvedValueOnce(resetEntry)

    app = createApp({
      render() {
        return h(SubscriptionEntriesDrawer, {
          modelValue: true,
          subscription: sampleSub,
        })
      },
    })
    app.mount(container)

    await nextTick()
    await new Promise((r) => setTimeout(r, 20))

    const autoBtn = document.body.querySelector<HTMLButtonElement>('[data-testid="override-auto-btn"]')
    expect(autoBtn).not.toBeNull()

    autoBtn!.click()
    await nextTick()
    await new Promise((r) => setTimeout(r, 20))

    expect(postSpy).toHaveBeenCalledWith(
      '/api/v1/subscriptions/sub-sample-1/entries/entry-custom-1/override',
      { user_kind_override: null }
    )
  })

  it('renders conflict description clearly when server returns conflict on an entry', async () => {
    const conflictedEntry: SubscriptionEntryDTO = {
      entry_id: 'entry-conflicted-1',
      subscription_id: 'sub-sample-1',
      payload_id: 'payload-conflicted-1',
      ordinal: 1,
      name: 'Conflict Node HK-01',
      entry_kind: 'proxy',
      user_kind_override: null,
      effective_kind: 'proxy',
      conflict: '逻辑 ID 与订阅源 sub-upstream-2 发生冲突，已被次优保留',
    }

    vi.spyOn(api, 'get').mockResolvedValueOnce({
      items: [conflictedEntry],
      total: 1,
    })

    app = createApp({
      render() {
        return h(SubscriptionEntriesDrawer, {
          modelValue: true,
          subscription: sampleSub,
        })
      },
    })
    app.mount(container)

    await nextTick()
    await new Promise((r) => setTimeout(r, 20))

    const conflictCard = document.body.querySelector('[data-testid="entry-conflict-warning"]')
    expect(conflictCard).not.toBeNull()
    expect(conflictCard?.textContent).toContain('逻辑 ID 与订阅源 sub-upstream-2 发生冲突')
  })

  it('opens SubscriptionEntriesDrawer when clicking view entries from SubscriptionsView card', async () => {
    vi.spyOn(api, 'get').mockImplementation(async (path: string) => {
      if (path === '/api/v1/subscriptions') {
        return {
          items: [sampleSub],
          total: 1,
          page: 1,
          page_size: 50,
        }
      }
      if (path.includes('/entries')) {
        return {
          items: [
            {
              entry_id: 'e-1',
              subscription_id: sampleSub.id,
              payload_id: 'p-1',
              ordinal: 1,
              name: 'Sample Node',
              entry_kind: 'proxy',
              effective_kind: 'proxy',
            },
          ],
          total: 1,
        }
      }
      return { items: [], total: 0 }
    })

    app = createApp({
      render() {
        return h(SubscriptionsView)
      },
    })
    app.mount(container)

    await nextTick()
    await new Promise((r) => setTimeout(r, 20))

    const viewEntriesBtn = container.querySelector<HTMLButtonElement>('[data-testid="sub-view-entries"]')
    expect(viewEntriesBtn).not.toBeNull()

    viewEntriesBtn!.click()
    await nextTick()
    await new Promise((r) => setTimeout(r, 20))

    const drawer = document.body.querySelector('[role="dialog"]')
    expect(drawer).not.toBeNull()
    expect(document.body.textContent).toContain('最新来源条目')
  })
})
