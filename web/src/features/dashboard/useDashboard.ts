import { ref } from 'vue'
import { api } from '../../api/client'
import type { DashboardMetrics, HealthStatus, MetricItem } from './dashboardTypes'

export function useDashboard() {
  const metrics = ref<DashboardMetrics>({
    subscriptions: { value: null, status: 'idle', error: null, isStale: false },
    nodes: { value: null, status: 'idle', error: null, isStale: false },
    activeNodes: { value: null, status: 'idle', error: null, isStale: false },
    policies: { value: null, status: 'idle', error: null, isStale: false },
    rules: { value: null, status: 'idle', error: null, isStale: false },
    probes: { value: null, status: 'idle', error: null, isStale: false },
  })

  const healthStatus = ref<HealthStatus>('checking')
  const healthLatency = ref<number | null>(null)
  const refreshingHealth = ref(false)
  const isRefreshingStats = ref(false)

  let statsSeq = 0
  let healthSeq = 0

  function extractTotal(val: any): number | null {
    if (val && typeof val.total === 'number') return val.total
    if (val && Array.isArray(val.items)) return val.items.length
    if (val && Array.isArray(val.policy_rules)) return val.policy_rules.length
    return null
  }

  function applyMetricResult(
    key: keyof DashboardMetrics,
    res: PromiseSettledResult<any>,
    isRefresh: boolean
  ): void {
    const current = metrics.value[key]

    if (res.status === 'fulfilled') {
      const total = extractTotal(res.value)
      if (total !== null) {
        metrics.value[key] = {
          value: total,
          status: 'success',
          error: null,
          isStale: false,
        }
        return
      }
    }

    // Either promise rejected, or returned value lacked a valid total
    const errorMsg =
      res.status === 'rejected'
        ? res.reason?.message || 'Request failed'
        : 'Invalid response format'

    if (isRefresh && current.value !== null) {
      // Preserve old value on refresh failure and flag as stale
      metrics.value[key] = {
        value: current.value,
        status: 'error',
        error: errorMsg,
        isStale: true,
      }
    } else {
      // Initial failure: do not display 0!
      metrics.value[key] = {
        value: null,
        status: 'error',
        error: errorMsg,
        isStale: false,
      }
    }
  }

  async function checkHealth(): Promise<void> {
    const seq = ++healthSeq
    refreshingHealth.value = true
    const start = Date.now()

    try {
      await api.get('/healthz')
      if (seq !== healthSeq) return // Stale request discarded
      healthStatus.value = 'healthy'
      healthLatency.value = Math.max(0, Date.now() - start)
    } catch {
      if (seq !== healthSeq) return // Stale request discarded
      healthStatus.value = 'unhealthy'
      healthLatency.value = null
    } finally {
      if (seq === healthSeq) {
        refreshingHealth.value = false
      }
    }
  }

  async function loadStats(isRefresh = false): Promise<void> {
    const seq = ++statsSeq
    if (!isRefresh) {
      for (const key of Object.keys(metrics.value) as Array<keyof DashboardMetrics>) {
        metrics.value[key].status = 'loading'
      }
    } else {
      isRefreshingStats.value = true
    }

    try {
      const [subsRes, nodesRes, activeNodesRes, policiesRes, rulesRes, probesRes] =
        await Promise.allSettled([
          api.get<{ total: number }>('/api/v1/subscriptions', { params: { page: 1, page_size: 1 } }),
          api.get<{ total: number }>('/api/v1/nodes', {
            params: { page: 1, page_size: 1, scope: 'enabled_subscriptions' },
          }),
          api.get<{ total: number }>('/api/v1/nodes', {
            params: { page: 1, page_size: 1, scope: 'enabled_subscriptions', active_only: 'true' },
          }),
          api.get<{ total: number }>('/api/v1/policies/groups', {
            params: { page: 1, page_size: 1 },
          }),
          api.get<{ total: number }>('/api/v1/policies/rules', {
            params: { page: 1, page_size: 1 },
          }),
          api.get<{ total: number }>('/api/v1/probes/runs', { params: { page: 1, page_size: 1 } }),
        ])

      // Stale response discarded: a newer request was dispatched while this was in flight
      if (seq !== statsSeq) {
        return
      }

      applyMetricResult('subscriptions', subsRes, isRefresh)
      applyMetricResult('nodes', nodesRes, isRefresh)
      applyMetricResult('activeNodes', activeNodesRes, isRefresh)
      applyMetricResult('policies', policiesRes, isRefresh)
      applyMetricResult('rules', rulesRes, isRefresh)
      applyMetricResult('probes', probesRes, isRefresh)
    } finally {
      if (seq === statsSeq) {
        isRefreshingStats.value = false
      }
    }
  }

  async function refreshAll(): Promise<void> {
    await Promise.allSettled([checkHealth(), loadStats(true)])
  }

  return {
    metrics,
    healthStatus,
    healthLatency,
    refreshingHealth,
    isRefreshingStats,
    checkHealth,
    loadStats,
    refreshAll,
  }
}
