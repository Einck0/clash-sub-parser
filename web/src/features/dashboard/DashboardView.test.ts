// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { createApp, h, nextTick } from 'vue'
import DashboardView from './DashboardView.vue'
import { api } from '../../api/client'

describe('DashboardView Component', () => {
  let container: HTMLDivElement
  let app: ReturnType<typeof createApp> | null = null

  beforeEach(() => {
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

  // 5. Stale Out-of-Order Response Discarded (过期request/session不覆新状态)
  it('discards stale out-of-order response when a newer request settles first', async () => {
    let resolveFirstRequest: ((val: any) => void) | null = null

    let callCount = 0
    vi.spyOn(api, 'get').mockImplementation(async (path: string, options?: any) => {
      if (path === '/healthz') return { status: 'healthy' }
      if (path === '/api/v1/nodes') {
        if (!options?.params?.active_only) {
          callCount++
          if (callCount === 1) {
            // Slow first request
            return new Promise((resolve) => {
              resolveFirstRequest = resolve
            })
          }
          // Fast second request
          return { total: 99 }
        }
        return { total: 50 }
      }
      return { total: 1 }
    })

    app = createApp({
      render() {
        return h(DashboardView)
      },
    })
    app.mount(container)
    await nextTick()
    // First request is still pending

    // Trigger second request (refresh)
    const refreshBtn = container.querySelector('[data-testid="refresh-all-btn"]') as HTMLButtonElement
    expect(refreshBtn).toBeTruthy()
    refreshBtn.click()
    await nextTick()
    await new Promise((r) => setTimeout(r, 25))

    // Second request resolved: nodes is 99
    expect(container.querySelector('[data-testid="metric-value-nodes"]')?.textContent?.trim()).toBe('99')

    // Now resolve the first (stale) request with outdated data (10)
    if (resolveFirstRequest) {
      ;(resolveFirstRequest as any)({ total: 10 })
    }
    await nextTick()
    await new Promise((r) => setTimeout(r, 25))

    // Stale response must be discarded; value must remain 99!
    expect(container.querySelector('[data-testid="metric-value-nodes"]')?.textContent?.trim()).toBe('99')
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
    const nodeCard = container.querySelector('[data-testid="metric-card-nodes"]') as HTMLDivElement
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
})
