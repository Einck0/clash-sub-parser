// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { createApp, h, nextTick } from 'vue'
import { api } from '../../api/client'
import { setLocale } from '../../locales'
import { toastStore } from '../../ui/toast'
import NodesView from './NodesView.vue'
import type { NodeRecord } from './nodeView'

vi.mock('../../locales/redesign', () => ({
  uiText: (zh: string, _en: string) => zh,
}))

vi.mock('vue-router', () => ({
  useRoute: () => ({ query: {} }),
  useRouter: () => ({ push: vi.fn(), replace: vi.fn() }),
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

describe('NodesView manual pagination', () => {
  let container: HTMLDivElement
  let app: ReturnType<typeof createApp> | null = null

  beforeEach(() => {
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
    app = createApp({
      render() {
        return h(NodesView)
      },
    })
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

    expect(emptyState().textContent).toContain('没有匹配当前条件的节点')
    const summary = container.querySelector('[data-testid="node-empty-filter-summary"]')?.textContent
    expect(summary).toContain('fixture missing')
    expect(summary).toContain('SS')
    expect(summary).toContain('正常')
    expect(summary).toContain('20')
    expect(requests.at(-1)).toMatchObject({ protocol: 'ss', health_status: 'healthy', ip_risk_band: 'low', ip_risk_score_max: '20', search: 'fixture missing' })

    emptyState().querySelector<HTMLButtonElement>('button')!.click()
    await settleView()
    expect(search.value).toBe('')
    expect(risk.value).toBe('all')
    expect(score.value).toBe('')
    expect(emptyState().textContent).not.toContain('没有匹配当前条件的节点')
    expect(requests.at(-1)).toMatchObject({ page: 1, scope: 'enabled_subscriptions' })
    for (const key of ['protocol', 'health_status', 'ip_risk_band', 'ip_risk_score_max', 'search']) {
      expect(requests.at(-1)).not.toHaveProperty(key)
    }
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
