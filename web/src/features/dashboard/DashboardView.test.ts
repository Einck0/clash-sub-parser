// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { createApp, h, nextTick } from 'vue'
import DashboardView from './DashboardView.vue'
import { api } from '../../api/client'
import { setLocale } from '../../locales'
import MetricSummary from './MetricSummary.vue'
import type { DashboardMetrics } from './dashboardTypes'

describe('DashboardView Component', () => {
  let container: HTMLDivElement
  let app: ReturnType<typeof createApp> | null = null

  beforeEach(() => {
    setLocale('zh-CN')
    container = document.createElement('div')
    document.body.appendChild(container)
    vi.restoreAllMocks()
  })

  afterEach(() => {
    if (app) {
      app.unmount()
      app = null
    }
    container.remove()
    document.body.innerHTML = ''
    vi.restoreAllMocks()
  })

  // 1. Initial Loading State: does NOT display false '0' values while loading
  it('displays loading state initially and does not render false 0 values', async () => {
    // Return pending promises that don't resolve immediately
    vi.spyOn(api, 'get').mockImplementation(() => new Promise(() => {}))

    app = createApp({
      render() {
        return h(DashboardView)
      },
    })
    app.mount(container)
    await nextTick()

    // Loading spinners should be visible for metrics
    expect(container.querySelector('[data-testid="metric-loading-subscriptions"]')).toBeTruthy()
    expect(container.querySelector('[data-testid="metric-loading-nodes"]')).toBeTruthy()
    expect(container.querySelector('[data-testid="metric-loading-policies"]')).toBeTruthy()
    expect(container.querySelector('[data-testid="metric-loading-rules"]')).toBeTruthy()
    expect(container.querySelector('[data-testid="metric-loading-probes"]')).toBeTruthy()

    // No metric values should be rendered yet
    expect(container.querySelector('[data-testid="metric-value-subscriptions"]')).toBeNull()
    expect(container.querySelector('[data-testid="metric-value-nodes"]')).toBeNull()

    // Health indicator should be in checking state
    const healthIndicator = container.querySelector('[data-testid="health-status-indicator"]')
    expect(healthIndicator).toBeTruthy()
    expect(healthIndicator?.className).toContain('animate-pulse')
  })

  // 2. True Zero: endpoints returning 0 render '0' with success styling
  it('displays true zero (成功 0) distinctly when endpoints return total: 0', async () => {
    vi.spyOn(api, 'get').mockImplementation(async (path: string) => {
      if (path === '/healthz') return { status: 'healthy' }
      if (path === '/api/v1/subscriptions') return { total: 0 }
      if (path === '/api/v1/nodes') return { total: 0 }
      if (path === '/api/v1/policies/groups') return { total: 0 }
      if (path === '/api/v1/policies/rules') return { total: 0 }
      if (path === '/api/v1/probes/runs') return { total: 0 }
      return {}
    })

    app = createApp({
      render() {
        return h(DashboardView)
      },
    })
    app.mount(container)
    await nextTick()
    await new Promise((r) => setTimeout(r, 25))

    const subsVal = container.querySelector('[data-testid="metric-value-subscriptions"]')
    const nodesVal = container.querySelector('[data-testid="metric-value-nodes"]')
    const policiesVal = container.querySelector('[data-testid="metric-value-policies"]')
    const rulesVal = container.querySelector('[data-testid="metric-value-rules"]')
    const probesVal = container.querySelector('[data-testid="metric-value-probes"]')

    expect(subsVal?.textContent?.trim()).toBe('0')
    expect(nodesVal?.textContent?.trim()).toBe('0')
    expect(policiesVal?.textContent?.trim()).toBe('0')
    expect(rulesVal?.textContent?.trim()).toBe('0')
    expect(probesVal?.textContent?.trim()).toBe('0')

    // No error indicators
    expect(container.querySelector('[data-testid="metric-error-subscriptions"]')).toBeNull()
    expect(container.querySelector('[data-testid="metric-error-nodes"]')).toBeNull()
  })

  // 3. Partial Failure in Promise.allSettled: failed metrics show error, NEVER 0
  it('handles partial failure in Promise.allSettled without displaying 0 for failed metrics', async () => {
    vi.spyOn(api, 'get').mockImplementation(async (path: string, options?: any) => {
      if (path === '/healthz') return { status: 'healthy' }
      if (path === '/api/v1/subscriptions') return { total: 15 }
      if (path === '/api/v1/nodes') {
        // Simulating node service failure
        throw new Error('Node storage engine down')
      }
      if (path === '/api/v1/policies/groups') return { total: 3 }
      if (path === '/api/v1/policies/rules') return { total: 80 }
      if (path === '/api/v1/probes/runs') return { total: 4 }
      return {}
    })

    app = createApp({
      render() {
        return h(DashboardView)
      },
    })
    app.mount(container)
    await nextTick()
    await new Promise((r) => setTimeout(r, 25))

    // Subscriptions succeeded with 15
    const subsVal = container.querySelector('[data-testid="metric-value-subscriptions"]')
    expect(subsVal?.textContent?.trim()).toBe('15')

    // Policies succeeded with 3
    const policiesVal = container.querySelector('[data-testid="metric-value-policies"]')
    expect(policiesVal?.textContent?.trim()).toBe('3')

    // Rules succeeded with 80
    const rulesVal = container.querySelector('[data-testid="metric-value-rules"]')
    expect(rulesVal?.textContent?.trim()).toBe('80')

    // Probes succeeded with 4
    const probesVal = container.querySelector('[data-testid="metric-value-probes"]')
    expect(probesVal?.textContent?.trim()).toBe('4')

    // Nodes failed: MUST show error, MUST NOT display 0!
    expect(container.querySelector('[data-testid="metric-value-nodes"]')).toBeNull()
    const nodeError = container.querySelector('[data-testid="metric-error-nodes"]')
    expect(nodeError).toBeTruthy()
    expect(nodeError?.textContent).toContain('不可用')
  })

  // 4. Refresh Failure Preserves Old Value (保旧值 + 不可得说明)
  it('preserves previous value on refresh failure and displays stale indicator', async () => {
    let failNodes = false

    vi.spyOn(api, 'get').mockImplementation(async (path: string) => {
      if (path === '/healthz') return { status: 'healthy' }
      if (path === '/api/v1/subscriptions') return { total: 8 }
      if (path === '/api/v1/nodes') {
        if (failNodes) {
          throw new Error('Temporary gateway timeout')
        }
        return { total: 42 }
      }
      if (path === '/api/v1/policies/groups') return { total: 2 }
      if (path === '/api/v1/policies/rules') return { total: 50 }
      if (path === '/api/v1/probes/runs') return { total: 6 }
      return {}
    })

    app = createApp({
      render() {
        return h(DashboardView)
      },
    })
    app.mount(container)
    await nextTick()
    await new Promise((r) => setTimeout(r, 25))

    // Initial load succeeded: nodes is 42
    let nodesVal = container.querySelector('[data-testid="metric-value-nodes"]')
    expect(nodesVal?.textContent?.trim()).toBe('42')
    expect(container.querySelector('[data-testid="metric-stale-nodes"]')).toBeNull()

    // Trigger refresh where nodes fails
    failNodes = true
    const refreshBtn = container.querySelector('[data-testid="refresh-all-btn"]') as HTMLButtonElement
    expect(refreshBtn).toBeTruthy()
    refreshBtn.click()

    await nextTick()
    await new Promise((r) => setTimeout(r, 25))

    // Nodes MUST still be 42 (preserved old value)
    nodesVal = container.querySelector('[data-testid="metric-value-nodes"]')
    expect(nodesVal?.textContent?.trim()).toBe('42')

    // Stale indicator must be displayed
    const staleBadge = container.querySelector('[data-testid="metric-stale-nodes"]')
    expect(staleBadge).toBeTruthy()
    expect(staleBadge?.textContent).toContain('缓存')
  })

  it('blocks duplicate refresh while initial sections are pending', async () => {
    let finishNodes!: (value: { total: number }) => void
    const getSpy = vi.spyOn(api, 'get').mockImplementation((path: string, options?: any) => {
      if (path === '/healthz') return Promise.resolve({ status: 'healthy' })
      if (path === '/api/v1/nodes' && !options?.params?.active_only) {
        return new Promise(resolve => { finishNodes = resolve })
      }
      return Promise.resolve({ total: 5 })
    })
    app = createApp({ render: () => h(DashboardView) })
    app.mount(container)
    await nextTick()
    await nextTick()
    const refresh = container.querySelector('[data-testid="refresh-all-btn"]') as HTMLButtonElement
    expect(refresh.disabled).toBe(true)
    expect(refresh.getAttribute('aria-busy')).toBe('true')
    const requests = getSpy.mock.calls.length
    refresh.click()
    expect(getSpy).toHaveBeenCalledTimes(requests)
    expect(container.querySelector('[data-testid="metric-value-subscriptions"]')?.textContent?.trim()).toBe('5')
    finishNodes({ total: 42 })
    await Promise.resolve()
    await Promise.resolve()
    await nextTick()
    await nextTick()
    expect(refresh.disabled).toBe(false)
    expect(container.querySelector('[data-testid="metric-value-nodes"]')?.textContent?.trim()).toBe('42')
  })

  it('keeps refresh-all disabled while only the health check remains pending', async () => {
    let finishHealth!: (value: { status: string }) => void
    vi.spyOn(api, 'get').mockImplementation((path: string) => path === '/healthz'
      ? new Promise(resolve => { finishHealth = resolve })
      : Promise.resolve({ total: 4 }))
    app = createApp({ render: () => h(DashboardView) })
    app.mount(container)
    await nextTick()
    await nextTick()
    const refresh = container.querySelector('[data-testid="refresh-all-btn"]') as HTMLButtonElement
    expect(container.querySelector('[data-testid="metric-value-nodes"]')?.textContent?.trim()).toBe('4')
    expect(refresh.disabled).toBe(true)
    finishHealth({ status: 'healthy' })
    await nextTick()
    await nextTick()
    expect(refresh.disabled).toBe(false)
  })

  // 6. Pure Navigation: quick actions and cards do NOT trigger probe or refresh mutations
  it('performs pure navigation on quick actions without triggering probe or mutation APIs', async () => {
    const postSpy = vi.spyOn(api, 'post').mockImplementation(async () => ({}))
    const putSpy = vi.spyOn(api, 'put').mockImplementation(async () => ({}))

    vi.spyOn(api, 'get').mockImplementation(async () => ({ total: 5, status: 'healthy' }))

    app = createApp({
      render() {
        return h(DashboardView)
      },
    })
    app.mount(container)
    await nextTick()
    await new Promise((r) => setTimeout(r, 25))

    // Click quick action for subscriptions
    const subAction = container.querySelector('[data-testid="quick-action-subscriptions"]') as HTMLButtonElement
    expect(subAction).toBeTruthy()
    subAction.click()

    // Click quick action for rules
    const rulesAction = container.querySelector('[data-testid="quick-action-rules"]') as HTMLButtonElement
    expect(rulesAction).toBeTruthy()
    rulesAction.click()

    // Click quick action for publications (export)
    const pubAction = container.querySelector('[data-testid="quick-action-publications"]') as HTMLButtonElement
    expect(pubAction).toBeTruthy()
    pubAction.click()

    // Click quick action for settings
    const settingsAction = container.querySelector('[data-testid="quick-action-settings"]') as HTMLButtonElement
    expect(settingsAction).toBeTruthy()
    settingsAction.click()

    // Click metric card
    const nodeCard = container.querySelector('[data-testid="metric-card-nodes"]') as HTMLButtonElement
    expect(nodeCard).toBeTruthy()
    nodeCard.click()

    await nextTick()

    // No mutation or external probe triggering
    expect(postSpy).not.toHaveBeenCalled()
    expect(putSpy).not.toHaveBeenCalled()
  })

  // 7. Health Telemetry & Topology Status: real latency in ms (no <10ms fallback) and unvalidated topology
  it('displays real latency in ms and marks topology explicitly as unvalidated', async () => {
    vi.spyOn(api, 'get').mockImplementation(async (path: string) => {
      if (path === '/healthz') return { status: 'healthy' }
      return { total: 0 }
    })

    app = createApp({
      render() {
        return h(DashboardView)
      },
    })
    app.mount(container)
    await nextTick()
    await new Promise((r) => setTimeout(r, 25))

    // Real latency: must end with 'ms' and not be '<10ms'
    const latencyEl = container.querySelector('[data-testid="health-latency"]')
    expect(latencyEl).toBeTruthy()
    expect(latencyEl?.textContent?.trim()).toMatch(/^\d+ms$/)
    expect(latencyEl?.textContent?.trim()).not.toContain('<10ms')

    // Topology status: MUST be '未校验' (Unvalidated), NOT 'ready' or 'badge-success'
    const topologyBadge = container.querySelector('[data-testid="topology-status-badge"]')
    expect(topologyBadge).toBeTruthy()
    expect(topologyBadge?.textContent).toContain('未校验')
    expect(topologyBadge?.className).not.toContain('badge-success')
    expect(topologyBadge?.className).toContain('badge-neutral')
  })

  // 8. Active Node Subset Semantics: clarifies active count != probe health
  it('clarifies active node subset semantics and passes scope: enabled_subscriptions', async () => {
    const recordedNodeParams: any[] = []

    vi.spyOn(api, 'get').mockImplementation(async (path: string, options?: any) => {
      if (path === '/healthz') return { status: 'healthy' }
      if (path === '/api/v1/nodes') {
        recordedNodeParams.push(options?.params || {})
        if (options?.params?.active_only === 'true') {
          return { total: 18 }
        }
        return { total: 50 }
      }
      return { total: 0 }
    })

    app = createApp({
      render() {
        return h(DashboardView)
      },
    })
    app.mount(container)
    await nextTick()
    await new Promise((r) => setTimeout(r, 25))

    // Verify scope enabled_subscriptions was passed for both calls
    expect(recordedNodeParams.length).toBeGreaterThanOrEqual(2)
    for (const p of recordedNodeParams) {
      expect(p.scope).toBe('enabled_subscriptions')
    }

    // Active subset text
    const activeText = container.querySelector('[data-testid="metric-active-nodes"]')
    expect(activeText).toBeTruthy()
    expect(activeText?.textContent).toContain('18')

    // Explicit disclaimer that active count is NOT probe health
    expect(container.textContent).toContain('非探针健康')
  })

  // 9. Historical Probe Batches Semantics: clarifies probe runs != live node health
  it('clarifies probe runs count is execution history, not live node health', async () => {
    vi.spyOn(api, 'get').mockImplementation(async (path: string) => {
      if (path === '/healthz') return { status: 'healthy' }
      if (path === '/api/v1/probes/runs') return { total: 12 }
      return { total: 0 }
    })

    app = createApp({
      render() {
        return h(DashboardView)
      },
    })
    app.mount(container)
    await nextTick()
    await new Promise((r) => setTimeout(r, 25))

    const probesVal = container.querySelector('[data-testid="metric-value-probes"]')
    expect(probesVal?.textContent?.trim()).toBe('12')

    // Explicit disclaimer that probe batches are historical runs, not node health
    const probesCard = container.querySelector('[data-testid="metric-card-probes"]')
    expect(probesCard?.textContent).toContain('历史批次 (非节点健康)')
  })

  it('uses native focusable metric buttons with one navigation per activation', () => {
    const metrics = Object.fromEntries(['subscriptions', 'nodes', 'activeNodes', 'policies', 'rules', 'probes'].map(key => [key, {
      value: 3, status: 'success', error: null, isStale: false,
    }])) as unknown as DashboardMetrics
    const navigate = vi.fn()
    app = createApp({ render: () => h(MetricSummary, { metrics, onNavigate: navigate }) })
    app.mount(container)
    for (const [key, route] of [['subscriptions', 'subscriptions'], ['nodes', 'nodes'], ['policies', 'policy'], ['rules', 'rules'], ['probes', 'nodes']]) {
      const card = container.querySelector(`[data-testid="metric-card-${key}"]`) as HTMLButtonElement
      expect(card.tagName).toBe('BUTTON')
      expect(card.type).toBe('button')
      expect(card.tabIndex).toBe(0)
      card.focus()
      expect(document.activeElement).toBe(card)
      card.click()
      expect(navigate).toHaveBeenLastCalledWith(route)
    }
    expect(navigate).toHaveBeenCalledTimes(5)
    // jsdom does not synthesize native Enter/Space activation; real-browser keys remain an isolation acceptance check.
  })

  it('updates quick actions and metric labels on a live locale switch', async () => {
    vi.spyOn(api, 'get').mockResolvedValue({ total: 4 })
    app = createApp({ render: () => h(DashboardView) })
    app.mount(container)
    await nextTick()
    expect(container.querySelector('[data-testid="quick-action-nodes"]')?.textContent).toContain('节点台账')
    setLocale('en-US')
    await nextTick()
    expect(container.querySelector('[data-testid="quick-action-nodes"]')?.textContent).toContain('Node Ledger')
    expect(container.querySelector('[data-testid="quick-action-subscriptions"]')?.textContent).toContain('Subscriptions')
    expect(container.querySelector('[data-testid="metric-card-nodes"]')?.textContent).toContain('Not probe health')
    expect(container.querySelector('[data-testid="metric-card-probes"]')?.textContent).toContain('Probe runs')
  })

  it('shows full wrapped error details and a stale active-subset count independently', () => {
    const longError = `Unavailable: ${'gateway-'.repeat(50)}`
    const metrics = Object.fromEntries(['subscriptions', 'nodes', 'activeNodes', 'policies', 'rules', 'probes'].map(key => [key, {
      value: 3, status: 'success', error: null, isStale: false,
    }])) as unknown as DashboardMetrics
    metrics.activeNodes = { value: 2, status: 'error', error: longError, isStale: true }
    app = createApp({ render: () => h(MetricSummary, { metrics }) })
    app.mount(container)
    expect(container.querySelector('[data-testid="metric-value-nodes"]')?.textContent?.trim()).toBe('3')
    expect(container.querySelector('[data-testid="metric-value-activeNodes"]')?.textContent).toBe('2')
    expect(container.querySelector('[data-testid="metric-stale-activeNodes"]')).not.toBeNull()
    const detail = container.querySelector('[data-testid="metric-error-detail-activeNodes"]')
    expect(detail?.textContent).toBe(longError)
    expect(detail?.className).not.toContain('truncate')
  })
})
