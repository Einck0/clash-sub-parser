// @vitest-environment jsdom
import { describe, expect, it, vi, beforeEach } from 'vitest'
import {
  probeStateTone,
  probeVerdictTone,
  formatLatency,
  generateIdempotencyKey,
  type ProbeRun,
  type ProbeObservation,
} from './probeTypes'
import { useProbes } from './useProbes'
import { api } from '../../api/client'

describe('probes types and helpers', () => {
  it('maps probe run states to accurate badge tones', () => {
    expect(probeStateTone('queued')).toBe('warning')
    expect(probeStateTone('running')).toBe('primary')
    expect(probeStateTone('succeeded')).toBe('success')
    expect(probeStateTone('failed')).toBe('error')
    expect(probeStateTone('cancelled')).toBe('neutral')
    expect(probeStateTone('expired')).toBe('neutral')
  })

  it('maps probe verdicts to accurate badge tones', () => {
    expect(probeVerdictTone('available')).toBe('success')
    expect(probeVerdictTone('restricted')).toBe('warning')
    expect(probeVerdictTone('unknown')).toBe('info')
    expect(probeVerdictTone('error')).toBe('error')
    expect(probeVerdictTone('stale')).toBe('neutral')
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

  it('renders ProbeEvidenceSheet with fluid responsive max-height', async () => {
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

    testApp.unmount()
    mountEl.remove()
  })
})
