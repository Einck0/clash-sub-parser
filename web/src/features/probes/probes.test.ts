// @vitest-environment jsdom
import { describe, expect, it, vi, beforeEach, afterEach } from 'vitest'
import { createApp, h, nextTick } from 'vue'
import {
  probeKindLabel,
  probeStateLabel,
  probeStateTone,
  probeVerdictLabel,
  probeVerdictTone,
  probeBatchStateTone,
  formatLatency,
  generateIdempotencyKey,
  parseRedactedSummary,
  intervalLabel,
  latencyTone,
  type ProbeRun,
  type ProbeObservation,
  type ProbeBatch,
} from './probeTypes'
import { useProbes } from './useProbes'
import { api, ApiError } from '../../api/client'

describe('probes types and helpers', () => {
  it('maps probe run states to accurate badge tones and Chinese labels', () => {
    expect(probeStateTone('queued')).toBe('warning')
    expect(probeStateTone('running')).toBe('primary')
    expect(probeStateTone('succeeded')).toBe('success')
    expect(probeStateTone('failed')).toBe('error')
    expect(probeStateTone('cancelled')).toBe('neutral')
    expect(probeStateTone('expired')).toBe('neutral')

    expect(probeStateLabel('pending')).toBe('等待中')
    expect(probeStateLabel('queued')).toBe('排队中')
    expect(probeStateLabel('running')).toBe('运行中')
    expect(probeStateLabel('succeeded')).toBe('已完成')
    expect(probeStateLabel('failed')).toBe('已失败')
    expect(probeStateLabel('cancelled')).toBe('已取消')
    expect(probeStateLabel('expired')).toBe('已过期')
  })

  it('maps probe verdicts and kinds to accurate badge tones and Chinese labels', () => {
    expect(probeVerdictTone('available')).toBe('success')
    expect(probeVerdictTone('restricted')).toBe('warning')
    expect(probeVerdictTone('unknown')).toBe('info')
    expect(probeVerdictTone('error')).toBe('error')
    expect(probeVerdictTone('stale')).toBe('neutral')

    expect(probeVerdictLabel('available')).toBe('可用')
    expect(probeVerdictLabel('restricted')).toBe('降级/受限')
    expect(probeVerdictLabel('unknown')).toBe('未知')
    expect(probeVerdictLabel('error')).toBe('不可达')
    expect(probeVerdictLabel('stale')).toBe('已过期')

    expect(probeKindLabel('baseline')).toBe('基础连通性')
    expect(probeKindLabel('geo')).toBe('地域与出口 IP')
    expect(probeKindLabel('streaming')).toBe('流媒体解锁')
    expect(probeKindLabel('ai')).toBe('AI 服务')
    expect(probeKindLabel('speed')).toBe('带宽测速')
    expect(probeKindLabel('ip_risk')).toBe('IP 风险')
  })

  it('formats latency cleanly and parses machine redacted_summary into human-readable Chinese labels', () => {
    expect(formatLatency(42)).toBe('42 ms')
    expect(formatLatency(0)).toBe('0 ms')
    expect(formatLatency(-1)).toBe('--')

    expect(latencyTone(42)).toBe('success')
    expect(latencyTone(180)).toBe('warning')
    expect(latencyTone(320)).toBe('error')
    expect(latencyTone(null)).toBe('neutral')

    expect(intervalLabel(900)).toBe('每 15 分钟')
    expect(intervalLabel(3600)).toBe('每 1 小时')
    expect(intervalLabel(86400)).toBe('每 1 天')

    const parsed = parseRedactedSummary(
      'profile=baseline version=baseline-v1 verdict=available reason=contract_matched status=204 latency_ms=42'
    )
    expect(parsed.profile).toBe('baseline')
    expect(parsed.statusCode).toBe(204)
    expect(parsed.latencyMs).toBe(42)
    expect(parsed.reasonLabel).toBe('协议握手与响应校验通过')
  })

  it('generates non-empty unique idempotency keys', () => {
    const key1 = generateIdempotencyKey()
    const key2 = generateIdempotencyKey()
    expect(key1).toBeTruthy()
    expect(key2).toBeTruthy()
    expect(key1).not.toBe(key2)
  })
})

describe('useProbes composable', () => {
  beforeEach(() => {
    vi.restoreAllMocks()
  })

  it('loads runs from API and updates state', async () => {
    const mockRuns: ProbeRun[] = [
      {
        id: 'run-1',
        idempotency_key: 'idem-1',
        actor_scope: 'default',
        config_revision: 'rev-1',
        state: 'succeeded',
        deadline_at: '2026-09-16T12:00:00Z',
        created_at: '2026-09-16T10:00:00Z',
        updated_at: '2026-09-16T10:05:00Z',
      },
    ]

    vi.spyOn(api, 'get').mockResolvedValueOnce({
      items: mockRuns,
      page: 1,
      page_size: 50,
      total: 1,
    })

    const { runs, loadingRuns, loadRuns } = useProbes()
    await loadRuns()

    expect(loadingRuns.value).toBe(false)
    expect(runs.value).toHaveLength(1)
    expect(runs.value[0].id).toBe('run-1')
  })

  it('creates a new probe run with generated idempotency key and handles API response', async () => {
    const mockCreated = {
      run_id: 'run-new-123',
      state: 'queued',
      deadline_at: '2026-09-16T12:10:00Z',
    }

    const postSpy = vi.spyOn(api, 'post').mockResolvedValueOnce(mockCreated)
    vi.spyOn(api, 'get').mockResolvedValueOnce({
      items: [
        {
          id: 'run-new-123',
          idempotency_key: 'idem-auto',
          actor_scope: 'default',
          config_revision: '',
          state: 'queued',
          deadline_at: '2026-09-16T12:10:00Z',
          created_at: '2026-09-16T10:00:00Z',
          updated_at: '2026-09-16T10:00:00Z',
        },
      ],
      page: 1,
      page_size: 50,
      total: 1,
    })

    const { createRun } = useProbes()
    const result = await createRun({ kinds: ['baseline', 'streaming'] })

    expect(postSpy).toHaveBeenCalledWith(
      '/api/v1/probes/runs',
      expect.objectContaining({
        kinds: ['baseline', 'streaming'],
      }),
      expect.objectContaining({
        headers: expect.objectContaining({
          'Idempotency-Key': expect.any(String),
        }),
      })
    )
    expect(result.run_id).toBe('run-new-123')
  })

  it('cancels an active run via POST /api/v1/probes/runs/{id}/cancel', async () => {
    const postSpy = vi.spyOn(api, 'post').mockResolvedValueOnce({
      run_id: 'run-to-cancel',
      state: 'cancelled',
    })

    const { cancelRun, runs } = useProbes()
    runs.value = [
      {
        id: 'run-to-cancel',
        idempotency_key: 'key-1',
        actor_scope: 'default',
        config_revision: '',
        state: 'running',
        deadline_at: '2026-09-16T12:00:00Z',
        created_at: '2026-09-16T10:00:00Z',
        updated_at: '2026-09-16T10:00:00Z',
      },
    ]

    await cancelRun('run-to-cancel')
    expect(postSpy).toHaveBeenCalledWith('/api/v1/probes/runs/run-to-cancel/cancel')
    expect(runs.value[0].state).toBe('cancelled')
  })

  it('loads run observations via GET /api/v1/probes/runs/{id}/observations', async () => {
    const mockObservations: ProbeObservation[] = [
      {
        id: 'obs-1',
        probe_run_id: 'run-1',
        node_logical_id: 'node-tokyo-1',
        kind: 'streaming',
        verdict: 'available',
        evidence_digest: 'sha256:abc1234',
        observed_at: '2026-09-16T10:01:00Z',
        latency_ms: 56,
        redacted_summary: 'Netflix: available; YouTube: available',
      },
    ]

    vi.spyOn(api, 'get').mockResolvedValueOnce({
      items: mockObservations,
      page: 1,
      page_size: 50,
      total: 1,
    })

    const { observations, loadObservations } = useProbes()
    await loadObservations('run-1')

    expect(observations.value).toHaveLength(1)
    expect(observations.value[0].verdict).toBe('available')
    expect(observations.value[0].latency_ms).toBe(56)
  })

  it('renders ProbeEvidenceSheet with fluid responsive max-height and Chinese labels', async () => {
    const { default: ProbeEvidenceSheet } = await import('./ProbeEvidenceSheet.vue')
    const { createApp, h, nextTick } = await import('vue')

    const mountEl = document.createElement('div')
    document.body.appendChild(mountEl)
    const testApp = createApp({
      render() {
        return h(ProbeEvidenceSheet, {
          open: true,
          run: {
            id: 'run-1',
            idempotency_key: 'key-1',
            actor_scope: 'default',
            config_revision: '',
            state: 'succeeded',
            deadline_at: '2026-09-16T12:00:00Z',
            created_at: '2026-09-16T10:00:00Z',
            updated_at: '2026-09-16T10:00:00Z',
          },
          observations: [],
          loading: false,
        })
      },
    })
    testApp.mount(mountEl)
    await nextTick()

    const sheetDialog = mountEl.querySelector('[role="dialog"]')
    expect(sheetDialog).not.toBeNull()
    expect(sheetDialog?.className).toContain('adaptive-surface-sheet')
    expect(sheetDialog?.className).toContain('md:max-h-[82vh]')
    expect(sheetDialog?.textContent).toContain('节点测速与可用性报告')
    expect(sheetDialog?.textContent).toContain('已完成')
    expect(sheetDialog?.textContent).toContain('暂无观测记录')

    testApp.unmount()
    mountEl.remove()
  })

  it('loads, enables and updates periodic probe schedule', async () => {
    const mockSchedule = {
      enabled: false,
      interval_seconds: 3600,
      kinds: ['baseline' as const],
      next_due_at: '2026-09-25T11:00:00Z',
      generation: 1,
      updated_at: '2026-09-25T10:00:00Z',
    }

    vi.spyOn(api, 'get').mockResolvedValueOnce(mockSchedule)
    const putSpy = vi.spyOn(api, 'put').mockResolvedValueOnce({
      ...mockSchedule,
      enabled: true,
      interval_seconds: 1800,
    })

    const { schedule, loadSchedule, updateSchedule } = useProbes()
    await loadSchedule()

    expect(schedule.value?.enabled).toBe(false)
    expect(schedule.value?.interval_seconds).toBe(3600)

    const updated = await updateSchedule({ enabled: true, interval_seconds: 1800 })
    expect(putSpy).toHaveBeenCalledWith('/api/v1/probes/schedule', {
      enabled: true,
      interval_seconds: 1800,
    })
    expect(updated.enabled).toBe(true)
    expect(updated.interval_seconds).toBe(1800)
  })

  it('loads periodic batches and cancels active batch', async () => {
    const mockBatches = [
      {
        id: 'batch-1',
        window_at: '2026-09-25T10:00:00Z',
        generation: 1,
        owner: 'test-runner',
        state: 'running' as const,
        run_ids: ['run-1', 'run-2'],
        counts: {
          total_nodes: 50,
          dispatched_runs: 50,
          completed_runs: 25,
          skipped_nodes: 0,
        },
        created_at: '2026-09-25T10:00:00Z',
        updated_at: '2026-09-25T10:00:00Z',
      },
    ]

    vi.spyOn(api, 'get').mockResolvedValueOnce({
      items: mockBatches,
      total: 1,
      page: 1,
      page_size: 20,
    })

    const postSpy = vi.spyOn(api, 'post').mockResolvedValueOnce({
      ...mockBatches[0],
      state: 'cancelled',
    })

    const { batches, loadBatches, cancelBatch } = useProbes()
    await loadBatches()

    expect(batches.value).toHaveLength(1)
    expect(batches.value[0].state).toBe('running')

    await cancelBatch('batch-1')
    expect(postSpy).toHaveBeenCalledWith('/api/v1/probes/batches/batch-1/cancel')
    expect(batches.value[0].state).toBe('cancelled')
  })

  it('surfaces errors visibly when loadSchedule or loadBatches fails with 5xx or network error', async () => {
    vi.spyOn(api, 'get').mockImplementation(async (path: string) => {
      if (path === '/api/v1/probes/schedule') {
        throw new ApiError(500, 'internal_error', 'Database connection refused')
      }
      if (path === '/api/v1/probes/batches') {
        throw new Error('Network timeout loading batches')
      }
      return { items: [], total: 0 }
    })

    const { error, loadSchedule, loadBatches } = useProbes()

    const schedRes = await loadSchedule()
    expect(schedRes).toBeNull()
    expect(error.value).toBe('Database connection refused')

    const batchRes = await loadBatches()
    expect(batchRes).toBeNull()
    expect(error.value).toBe('Network timeout loading batches')
  })

  it('fetches single batch detail and handles cancel error cleanly', async () => {
    const mockBatch: ProbeBatch = {
      id: 'batch-detail-1',
      window_at: '2026-09-25T11:00:00Z',
      generation: 2,
      owner: 'csp-runner',
      state: 'running',
      run_ids: ['run-x'],
      counts: { total_nodes: 10, dispatched_runs: 10, completed_runs: 5, skipped_nodes: 0 },
      created_at: '2026-09-25T11:00:00Z',
      updated_at: '2026-09-25T11:00:00Z',
    }

    vi.spyOn(api, 'get').mockResolvedValueOnce(mockBatch)
    vi.spyOn(api, 'post').mockRejectedValueOnce(new ApiError(409, 'batch_conflict', 'Batch is already finalized'))

    const { getBatch, cancelBatch, error } = useProbes()

    const fetched = await getBatch('batch-detail-1')
    expect(fetched?.id).toBe('batch-detail-1')

    await expect(cancelBatch('batch-detail-1')).rejects.toThrow('Batch is already finalized')
    expect(error.value).toBe('Batch is already finalized')
  })

  it('loads ProbePoolStatus from GET /api/v1/probes/pool, falls back to local nodes when offline, and triggers POST /api/v1/probes/schedule/trigger', async () => {
    const serverPool = {
      queue_nodes_count: 5,
      probing_count: 2,
      queued_waiting_count: 3,
      untested_count: 3,
      total_count: 10,
      unavailable_count: 2,
      available_count: 5,
      healthy_count: 4,
      degraded_count: 1,
      probing_node_ids: ['node-hk-01', 'node-jp-02'],
      queued_node_ids: ['node-sg-03', 'node-us-04', 'node-kr-05'],
      updated_at: '2026-09-27T03:00:00Z',
    }

    vi.spyOn(api, 'get').mockImplementation(async (path: string) => {
      if (path === '/api/v1/probes/pool') {
        return serverPool
      }
      if (path === '/api/v1/probes/batches') {
        return { items: [], total: 0 }
      }
      if (path === '/api/v1/nodes') {
        return { items: [], total: 0 }
      }
      return {}
    })

    const postSpy = vi.spyOn(api, 'post').mockResolvedValueOnce({
      ...serverPool,
      queue_nodes_count: 7,
      queued_waiting_count: 5,
    })

    const { poolStatus, probingNodeIds, queuedNodeIds, loadPoolStatus, triggerPeriodicPoolEnqueue } = useProbes()
    const loaded = await loadPoolStatus()

    expect(loaded.queue_nodes_count).toBe(5)
    expect(loaded.probing_count).toBe(2)
    expect(loaded.queued_waiting_count).toBe(3)
    expect(loaded.untested_count).toBe(3)
    expect(loaded.total_count).toBe(10)
    expect(loaded.unavailable_count).toBe(2)
    expect(loaded.available_count).toBe(5)
    expect(probingNodeIds.value.has('node-hk-01')).toBe(true)
    expect(queuedNodeIds.value.has('node-sg-03')).toBe(true)

    const triggered = await triggerPeriodicPoolEnqueue()
    expect(postSpy).toHaveBeenCalledWith('/api/v1/probes/schedule/trigger')
    expect(triggered?.queue_nodes_count).toBe(7)
    expect(poolStatus.value.queue_nodes_count).toBe(7)
  })
})
