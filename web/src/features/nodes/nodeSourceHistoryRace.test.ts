// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { setImmediate } from 'node:timers'
import { createApp, h, nextTick, ref } from 'vue'
import { createMemoryHistory, createRouter } from 'vue-router'
import { api } from '../../api/client'
import NodesView from './NodesView.vue'
import NodeSourceHistoryPanel from './NodeSourceHistoryPanel.vue'
import type { NodeSourceHistoryData } from './sourceHistoryTypes'

vi.mock('../../locales/redesign', () => ({
  uiText: (zh: string, _en: string) => zh,
}))

vi.mock('../../ui/PageToolbar.vue', () => ({
  default: {
    name: 'PageToolbar',
    template: '<div data-testid="page-toolbar"><slot/><slot name="actions"/></div>',
  },
}))

let app: ReturnType<typeof createApp> | null = null
beforeEach(() => {
  vi.stubGlobal('ResizeObserver', class {
    observe() {}
    unobserve() {}
    disconnect() {}
  })
})
afterEach(() => {
  app?.unmount()
  app = null
  vi.restoreAllMocks()
  vi.unstubAllGlobals()
  document.body.innerHTML = ''
})

function deferred<T>() {
  let resolve!: (value: T) => void
  const promise = new Promise<T>((done) => { resolve = done })
  return { promise, resolve }
}

const node = (logical_id: string) => ({ logical_id, display_name: logical_id, protocol: 'ss', active: true })
const history = (source_label: string): NodeSourceHistoryData => ({
  attribution_status: 'historical_verified',
  current_sources: [],
  history: [{ source_label, relation_state: 'verified', cause: 'legacy_import', evidence_kind: 'fixture', source_deleted: false }],
})

describe('node source history request ownership', () => {
  it('discards stale history across A-B-A drawer selection and displays only the newest A response', async () => {
    const sourceCalls: string[] = []
    const responses = [deferred<NodeSourceHistoryData>(), deferred<NodeSourceHistoryData>(), deferred<NodeSourceHistoryData>()]
    const requested = [deferred<void>(), deferred<void>(), deferred<void>()]
    vi.spyOn(api, 'get').mockImplementation(async (path: string) => {
      if (path === '/api/v1/nodes') return { items: [node('node-a'), node('node-b')], page: 1, page_size: 100, total: 2 }
      if (path === '/api/v1/probes/pool') return { probing_node_ids: [], queued_node_ids: [] }
      if (path === '/api/v1/probes/schedule') return { kinds: [] }
      if (path === '/api/v1/nodes/node-a' || path === '/api/v1/nodes/node-b') return { node: node(path.endsWith('node-a') ? 'node-a' : 'node-b') }
      const match = /^\/api\/v1\/nodes\/(node-a|node-b)\/source-history$/.exec(path)
      if (match) {
        sourceCalls.push(match[1]!)
        requested[sourceCalls.length - 1]!.resolve()
        return responses[sourceCalls.length - 1]!.promise
      }
      return {}
    })

    const router = createRouter({ history: createMemoryHistory(), routes: [{ path: '/nodes', component: NodesView }] })
    await router.push('/nodes?node=node-a')
    const host = document.createElement('div')
    document.body.appendChild(host)
    app = createApp({ render: () => h(NodesView) })
    app.use(router)
    app.mount(host)
    await router.isReady()
    await requested[0]!.promise
    expect(sourceCalls).toEqual(['node-a'])

    await router.push('/nodes?node=node-b')
    await requested[1]!.promise
    expect(sourceCalls).toEqual(['node-a', 'node-b'])
    await router.push('/nodes?node=node-a')
    await requested[2]!.promise
    expect(sourceCalls).toEqual(['node-a', 'node-b', 'node-a'])

    responses[2]!.resolve(history('Newest-A-history'))
    responses[0]!.resolve(history('Stale-A-history'))
    responses[1]!.resolve(history('Stale-B-history'))
    await new Promise<void>((resolve) => setImmediate(resolve))
    await nextTick()

    const drawer = document.body.querySelector('[data-testid="node-detail-drawer"]')
    expect(drawer?.textContent).toContain('Newest-A-history')
    expect(drawer?.textContent).not.toContain('Stale-A-history')
    expect(drawer?.textContent).not.toContain('Stale-B-history')
    app.unmount()
    app = null
    host.remove()
  })

  it('bounds current, verified and conflict records independently and resets history pages when searching or changing entity', async () => {
    const logicalId = ref('node-a')
    const data: NodeSourceHistoryData = {
      attribution_status: 'conflict',
      current_sources: Array.from({ length: 25 }, (_, index) => ({ subscription_id: `sub-${index}`, name: `Current-${index}`, enabled: index % 2 === 0 })),
      history: (['verified', 'conflict'] as const).flatMap((relation_state) => Array.from({ length: 25 }, (_, index): NodeSourceHistoryData['history'][number] => ({ source_label: `${relation_state}-Source-${index}`, relation_state, cause: 'legacy_import', evidence_kind: 'fixture', source_deleted: false }))),
    }
    const host = document.createElement('div')
    document.body.appendChild(host)
    app = createApp({ render: () => h(NodeSourceHistoryPanel, { logicalId: logicalId.value, sourceHistory: data, loading: false }) })
    app.mount(host)
    await nextTick()
    const section = (id: string) => host.querySelector<HTMLElement>(`[data-testid="${id}"]`)!
    const verified = () => host.querySelectorAll('[data-testid="verified-history-item"]')
    const conflicts = () => host.querySelectorAll('[data-testid="conflict-history-item"]')
    expect(host.querySelectorAll('[data-testid="current-source-item"]')).toHaveLength(20)
    expect(verified()).toHaveLength(20)
    expect(conflicts()).toHaveLength(20)
    section('verified-history-section').querySelectorAll<HTMLButtonElement>('nav button')[1]!.click()
    await nextTick()
    expect(verified()).toHaveLength(5)
    expect(verified()[0]?.textContent).toContain('verified-Source-20')
    expect(conflicts()).toHaveLength(20)
    section('conflict-history-section').querySelectorAll<HTMLButtonElement>('nav button')[1]!.click()
    await nextTick()
    expect(conflicts()).toHaveLength(5)
    const query = host.querySelector<HTMLInputElement>('[data-testid="source-history-search"]')!
    query.value = 'Source-24'
    query.dispatchEvent(new Event('input', { bubbles: true }))
    await nextTick()
    expect(verified()).toHaveLength(1)
    expect(conflicts()).toHaveLength(1)
    expect(section('verified-history-section').querySelector('nav')).toBeNull()
    expect(host.querySelectorAll('[data-testid="current-source-item"]')).toHaveLength(20)
    query.value = ''
    query.dispatchEvent(new Event('input', { bubbles: true }))
    await nextTick()
    expect(verified()).toHaveLength(20)
    section('verified-history-section').querySelectorAll<HTMLButtonElement>('nav button')[1]!.click()
    await nextTick()
    logicalId.value = 'node-b'
    await nextTick()
    expect(verified()).toHaveLength(20)
    expect(verified()[0]?.textContent).toContain('verified-Source-0')
    expect(host.querySelector('[data-testid="source-history-disclaimer"]')?.textContent).toContain('不代表当前订阅成员或启用状态')
  })
})
