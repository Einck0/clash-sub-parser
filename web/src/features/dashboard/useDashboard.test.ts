import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { useDashboard } from './useDashboard'
import { api } from '../../api/client'

describe('useDashboard composable', () => {
  beforeEach(() => {
    vi.restoreAllMocks()
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('initializes with idle metrics and checking health', () => {
    const { metrics, healthStatus, healthLatency } = useDashboard()

    expect(healthStatus.value).toBe('checking')
    expect(healthLatency.value).toBeNull()

    expect(metrics.value.subscriptions.value).toBeNull()
    expect(metrics.value.subscriptions.status).toBe('idle')
    expect(metrics.value.nodes.value).toBeNull()
    expect(metrics.value.activeNodes.value).toBeNull()
    expect(metrics.value.policies.value).toBeNull()
    expect(metrics.value.rules.value).toBeNull()
    expect(metrics.value.probes.value).toBeNull()
  })

  it('updates health telemetry with measured latency on checkHealth success', async () => {
    vi.spyOn(api, 'get').mockResolvedValueOnce({ status: 'ok' })

    const { healthStatus, healthLatency, checkHealth } = useDashboard()

    await checkHealth()

    expect(healthStatus.value).toBe('healthy')
    expect(typeof healthLatency.value).toBe('number')
    expect(healthLatency.value).toBeGreaterThanOrEqual(0)
  })

  it('sets health to unhealthy and latency to null on checkHealth failure', async () => {
    vi.spyOn(api, 'get').mockRejectedValueOnce(new Error('Connection refused'))

    const { healthStatus, healthLatency, checkHealth } = useDashboard()

    await checkHealth()

    expect(healthStatus.value).toBe('unhealthy')
    expect(healthLatency.value).toBeNull()
  })

  it('loads all stats successfully via Promise.allSettled', async () => {
    vi.spyOn(api, 'get').mockImplementation(async (path: string, options?: any) => {
      if (path === '/api/v1/subscriptions') return { total: 5 }
      if (path === '/api/v1/nodes') {
        if (options?.params?.active_only === 'true') {
          return { total: 12 }
        }
        return { total: 20 }
      }
      if (path === '/api/v1/policies/groups') return { total: 4 }
      if (path === '/api/v1/policies/rules') return { total: 100 }
      if (path === '/api/v1/probes/runs') return { total: 30 }
      return {}
    })

    const { metrics, loadStats } = useDashboard()

    await loadStats()

    expect(metrics.value.subscriptions).toEqual({
      value: 5,
      status: 'success',
      error: null,
      isStale: false,
    })
    expect(metrics.value.nodes).toEqual({
      value: 20,
      status: 'success',
      error: null,
      isStale: false,
    })
    expect(metrics.value.activeNodes).toEqual({
      value: 12,
      status: 'success',
      error: null,
      isStale: false,
    })
    expect(metrics.value.policies).toEqual({
      value: 4,
      status: 'success',
      error: null,
      isStale: false,
    })
    expect(metrics.value.rules).toEqual({
      value: 100,
      status: 'success',
      error: null,
      isStale: false,
    })
    expect(metrics.value.probes).toEqual({
      value: 30,
      status: 'success',
      error: null,
      isStale: false,
    })
  })

  it('sets metric to error and value to null on initial partial failure without showing 0', async () => {
    vi.spyOn(api, 'get').mockImplementation(async (path: string) => {
      if (path === '/api/v1/subscriptions') return { total: 10 }
      if (path === '/api/v1/nodes') throw new Error('Nodes backend crashed')
      return { total: 0 }
    })

    const { metrics, loadStats } = useDashboard()

    await loadStats()

    expect(metrics.value.subscriptions.value).toBe(10)
    expect(metrics.value.subscriptions.status).toBe('success')

    // Nodes failed on initial load: must NOT be 0!
    expect(metrics.value.nodes.value).toBeNull()
    expect(metrics.value.nodes.status).toBe('error')
    expect(metrics.value.nodes.error).toContain('Nodes backend crashed')
    expect(metrics.value.nodes.isStale).toBe(false)
  })

  it('preserves cached value on refresh failure and marks isStale true', async () => {
    let failRules = false
    vi.spyOn(api, 'get').mockImplementation(async (path: string) => {
      if (path === '/api/v1/policies/rules') {
        if (failRules) throw new Error('Database locked')
        return { total: 75 }
      }
      return { total: 1 }
    })

    const { metrics, loadStats } = useDashboard()

    // 1. First load succeeds
    await loadStats()
    expect(metrics.value.rules.value).toBe(75)
    expect(metrics.value.rules.status).toBe('success')
    expect(metrics.value.rules.isStale).toBe(false)

    // 2. Refresh fails for rules
    failRules = true
    await loadStats(true) // isRefresh = true

    // Preserved cached value!
    expect(metrics.value.rules.value).toBe(75)
    expect(metrics.value.rules.status).toBe('error')
    expect(metrics.value.rules.error).toContain('Database locked')
    expect(metrics.value.rules.isStale).toBe(true)
  })

  it('discards stale out-of-order stats response', async () => {
    let resolveFirstCall: ((val: any) => void) | null = null

    let callCount = 0
    vi.spyOn(api, 'get').mockImplementation(async (path: string) => {
      if (path === '/api/v1/subscriptions') {
        callCount++
        if (callCount === 1) {
          // Slow first call
          return new Promise((resolve) => {
            resolveFirstCall = resolve
          })
        }
        // Fast second call
        return { total: 88 }
      }
      return { total: 1 }
    })

    const { metrics, loadStats } = useDashboard()

    // Dispatch request 1 (slow)
    const p1 = loadStats()

    // Dispatch request 2 (fast)
    const p2 = loadStats(true)
    await p2

    // Request 2 resolved
    expect(metrics.value.subscriptions.value).toBe(88)

    // Now request 1 resolves with outdated data (7)
    if (resolveFirstCall) {
      ;(resolveFirstCall as any)({ total: 7 })
    }
    await p1

    // Outdated request 1 was discarded, value remains 88
    expect(metrics.value.subscriptions.value).toBe(88)
  })
})
