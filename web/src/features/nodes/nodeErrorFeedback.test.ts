// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { createApp, h, nextTick, ref } from 'vue'
import { api, ApiError } from '../../api/client'
import { setLocale } from '../../locales'
import { toastStore } from '../../ui/toast'
import NodesView from './NodesView.vue'
import NodeDetailDrawer from './NodeDetailDrawer.vue'
import { useNodes } from './useNodes'
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

describe('FR-RM3 Node Detail Error Differentiation & Stale Feedback', () => {
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

  it('differentiates 404, 500, and network error when node is not in cache', async () => {
    const nodes = useNodes()

    // 1. 404 Not Found
    vi.spyOn(api, 'get').mockRejectedValueOnce(
      new ApiError(404, 'not_found', 'Node not found')
    )
    const res404 = await nodes.fetchNodeDetail('node-missing')
    expect(res404).toBeNull()
    expect(nodes.nodeDetailError.value).toMatchObject({
      logicalId: 'node-missing',
      status: 404,
      isStale: false,
    })
    expect(nodes.nodeDetailError.value?.message).toContain('未找到节点「node-missing」(404)')

    // 2. 500 Server Error
    vi.spyOn(api, 'get').mockRejectedValueOnce(
      new ApiError(500, 'internal_error', 'Internal server error')
    )
    const res500 = await nodes.fetchNodeDetail('node-broken')
    expect(res500).toBeNull()
    expect(nodes.nodeDetailError.value).toMatchObject({
      logicalId: 'node-broken',
      status: 500,
      isStale: false,
    })
    expect(nodes.nodeDetailError.value?.message).toContain('获取节点详情失败 (500)')

    // 3. Network error (TypeError / fetch failed)
    vi.spyOn(api, 'get').mockRejectedValueOnce(
      new TypeError('Failed to fetch')
    )
    const resNet = await nodes.fetchNodeDetail('node-net-fail')
    expect(resNet).toBeNull()
    expect(nodes.nodeDetailError.value).toMatchObject({
      logicalId: 'node-net-fail',
      status: 0,
      isStale: false,
    })
    expect(nodes.nodeDetailError.value?.message).toContain('Failed to fetch')
  })

  it('preserves cached node data and sets stale state with retry when detail fetch fails with 500', async () => {
    const cachedNode: NodeRecord = {
      logical_id: 'node-cached-1',
      display_name: 'Cached Japan Node',
      protocol: 'vmess',
      active: true,
      server: 'jp.example.com',
      port: 443,
    }

    vi.spyOn(api, 'get').mockImplementation(async (path: string) => {
      if (path === '/api/v1/nodes') {
        return { items: [cachedNode], page: 1, page_size: 100, total: 1 }
      }
      if (path === '/api/v1/probes/pool') {
        return { probing_node_ids: [], queued_node_ids: [] }
      }
      if (path === '/api/v1/nodes/node-cached-1') {
        throw new ApiError(500, 'server_error', 'Database connection timeout')
      }
      return {}
    })

    const nodes = useNodes()
    await nodes.load()
    expect(nodes.items.value).toHaveLength(1)

    // Request detail for cached node
    const result = await nodes.fetchNodeDetail('node-cached-1')
    expect(result).not.toBeNull()
    expect(result?.logicalId).toBe('node-cached-1')
    expect(result?.displayName).toBe('Cached Japan Node')

    // Stale indicators
    expect(nodes.isDetailStale.value).toBe(true)
    expect(nodes.nodeDetailError.value).toMatchObject({
      logicalId: 'node-cached-1',
      status: 500,
      isStale: true,
    })
    expect(nodes.detailErrorMessage.value).toContain('获取节点详情失败 (500)')

    // Retry succeeds
    vi.spyOn(api, 'get').mockImplementation(async (path: string) => {
      if (path === '/api/v1/nodes/node-cached-1') {
        return {
          node: {
            ...cachedNode,
            display_name: 'Fresh Japan Node (Updated)',
          },
        }
      }
      return {}
    })

    const retried = await nodes.fetchNodeDetail('node-cached-1')
    expect(retried?.displayName).toBe('Fresh Japan Node (Updated)')
    expect(nodes.isDetailStale.value).toBe(false)
    expect(nodes.nodeDetailError.value).toBeNull()
  })

  it('race condition: outdated request failure does not pollute newly selected node', async () => {
    let resolveNodeA!: (err: any) => void
    const nodeAPromise = new Promise((_, reject) => {
      resolveNodeA = reject
    })

    const nodeBRecord: NodeRecord = {
      logical_id: 'node-b',
      display_name: 'Node B',
      protocol: 'ss',
      active: true,
      server: 'b.example.com',
      port: 8388,
    }

    vi.spyOn(api, 'get').mockImplementation(async (path: string) => {
      if (path === '/api/v1/nodes/node-a') {
        return await nodeAPromise
      }
      if (path === '/api/v1/nodes/node-b') {
        return { node: nodeBRecord }
      }
      return {}
    })

    const nodes = useNodes()

    // Start fetch for Node A (slow / pending)
    const promiseA = nodes.fetchNodeDetail('node-a')

    // User quickly switches to Node B
    const promiseB = nodes.fetchNodeDetail('node-b')
    await promiseB

    expect(nodes.selectedNode.value?.logicalId).toBe('node-b')
    expect(nodes.nodeDetailError.value).toBeNull()

    // Now Node A fails with 500
    resolveNodeA(new ApiError(500, 'internal_error', 'Node A exploded'))
    await promiseA

    // Selected node and error MUST NOT be contaminated by Node A
    expect(nodes.selectedNode.value?.logicalId).toBe('node-b')
    expect(nodes.nodeDetailError.value).toBeNull()
    expect(nodes.isDetailStale.value).toBe(false)
  })

  it('prevents unhandled promise rejection in probeSingleNode when refresh fails', async () => {
    vi.spyOn(api, 'post').mockResolvedValueOnce({
      run_id: 'run-probe-fail',
      state: 'queued',
    })
    // Detail refresh fails with 500
    vi.spyOn(api, 'get').mockImplementation(async (path: string) => {
      if (path.includes('/api/v1/nodes/node-probe-1')) {
        throw new ApiError(500, 'server_error', 'Detail service unavailable')
      }
      return {}
    })

    const nodes = useNodes()
    // Should not throw unhandled rejection
    const probeRes = await nodes.probeSingleNode('node-probe-1')
    expect(probeRes.ok).toBe(true)
    expect(probeRes.runId).toBe('run-probe-fail')
    expect(nodes.probingNodeId.value).toBeNull()
  })

  it('renders stale banner with retry button in NodeDetailDrawer when detailStale is true', async () => {
    const mockNode = {
      logicalId: 'node-test-1',
      displayName: 'Test Node',
      protocol: 'wireguard' as const,
      active: true,
      healthStatus: 'healthy' as const,
      underlyingHealthStatus: 'healthy',
      probeState: 'idle' as const,
      connection: {
        server: '1.2.3.4',
        port: 51820,
        localAddress: ['10.0.0.2/32'],
        publicKey: 'pubkey',
        privateKey: '',
        preSharedKey: '',
        dns: [],
        reserved: [],
        uuid: '',
        password: '',
        method: '',
        congestionControl: '',
        udpRelayMode: '',
        alpn: [],
        sni: '',
        disableSni: false,
      },
    }

    let retriedId = ''
    app = createApp({
      render() {
        return h(NodeDetailDrawer, {
          modelValue: true,
          node: mockNode as any,
          sourceHistory: null,
          loadingSourceHistory: false,
          sourceHistoryError: '',
          probeState: 'idle',
          probeFeedback: '',
          savingConnection: false,
          loadingDetail: false,
          detailStale: true,
          detailErrorMessage: '获取节点详情失败 (500)',
          onRetryDetail: (id: string) => {
            retriedId = id
          },
        })
      },
    })
    app.mount(container)
    await nextTick()

    const staleBanner = container.querySelector('[data-testid="node-detail-stale-banner"]')
    expect(staleBanner).not.toBeNull()
    expect(staleBanner?.textContent).toContain('获取节点详情失败 (500)')

    const retryBtn = container.querySelector('[data-testid="node-detail-retry-btn"]') as HTMLButtonElement
    expect(retryBtn).not.toBeNull()
    retryBtn.click()
    await nextTick()

    expect(retriedId).toBe('node-test-1')
  })
})
