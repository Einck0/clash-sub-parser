import { ref } from 'vue'
import { api } from '../../api/client'
import {
  generateIdempotencyKey,
  type CreateProbeRunInput,
  type CreateProbeRunResponse,
  type ProbeObservation,
  type ProbeRun,
  type ProbeRunState,
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
  const loadingRuns = ref(false)
  const loadingObservations = ref(false)
  const submitting = ref(false)
  const cancelling = ref(false)
  const error = ref('')
  const totalRuns = ref(0)

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
    loadingRuns,
    loadingObservations,
    submitting,
    cancelling,
    error,
    totalRuns,
    loadRuns,
    createRun,
    cancelRun,
    loadObservations,
    fetchRunDetail,
  }
}
