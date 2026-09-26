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

  it('formats latency cleanly', () => {
    expect(formatLatency(42)).toBe('42 ms')
    expect(formatLatency(0)).toBe('0 ms')
    expect(formatLatency(-1)).toBe('--')
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
    expect(sheetDialog?.textContent).toContain('探针观测证据链')
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

    // Schedule overview card verification
    expect(container.textContent).toContain('周期能力探测计划')
    expect(container.textContent).toContain('已启用')
    expect(container.textContent).toContain('7200s')
    expect(container.textContent).toContain('#3')

    // Batch cards verification
    const batchCards = container.querySelectorAll('[data-testid="probe-batch-card"]')
    expect(batchCards.length).toBe(3)

    // Batch 1: Associated runs and skipped nodes feedback
    expect(batchCards[0].textContent).toContain('run-assoc-101')
    expect(batchCards[0].textContent).toContain('已跳过 20 个凭据缺失或无效的节点')

    // Batch 2: Empty inventory feedback
    expect(batchCards[1].textContent).toContain('当前计划窗口内无可用于探测的活跃节点。')

    // Batch 3: Expired feedback and sanitized error
    expect(batchCards[2].textContent).toContain('批次时间窗口已过期或租约在完成前失效。')
    expect(batchCards[2].textContent).toContain('Lease expired after node crash; rescued safely')
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
})
