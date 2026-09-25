import { ref } from 'vue'
import { api } from '../../api/client'
import {
  generateIdempotencyKey,
  type CreateProbeRunInput,
  type CreateProbeRunResponse,
  type ProbeBatch,
  type ProbeObservation,
  type ProbeRun,
  type ProbeRunState,
  type ProbeSchedule,
} from './probeTypes'

interface PaginatedResult<T> {
  items: T[]
  page: number
  page_size: number
  total: number
}

export function useProbes() {
  const runs = ref<ProbeRun[]>([])
  const activeRun = ref<ProbeRun | null>(null)
  const observations = ref<ProbeObservation[]>([])
  const schedule = ref<ProbeSchedule | null>(null)
  const batches = ref<ProbeBatch[]>([])
  const loadingRuns = ref(false)
  const loadingObservations = ref(false)
  const loadingSchedule = ref(false)
  const loadingBatches = ref(false)
  const savingSchedule = ref(false)
  const cancellingBatch = ref(false)
  const submitting = ref(false)
  const cancelling = ref(false)
  const error = ref('')
  const totalRuns = ref(0)
  const totalBatches = ref(0)

  async function loadSchedule() {
    loadingSchedule.value = true
    error.value = ''
    try {
      const res = await api.get<ProbeSchedule>('/api/v1/probes/schedule')
      schedule.value = res
      return res
    } catch (err) {
      error.value = err instanceof Error ? err.message : 'Failed to load probe schedule'
      return null
    } finally {
      loadingSchedule.value = false
    }
  }

  async function updateSchedule(data: {
    enabled?: boolean
    interval_seconds?: number
    kinds?: ProbeSchedule['kinds']
  }): Promise<ProbeSchedule> {
    savingSchedule.value = true
    error.value = ''
    try {
      const updated = await api.put<ProbeSchedule>('/api/v1/probes/schedule', data)
      schedule.value = updated
      return updated
    } catch (err) {
      const msg = err instanceof Error ? err.message : 'Failed to update probe schedule'
      error.value = msg
      throw err
    } finally {
      savingSchedule.value = false
    }
  }

  async function loadBatches(page = 1) {
    loadingBatches.value = true
    error.value = ''
    try {
      const res = await api.get<PaginatedResult<ProbeBatch>>('/api/v1/probes/batches', {
        params: { page, page_size: 20 },
      })
      batches.value = res.items || []
      totalBatches.value = res.total || 0
      return res
    } catch (err) {
      error.value = err instanceof Error ? err.message : 'Failed to load probe batches'
      return null
    } finally {
      loadingBatches.value = false
    }
  }

  async function getBatch(batchId: string): Promise<ProbeBatch | null> {
    try {
      return await api.get<ProbeBatch>(`/api/v1/probes/batches/${batchId}`)
    } catch (err) {
      const msg = err instanceof Error ? err.message : 'Failed to get probe batch'
      error.value = msg
      throw err
    }
  }

  async function cancelBatch(batchId: string) {
    cancellingBatch.value = true
    error.value = ''
    try {
      const res = await api.post<ProbeBatch>(`/api/v1/probes/batches/${batchId}/cancel`)
      const found = batches.value.find((b) => b.id === batchId)
      if (found && res) {
        Object.assign(found, res)
      }
      return res
    } catch (err) {
      const msg = err instanceof Error ? err.message : 'Failed to cancel probe batch'
      error.value = msg
      throw err
    } finally {
      cancellingBatch.value = false
    }
  }

  async function loadRuns(stateFilter?: ProbeRunState) {
    loadingRuns.value = true
    error.value = ''
    try {
      const params: Record<string, string | number> = { page: 1, page_size: 50 }
      if (stateFilter) {
        params.state = stateFilter
      }
      const res = await api.get<PaginatedResult<ProbeRun>>('/api/v1/probes/runs', { params })
      runs.value = res.items || []
      totalRuns.value = res.total || 0
    } catch (err) {
      error.value = err instanceof Error ? err.message : 'Failed to load probe runs'
    } finally {
      loadingRuns.value = false
    }
  }

  async function createRun(input: CreateProbeRunInput): Promise<CreateProbeRunResponse> {
    submitting.value = true
    error.value = ''
    try {
      const idempotencyKey = generateIdempotencyKey()
      const body: Record<string, unknown> = {
        config_revision: input.config_revision || '',
        node_logical_ids: input.node_logical_ids || [],
        kinds: input.kinds || ['baseline', 'geo', 'streaming', 'ai', 'speed', 'ip_risk'],
      }
      if (input.deadline_minutes && input.deadline_minutes > 0) {
        const deadline = new Date(Date.now() + input.deadline_minutes * 60 * 1000)
        body.deadline = deadline.toISOString()
      }

      const res = await api.post<CreateProbeRunResponse>('/api/v1/probes/runs', body, {
        headers: {
          'Idempotency-Key': idempotencyKey,
        },
      })
      await loadRuns()
      return res
    } catch (err) {
      const msg = err instanceof Error ? err.message : 'Failed to trigger probe run'
      error.value = msg
      throw err
    } finally {
      submitting.value = false
    }
  }

  async function cancelRun(runId: string) {
    cancelling.value = true
    error.value = ''
    try {
      await api.post<{ run_id: string; state: ProbeRunState }>(`/api/v1/probes/runs/${runId}/cancel`)
      const found = runs.value.find((r) => r.id === runId)
      if (found) {
        found.state = 'cancelled'
      }
      if (activeRun.value?.id === runId) {
        activeRun.value.state = 'cancelled'
      }
    } catch (err) {
      error.value = err instanceof Error ? err.message : 'Failed to cancel probe run'
      throw err
    } finally {
      cancelling.value = false
    }
  }

  async function loadObservations(runId: string) {
    loadingObservations.value = true
    error.value = ''
    try {
      const res = await api.get<PaginatedResult<ProbeObservation>>(`/api/v1/probes/runs/${runId}/observations`, {
        params: { page: 1, page_size: 100 },
      })
      observations.value = res.items || []
    } catch (err) {
      error.value = err instanceof Error ? err.message : 'Failed to load observations'
    } finally {
      loadingObservations.value = false
    }
  }

  async function fetchRunDetail(runId: string): Promise<ProbeRun | null> {
    try {
      const run = await api.get<ProbeRun>(`/api/v1/probes/runs/${runId}`)
      const idx = runs.value.findIndex((r) => r.id === runId)
      if (idx >= 0) {
        runs.value[idx] = run
      }
      if (activeRun.value?.id === runId) {
        activeRun.value = run
      }
      return run
    } catch (err) {
      return null
    }
  }

  return {
    runs,
    activeRun,
    observations,
    schedule,
    batches,
    loadingRuns,
    loadingObservations,
    loadingSchedule,
    loadingBatches,
    savingSchedule,
    cancellingBatch,
    submitting,
    cancelling,
    error,
    totalRuns,
    totalBatches,
    loadRuns,
    createRun,
    cancelRun,
    loadObservations,
    fetchRunDetail,
    loadSchedule,
    updateSchedule,
    loadBatches,
    getBatch,
    cancelBatch,
  }
}
