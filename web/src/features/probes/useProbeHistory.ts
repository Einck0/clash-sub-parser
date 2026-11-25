import { reactive, ref } from 'vue'
import { api } from '../../api/client'
import { uiText } from '../../locales'
import { displayText } from '../../utils/displayNames'
import { toastStore } from '../../ui/toast'
import type { ProbeBatch, ProbeObservation, ProbeRun, ProbeRunState } from './probeTypes'

interface HistoryPage<T> {
  items: T[]
  total: number
}

function historyError(error: unknown): string {
  return displayText(error instanceof Error ? error.message : uiText('检测历史请求失败，请重试', 'Could not load probe history. Try again.'))
}

function createHistoryPage<T>(endpoint: () => string) {
  // 页码只在成功后切换，加载下一页失败时不会把上一页冒充新页
  const state = reactive({
    items: [] as T[], total: 0, page: 1, requestedPage: 1, pageSize: 25,
    loading: false, loaded: false, stale: false, error: '',
  })
  let sequence = 0

  async function load(page = state.page) {
    const request = ++sequence
    state.loading = true
    state.requestedPage = page
    state.error = ''
    state.stale = state.loaded
    try {
      const response = await api.get<HistoryPage<T>>(endpoint(), {
        params: { page, page_size: state.pageSize },
      })
      if (request !== sequence) return
      if (!Array.isArray(response?.items) || !Number.isInteger(response.total) || response.total < 0) {
        throw new Error(uiText('检测历史响应格式错误，请重试', 'Invalid probe history response. Try again.'))
      }
      const lastPage = Math.max(1, Math.ceil(response.total / state.pageSize))
      if (page > lastPage) {
        await load(lastPage)
        return
      }
      Object.assign(state, { items: response.items, total: response.total, page, loaded: true, stale: false })
    } catch (error) {
      if (request === sequence) state.error = historyError(error)
    } finally {
      if (request === sequence) state.loading = false
    }
  }

  function reset() {
    sequence++
    Object.assign(state, { items: [], total: 0, page: 1, requestedPage: 1, loading: false, loaded: false, stale: false, error: '' })
  }

  function invalidatePending() {
    if (!state.loading) return
    // 取消结果晚于列表快照，丢弃旧列表响应以免将终态改回运行中
    sequence++
    state.loading = false
    state.stale = state.loaded
  }

  return { state, load, reset, invalidatePending }
}

export function useProbeHistory() {
  const runPage = createHistoryPage<ProbeRun>(() => '/api/v1/probes/runs')
  const batchPage = createHistoryPage<ProbeBatch>(() => '/api/v1/probes/batches')
  const inspectingRun = ref<ProbeRun | null>(null)
  const evidenceOpen = ref(false)
  const observationPage = createHistoryPage<ProbeObservation>(() =>
    `/api/v1/probes/runs/${encodeURIComponent(inspectingRun.value!.id)}/observations`)
  const cancellingRuns = reactive(new Set<string>())
  const cancellingBatches = reactive(new Set<string>())

  async function refreshHistory() {
    // 各页自行记录错误，不会把部分请求失败当作空历史
    await Promise.all([runPage.load(), batchPage.load()])
  }

  async function inspectRun(run: ProbeRun) {
    observationPage.reset()
    inspectingRun.value = run
    evidenceOpen.value = true
    await observationPage.load(1)
  }

  function closeEvidence() {
    evidenceOpen.value = false
    observationPage.reset()
    inspectingRun.value = null
  }

  async function cancelRun(id: string) {
    if (cancellingRuns.has(id)) return
    cancellingRuns.add(id)
    try {
      const response = await api.post<{ run_id: string; state: ProbeRunState }>(
        `/api/v1/probes/runs/${encodeURIComponent(id)}/cancel`)
      if (response?.run_id !== id || !['queued', 'running', 'succeeded', 'failed', 'cancelled', 'expired'].includes(response.state)) {
        throw new Error(uiText('取消响应无法确认任务状态，请刷新历史', 'Cancellation response could not confirm the run state. Refresh history.'))
      }
      runPage.invalidatePending()
      const run = runPage.state.items.find((item) => item.id === id)
      if (run) run.state = response.state
      if (inspectingRun.value?.id === id) inspectingRun.value = { ...inspectingRun.value, state: response.state }
      toastStore.push({
        message: response.state === 'cancelled'
          ? uiText('已取消检测任务', 'Probe run cancelled')
          : uiText('已读回任务状态，任务未取消', 'Run state received; the run was not cancelled'),
        tone: response.state === 'cancelled' ? 'info' : 'warning',
      })
    } catch (error) {
      toastStore.push({ message: historyError(error), tone: 'error' })
    } finally {
      cancellingRuns.delete(id)
    }
  }

  async function cancelBatch(id: string) {
    if (cancellingBatches.has(id)) return
    cancellingBatches.add(id)
    try {
      const response = await api.post<Pick<ProbeBatch, 'id' | 'state'>>(
        `/api/v1/probes/batches/${encodeURIComponent(id)}/cancel`)
      if (response?.id !== id || !['pending', 'running', 'succeeded', 'failed', 'cancelled', 'expired'].includes(response.state)) {
        throw new Error(uiText('取消响应无法确认批次状态，请刷新历史', 'Cancellation response could not confirm the batch state. Refresh history.'))
      }
      batchPage.invalidatePending()
      const batch = batchPage.state.items.find((item) => item.id === id)
      if (batch) batch.state = response.state
      toastStore.push({
        message: response.state === 'cancelled'
          ? uiText('已取消巡检批次', 'Inspection batch cancelled')
          : uiText('已读回批次状态，批次未取消', 'Batch state received; the batch was not cancelled'),
        tone: response.state === 'cancelled' ? 'info' : 'warning',
      })
    } catch (error) {
      toastStore.push({ message: historyError(error), tone: 'error' })
    } finally {
      cancellingBatches.delete(id)
    }
  }

  return {
    runs: runPage.state, batches: batchPage.state, observations: observationPage.state,
    loadRuns: runPage.load, loadBatches: batchPage.load, loadObservations: observationPage.load,
    refreshHistory, inspectingRun, evidenceOpen, inspectRun, closeEvidence,
    cancellingRuns, cancellingBatches, cancelRun, cancelBatch,
  }
}
