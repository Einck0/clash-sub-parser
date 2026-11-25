// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { createApp, h, nextTick, ref } from 'vue'
import { createMemoryHistory, createRouter, RouterView } from 'vue-router'
import { api } from '../../api/client'
import { setLocale } from '../../locales'
import { toastStore } from '../../ui/toast'
import SubscriptionConfigDrawer from './SubscriptionConfigDrawer.vue'
import SubscriptionEntriesDrawer from './SubscriptionEntriesDrawer.vue'
import SubscriptionsView from './SubscriptionsView.vue'
import {
  defaultConfig,
  defaultPolicy,
  useSubscriptionEntries,
  useSubscriptions,
  type SubscriptionDraft,
  type SubscriptionEntryDTO,
  type SubscriptionRecord,
} from './useSubscriptions'

function subscription(id = 'source-a'): SubscriptionRecord {
  return {
    id, name: `Provider ${id.replace(/-/g, ' ')}`, source_url_secret_ref: '', enabled: true,
    refresh_policy: defaultPolicy(), config: defaultConfig(), revision: 'revision-1', created_at: '', updated_at: '',
  }
}
function entry(id: string, sourceId = 'source-a'): SubscriptionEntryDTO {
  return {
    entry_id: id, subscription_id: sourceId, payload_id: 'payload', ordinal: 1,
    name: id.replace(/-/g, ' '), entry_kind: 'unknown', effective_kind: 'unknown', user_kind_override: null,
  }
}
function deferred<T>() {
  let resolve!: (value: T) => void
  let reject!: (error: Error) => void
  const promise = new Promise<T>((res, rej) => { resolve = res; reject = rej })
  return { promise, resolve, reject }
}
async function flush() {
  for (let i = 0; i < 5; i++) {
    await Promise.resolve()
    await nextTick()
  }
}

let app: ReturnType<typeof createApp> | undefined
let root: HTMLDivElement
function mount(render: () => ReturnType<typeof h>) {
  root = document.createElement('div')
  document.body.append(root)
  app = createApp({ render })
  app.mount(root)
}
beforeEach(() => {
  setLocale('zh-CN')
  toastStore.clear()
})
afterEach(() => {
  app?.unmount()
  app = undefined
  root?.remove()
  toastStore.clear()
  vi.restoreAllMocks()
  setLocale('zh-CN')
})

describe('Subscription draft lifecycle', () => {
  it.each(['header', 'backdrop', 'escape', 'cancel'])('consumes one %s close request and preserves a rejected discard', async (control) => {
    const updates = vi.fn()
    const confirm = vi.spyOn(window, 'confirm').mockReturnValue(false)
    const source = subscription()
    source.config = { rename_rules: [{ pattern: 'before', replace: 'after' }], filter_rules: [{ type: 'include', pattern: 'HK' }] }
    mount(() => h(SubscriptionConfigDrawer, { modelValue: true, subscription: source, 'onUpdate:modelValue': updates }))
    await flush()
    const rules = document.querySelectorAll<HTMLInputElement>('input[placeholder="匹配正则"]')
    rules[0].value = 'unsaved-rename'
    rules[0].dispatchEvent(new Event('input'))
    rules[1].value = 'unsaved-filter'
    rules[1].dispatchEvent(new Event('input'))
    await flush()
    const close = () => {
      if (control === 'escape') window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', cancelable: true }))
      else if (control === 'header') document.querySelector<HTMLButtonElement>('[data-drawer-panel] button')!.click()
      else if (control === 'backdrop') document.querySelector<HTMLElement>('[data-testid="drawer-backdrop"]')!.click()
      else Array.from(document.querySelectorAll('button')).find((button) => button.textContent?.trim() === '取消')!.click()
    }
    close()
    await flush()
    expect(confirm).toHaveBeenCalledTimes(1)
    expect(updates).not.toHaveBeenCalled()
    expect(rules[0].value).toBe('unsaved-rename')
    expect(rules[1].value).toBe('unsaved-filter')
    expect(source.config.rename_rules?.[0].pattern).toBe('before')
    expect(source.config.filter_rules?.[0].pattern).toBe('HK')
    confirm.mockReturnValue(true)
    close()
    await flush()
    expect(confirm).toHaveBeenCalledTimes(2)
    expect(updates).toHaveBeenCalledTimes(1)
    expect(updates).toHaveBeenCalledWith(false)
  })

  it('blocks route departure during save and preserves the draft after a failed save', async () => {
    let settle!: (error?: string) => void
    const source = subscription()
    const page = { render: () => h(SubscriptionConfigDrawer, {
      modelValue: true, subscription: source,
      onSave: (_draft: SubscriptionDraft, done: (error?: string) => void) => { settle = done },
    }) }
    const router = createRouter({ history: createMemoryHistory(), routes: [
      { path: '/subscriptions', component: page }, { path: '/other', component: { template: '<p>Other</p>' } },
    ] })
    await router.push('/subscriptions')
    root = document.createElement('div')
    document.body.append(root)
    app = createApp({ render: () => h(RouterView) })
    app.use(router)
    app.mount(root)
    await flush()
    const name = document.querySelector<HTMLInputElement>('[data-testid="subscription-name-input"]')!
    name.value = 'Pending draft'
    name.dispatchEvent(new Event('input'))
    await flush()
    document.querySelector<HTMLButtonElement>('[data-testid="subscription-drawer-save-btn"]')!.click()
    await flush()
    const confirm = vi.spyOn(window, 'confirm').mockReturnValue(true)
    await router.push('/other')
    expect(router.currentRoute.value.path).toBe('/subscriptions')
    expect(confirm).not.toHaveBeenCalled()
    settle('Request failed')
    await flush()
    confirm.mockReturnValue(false)
    await router.push('/other')
    expect(router.currentRoute.value.path).toBe('/subscriptions')
    expect(name.value).toBe('Pending draft')
    expect(confirm).toHaveBeenCalledOnce()
  })

  it('locks duplicate submits and input while pending, retains failed draft, and cleans only after acknowledged success', async () => {
    let settle!: (error?: string) => void
    const save = vi.fn((_draft: SubscriptionDraft, done: (error?: string) => void) => { settle = done })
    const updates = vi.fn()
    const confirm = vi.spyOn(window, 'confirm').mockReturnValue(false)
    mount(() => h(SubscriptionConfigDrawer, { modelValue: true, subscription: subscription(), onSave: save, 'onUpdate:modelValue': updates }))
    await flush()
    const name = document.querySelector<HTMLInputElement>('[data-testid="subscription-name-input"]')!
    name.value = 'my draft'
    name.dispatchEvent(new Event('input'))
    await flush()
    const submit = document.querySelector<HTMLButtonElement>('[data-testid="subscription-drawer-save-btn"]')!
    submit.click()
    submit.click()
    await flush()
    expect(save).toHaveBeenCalledTimes(1)
    expect(document.querySelector('fieldset')?.disabled).toBe(true)
    document.querySelector<HTMLElement>('[data-testid="drawer-backdrop"]')!.click()
    expect(updates).not.toHaveBeenCalled()
    expect(confirm).not.toHaveBeenCalled()
    settle('save rejected')
    await flush()
    expect(submit.disabled).toBe(false)
    expect(name.value).toBe('my draft')
    expect(document.querySelector('[data-testid="subscription-drawer-error"]')?.textContent).toContain('save rejected')
    document.querySelector<HTMLElement>('[data-testid="drawer-backdrop"]')!.click()
    expect(confirm).toHaveBeenCalledTimes(1)
    expect(updates).not.toHaveBeenCalled()
    submit.click()
    await flush()
    expect(save).toHaveBeenCalledTimes(2)
    settle()
    await flush()
    document.querySelector<HTMLElement>('[data-testid="drawer-backdrop"]')!.click()
    expect(confirm).toHaveBeenCalledTimes(1)
    expect(updates).toHaveBeenCalledWith(false)
  })
})

describe('Latest source entry isolation', () => {
  it('ignores A slow success after B has loaded and does not retain A while B is pending', async () => {
    const a = deferred<{ items: SubscriptionEntryDTO[]; total: number }>()
    const b = deferred<{ items: SubscriptionEntryDTO[]; total: number }>()
    vi.spyOn(api, 'get').mockReturnValueOnce(a.promise).mockReturnValueOnce(b.promise)
    const state = useSubscriptionEntries()
    const first = state.loadEntries('source-a')
    const second = state.loadEntries('source-b')
    b.resolve({ items: [entry('B', 'source-b')], total: 1 })
    await second
    a.resolve({ items: [entry('A')], total: 1 })
    await first
    expect(state.entries.value.map((item) => item.name)).toEqual(['B'])
    expect(state.loading.value).toBe(false)
    expect(state.error.value).toBe('')
  })

  it('ignores A late failure after B success and invalidates requests on drawer close', async () => {
    const a = deferred<{ items: SubscriptionEntryDTO[]; total: number }>()
    vi.spyOn(api, 'get').mockReturnValueOnce(a.promise).mockResolvedValueOnce({ items: [entry('B', 'source-b')], total: 1 })
    const state = useSubscriptionEntries()
    const first = state.loadEntries('source-a')
    await state.loadEntries('source-b')
    a.reject(new Error('A unavailable'))
    await first
    expect(state.error.value).toBe('')
    expect(state.entries.value[0].name).toBe('B')

    const late = deferred<{ items: SubscriptionEntryDTO[]; total: number }>()
    vi.mocked(api.get).mockReturnValueOnce(late.promise)
    const closed = state.loadEntries('source-b')
    state.resetEntries()
    late.resolve({ items: [entry('late', 'source-b')], total: 1 })
    await closed
    expect(state.entries.value).toEqual([])
    expect(state.loading.value).toBe(false)
  })

  it('tracks each pending override independently and ignores mutation results from the old source', async () => {
    const first = entry('entry-1')
    const second = entry('entry-2')
    const a = deferred<SubscriptionEntryDTO>()
    const b = deferred<SubscriptionEntryDTO>()
    vi.spyOn(api, 'get').mockResolvedValueOnce({ items: [first, second], total: 2 }).mockResolvedValueOnce({ items: [entry('entry-1', 'source-b')], total: 1 })
    const post = vi.spyOn(api, 'post').mockReturnValueOnce(a.promise).mockReturnValueOnce(b.promise)
    const state = useSubscriptionEntries()
    await state.loadEntries('source-a')
    const pendingA = state.setOverride('source-a', first.entry_id, 'proxy')
    const pendingB = state.setOverride('source-a', second.entry_id, 'notice')
    await state.setOverride('source-a', first.entry_id, 'notice')
    expect(post).toHaveBeenCalledTimes(2)
    expect(state.isOverriding('source-a', first.entry_id)).toBe(true)
    b.resolve({ ...second, user_kind_override: 'notice', effective_kind: 'notice' })
    await pendingB
    expect(state.isOverriding('source-a', first.entry_id)).toBe(true)
    expect(state.isOverriding('source-a', second.entry_id)).toBe(false)
    await state.loadEntries('source-b')
    a.resolve({ ...first, user_kind_override: 'proxy', effective_kind: 'proxy' })
    await pendingA
    expect(state.entries.value[0].subscription_id).toBe('source-b')
    expect(state.entries.value[0].effective_kind).toBe('unknown')
  })

  it('preserves an entry on override failure and exposes the failure without success feedback', async () => {
    vi.spyOn(api, 'get').mockResolvedValueOnce({ items: [entry('failed')], total: 1 })
    vi.spyOn(api, 'post').mockRejectedValueOnce(new Error('override rejected'))
    const state = useSubscriptionEntries()
    await state.loadEntries('source-a')
    await expect(state.setOverride('source-a', 'failed', 'proxy')).rejects.toThrow('override rejected')
    expect(state.entries.value[0].effective_kind).toBe('unknown')
    expect(state.entryErrors.value.failed).toBe('override rejected')
    expect(state.isOverriding('source-a', 'failed')).toBe(false)
    expect(toastStore.items.value.some((toast) => toast.tone === 'success')).toBe(false)
  })

  it('keeps format_error read-only for either original or effective kind in every rendered location', async () => {
    const diagnostics: SubscriptionEntryDTO[] = [
      { ...entry('original-error'), entry_kind: 'format_error', effective_kind: 'notice' },
      { ...entry('effective-error'), effective_kind: 'format_error' },
    ]
    vi.spyOn(api, 'get').mockResolvedValue({ items: diagnostics, total: 2 })
    const post = vi.spyOn(api, 'post')
    mount(() => h(SubscriptionEntriesDrawer, { modelValue: true, subscription: subscription() }))
    await flush()
    for (const diagnostic of diagnostics) {
      const locations = document.querySelectorAll(`[data-entry-id="${diagnostic.entry_id}"]`)
      expect(locations).toHaveLength(1)
      expect(locations[0].querySelector('button')).toBeNull()
    }
    const state = useSubscriptionEntries()
    await state.loadEntries('source-a')
    for (const diagnostic of diagnostics) await state.setOverride('source-a', diagnostic.entry_id, 'proxy')
    expect(post).not.toHaveBeenCalled()
  })

  it('renders at most 50 entries once each, with type/text filtering and the next page reachable', async () => {
    const items = Array.from({ length: 61 }, (_, index) => ({ ...entry(`node-${index}`), ordinal: index + 1 }))
    items[0] = { ...items[0], entry_kind: 'notice', effective_kind: 'notice' }
    vi.spyOn(api, 'get').mockResolvedValueOnce({ items, total: items.length })
    mount(() => h(SubscriptionEntriesDrawer, { modelValue: true, subscription: subscription() }))
    await flush()
    expect(document.querySelectorAll('[data-entry-id]')).toHaveLength(50)
    expect(document.querySelectorAll('[data-entry-id="node-0"]')).toHaveLength(1)
    document.querySelector<HTMLButtonElement>('[data-testid="entries-page-next"]')!.click()
    await flush()
    expect(document.querySelectorAll('[data-entry-id]')).toHaveLength(11)
    expect(document.body.textContent).toContain('node 60')
    const type = document.querySelector<HTMLSelectElement>('[data-testid="entries-kind-filter"]')!
    type.value = 'notice'
    type.dispatchEvent(new Event('change'))
    await flush()
    expect(document.querySelectorAll('[data-entry-id]')).toHaveLength(1)
    const search = document.querySelector<HTMLInputElement>('[data-testid="entries-search"]')!
    search.value = 'missing-name'
    search.dispatchEvent(new Event('input'))
    await flush()
    expect(document.querySelectorAll('[data-entry-id]')).toHaveLength(0)
    expect(document.body.textContent).toContain('没有匹配的条目')
  })

  it('switches visible source without showing the old source when mounted', async () => {
    const selected = ref(subscription('source-a'))
    const b = deferred<{ items: SubscriptionEntryDTO[]; total: number }>()
    vi.spyOn(api, 'get').mockResolvedValueOnce({ items: [entry('only-A')], total: 1 }).mockReturnValueOnce(b.promise)
    mount(() => h(SubscriptionEntriesDrawer, { modelValue: true, subscription: selected.value }))
    await flush()
    selected.value = subscription('source-b')
    await flush()
    expect(document.body.textContent).not.toContain('only A')
    b.resolve({ items: [entry('only-B', 'source-b')], total: 1 })
    await flush()
    expect(document.body.textContent).toContain('only B')
  })
})

describe('Subscription paged list and mutation results', () => {
  it('reaches page 2, uses full total and server search, and separates list GET from source POST', async () => {
    const get = vi.spyOn(api, 'get').mockImplementation(async (_path, options) => {
      const params = options?.params
      return params?.page === 2
        ? { items: [subscription('last-source')], total: 51, page: 2, page_size: 50 }
        : { items: [subscription('first-source')], total: params?.search_text ? 1 : 51, page: 1, page_size: 50 }
    })
    const post = vi.spyOn(api, 'post').mockResolvedValue(undefined)
    mount(() => h(SubscriptionsView))
    await flush()
    expect(document.querySelector('[data-testid="toolbar-sources-count"]')?.textContent).toContain('51')
    // A source not on the current page cannot be assumed to be the primary.
    expect(document.querySelector('[data-testid="toolbar-primary-badge"]')).toBeNull()
    document.querySelector<HTMLButtonElement>('[data-testid="subscription-page-next"]')!.click()
    await flush()
    expect(document.body.textContent).toContain('Provider last source')
    expect(get).toHaveBeenLastCalledWith('/api/v1/subscriptions', { params: { page: 2, page_size: 50, search_text: undefined } })
    document.querySelector<HTMLButtonElement>('[data-testid="subscription-list-reload"]')!.click()
    await flush()
    expect(post).not.toHaveBeenCalled()
    document.querySelector<HTMLButtonElement>('[data-testid="subscription-card-refresh-btn"]')!.click()
    await flush()
    expect(post).toHaveBeenCalledWith('/api/v1/subscriptions/last-source/refresh', undefined, expect.any(Object))
    const search = document.querySelector<HTMLInputElement>('[data-testid="subscription-search-input"]')!
    search.value = 'matched source'
    search.dispatchEvent(new Event('input'))
    document.querySelector<HTMLButtonElement>('[data-testid="subscription-search-submit"]')!.click()
    await flush()
    expect(get).toHaveBeenLastCalledWith('/api/v1/subscriptions', { params: { page: 1, page_size: 50, search_text: 'matched source' } })
    expect(document.querySelector('[data-testid="toolbar-sources-count"]')?.textContent).toContain('1')
  })

  it('returns save success with a reload warning, and rejects failed saves without success feedback', async () => {
    const draft: SubscriptionDraft = { name: 'draft', source_url_secret_ref: 'https://example.com', enabled: true, refresh_policy: defaultPolicy(), config: defaultConfig() }
    vi.spyOn(api, 'post').mockResolvedValueOnce(subscription()).mockRejectedValueOnce(new Error('save failed'))
    vi.spyOn(api, 'get').mockRejectedValue(new Error('list unavailable'))
    const state = useSubscriptions()
    expect(await state.save(draft)).toBe(true)
    expect(toastStore.items.value.some((toast) => toast.tone === 'warning' && toast.message.includes('刷新列表失败'))).toBe(true)
    toastStore.clear()
    await expect(state.save(draft)).rejects.toThrow('save failed')
    expect(state.saving.value).toBe(false)
    expect(toastStore.items.value.some((toast) => toast.tone === 'success')).toBe(false)
  })
})
