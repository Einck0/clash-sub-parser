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
import ProbesView from './ProbesView.vue'
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

describe('ProbesView Component Interaction & Feedback', () => {
  let container: HTMLDivElement
  let app: ReturnType<typeof createApp> | null = null

  const mockSchedule = {
    enabled: true,
    interval_seconds: 7200,
    kinds: ['baseline' as const, 'ai' as const],
    next_due_at: '2026-09-25T13:00:00Z',
    generation: 3,
    updated_at: '2026-09-25T11:00:00Z',
  }

  const mockBatches: ProbeBatch[] = [
    {
      id: 'batch-active-1',
      window_at: '2026-09-25T11:00:00Z',
      generation: 3,
      owner: 'csp-worker-1',
      state: 'running',
      run_ids: ['run-assoc-101', 'run-assoc-102'],
      counts: {
        total_nodes: 100,
        dispatched_runs: 80,
        completed_runs: 40,
        skipped_nodes: 20,
      },
      created_at: '2026-09-25T11:00:00Z',
      updated_at: '2026-09-25T11:05:00Z',
    },
    {
      id: 'batch-empty-inv',
      window_at: '2026-09-25T09:00:00Z',
      generation: 2,
      owner: 'csp-worker-1',
      state: 'succeeded',
      run_ids: [],
      counts: {
        total_nodes: 0,
        dispatched_runs: 0,
        completed_runs: 0,
        skipped_nodes: 0,
      },
      created_at: '2026-09-25T09:00:00Z',
      updated_at: '2026-09-25T09:00:00Z',
    },
    {
      id: 'batch-expired-1',
      window_at: '2026-09-25T07:00:00Z',
      generation: 1,
      owner: 'csp-worker-old',
      state: 'expired',
      run_ids: ['run-assoc-50'],
      counts: {
        total_nodes: 50,
        dispatched_runs: 50,
        completed_runs: 10,
        skipped_nodes: 0,
      },
      redacted_error: 'Lease expired after node crash; rescued safely',
      created_at: '2026-09-25T07:00:00Z',
      updated_at: '2026-09-25T07:30:00Z',
    },
  ]

  beforeEach(() => {
    container = document.createElement('div')
    document.body.appendChild(container)

    if (!HTMLDialogElement.prototype.showModal) {
      HTMLDialogElement.prototype.showModal = function (this: HTMLDialogElement) {
        this.open = true
      }
    }
    if (!HTMLDialogElement.prototype.close) {
      HTMLDialogElement.prototype.close = function (this: HTMLDialogElement) {
        this.open = false
      }
    }

    vi.spyOn(api, 'get').mockImplementation(async (path: string) => {
      if (path === '/api/v1/probes/runs') {
        return { items: [], total: 0 }
      }
      if (path === '/api/v1/probes/schedule') {
        return { ...mockSchedule }
      }
      if (path === '/api/v1/probes/batches') {
        return { items: [...mockBatches], total: 3 }
      }
      return { items: [], total: 0 }
    })
  })

  afterEach(() => {
    if (app) {
      app.unmount()
      app = null
    }
    container.remove()
    vi.restoreAllMocks()
  })

  async function mountProbesView() {
    app = createApp({
      render() {
        return h(ProbesView)
      },
    })
    app.mount(container)
    await nextTick()
    await new Promise((r) => setTimeout(r, 20))
  }

  it('switches to schedule tab and renders schedule details and execution batches', async () => {
    await mountProbesView()

    // Find and click the Schedule Tab
    const scheduleTab = container.querySelector('[data-testid="schedule-tab"]') as HTMLButtonElement | null
    expect(scheduleTab).not.toBeNull()
    scheduleTab?.click()
    await nextTick()
    await new Promise((r) => setTimeout(r, 20))

    // Schedule overview card verification (human-readable interval instead of raw seconds/generation jargon)
    expect(container.textContent).toContain('周期能力探测计划')
    expect(container.textContent).toContain('已启用')
    expect(container.textContent).toContain('每 2 小时 (120 分钟)')

    // Batch cards verification
    const batchCards = container.querySelectorAll('[data-testid="probe-batch-card"]')
    expect(batchCards.length).toBe(3)

    // Batch 1: Human-readable batch title, sub-task links and skipped nodes feedback
    expect(batchCards[0].textContent).toContain('自动定时批次')
    expect(batchCards[0].textContent).toContain('子任务 #1')
    expect(batchCards[0].textContent).toContain('已跳过 20 个凭据缺失或无效的节点')

    // Batch 2: Empty inventory feedback
    expect(batchCards[1].textContent).toContain('当前计划窗口内无可用于探测的活跃节点。')

    // Batch 3: Expired feedback and sanitized error
    expect(batchCards[2].textContent).toContain('批次时间窗口已超时结束。')
    expect(batchCards[2].textContent).toContain('Lease expired after node crash; rescued safely')
  })

  it('renders 3-tier Node Speed & Availability Workbench, supports one-click probe, filtering, sorting, and human-readable node evidence sheet', async () => {
    const mockNodes = [
      {
        logical_id: 'node-tokyo-01',
        protocol: 'vless',
        display_name: 'Tokyo HighSpeed 01',
        active: true,
        server: '203.0.113.10',
        port: 443,
        latency_ms: 38,
        last_probed_at: new Date().toISOString(),
        health_status: 'healthy' as const,
        probe_missing: false,
        probe_stale: false,
        capabilities: {
          baseline: { verdict: 'available' as const, latency_ms: 38, summary: 'profile=baseline version=baseline-v1 verdict=available reason=contract_matched status=204 latency_ms=38' },
          streaming: { verdict: 'available' as const, latency_ms: 65, summary: 'profile=streaming version=streaming-v1 verdict=available reason=contract_matched status=200 latency_ms=65' },
          ai: { verdict: 'available' as const, latency_ms: 72 },
        },
        ip_risk_summary: { risk_band: 'low' as const, decision: 'allow' as const, status: 'fresh' as const },
        sources: [{ node_logical_id: 'node-tokyo-01', subscription_id: 'sub-main', last_seen_fetch_id: 'f1' }],
      },
      {
        logical_id: 'node-us-slow',
        protocol: 'trojan',
        display_name: 'US West Relay 02',
        active: true,
        server: '198.51.100.55',
        port: 8443,
        latency_ms: 210,
        last_probed_at: new Date().toISOString(),
        health_status: 'degraded' as const,
        probe_missing: false,
        probe_stale: false,
        capabilities: {
          baseline: { verdict: 'available' as const, latency_ms: 210 },
          streaming: { verdict: 'restricted' as const, latency_ms: 240 },
          ai: { verdict: 'error' as const, latency_ms: 0 },
        },
        sources: [{ node_logical_id: 'node-us-slow', subscription_id: 'sub-backup', last_seen_fetch_id: 'f2' }],
      },
    ]

    vi.spyOn(api, 'get').mockImplementation(async (path: string) => {
      if (path === '/api/v1/nodes') {
        return { items: mockNodes, page: 1, page_size: 100, total: 2 }
      }
      if (path === '/api/v1/subscriptions') {
        return { items: [{ id: 'sub-main', name: '主力专线订阅' }, { id: 'sub-backup', name: '备用美西订阅' }], total: 2 }
      }
      if (path === '/api/v1/nodes/node-tokyo-01/observations') {
        return {
          items: [
            {
              id: 'obs-tokyo-1',
              probe_run_id: 'run-1',
              node_logical_id: 'node-tokyo-01',
              kind: 'baseline',
              verdict: 'available',
              evidence_digest: 'sha256:hidden',
              observed_at: '2026-09-26T10:00:00Z',
              latency_ms: 38,
              redacted_summary: 'profile=baseline version=baseline-v1 verdict=available reason=contract_matched status=204 latency_ms=38',
            },
          ],
          page: 1,
          page_size: 100,
          total: 1,
        }
      }
      if (path === '/api/v1/probes/runs') return { items: [], total: 0 }
      if (path === '/api/v1/probes/schedule') return { ...mockSchedule }
      if (path === '/api/v1/probes/batches') return { items: [], total: 0 }
      return { items: [], total: 0 }
    })

    const postSpy = vi.spyOn(api, 'post').mockResolvedValue({
      run_id: 'run-quick-1',
      state: 'running',
      deadline_at: '2026-09-26T10:10:00Z',
    })

    await mountProbesView()

    // Tier 1: KPI Summary Bar
    const kpiBar = container.querySelector('[data-testid="probe-kpi-bar"]')
    expect(kpiBar).not.toBeNull()
    expect(kpiBar?.textContent).toContain('在线可用率')
    expect(kpiBar?.textContent).toContain('100%')
    expect(kpiBar?.textContent).toContain('124 ms') // avg of 38 and 210

    // Tier 3: Node Probe Workbench Table
    const rows = container.querySelectorAll('[data-testid="probe-node-row"]')
    expect(rows.length).toBe(2)
    expect(rows[0].textContent).toContain('Tokyo HighSpeed 01')
    expect(rows[0].textContent).toContain('38 ms')
    expect(rows[0].textContent).toContain('主力专线订阅')
    // The same node's detail and reprobe actions remain in the node row for both
    // the desktop table and its narrow-screen card presentation.
    expect(rows[0].querySelector('[data-testid="row-inspect-btn"]')).not.toBeNull()
    expect(rows[0].querySelector('[data-testid="row-reprobe-btn"]')).not.toBeNull()
    expect(rows[0].querySelector('[data-label="探测结果"]')).not.toBeNull()
    expect(rows[0].querySelector('[data-label="快捷操作"]')).not.toBeNull()
    const nodeCell = rows[0].querySelector('[data-label="节点 / 协议与入口"]') as HTMLElement
    expect(nodeCell.className).toContain('min-w-0')
    expect(nodeCell.querySelector('.font-semibold')?.className).toContain('break-all')
    const sourceBadge = nodeCell.querySelector('[data-testid="probe-node-source-badge"]') as HTMLElement
    expect(sourceBadge).not.toBeNull()
    expect(sourceBadge.className).toContain('break-all')
    expect(sourceBadge.className).toContain('h-auto')
    const actionsCell = rows[0].querySelector('[data-label="快捷操作"]') as HTMLElement
    expect(actionsCell.className).toContain('whitespace-normal')
    expect(nodeCell.className).toContain('probe-node-identity')
    expect(actionsCell.querySelectorAll('.probe-node-action-button')).toHaveLength(2)
    expect(actionsCell.querySelector('[data-testid="row-inspect-btn"]')?.className).toContain('probe-node-action-button')
    expect(actionsCell.querySelector('[data-testid="row-reprobe-btn"]')?.className).toContain('probe-node-action-button')

    // One-click full probe without config_revision modal
    const fullProbeBtn = container.querySelector('[data-testid="quick-full-probe-btn"]') as HTMLButtonElement | null
    expect(fullProbeBtn).not.toBeNull()
    fullProbeBtn?.click()
    await nextTick()
    await new Promise((r) => setTimeout(r, 20))

    expect(postSpy).toHaveBeenCalledWith(
      '/api/v1/probes/runs',
      expect.objectContaining({
        config_revision: '',
        node_logical_ids: [],
        kinds: expect.arrayContaining(['baseline', 'streaming', 'ai']),
      }),
      expect.any(Object)
    )

    // Open node detail sheet & verify structured Chinese explanation without raw key=value noise
    const inspectBtn = rows[0].querySelector('[data-testid="row-inspect-btn"]') as HTMLButtonElement | null
    inspectBtn?.click()
    await nextTick()
    await new Promise((r) => setTimeout(r, 20))

    const sheet = document.body.querySelector('[data-testid="probe-evidence-sheet"]')
    expect(sheet).not.toBeNull()
    expect(sheet?.textContent).toContain('Tokyo HighSpeed 01')
    expect(sheet?.textContent).toContain('HTTP 204 连通正常')
    expect(sheet?.textContent).toContain('协议握手与响应校验通过')
    expect(sheet?.textContent).not.toContain('profile=baseline version=baseline-v1')
  })

  it('constrains unbroken long node names and source badges while keeping row inspect and reprobe actions operable', async () => {
    const longNodeName = 'LONG_UNBROKEN_NODE_' + 'N'.repeat(240)
    const longSourceName = 'LONG_UNBROKEN_SOURCE_' + 'S'.repeat(240)

    vi.spyOn(api, 'get').mockImplementation(async (path: string) => {
      if (path.startsWith('/api/v1/nodes')) {
        return {
          items: [
            {
              logical_id: 'node-long-unbroken',
              display_name: longNodeName,
              protocol: 'vless',
              server: 'edge-long.example.com',
              port: 443,
              health_status: 'healthy',
              probe_state: 'idle',
              latency_ms: 42,
              last_probed_at: '2026-09-26T10:00:00Z',
              probe_stale: false,
              probe_missing: false,
              sources: [{ subscription_id: 'sub-long', raw_name: longNodeName }],
              capabilities: {
                baseline: { verdict: 'available', latency_ms: 42, observed_at: '2026-09-26T10:00:00Z', stale: false },
              },
            },
          ],
          page: 1,
          page_size: 200,
          total: 1,
        }
      }
      if (path.startsWith('/api/v1/subscriptions')) {
        return {
          items: [{ id: 'sub-long', name: longSourceName }],
          total: 1,
        }
      }
      if (path === '/api/v1/probes/runs') return { items: [], total: 0 }
      if (path === '/api/v1/probes/schedule') return { ...mockSchedule }
      if (path === '/api/v1/probes/batches') return { items: [], total: 0 }
      return { items: [], total: 0 }
    })

    const postSpy = vi.spyOn(api, 'post').mockResolvedValue({
      run_id: 'run-single-long',
      state: 'queued',
      deadline_at: '2026-09-26T10:10:00Z',
    })

    await mountProbesView()

    const row = container.querySelector('[data-testid="probe-node-row"]') as HTMLElement
    expect(row).not.toBeNull()
    const identityCell = row.querySelector('[data-label="节点 / 协议与入口"]') as HTMLElement
    expect(identityCell.className).toContain('probe-node-identity')
    expect(identityCell.className).toContain('min-w-0')

    const sourceBadge = identityCell.querySelector('[data-testid="probe-node-source-badge"]') as HTMLElement
    expect(sourceBadge).not.toBeNull()
    expect(sourceBadge.textContent).toContain(longSourceName)
    expect(sourceBadge.className).toContain('h-auto')
    expect(sourceBadge.className).toContain('whitespace-normal')
    expect(sourceBadge.className).toContain('break-all')

    const reprobeBtn = row.querySelector('[data-testid="row-reprobe-btn"]') as HTMLButtonElement
    const inspectBtn = row.querySelector('[data-testid="row-inspect-btn"]') as HTMLButtonElement
    expect(reprobeBtn.className).toContain('probe-node-action-button')
    expect(inspectBtn.className).toContain('probe-node-action-button')

    reprobeBtn.click()
    await nextTick()
    await new Promise((r) => setTimeout(r, 20))
    expect(postSpy).toHaveBeenCalledWith(
      '/api/v1/probes/runs',
      expect.objectContaining({
        node_logical_ids: ['node-long-unbroken'],
      }),
      expect.any(Object)
    )
  })

  it('opens schedule configuration modal, updates values and submits PUT /api/v1/probes/schedule', async () => {
    const putSpy = vi.spyOn(api, 'put').mockResolvedValueOnce({
      ...mockSchedule,
      enabled: false,
      interval_seconds: 3600,
    })

    await mountProbesView()

    // Switch to schedule tab
    const scheduleTab = container.querySelector('[data-testid="schedule-tab"]') as HTMLButtonElement | null
    scheduleTab?.click()
    await nextTick()
    await new Promise((r) => setTimeout(r, 20))

    // Click Configure Schedule button
    const configBtn = container.querySelector('[data-testid="configure-schedule-btn"]') as HTMLButtonElement | null
    expect(configBtn).not.toBeNull()
    configBtn?.click()
    await nextTick()
    await new Promise((r) => setTimeout(r, 20))

    // Dialog should be present
    const allDialogs = Array.from(document.body.querySelectorAll('dialog'))
    const dialog = allDialogs.find((d) => d.textContent?.includes('配置周期探测计划'))
    expect(dialog).toBeDefined()
    expect(dialog?.textContent).toContain('配置周期探测计划')

    // Toggle enabled checkbox
    const toggle = dialog?.querySelector('input[type="checkbox"]') as HTMLInputElement | null
    expect(toggle).not.toBeNull()
    toggle?.click()
    await nextTick()

    // Save schedule
    const saveBtn = Array.from(dialog?.querySelectorAll('button') || []).find((b) => b.textContent?.includes('保存周期计划'))
    expect(saveBtn).toBeDefined()
    saveBtn?.click()
    await nextTick()

    expect(putSpy).toHaveBeenCalledWith('/api/v1/probes/schedule', expect.objectContaining({
      enabled: false,
    }))
  })

  it('cancels active batch upon user click and updates state from server response', async () => {
    const postSpy = vi.spyOn(api, 'post').mockResolvedValueOnce({
      ...mockBatches[0],
      state: 'cancelled',
    })

    await mountProbesView()

    // Switch to schedule tab
    const scheduleTab = container.querySelector('[data-testid="schedule-tab"]') as HTMLButtonElement | null
    scheduleTab?.click()
    await nextTick()
    await new Promise((r) => setTimeout(r, 20))

    const batchCards = container.querySelectorAll('[data-testid="probe-batch-card"]')
    const cancelBtn = Array.from(batchCards[0].querySelectorAll('button')).find((b) => b.textContent?.includes('取消批次'))
    expect(cancelBtn).toBeDefined()
    cancelBtn?.click()
    await nextTick()

    expect(postSpy).toHaveBeenCalledWith('/api/v1/probes/batches/batch-active-1/cancel')
  })

  it('handles 401 unauthorized via auth callback and 403/500 via ErrorStateCard', async () => {
    let authCallbackTriggered = false
    api.setOnUnauthorized(() => {
      authCallbackTriggered = true
    })

    vi.spyOn(api, 'get').mockImplementation(async (path: string) => {
      if (path === '/api/v1/probes/schedule') {
        const err = new ApiError(401, 'unauthorized', 'Invalid admin token')
        ;(api as any).onUnauthorizedCallback?.()
        throw err
      }
      throw new ApiError(500, 'internal_error', 'Server connection failure')
    })

    await mountProbesView()

    // 401 should invoke registered unauthorized callback
    expect(authCallbackTriggered).toBe(true)

    // Error alert card should be displayed
    const errorCard = container.querySelector('[data-testid="error-state-card"]')
    expect(errorCard).not.toBeNull()
    expect(errorCard?.textContent).toContain('Server connection failure')
  })

  it('guarantees zero secret leakage in rendered UI and batch evidence', async () => {
    await mountProbesView()

    // Switch to schedule tab
    const scheduleTab = container.querySelector('[data-testid="schedule-tab"]') as HTMLButtonElement | null
    scheduleTab?.click()
    await nextTick()
    await new Promise((r) => setTimeout(r, 20))

    const fullText = container.textContent || ''
    // Ensure no secret tokens, passwords, private keys, or raw bearer credentials leaked in DOM
    expect(fullText).not.toMatch(/bearer\s+[a-zA-Z0-9_\-\.]+/i)
    expect(fullText).not.toMatch(/password/i)
    expect(fullText).not.toMatch(/private_key/i)
    expect(fullText).not.toMatch(/BEGIN RSA PRIVATE KEY/)
  })

  it('renders Node Pool 5 core metrics dashboard, probing/queued badges, status filter linkage, manual preemption feedback, and periodic deduplication trigger', async () => {
    const poolSnapshot = {
      queue_nodes_count: 2,
      probing_count: 1,
      queued_waiting_count: 1,
      untested_count: 1,
      total_count: 4,
      unavailable_count: 1,
      available_count: 2,
      healthy_count: 1,
      degraded_count: 1,
      probing_node_ids: ['node-hk-probing'],
      queued_node_ids: ['node-sg-queued'],
      updated_at: '2026-09-27T03:05:00Z',
    }

    const mockPoolNodes = [
      {
        logical_id: 'node-hk-probing',
        protocol: 'vless',
        display_name: 'HK Pool Probing 01',
        active: true,
        server: '203.0.113.1',
        port: 443,
        latency_ms: 32,
        probe_state: 'probing' as const,
        health_status: 'probing' as const,
        capabilities: { baseline: { verdict: 'available' as const, latency_ms: 32 } },
      },
      {
        logical_id: 'node-sg-queued',
        protocol: 'trojan',
        display_name: 'SG Pool Queued 02',
        active: true,
        server: '203.0.113.2',
        port: 443,
        latency_ms: 140,
        probe_state: 'queued' as const,
        health_status: 'degraded' as const,
        capabilities: { baseline: { verdict: 'restricted' as const, latency_ms: 140 } },
      },
      {
        logical_id: 'node-us-down',
        protocol: 'ss',
        display_name: 'US Down 03',
        active: true,
        server: '203.0.113.3',
        port: 8388,
        latency_ms: 0,
        probe_state: 'idle' as const,
        health_status: 'unhealthy' as const,
        capabilities: { baseline: { verdict: 'error' as const, latency_ms: 0 } },
      },
      {
        logical_id: 'node-kr-untested',
        protocol: 'hysteria2',
        display_name: 'KR Untested 04',
        active: true,
        server: '203.0.113.4',
        port: 8443,
        probe_state: 'idle' as const,
        probe_missing: true,
        capabilities: {},
      },
    ]

    vi.spyOn(api, 'get').mockImplementation(async (path: string) => {
      if (path === '/api/v1/probes/pool') return { ...poolSnapshot }
      if (path === '/api/v1/nodes') return { items: mockPoolNodes, page: 1, page_size: 100, total: 4 }
      if (path === '/api/v1/probes/runs') return { items: [], total: 0 }
      if (path === '/api/v1/probes/schedule') return { ...mockSchedule }
      if (path === '/api/v1/probes/batches') return { items: [], total: 0 }
      return { items: [], total: 0 }
    })

    const postSpy = vi.spyOn(api, 'post').mockImplementation(async (path: string) => {
      if (path === '/api/v1/probes/schedule/trigger') {
        return {
          ...poolSnapshot,
          queue_nodes_count: 3,
          queued_waiting_count: 2,
          queued_node_ids: ['node-sg-queued', 'node-kr-untested'],
        }
      }
      return {
        run_id: 'run-preempt-1',
        state: 'running',
        deadline_at: '2026-09-27T03:15:00Z',
      }
    })

    await mountProbesView()

    // Verify 3x2 spacious grid class on probe-pool-dashboard (no xl:grid-cols-6 cramping)
    const poolDashboard = container.querySelector('[data-testid="probe-pool-dashboard"]') as HTMLElement | null
    expect(poolDashboard?.className).toContain('lg:grid-cols-3')
    expect(poolDashboard?.className).not.toContain('xl:grid-cols-6')

    // Verify 5 core metrics cards in Node Pool dashboard
    const metricQueue = container.querySelector('[data-testid="pool-metric-queue"]') as HTMLElement | null
    const metricTotal = container.querySelector('[data-testid="pool-metric-total"]') as HTMLElement | null
    const metricAvailable = container.querySelector('[data-testid="pool-metric-available"]') as HTMLElement | null
    const metricUnavailable = container.querySelector('[data-testid="pool-metric-unavailable"]') as HTMLElement | null
    const metricUntested = container.querySelector('[data-testid="pool-metric-untested"]') as HTMLElement | null

    expect(metricQueue?.textContent).toContain('当前队列中的节点数')
    expect(metricQueue?.textContent).toContain('2')
    expect(metricQueue?.textContent).toContain('检测中 1')
    expect(metricQueue?.textContent).toContain('排队等待 1')
    expect(metricTotal?.textContent).toContain('总数')
    expect(metricTotal?.textContent).toContain('4')
    expect(metricTotal?.textContent).toContain('86 ms') // avg of 32 and 140, excluding 0ms failed node
    expect(metricAvailable?.textContent).toContain('可用数')
    expect(metricAvailable?.textContent).toContain('2')
    expect(metricUnavailable?.textContent).toContain('不可用数')
    expect(metricUnavailable?.textContent).toContain('1')
    expect(metricUntested?.textContent).toContain('未测数')
    expect(metricUntested?.textContent).toContain('1')

    // Verify table rows show "检测中" and "队列中" badges, and failed node shows "--" (not "0 ms")
    const allRows = container.querySelectorAll('[data-testid="probe-node-row"]')
    expect(allRows.length).toBe(4)
    expect(allRows[0].textContent).toContain('HK Pool Probing 01')
    expect(allRows[0].textContent).toContain('检测中')
    expect(allRows[1].textContent).toContain('SG Pool Queued 02')
    expect(allRows[1].textContent).toContain('队列中')
    const failedRow = Array.from(allRows).find((r) => r.textContent?.includes('US Down 03'))
    expect(failedRow?.textContent).not.toContain('0 ms')
    expect(failedRow?.querySelector('[data-testid="probe-node-latency-badge"]')?.textContent?.trim()).toBe('--')

    // Click "当前队列中的节点数" metric card to filter by probing/queued nodes
    metricQueue?.click()
    await nextTick()
    const probingRows = container.querySelectorAll('[data-testid="probe-node-row"]')
    expect(probingRows.length).toBe(2)

    // Click "未测数" metric card to filter by untested nodes
    metricUntested?.click()
    await nextTick()
    const untestedRows = container.querySelectorAll('[data-testid="probe-node-row"]')
    expect(untestedRows.length).toBe(1)
    expect(untestedRows[0].textContent).toContain('KR Untested 04')

    // Click "立即重测" on the untested node and verify front-of-queue preemption feedback
    const reprobeBtn = untestedRows[0].querySelector('[data-testid="row-reprobe-btn"]') as HTMLButtonElement | null
    reprobeBtn?.click()
    await nextTick()
    await new Promise((r) => setTimeout(r, 20))

    expect(postSpy).toHaveBeenCalledWith(
      '/api/v1/probes/runs',
      expect.objectContaining({
        node_logical_ids: ['node-kr-untested'],
      }),
      expect.any(Object)
    )
    const feedbackEl = container.querySelector('[data-testid="pool-action-feedback"]')
    expect(feedbackEl?.textContent).toContain('插队至节点池最前面优先检测')

    // Trigger periodic deduplicated pool enqueue
    const periodicTriggerBtn = container.querySelector('[data-testid="trigger-periodic-pool-btn"]') as HTMLButtonElement | null
    expect(periodicTriggerBtn).not.toBeNull()
    periodicTriggerBtn?.click()
    await nextTick()
    await new Promise((r) => setTimeout(r, 20))

    expect(postSpy).toHaveBeenCalledWith('/api/v1/probes/schedule/trigger')
    expect(feedbackEl?.textContent).toContain('自动去重跳过')
  })

  it('renders pool schedule card with 10-minute sweep interval and per-node validity, and supports idle baseline refresh and tracking', async () => {
    let getCalls: string[] = []
    vi.spyOn(api, 'get').mockImplementation(async (path: string) => {
      getCalls.push(path)
      if (path === '/api/v1/probes/schedule') return { ...mockSchedule }
      if (path === '/api/v1/probes/pool') return { queue_nodes_count: 0, probing_count: 0, queued_waiting_count: 0, untested_count: 0, total_count: 2, unavailable_count: 0, available_count: 2, healthy_count: 2, degraded_count: 0, probing_node_ids: [], queued_node_ids: [], updated_at: new Date().toISOString() }
      if (path === '/api/v1/probes/batches') return { items: mockBatches, total: mockBatches.length, page: 1, page_size: 20 }
      if (path === '/api/v1/probes/runs') return { items: [], total: 0, page: 1, page_size: 50 }
      if (path === '/api/v1/nodes') return { items: [], total: 0, page: 1, page_size: 100 }
      return {}
    })

    vi.spyOn(api, 'post').mockImplementation(async (path: string) => {
      if (path === '/api/v1/probes/schedule/trigger') {
        return { queue_nodes_count: 2, probing_count: 0, queued_waiting_count: 2, untested_count: 0, total_count: 2, unavailable_count: 0, available_count: 2, healthy_count: 2, degraded_count: 0, probing_node_ids: [], queued_node_ids: ['node-1'], updated_at: new Date().toISOString() }
      }
      return {}
    })

    await mountProbesView()

    // 1. Verify pool schedule card text
    const poolCard = container.querySelector('[data-testid="pool-schedule-card"]')
    expect(poolCard).not.toBeNull()
    expect(poolCard?.textContent).toContain('每 10 分钟巡检过期节点')
    expect(poolCard?.textContent).toContain('每节点/类别有效期=每 2 小时')
    expect(poolCard?.textContent).toContain('下次扫描：')

    // 2. Trigger periodic pool enqueue and verify tracking and batch/runs refresh
    const triggerBtn = container.querySelector('[data-testid="trigger-periodic-pool-btn"]') as HTMLButtonElement | null
    expect(triggerBtn).not.toBeNull()
    getCalls = []
    triggerBtn?.click()
    await nextTick()
    await new Promise((r) => setTimeout(r, 50))

    expect(getCalls).toContain('/api/v1/probes/batches')
  })

  it('does not poll /api/v1/nodes on each 2-second heartbeat during active probe runs', async () => {
    const getCalls: string[] = []

    vi.spyOn(api, 'get').mockImplementation(async (path: string) => {
      getCalls.push(path)
      if (path === '/api/v1/probes/schedule') return { ...mockSchedule }
      if (path === '/api/v1/probes/pool') {
        return {
          queue_nodes_count: 5,
          probing_count: 2,
          queued_waiting_count: 3,
          untested_count: 0,
          total_count: 10,
          unavailable_count: 0,
          available_count: 10,
          healthy_count: 10,
          degraded_count: 0,
          probing_node_ids: ['node-1'],
          queued_node_ids: ['node-2'],
          updated_at: new Date().toISOString(),
        }
      }
      if (path === '/api/v1/probes/batches') return { items: [], total: 0, page: 1, page_size: 20 }
      if (path === '/api/v1/probes/runs') {
        return {
          items: [{ id: 'run-active-1', state: 'running', kinds: ['baseline'], total_nodes: 5, completed_nodes: 1, deadline_at: new Date(Date.now() + 60000).toISOString() }],
          total: 1,
          page: 1,
          page_size: 50,
        }
      }
      if (path.includes('/observations')) return { items: [], total: 0 }
      if (path === '/api/v1/nodes') return { items: [], total: 0, page: 1, page_size: 100 }
      return { items: [], total: 0 }
    })

    vi.useFakeTimers({ toFake: ['setInterval', 'clearInterval'] })
    await mountProbesView()
    // Initial mount calls loadProbeNodes once
    const initialNodesCalls = getCalls.filter((p) => p === '/api/v1/nodes').length
    expect(initialNodesCalls).toBeGreaterThanOrEqual(1)

    getCalls.length = 0
    await vi.advanceTimersByTimeAsync(6000)

    // During active probing, heartbeat MUST NOT query /api/v1/nodes repeatedly
    const heartbeatNodeCalls = getCalls.filter((p) => p === '/api/v1/nodes').length
    expect(heartbeatNodeCalls).toBe(0)

    // But active runs and pool status MUST be polled
    expect(getCalls.filter((p) => p === '/api/v1/probes/pool').length).toBeGreaterThanOrEqual(2)
    expect(getCalls.filter((p) => p === '/api/v1/probes/runs').length).toBeGreaterThanOrEqual(2)

    vi.useRealTimers()
  })
})
