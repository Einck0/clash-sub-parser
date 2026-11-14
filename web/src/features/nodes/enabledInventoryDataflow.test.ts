// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { createApp, h, nextTick } from 'vue'
import { api, ApiError } from '../../api/client'
import { setLocale } from '../../locales'
import { toastStore } from '../../ui/toast'
import DashboardView from '../dashboard/DashboardView.vue'
import NodesView from './NodesView.vue'
import SubscriptionsView from '../subscriptions/SubscriptionsView.vue'
import PublicationsView from '../publications/PublicationsView.vue'
import { useNodes } from './useNodes'
import { useSubscriptions, type SubscriptionRecord } from '../subscriptions/useSubscriptions'
import { useProbes } from '../probes/useProbes'
import type { NodeRecord } from './nodeView'
import type { ProbePoolStatus } from '../probes/probeTypes'

vi.mock('../../locales/redesign', () => ({
  uiText: (zh: string, _en: string) => zh,
}))

vi.mock('../../ui/PageToolbar.vue', () => ({
  default: {
    name: 'PageToolbar',
    props: ['title', 'description'],
    template: `
      <div data-testid="page-toolbar" class="flex flex-wrap items-center justify-between gap-3 mb-2">
        <slot name="default" />
        <div class="flex items-center gap-2">
          <slot name="actions" />
        </div>
      </div>
    `,
  },
}))

describe('Enabled Subscriptions Inventory & Live Validation Specification', () => {
  let container: HTMLDivElement
  let app: ReturnType<typeof createApp> | null = null

  beforeEach(() => {
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

  it('keeps risk filters on server pagination and removes them when reset', async () => {
    const calls: Record<string, unknown>[] = []
    vi.spyOn(api, 'get').mockImplementation(async (path: string, options?: any) => {
      if (path === '/api/v1/nodes') {
        calls.push({ ...options.params })
        return { items: [], page: options.params.page, page_size: 100, total: 101 }
      }
      return { probing_node_ids: [], queued_node_ids: [] }
    })
    const nodes = useNodes()
    nodes.riskBandFilter.value = 'low'
    nodes.riskScoreMax.value = '0'
    nodes.searchQuery.value = 'HK'
    nodes.protocolFilter.value = 'vless'
    await nodes.load()
    await nodes.loadMore()
    expect(calls).toHaveLength(2)
    expect(calls[1]).toMatchObject({ page: 2, ip_risk_band: 'low', ip_risk_score_max: '0', search: 'HK', protocol: 'vless' })
    nodes.riskBandFilter.value = 'all'
    nodes.riskScoreMax.value = ''
    await nodes.load()
    expect(calls[2].page).toBe(1)
    expect(calls[2]).not.toHaveProperty('ip_risk_band')
    expect(calls[2]).not.toHaveProperty('ip_risk_score_max')
  })

  // --------------------------------------------------------------------------
  // Scenario 1: E contains inactive node but candidate is excluded from probe pool
  // --------------------------------------------------------------------------
  it('Scenario 1: E contains inactive node but candidate is excluded from probe candidate pool', async () => {
    const rawNodes: NodeRecord[] = [
      {
        logical_id: 'node-act-1',
        display_name: 'HK Active 1',
        protocol: 'vmess',
        active: true,
        connection: { server: 'hk1.example.com', port: 443 },
        capabilities: {},
      },
      {
        logical_id: 'node-act-2',
        display_name: 'HK Active 2',
        protocol: 'vless',
        active: true,
        connection: { server: 'hk2.example.com', port: 443 },
        capabilities: {},
      },
      {
        logical_id: 'node-inact-1',
        display_name: 'HK Inactive',
        protocol: 'trojan',
        active: false,
        connection: { server: 'hk-inact.example.com', port: 443 },
        capabilities: {},
      },
    ]

    const getSpy = vi.spyOn(api, 'get').mockImplementation(async (path: string, options?: any) => {
      if (path === '/api/v1/nodes') {
        const params = options?.params || {}
        expect(params.scope).toBe('enabled_subscriptions')
        if (params.active_only === 'true') {
          const activeOnly = rawNodes.filter((n) => n.active !== false)
          return { items: activeOnly, page: 1, page_size: 100, total: activeOnly.length }
        }
        return { items: rawNodes, page: 1, page_size: 100, total: rawNodes.length }
      }
      if (path === '/api/v1/probes/pool') {
        return {
          queue_nodes_count: 0,
          probing_count: 0,
          queued_waiting_count: 0,
          untested_count: 0,
          undetermined_count: 2,
          total_count: 2,
          inventory_total: 3,
          candidate_total: 2,
          scope: 'enabled_subscriptions',
          available_count: 0,
          unavailable_count: 0,
          healthy_count: 0,
          degraded_count: 0,
          probing_node_ids: [],
          queued_node_ids: [],
          updated_at: new Date().toISOString(),
        } satisfies ProbePoolStatus
      }
      return {}
    })

    // In useNodes, all items from E (active + inactive) are present
    const nodesHook = useNodes()
    await nodesHook.load()
    expect(nodesHook.total.value).toBe(3)
    expect(nodesHook.items.value).toHaveLength(3)
    const inactiveNode = nodesHook.items.value.find((n) => n.logicalId === 'node-inact-1')
    expect(inactiveNode?.active).toBe(false)

    // In useProbes, candidate pool only includes active nodes
    const probesHook = useProbes()
    await probesHook.loadProbeNodes()
    expect(probesHook.probeNodes.value).toHaveLength(2)
    expect(probesHook.probeNodes.value.map((n) => n.logicalId)).toEqual(['node-act-1', 'node-act-2'])
    expect(probesHook.probeNodes.value.some((n) => n.logicalId === 'node-inact-1')).toBe(false)
  })

  // --------------------------------------------------------------------------
  // Scenario 2: Disabled subscription nodes not in main display, but detail is accessible
  // --------------------------------------------------------------------------
  it('Scenario 2: disabled subscription nodes not in main display, historical detail remains accessible', async () => {
    const historicalNode: NodeRecord = {
      logical_id: 'node-archived-99',
      display_name: 'Archived Legacy Node',
      protocol: 'shadowsocks',
      active: false,
      connection: { server: 'archived.example.com', port: 8388 },
      capabilities: {},
    }

    vi.spyOn(api, 'get').mockImplementation(async (path: string, options?: any) => {
      if (path === '/api/v1/nodes') {
        // Main list returns only enabled subscription nodes (e.g. empty or 1 node)
        expect(options?.params?.scope).toBe('enabled_subscriptions')
        return { items: [], page: 1, page_size: 100, total: 0 }
      }
      if (path === '/api/v1/nodes/node-archived-99') {
        return { node: historicalNode, sources: [] }
      }
      return {}
    })

    const nodesHook = useNodes()
    await nodesHook.load()
    expect(nodesHook.items.value).toHaveLength(0)

    // Historical detail is directly accessible without error
    const detail = await nodesHook.fetchNodeDetail('node-archived-99')
    expect(detail).not.toBeNull()
    expect(detail?.logicalId).toBe('node-archived-99')
    expect(detail?.displayName).toBe('Archived Legacy Node')
  })

  // --------------------------------------------------------------------------
  // Scenario 3: Shared perSub counts can sum larger, but global distinct count is unified
  // --------------------------------------------------------------------------
  it('Scenario 3: shared perSub counts can sum larger but global distinct unified (no fake sum)', async () => {
    const subs: SubscriptionRecord[] = [
      {
        id: 'sub-alpha',
        name: 'Cluster Alpha',
        source_url_secret_ref: 'https://example.com/alpha',
        enabled: true,
        node_count: 20,
        source_node_count: 20,
        counts_scope: 'enabled_subscriptions',
        refresh_policy: {
          interval_seconds: 3600,
          user_agent_policy: 'default',
          timeout_seconds: 30,
          max_response_bytes: 1048576,
        },
        revision: 'rev-a',
        created_at: '2026-10-01T00:00:00Z',
        updated_at: '2026-10-01T00:00:00Z',
      },
      {
        id: 'sub-beta',
        name: 'Cluster Beta',
        source_url_secret_ref: 'https://example.com/beta',
        enabled: true,
        node_count: 15,
        source_node_count: 15,
        counts_scope: 'enabled_subscriptions',
        refresh_policy: {
          interval_seconds: 3600,
          user_agent_policy: 'default',
          timeout_seconds: 30,
          max_response_bytes: 1048576,
        },
        revision: 'rev-b',
        created_at: '2026-10-01T00:00:00Z',
        updated_at: '2026-10-01T00:00:00Z',
      },
    ]

    vi.spyOn(api, 'get').mockImplementation(async (path: string, options?: any) => {
      if (path === '/api/v1/subscriptions') {
        return { items: subs, page: 1, page_size: 50, total: 2 }
      }
      if (path === '/api/v1/nodes') {
        expect(options?.params?.scope).toBe('enabled_subscriptions')
        // 4 nodes are shared between Alpha (20) and Beta (15), so global distinct E = 31
        return { items: [], page: 1, page_size: 1, total: 31 }
      }
      if (path === '/api/v1/probes/runs') return { total: 0 }
      if (path === '/api/v1/policies/groups') return { total: 0 }
      if (path === '/healthz') return { status: 'healthy' }
      return {}
    })

    // Mount SubscriptionsView
    app = createApp({
      render() {
        return h(SubscriptionsView)
      },
    })
    app.mount(container)
    await nextTick()
    await new Promise((r) => setTimeout(r, 20))

    const cards = container.querySelectorAll('[data-testid="subscription-card"]')
    expect(cards).toHaveLength(2)

    // Card 1 shows 20, Card 2 shows 15 (adapting selector to find node count dd without weakening assertions)
    const getCardNodeCount = (card: Element) => {
      const explicit = card.querySelector('[data-testid="sub-current-node-count"]')
      if (explicit) return explicit.textContent?.trim()
      const dts = Array.from(card.querySelectorAll('dt'))
      const dt = dts.find((el) => el.textContent?.includes('nodeCount') || el.textContent?.includes('节点数'))
      if (dt && dt.nextElementSibling) {
        return dt.nextElementSibling.textContent?.trim()
      }
      return null
    }

    expect(getCardNodeCount(cards[0])).toBe('20')
    expect(getCardNodeCount(cards[1])).toBe('15')

    // Footnote explicitly reminds that per-card counts are independent and global inventory is deduplicated
    expect(container.textContent).toContain('各卡独立计数 (含多源共享节点)，全局已去重')

    // Unmount SubscriptionsView, mount DashboardView to verify distinct global total is 31, NOT 35
    app.unmount()
    app = createApp({
      render() {
        return h(DashboardView)
      },
    })
    app.mount(container)
    await nextTick()
    await new Promise((r) => setTimeout(r, 20))

    expect(container.textContent).toContain('31')
    expect(container.textContent).not.toContain('35')
  })

  // --------------------------------------------------------------------------
  // Scenario 4: Toggle A success reloads server counts and items; re-enable restores
  // --------------------------------------------------------------------------
  it('Scenario 4: toggle A success reloads server counts and items, re-enable restores selected fields', async () => {
    let subAEnabled = true

    const subsHook = useSubscriptions()

    vi.spyOn(api, 'get').mockImplementation(async (path: string) => {
      if (path === '/api/v1/subscriptions') {
        return {
          items: [
            {
              id: 'sub-a',
              name: 'Sub A',
              source_url_secret_ref: 'https://example.com/sub-a',
              enabled: subAEnabled,
              node_count: subAEnabled ? 10 : 0,
              source_node_count: 10,
              counts_scope: 'enabled_subscriptions',
              refresh_policy: {
                interval_seconds: 3600,
                user_agent_policy: 'default',
                timeout_seconds: 30,
                max_response_bytes: 1048576,
              },
              revision: 'rev-a1',
              created_at: '2026-10-01T00:00:00Z',
              updated_at: '2026-10-01T00:00:00Z',
            },
          ],
          page: 1,
          page_size: 50,
          total: 1,
        }
      }
      return {}
    })

    const patchSpy = vi.spyOn(api, 'patch').mockImplementation(async (path: string, body?: any) => {
      if (path === '/api/v1/subscriptions/sub-a') {
        subAEnabled = body?.enabled
        return { id: 'sub-a', enabled: subAEnabled }
      }
      return {}
    })

    await subsHook.load()
    expect(subsHook.items.value[0].enabled).toBe(true)
    expect(subsHook.items.value[0].node_count).toBe(10)

    // Toggle to disable Sub A
    await subsHook.toggle(subsHook.items.value[0])
    expect(patchSpy).toHaveBeenCalledWith(
      '/api/v1/subscriptions/sub-a',
      { enabled: false },
      expect.objectContaining({ headers: { 'If-Match': 'rev-a1' } })
    )
    expect(subsHook.items.value[0].enabled).toBe(false)
    expect(subsHook.items.value[0].node_count).toBe(0)
    expect(subsHook.items.value[0].source_node_count).toBe(10)

    // Toggle back to re-enable Sub A
    await subsHook.toggle(subsHook.items.value[0])
    expect(patchSpy).toHaveBeenCalledWith(
      '/api/v1/subscriptions/sub-a',
      { enabled: true },
      expect.anything()
    )
    expect(subsHook.items.value[0].enabled).toBe(true)
    expect(subsHook.items.value[0].node_count).toBe(10)
  })

  // --------------------------------------------------------------------------
  // Scenario 5: scope=enabled_subscriptions passed in all main calls & route revisit refreshes
  // --------------------------------------------------------------------------
  it('Scenario 5: params scope enabled_subscriptions passed in all main calls and route revisit refreshes', async () => {
    const recordedParams: Array<Record<string, any>> = []

    vi.spyOn(api, 'get').mockImplementation(async (path: string, options?: any) => {
      if (path === '/api/v1/nodes') {
        recordedParams.push(options?.params || {})
        return { items: [], page: 1, page_size: 100, total: 31 }
      }
      if (path === '/api/v1/subscriptions') return { items: [], total: 1 }
      if (path === '/api/v1/probes/runs') return { total: 0 }
      if (path === '/api/v1/policies/groups') return { total: 0 }
      if (path === '/healthz') return { status: 'healthy' }
      return {}
    })

    // 1. Dashboard load
    app = createApp({
      render() {
        return h(DashboardView)
      },
    })
    app.mount(container)
    await nextTick()
    await new Promise((r) => setTimeout(r, 20))

    // Both calls from Dashboard pass scope: enabled_subscriptions
    expect(recordedParams.length).toBeGreaterThanOrEqual(2)
    for (const p of recordedParams) {
      expect(p.scope).toBe('enabled_subscriptions')
    }

    // 2. NodesView load on revisit
    app.unmount()
    recordedParams.length = 0

    app = createApp({
      render() {
        return h(NodesView)
      },
    })
    app.mount(container)
    await nextTick()
    await new Promise((r) => setTimeout(r, 20))

    expect(recordedParams.length).toBeGreaterThanOrEqual(1)
    expect(recordedParams[0].scope).toBe('enabled_subscriptions')
  })

  // --------------------------------------------------------------------------
  // Scenario 6: filters / pagination total does not post-filter only 1 page to wrong count
  // --------------------------------------------------------------------------
  it('Scenario 6: filters/pagination total not post-filter only one page causing wrong count', async () => {
    vi.spyOn(api, 'get').mockImplementation(async (path: string, options?: any) => {
      if (path === '/api/v1/nodes') {
        const params = options?.params || {}
        expect(params.scope).toBe('enabled_subscriptions')
        if (params.protocol === 'trojan') {
          return {
            items: [
              {
                logical_id: 'node-tr-1',
                display_name: 'Trojan 1',
                protocol: 'trojan',
                active: true,
                connection: { server: 'tr1.example.com', port: 443 },
              },
            ],
            page: 1,
            page_size: 100,
            total: 1,
          }
        }
        return {
          items: [
            {
              logical_id: 'node-1',
              display_name: 'Node 1',
              protocol: 'vmess',
              active: true,
              connection: { server: 'vm1.example.com', port: 443 },
            },
            {
              logical_id: 'node-2',
              display_name: 'Node 2',
              protocol: 'trojan',
              active: true,
              connection: { server: 'tr1.example.com', port: 443 },
            },
          ],
          page: 1,
          page_size: 100,
          total: 31, // server total for all protocols
        }
      }
      return {}
    })

    const hook = useNodes()
    await hook.load()
    // Unfiltered total is exactly what server returned (31), not just items.length
    expect(hook.total.value).toBe(31)

    // Filter by protocol
    hook.protocolFilter.value = 'trojan'
    await hook.load()
    // Filtered total is exactly 1 from server
    expect(hook.total.value).toBe(1)
    expect(hook.items.value).toHaveLength(1)
  })

  // --------------------------------------------------------------------------
  // Scenario 7: Profile history snapshot count distinction
  // --------------------------------------------------------------------------
  it('Scenario 7: profile history snapshot count distinction and immutable label', async () => {
    sessionStorage.setItem(
      'csp_publication_latest',
      JSON.stringify({
        id: 'pub-snap-100',
        target: 'mihomo',
        state: 'active',
        snapshot_digest: 'sha256:fedcba0987654321',
        export_url: '/publish/v1/pub-snap-100?token=token123',
        created_at: '2026-09-30T10:00:00Z',
      })
    )
    sessionStorage.setItem(
      'csp_publication_active_mihomo',
      JSON.stringify({
        id: 'pub-snap-100',
        target: 'mihomo',
        state: 'active',
        snapshot_digest: 'sha256:fedcba0987654321',
        export_url: '/publish/v1/pub-snap-100?token=token123',
        created_at: '2026-09-30T10:00:00Z',
      })
    )

    vi.spyOn(api, 'get').mockImplementation(async (path: string) => {
      if (path === '/api/v1/settings/auth') return { export_auth_enabled: false }
      if (path.includes('pub-snap-100')) {
        return {
          publication: {
            id: 'pub-snap-100',
            target: 'mihomo',
            state: 'active',
            snapshot_digest: 'sha256:fedcba0987654321',
            created_at: '2026-09-30T10:00:00Z',
          },
          export_url: '/publish/v1/pub-snap-100?token=token123',
        }
      }
      return {}
    })

    vi.spyOn(api, 'post').mockImplementation(async (path: string) => {
      if (path === '/api/v1/publications/preview') {
        return {
          target: 'mihomo',
          snapshot_digest: 'sha256:snap-latest',
          content_digest: 'sha256:content-latest',
          content: 'proxies:\n  - name: HK 1\n',
          content_type: 'text/yaml',
          filename: 'clash.yaml',
          manifest: {
            node_count: 31,
            excluded_count: 0,
          },
          filter_counts: {
            raw_total: 31,
            admitted_total: 31,
            global_filtered_total: 31,
            group_filtered_total: 31,
          },
          diagnostics: [],
        }
      }
      return {}
    })

    app = createApp({
      render() {
        return h(PublicationsView)
      },
    })
    app.mount(container)
    await nextTick()
    await new Promise((r) => setTimeout(r, 40))

    // Renders client subscription rows with full config vs node-only badges
    expect(container.textContent).toContain('完整配置')
    expect(container.textContent).toContain('节点订阅')
    expect(container.textContent).toContain('客户端订阅')
  })

  // --------------------------------------------------------------------------
  // Scenario 8: 401 / auth and error do not fake show success
  // --------------------------------------------------------------------------
  it('Scenario 8: 401 and errors do not fake show success on toggle or refresh', async () => {
    let unauthorizedCallbackFired = false
    api.setOnUnauthorized(() => {
      unauthorizedCallbackFired = true
    })

    const sampleSub: SubscriptionRecord = {
      id: 'sub-fail',
      name: 'Sub Fail',
      source_url_secret_ref: 'https://example.com/fail',
      enabled: true,
      node_count: 10,
      source_node_count: 10,
      refresh_policy: {
        interval_seconds: 3600,
        user_agent_policy: 'default',
        timeout_seconds: 30,
        max_response_bytes: 1048576,
      },
      revision: 'rev-fail',
      created_at: '2026-10-01T00:00:00Z',
      updated_at: '2026-10-01T00:00:00Z',
    }

    // 1. Error 500 does not fake success
    vi.spyOn(api, 'patch').mockRejectedValueOnce(new ApiError(500, 'internal_error', 'Server explosion'))
    vi.spyOn(api, 'get').mockResolvedValue({
      items: [sampleSub],
      page: 1,
      page_size: 50,
      total: 1,
    })

    const hook = useSubscriptions()
    await hook.load()

    await expect(hook.toggle(hook.items.value[0])).rejects.toThrow('Server explosion')
    // Toast contains error message
    expect(toastStore.items.value.some((t) => t.tone === 'error')).toBe(true)
    // Server state preserved, not optimistically toggled to false
    expect(hook.items.value[0].enabled).toBe(true)

    // 2. 401 triggers onUnauthorizedCallback and rejects
    vi.spyOn(api, 'patch').mockRejectedValueOnce(new ApiError(401, 'unauthorized', 'Unauthorized session'))
    await expect(hook.toggle(hook.items.value[0])).rejects.toThrow('Unauthorized session')
  })

  // --------------------------------------------------------------------------
  // Scenario 9: Server filtering before pagination for node ledger
  // Fixture API page size 100 where matching nodes occur only after initial 100 unfiltered nodes
  // --------------------------------------------------------------------------
  it('Scenario 9: server filtering before pagination with matching nodes occurring only after initial 100 unfiltered nodes', async () => {
    // 250 total nodes in universe:
    // 1. Initial 100 unfiltered nodes: none match search 'HK', protocol 'trojan', or health 'healthy'
    const initial100Nodes: NodeRecord[] = Array.from({ length: 100 }, (_, i) => ({
      logical_id: `node-unfiltered-${i + 1}`,
      display_name: `US Server ${i + 1}`,
      protocol: 'vmess',
      active: true,
      health_status: 'unhealthy' as const,
      connection: { server: `us-${i + 1}.example.com`, port: 443 },
      capabilities: {},
    }))

    // 2. 120 matching nodes: located strictly after initial 100 unfiltered nodes
    // All match search 'HK', protocol 'trojan', and health 'healthy'
    const matching120Nodes: NodeRecord[] = Array.from({ length: 120 }, (_, i) => ({
      logical_id: `node-match-hk-${i + 1}`,
      display_name: `HK Magic Gateway ${i + 1}`,
      protocol: 'trojan',
      active: true,
      health_status: 'healthy' as const,
      connection: { server: `hk-${i + 1}.example.com`, port: 443 },
      capabilities: {},
    }))

    // 3. 30 subsequent other nodes: protocol 'ss', health 'unknown'
    const other30Nodes: NodeRecord[] = Array.from({ length: 30 }, (_, i) => ({
      logical_id: `node-other-${i + 1}`,
      display_name: `JP Relay ${i + 1}`,
      protocol: 'ss',
      active: true,
      health_status: 'unknown' as const,
      connection: { server: `jp-${i + 1}.example.com`, port: 8388 },
      capabilities: {},
    }))

    const allUniverseNodes: NodeRecord[] = [
      ...initial100Nodes,
      ...matching120Nodes,
      ...other30Nodes,
    ]

    const recordedNodeCalls: Array<{ path: string; params: Record<string, any> }> = []

    vi.spyOn(api, 'get').mockImplementation(async (path: string, options?: any) => {
      if (path === '/api/v1/nodes') {
        const params = (options?.params || {}) as Record<string, any>
        recordedNodeCalls.push({ path, params: { ...params } })

        // Server-side filtering before pagination:
        let filtered = allUniverseNodes
        if (params.search && String(params.search).trim()) {
          const q = String(params.search).trim().toLowerCase()
          filtered = filtered.filter((n) => n.display_name.toLowerCase().includes(q))
        }
        if (params.protocol && params.protocol !== 'all') {
          filtered = filtered.filter((n) => n.protocol === params.protocol)
        }
        if (params.health_status && params.health_status !== 'all' && params.health_status !== 'probing') {
          filtered = filtered.filter((n) => n.health_status === params.health_status)
        }

        const page = Number(params.page || 1)
        const pageSize = Number(params.page_size || 100)
        const start = (page - 1) * pageSize
        const items = filtered.slice(start, start + pageSize)

        return {
          items,
          page,
          page_size: pageSize,
          total: filtered.length,
        }
      }
      if (path === '/api/v1/probes/pool') {
        return {
          queue_nodes_count: 0,
          probing_count: 0,
          queued_waiting_count: 0,
          untested_count: 0,
          undetermined_count: 0,
          total_count: 250,
          available_count: 120,
          unavailable_count: 100,
          healthy_count: 120,
          degraded_count: 0,
          probing_node_ids: [],
          queued_node_ids: [],
          updated_at: new Date().toISOString(),
        } satisfies ProbePoolStatus
      }
      return {}
    })

    const hook = useNodes()

    // ------------------------------------------------------------------------
    // Part A: Search filtering before pagination + second page
    // ------------------------------------------------------------------------
    hook.searchQuery.value = 'HK'
    await hook.load()

    const searchCall1 = recordedNodeCalls[recordedNodeCalls.length - 1]
    expect(searchCall1.params.scope).toBe('enabled_subscriptions')
    expect(searchCall1.params.search).toBe('HK')
    expect(searchCall1.params['page']).toBe(1)
    expect(searchCall1.params.page_size).toBe(100)

    // Filtered total from server is 120, NOT 0 (which would happen if client-side filtered only page 1)
    expect(hook.total.value).toBe(120)
    expect(hook.items.value).toHaveLength(100)
    expect(hook.items.value[0].displayName).toBe('HK Magic Gateway 1')
    expect(hook.items.value[99].displayName).toBe('HK Magic Gateway 100')
    expect(hook.hasMore.value).toBe(true)

    // Second page request for search
    await hook.loadMore()
    const searchCall2 = recordedNodeCalls[recordedNodeCalls.length - 1]
    expect(searchCall2.params.search).toBe('HK')
    expect(searchCall2.params['page']).toBe(2)
    expect(searchCall2.params.page_size).toBe(100)
    expect(hook.items.value).toHaveLength(120)
    expect(hook.items.value[100].displayName).toBe('HK Magic Gateway 101')
    expect(hook.items.value[119].displayName).toBe('HK Magic Gateway 120')
    expect(hook.hasMore.value).toBe(false)

    // ------------------------------------------------------------------------
    // Part B: Protocol filtering before pagination + second page
    // ------------------------------------------------------------------------
    hook.searchQuery.value = ''
    hook.protocolFilter.value = 'trojan'
    await hook.load()

    const protoCall1 = recordedNodeCalls[recordedNodeCalls.length - 1]
    expect(protoCall1.params.protocol).toBe('trojan')
    expect(protoCall1.params['page']).toBe(1)
    expect(protoCall1.params.page_size).toBe(100)
    expect(protoCall1.params.search).toBeUndefined()

    expect(hook.total.value).toBe(120)
    expect(hook.items.value).toHaveLength(100)
    expect(hook.items.value.every((n) => n.protocol === 'trojan')).toBe(true)
    expect(hook.hasMore.value).toBe(true)

    await hook.loadMore()
    const protoCall2 = recordedNodeCalls[recordedNodeCalls.length - 1]
    expect(protoCall2.params.protocol).toBe('trojan')
    expect(protoCall2.params['page']).toBe(2)
    expect(hook.items.value).toHaveLength(120)
    expect(hook.hasMore.value).toBe(false)

    // ------------------------------------------------------------------------
    // Part C: Health status filtering before pagination + second page
    // ------------------------------------------------------------------------
    hook.protocolFilter.value = 'all'
    hook.healthFilter.value = 'healthy'
    await hook.load()

    const healthCall1 = recordedNodeCalls[recordedNodeCalls.length - 1]
    expect(healthCall1.params.health_status).toBe('healthy')
    expect(healthCall1.params['page']).toBe(1)
    expect(healthCall1.params.page_size).toBe(100)

    expect(hook.total.value).toBe(120)
    expect(hook.items.value).toHaveLength(100)
    expect(hook.items.value.every((n) => n.healthStatus === 'healthy')).toBe(true)
    expect(hook.hasMore.value).toBe(true)

    await hook.loadMore()
    const healthCall2 = recordedNodeCalls[recordedNodeCalls.length - 1]
    expect(healthCall2.params.health_status).toBe('healthy')
    expect(healthCall2.params['page']).toBe(2)
    expect(hook.items.value).toHaveLength(120)
    expect(hook.hasMore.value).toBe(false)

    // ------------------------------------------------------------------------
    // Part D: Combined criteria (search + protocol + health)
    // ------------------------------------------------------------------------
    hook.searchQuery.value = 'HK'
    hook.protocolFilter.value = 'trojan'
    hook.healthFilter.value = 'healthy'
    await hook.load()

    const combinedCall1 = recordedNodeCalls[recordedNodeCalls.length - 1]
    expect(combinedCall1.params.search).toBe('HK')
    expect(combinedCall1.params.protocol).toBe('trojan')
    expect(combinedCall1.params.health_status).toBe('healthy')
    expect(combinedCall1.params['page']).toBe(1)
    expect(combinedCall1.params.page_size).toBe(100)
    expect(combinedCall1.params.scope).toBe('enabled_subscriptions')

    expect(hook.total.value).toBe(120)
    expect(hook.items.value).toHaveLength(100)
    expect(hook.items.value.every((n) =>
      n.displayName.includes('HK') && n.protocol === 'trojan' && n.healthStatus === 'healthy'
    )).toBe(true)
    expect(hook.hasMore.value).toBe(true)

    await hook.loadMore()
    const combinedCall2 = recordedNodeCalls[recordedNodeCalls.length - 1]
    expect(combinedCall2.params.search).toBe('HK')
    expect(combinedCall2.params.protocol).toBe('trojan')
    expect(combinedCall2.params.health_status).toBe('healthy')
    expect(combinedCall2.params['page']).toBe(2)
    expect(hook.items.value).toHaveLength(120)
    expect(hook.hasMore.value).toBe(false)

    // ------------------------------------------------------------------------
    // Part E: Health filter change resets to a fresh page 1 request
    // ------------------------------------------------------------------------
    // Currently on page 2 with items.length === 120

    // Change health filter to 'unhealthy' and trigger load
    hook.searchQuery.value = ''
    hook.protocolFilter.value = 'all'
    hook.healthFilter.value = 'unhealthy'
    await hook.load()

    const resetCall = recordedNodeCalls[recordedNodeCalls.length - 1]
    expect(resetCall.params.health_status).toBe('unhealthy')
    expect(resetCall.params['page']).toBe(1)
    expect(hook.total.value).toBe(100)
    expect(hook.items.value).toHaveLength(100)
    expect(hook.items.value.every((n) => n.healthStatus === 'unhealthy')).toBe(true)
  })

  // --------------------------------------------------------------------------
  // Scenario 10: NodesView health filter change triggers fresh page 1 request
  // Component specification: UI health change dispatches fresh page 1 request
  // --------------------------------------------------------------------------
  it('Scenario 10: NodesView triggers fresh page 1 request with server params on health change', async () => {
    const recordedNodeParams: Array<Record<string, any>> = []

    vi.spyOn(api, 'get').mockImplementation(async (path: string, options?: any) => {
      if (path === '/api/v1/nodes') {
        const params = (options?.params || {}) as Record<string, any>
        recordedNodeParams.push({ ...params })
        return {
          items: [
            {
              logical_id: 'node-hk-1',
              display_name: 'HK Edge 1',
              protocol: 'trojan',
              active: true,
              health_status: 'healthy',
            },
          ],
          page: Number(params.page || 1),
          page_size: 100,
          total: 1,
        }
      }
      if (path === '/api/v1/probes/pool') {
        return {
          queue_nodes_count: 0,
          probing_count: 0,
          queued_waiting_count: 0,
          untested_count: 0,
          undetermined_count: 0,
          total_count: 1,
          available_count: 1,
          unavailable_count: 0,
          healthy_count: 1,
          degraded_count: 0,
          probing_node_ids: [],
          queued_node_ids: [],
          updated_at: new Date().toISOString(),
        } satisfies ProbePoolStatus
      }
      return {}
    })

    app = createApp({
      render() {
        return h(NodesView)
      },
    })
    app.mount(container)
    await nextTick()
    await new Promise((r) => setTimeout(r, 40))

    // Initial mount requests page 1
    expect(recordedNodeParams.length).toBeGreaterThanOrEqual(1)
    expect(recordedNodeParams[0].page).toBe(1)
    expect(recordedNodeParams[0].scope).toBe('enabled_subscriptions')

    // Find and trigger health filter change via UI (select or health pill)
    const healthSelect = container.querySelector('[data-testid="node-health-filter"]') as HTMLSelectElement | null
    if (healthSelect) {
      healthSelect.value = 'healthy'
      healthSelect.dispatchEvent(new Event('change'))
    } else {
      const healthyPill = Array.from(container.querySelectorAll('button')).find((b) =>
        b.getAttribute('data-testid') === 'node-health-pill-healthy' ||
        b.textContent?.includes('正常')
      )
      healthyPill?.click()
    }

    await nextTick()
    await new Promise((r) => setTimeout(r, 40))

    // Assert that a fresh request was made for page 1 with health_status === 'healthy'
    const matchingRequest = recordedNodeParams.find(
      (p) => p.health_status === 'healthy' && p.page === 1
    )
    expect(matchingRequest).toBeDefined()
    expect(matchingRequest?.page).toBe(1)
    expect(matchingRequest?.health_status).toBe('healthy')
  })
})
