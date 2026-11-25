// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { createApp, h, nextTick } from 'vue'
import { createMemoryHistory, createRouter, RouterView, type Router } from 'vue-router'
import { api } from '../../api/client'
import { setLocale } from '../../locales'
import { toastStore } from '../../ui/toast'
import NodesView from './NodesView.vue'
import { parseSpeedMinimum, useNodes } from './useNodes'
import type { NodeRecord } from './nodeView'

vi.mock('../../locales/redesign', () => ({
  uiText: (zh: string, _en: string) => zh,
}))

vi.mock('../../ui/PageToolbar.vue', () => ({
  default: {
    name: 'PageToolbar',
    props: ['title', 'description'],
    template: `
      <div data-testid="page-toolbar">
        <slot name="default" />
        <slot name="actions" />
      </div>
    `,
  },
}))

const pageOneNodes: NodeRecord[] = Array.from({ length: 100 }, (_, index) => ({
  logical_id: `fixture-node-${index + 1}`,
  display_name: `Fixture Node ${String(index + 1).padStart(3, '0')}`,
  protocol: 'ss',
  active: true,
}))

const pageTwoNodes: NodeRecord[] = [
  {
    logical_id: 'fixture-node-101',
    display_name: 'Fixture Node 101',
    protocol: 'ss',
    active: true,
  },
  {
    logical_id: 'fixture-node-102',
    display_name: 'Fixture Node 102',
    protocol: 'ss',
    active: true,
  },
]

function nodesPage(items: NodeRecord[], page: number, total: number) {
  return { items, page, page_size: 100, total }
}

function deferred<T>() {
  let resolve!: (value: T) => void
  const promise = new Promise<T>((resolvePromise) => {
    resolve = resolvePromise
  })
  return { promise, resolve }
}

async function settleView() {
  await nextTick()
  await new Promise((resolve) => setTimeout(resolve, 0))
  await nextTick()
}

describe('speed minimum parsing', () => {
  it.each([
    ['1e-400', ''], ['1e-324', ''], [`0.${'0'.repeat(324)}1`, ''],
    ['0.00E-400', '0'], ['5e-324', '5e-324'], ['1.3e1', '13'],
  ])('parses %s without silently rounding a positive minimum to zero', (input, expected) => {
    expect(parseSpeedMinimum(input)).toBe(expected)
  })
})

describe('NodesView manual pagination', () => {
  let container: HTMLDivElement
  let app: ReturnType<typeof createApp> | null = null
  let router: Router

  beforeEach(async () => {
    vi.restoreAllMocks()
    if (typeof globalThis.ResizeObserver === 'undefined') {
      globalThis.ResizeObserver = class {
        observe() {}
        unobserve() {}
        disconnect() {}
      } as any
    }
    container = document.createElement('div')
    document.body.appendChild(container)
    localStorage.clear()
    sessionStorage.clear()
    toastStore.clear()
    setLocale('zh-CN')
    router = createRouter({
      history: createMemoryHistory(),
      routes: [{ path: '/nodes', component: NodesView }],
    })
    await router.push('/nodes')
    await router.isReady()
  })

  afterEach(() => {
    if (app) {
      app.unmount()
      app = null
    }
    container.remove()
    localStorage.clear()
    sessionStorage.clear()
    toastStore.clear()
    setLocale('zh-CN')
    vi.restoreAllMocks()
  })

  function mountView() {
    app = createApp({ render: () => h(RouterView) })
    app.use(router)
    app.mount(container)
  }

  it('shows a no-scroll load-more control and loads the remaining page only once on concurrent clicks', async () => {
    const pageTwo = deferred<ReturnType<typeof nodesPage>>()
    const pageCalls: Array<{ page: number; scope: unknown }> = []
    vi.spyOn(api, 'get').mockImplementation(async (path: string, options?: any) => {
      if (path === '/api/v1/nodes') {
        const params = options?.params ?? {}
        const page = Number(params.page)
        pageCalls.push({ page, scope: params.scope })
        if (page === 1) return nodesPage(pageOneNodes, 1, 102)
        if (page === 2) return pageTwo.promise
        throw new Error(`Unexpected fixture page ${page}`)
      }
      if (path === '/api/v1/probes/pool') {
        return { probing_node_ids: [], queued_node_ids: [] }
      }
      if (path === '/api/v1/probes/schedule') return { kinds: [] }
      return {}
    })

    mountView()
    await settleView()

    const badge = container.querySelector('[data-testid="nodes-total-badge"]')
    expect(badge?.textContent).toContain('100 / 102')
    const button = container.querySelector<HTMLButtonElement>('[data-testid="manual-load-more-btn"]')
    expect(button).not.toBeNull()
    expect(pageCalls).toEqual([{ page: 1, scope: 'enabled_subscriptions' }])

    // Dispatch twice before Vue can remove/replace the button; only one page request may start.
    button!.click()
    button!.click()
    await nextTick()
    expect(pageCalls.filter(({ page }) => page === 2)).toHaveLength(1)

    pageTwo.resolve(nodesPage(pageTwoNodes, 2, 102))
    await settleView()

    expect(pageCalls.filter(({ page }) => page === 2)).toHaveLength(1)
    expect(container.querySelector('[data-testid="nodes-total-badge"]')?.textContent).toContain('102')
    expect(container.querySelector('[data-testid="manual-load-more-btn"]')).toBeNull()
  })

  it('retains the first page and visible error after a failed load, then retries successfully', async () => {
    let pageTwoAttempts = 0
    const pageCalls: number[] = []
    vi.spyOn(api, 'get').mockImplementation(async (path: string, options?: any) => {
      if (path === '/api/v1/nodes') {
        const page = Number(options?.params?.page)
        pageCalls.push(page)
        if (page === 1) return nodesPage(pageOneNodes, 1, 102)
        if (page === 2 && pageTwoAttempts++ === 0) throw new Error('Fixture page two failed')
        if (page === 2) return nodesPage(pageTwoNodes, 2, 102)
        throw new Error(`Unexpected fixture page ${page}`)
      }
      if (path === '/api/v1/probes/pool') {
        return { probing_node_ids: [], queued_node_ids: [] }
      }
      if (path === '/api/v1/probes/schedule') return { kinds: [] }
      return {}
    })

    mountView()
    await settleView()

    container.querySelector<HTMLButtonElement>('[data-testid="manual-load-more-btn"]')!.click()
    await settleView()

    expect(container.querySelector('[data-testid="error-state-card"]')?.textContent).toContain('Fixture page two failed')
    expect(container.querySelector('[data-testid="manual-load-more-btn"]')).not.toBeNull()
    expect(container.querySelector('[data-testid="nodes-total-badge"]')?.textContent).toContain('100 / 102')
    expect(pageCalls).toEqual([1, 2])
    window.dispatchEvent(new Event('scroll'))
    await settleView()
    expect(pageCalls).toEqual([1, 2])

    container.querySelector<HTMLButtonElement>('[data-testid="manual-load-more-btn"]')!.click()
    await settleView()

    expect(pageCalls).toEqual([1, 2, 2])
    expect(container.querySelector('[data-testid="nodes-total-badge"]')?.textContent).toContain('102')
    expect(container.querySelector('[data-testid="manual-load-more-btn"]')).toBeNull()
  })

  it('distinguishes empty inventory from zero matches and resets all filters in place', async () => {
    const requests: Record<string, unknown>[] = []
    vi.spyOn(api, 'get').mockImplementation(async (path: string, options?: any) => {
      if (path === '/api/v1/nodes') {
        requests.push({ ...options?.params })
        return nodesPage([], 1, 0)
      }
      if (path === '/api/v1/probes/pool') return { probing_node_ids: [], queued_node_ids: [] }
      if (path === '/api/v1/probes/schedule') return { kinds: [] }
      return {}
    })

    mountView()
    await settleView()
    const emptyState = () => container.querySelector('[data-testid="empty-state"]')!
    expect(emptyState().textContent).not.toContain('没有匹配当前条件的节点')
    expect(emptyState().querySelector('button')).toBeNull()

    const search = container.querySelector<HTMLInputElement>('[data-testid="node-search-input"]')!
    expect(search.closest('label')?.textContent).toContain('搜索节点')
    search.value = 'fixture missing'
    search.dispatchEvent(new Event('input', { bubbles: true }))
    await nextTick()
    expect(emptyState().textContent).not.toContain('没有匹配当前条件的节点')
    expect(requests).toHaveLength(1)
    search.dispatchEvent(new KeyboardEvent('keydown', { key: 'Enter', bubbles: true }))
    await settleView()

    const protocolButton = Array.from(container.querySelectorAll<HTMLButtonElement>('[data-testid="node-protocol-filter"] button'))
      .find((button) => button.textContent?.trim().toUpperCase() === 'SS')!
    protocolButton.click()
    await settleView()
    expect(protocolButton.getAttribute('aria-pressed')).toBe('true')

    const healthButton = container.querySelector<HTMLButtonElement>('[data-testid="node-health-pill-healthy"]')!
    healthButton.click()
    await settleView()
    expect(healthButton.getAttribute('aria-pressed')).toBe('true')

    const risk = container.querySelector<HTMLSelectElement>('[data-testid="node-risk-band-filter"]')!
    risk.value = 'low'
    risk.dispatchEvent(new Event('change', { bubbles: true }))
    await settleView()
    const score = container.querySelector<HTMLInputElement>('[data-testid="node-risk-score-max"]')!
    score.value = '20'
    score.dispatchEvent(new Event('change', { bubbles: true }))
    await settleView()
    const speed = container.querySelector<HTMLInputElement>('[data-testid="node-speed-min"]')!
    speed.value = '12.5'
    speed.dispatchEvent(new Event('change', { bubbles: true }))
    await settleView()
    expect(requests.at(-1)).toMatchObject({ speed_min_kib_s: '12.5', page: 1 })
    const appliedRequests = requests.length
    speed.value = '1e-400'
    speed.dispatchEvent(new Event('change', { bubbles: true }))
    await settleView()
    expect(speed.value).toBe('12.5')
    expect(requests).toHaveLength(appliedRequests)

    expect(emptyState().textContent).toContain('没有匹配当前条件的节点')
    const summary = container.querySelector('[data-testid="node-empty-filter-summary"]')?.textContent
    expect(summary).toContain('fixture missing')
    expect(summary).toContain('SS')
    expect(summary).toContain('正常')
    expect(summary).toContain('20')
    expect(summary).toContain('12.5 KiB/s')
    expect(requests.at(-1)).toMatchObject({ protocol: 'ss', health_status: 'healthy', ip_risk_band: 'low', ip_risk_score_max: '20', search: 'fixture missing' })

    emptyState().querySelector<HTMLButtonElement>('button')!.click()
    await settleView()
    expect(search.value).toBe('')
    expect(risk.value).toBe('all')
    expect(score.value).toBe('')
    expect(speed.value).toBe('')
    expect(emptyState().textContent).not.toContain('没有匹配当前条件的节点')
    expect(requests.at(-1)).toMatchObject({ page: 1, scope: 'enabled_subscriptions' })
    for (const key of ['protocol', 'health_status', 'ip_risk_band', 'ip_risk_score_max', 'speed_min_kib_s', 'search']) {
      expect(requests.at(-1)).not.toHaveProperty(key)
    }
  })

  it('removes each applied filter chip independently, including zero thresholds, without starting a probe', async () => {
    const requests: Record<string, unknown>[] = []
    const post = vi.spyOn(api, 'post').mockRejectedValue(new Error('Unexpected probe request in a filter-only fixture'))
    vi.spyOn(api, 'get').mockImplementation(async (path: string, options?: any) => {
      if (path === '/api/v1/nodes') {
        requests.push({ ...options?.params })
        return nodesPage([], 1, 0)
      }
      if (path === '/api/v1/probes/pool') return { probing_node_ids: [], queued_node_ids: [] }
      return {}
    })
    mountView()
    await settleView()
    const toggle = container.querySelector<HTMLButtonElement>('[data-testid="node-advanced-filters-toggle"]')!
    expect(toggle.getAttribute('aria-expanded')).toBe('false')
    toggle.click()
    await nextTick()
    expect(toggle.getAttribute('aria-expanded')).toBe('true')
    const protocol = Array.from(container.querySelectorAll<HTMLButtonElement>('[data-testid="node-protocol-filter"] button')).find((button) => button.textContent?.trim().toUpperCase() === 'SS')!
    protocol.click()
    container.querySelector<HTMLButtonElement>('[data-testid="node-health-pill-healthy"]')!.click()
    for (const [id, value] of [['node-risk-band-filter', 'low'], ['node-risk-score-max', '0'], ['node-speed-min', '0']]) {
      const input = container.querySelector<HTMLInputElement | HTMLSelectElement>(`[data-testid="${id}"]`)!
      input.value = value!
      input.dispatchEvent(new Event('change', { bubbles: true }))
      await settleView()
    }
    const search = container.querySelector<HTMLInputElement>('[data-testid="node-search-input"]')!
    search.value = 'fixture missing'
    search.dispatchEvent(new Event('input', { bubbles: true }))
    await nextTick()
    search.dispatchEvent(new KeyboardEvent('keydown', { key: 'Enter', bubbles: true }))
    await settleView()
    const expected: Record<string, unknown> = { protocol: 'ss', health_status: 'healthy', ip_risk_band: 'low', ip_risk_score_max: '0', speed_min_kib_s: '0', search: 'fixture missing' }
    expect(requests.at(-1)).toMatchObject(expected)
    const chipButtons = () => Array.from(container.querySelectorAll<HTMLButtonElement>('[data-testid="node-active-filter-chips"] button[aria-label]'))
    expect(chipButtons()).toHaveLength(6)
    // Applied advanced filters remain visible as chips when the advanced controls are collapsed.
    toggle.click()
    await nextTick()
    for (const [label, key] of [['风险分数', 'ip_risk_score_max'], ['≥ 0 KiB/s', 'speed_min_kib_s'], ['搜索', 'search'], ['IP 风险', 'ip_risk_band'], ['状态', 'health_status'], ['协议', 'protocol']]) {
      const chip = chipButtons().find((button) => button.textContent?.includes(label!))!
      chip.click()
      await settleView()
      delete expected[key!]
      expect(requests.at(-1)).toMatchObject({ ...expected, page: 1, scope: 'enabled_subscriptions' })
      expect(requests.at(-1)).not.toHaveProperty(key!)
      expect(chipButtons().some((button) => button.textContent?.includes(label!))).toBe(false)
    }
    expect(chipButtons()).toHaveLength(0)
    expect(search.value).toBe('')
    expect(post).not.toHaveBeenCalled()
  })

  it('does not register polling or listeners if the page unmounts before its first list request completes', async () => {
    const page = deferred<ReturnType<typeof nodesPage>>()
    vi.spyOn(api, 'get').mockImplementation(async (path: string) => {
      if (path === '/api/v1/nodes') return page.promise
      if (path === '/api/v1/probes/pool') return { probing_node_ids: ['fixture-node-1'], queued_node_ids: [] }
      return {}
    })
    const intervals = vi.spyOn(globalThis, 'setInterval')
    const listeners = vi.spyOn(document, 'addEventListener')
    const initialIntervalCount = intervals.mock.calls.length
    const initialVisibilityListenerCount = listeners.mock.calls.filter(([event]) => event === 'visibilitychange').length
    mountView()
    await nextTick()
    expect(intervals).toHaveBeenCalledTimes(initialIntervalCount)
    expect(listeners.mock.calls.filter(([event]) => event === 'visibilitychange')).toHaveLength(initialVisibilityListenerCount)
    app!.unmount()
    app = null
    page.resolve(nodesPage(pageOneNodes.slice(0, 1), 1, 1))
    await settleView()
    expect(intervals).toHaveBeenCalledTimes(initialIntervalCount)
    expect(listeners.mock.calls.filter(([event]) => event === 'visibilitychange')).toHaveLength(initialVisibilityListenerCount)
  })

  it('keeps a cleared filter on page one when an older speed-filtered page two arrives', async () => {
    const oldPage = deferred<ReturnType<typeof nodesPage>>()
    const calls: Array<Record<string, unknown>> = []
    vi.spyOn(api, 'get').mockImplementation(async (path: string, options?: any) => {
      if (path === '/api/v1/nodes') {
        const params = options?.params ?? {}
        calls.push(params)
        if (params.page === 2) return oldPage.promise
        return params.speed_min_kib_s === '10' ? nodesPage(pageOneNodes, 1, 102) : nodesPage(pageTwoNodes, 1, 2)
      }
      if (path === '/api/v1/probes/pool') return { probing_node_ids: [], queued_node_ids: [] }
      return {}
    })
    const state = useNodes()
    state.speedMinKiBS.value = '10'
    await state.load()
    const loadingOldPage = state.loadMore()
    expect(calls.at(-1)).toMatchObject({ speed_min_kib_s: '10', page: 2 })
    state.speedMinKiBS.value = ''
    await state.load()
    expect(calls.at(-1)).toMatchObject({ page: 1 })
    expect(calls.at(-1)).not.toHaveProperty('speed_min_kib_s')
    oldPage.resolve(nodesPage(pageTwoNodes, 2, 102))
    await loadingOldPage
    expect(state.hasMore.value).toBe(false)
    expect(state.total.value).toBe(2)
    expect(state.items.value.map((node) => node.logicalId)).toEqual(['fixture-node-101', 'fixture-node-102'])
  })

  it('does not report empty inventory when the initial request fails', async () => {
    vi.spyOn(api, 'get').mockImplementation(async (path: string) => {
      if (path === '/api/v1/nodes') throw new Error('Fixture list unavailable')
      if (path === '/api/v1/probes/pool') return { probing_node_ids: [], queued_node_ids: [] }
      if (path === '/api/v1/probes/schedule') return { kinds: [] }
      return {}
    })
    mountView()
    await settleView()
    expect(container.querySelector('[data-testid="error-state-card"]')?.textContent).toContain('Fixture list unavailable')
    expect(container.querySelector('[data-testid="empty-state"]')).toBeNull()
  })

  it('hides the load-more control when the total fits in the first page', async () => {
    vi.spyOn(api, 'get').mockImplementation(async (path: string, options?: any) => {
      if (path === '/api/v1/nodes') {
        expect(Number(options?.params?.page)).toBe(1)
        return nodesPage(pageOneNodes, 1, 100)
      }
      if (path === '/api/v1/probes/pool') {
        return { probing_node_ids: [], queued_node_ids: [] }
      }
      if (path === '/api/v1/probes/schedule') return { kinds: [] }
      return {}
    })

    mountView()
    await settleView()

    expect(container.querySelector('[data-testid="nodes-total-badge"]')?.textContent).toContain('100')
    expect(container.querySelector('[data-testid="manual-load-more-btn"]')).toBeNull()
  })
})
