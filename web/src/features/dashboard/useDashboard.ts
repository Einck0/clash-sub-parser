import { ref } from 'vue'
import { api } from '../../api/client'
import { uiText } from '../../locales'
import { displayText } from '../../utils/displayNames'
import type { DashboardMetrics, HealthStatus } from './dashboardTypes'

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

  function applyMetricResult(key: keyof DashboardMetrics, res: PromiseSettledResult<unknown>): void {
    const current = metrics.value[key]
    const total = res.status === 'fulfilled' && res.value && typeof res.value === 'object' && 'total' in res.value
      ? res.value.total
      : null
    if (typeof total === 'number' && Number.isSafeInteger(total) && total >= 0) {
      metrics.value[key] = { value: total, status: 'success', error: null, isStale: false }
      return
    }

    // A page's item count is not a total; unavailable data must never masquerade as zero.
    const error = res.status === 'rejected'
      ? displayText(res.reason?.message || uiText('请求失败', 'Request failed'))
      : uiText('返回的统计总数无效', 'Invalid response total')
    metrics.value[key] = {
      value: current.value,
      status: 'error',
      error,
      isStale: current.value !== null,
    }
  }

  async function checkHealth(): Promise<void> {
    const seq = ++healthSeq
    refreshingHealth.value = true
    const start = Date.now()
    try {
      await api.get('/healthz')
      if (seq !== healthSeq) return
      healthStatus.value = 'healthy'
      healthLatency.value = Math.max(0, Date.now() - start)
    } catch {
      if (seq !== healthSeq) return
      healthStatus.value = 'unhealthy'
      healthLatency.value = null
    } finally {
      if (seq === healthSeq) refreshingHealth.value = false
    }
  }

  async function loadStats(isRefresh = false): Promise<void> {
    const seq = ++statsSeq
    isRefreshingStats.value = true
    for (const metric of Object.values(metrics.value)) {
      if (!isRefresh || metric.value === null) metric.status = 'loading'
    }

    const requests: Array<[keyof DashboardMetrics, () => Promise<unknown>]> = [
      ['subscriptions', () => api.get('/api/v1/subscriptions', { params: { page: 1, page_size: 1 } })],
      ['nodes', () => api.get('/api/v1/nodes', {
        params: { page: 1, page_size: 1, scope: 'enabled_subscriptions' },
      })],
      ['activeNodes', () => api.get('/api/v1/nodes', {
        params: { page: 1, page_size: 1, scope: 'enabled_subscriptions', active_only: 'true' },
      })],
      ['policies', () => api.get('/api/v1/policies/groups', { params: { page: 1, page_size: 1 } })],
      ['rules', () => api.get('/api/v1/policies/rules', { params: { page: 1, page_size: 1 } })],
      ['probes', () => api.get('/api/v1/probes/runs', { params: { page: 1, page_size: 1 } })],
    ]
    try {
      await Promise.allSettled(requests.map(async ([key, request]) => {
        // Publish each section as it settles; one stalled service cannot hide the others.
        try {
          const value = await request()
          if (seq === statsSeq) applyMetricResult(key, { status: 'fulfilled', value })
        } catch (reason) {
          if (seq === statsSeq) applyMetricResult(key, { status: 'rejected', reason })
        }
      }))
    } finally {
      if (seq === statsSeq) isRefreshingStats.value = false
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
