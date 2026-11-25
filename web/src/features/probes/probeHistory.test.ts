// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { api } from '../../api/client'
import { setLocale } from '../../locales'
import { toastStore } from '../../ui/toast'
import { useProbeHistory } from './useProbeHistory'
import type { ProbeBatch, ProbeObservation, ProbeRun } from './probeTypes'

function run(id: string): ProbeRun {
  return {
    id, idempotency_key: `key-${id}`, actor_scope: 'default', config_revision: '', state: 'running',
    deadline_at: '2026-11-25T12:00:00Z', created_at: '2026-11-25T11:00:00Z', updated_at: '2026-11-25T11:00:00Z',
  }
}

function observation(id: string, runId: string): ProbeObservation {
  return {
    id, probe_run_id: runId, node_logical_id: 'node-fixture', kind: 'baseline', verdict: 'available',
    evidence_digest: 'digest', observed_at: '2026-11-25T11:00:00Z', latency_ms: 18, redacted_summary: '',
  }
}

function batch(id: string): ProbeBatch {
  return { id, state: 'running', generation: 1, owner: 'fixture', run_ids: [],
    window_at: '2026-11-25T11:00:00Z', created_at: '', updated_at: '',
    counts: { total_nodes: 1, completed_runs: 0, dispatched_runs: 1, skipped_nodes: 0 } }
}

function deferred<T>() {
  let resolve!: (value: T) => void
  const promise = new Promise<T>((done) => { resolve = done })
  return { promise, resolve }
}

const page = <T,>(items: T[], total = items.length) => ({ items, total })
beforeEach(() => {
  vi.restoreAllMocks()
  setLocale('zh-CN')
  vi.spyOn(toastStore, 'push').mockReturnValue('fixture-toast')
})
afterEach(() => vi.restoreAllMocks())

describe('real node probe history owner', () => {
  it.each([[true, false], [false, true], [true, true], [false, false]])(
    'keeps run failure=%s and batch failure=%s independent from valid empty results', async (failRuns, failBatches) => {
      vi.spyOn(api, 'get').mockImplementation(async (path: string) => {
        if ((path.endsWith('/runs') && failRuns) || (path.endsWith('/batches') && failBatches)) {
          throw new Error('history unavailable')
        }
        return page([])
      })
      const history = useProbeHistory()
      await history.refreshHistory()
      for (const [state, failed] of [[history.runs, failRuns], [history.batches, failBatches]] as const) {
        expect(state.loaded).toBe(!failed)
        expect(Boolean(state.error)).toBe(failed)
        expect(state.items).toEqual([])
        expect(state.total).toBe(0)
      }
    })

  it('finishes a successful tab without waiting for the other tab and retains stale rows on refresh failure', async () => {
    const slow = deferred<unknown>()
    const get = vi.spyOn(api, 'get').mockImplementation((path: string) => path.endsWith('/batches')
      ? slow.promise : Promise.resolve(page([run('run-a')])))
    const history = useProbeHistory()
    const refresh = history.refreshHistory()
    await Promise.resolve()
    expect(history.runs.loading).toBe(false)
    expect(history.batches.loading).toBe(true)
    slow.resolve(page([]))
    await refresh
    get.mockRejectedValue(new Error('temporary outage'))
    await history.loadRuns()
    expect(history.runs.items.map((item) => item.id)).toEqual(['run-a'])
    expect(history.runs.stale).toBe(true)
    expect(history.runs.error).toContain('temporary outage')
  })

  it('rejects malformed page data instead of claiming empty history', async () => {
    vi.spyOn(api, 'get').mockResolvedValue({ items: null, total: 0 })
    const history = useProbeHistory()
    await history.loadRuns()
    expect(history.runs.loaded).toBe(false)
    expect(history.runs.error).not.toBe('')
  })

  it('clears previous evidence immediately and ignores A-slow/B-fast and A-B-A responses', async () => {
    const firstA = deferred<unknown>()
    const secondA = deferred<unknown>()
    let aRequests = 0
    vi.spyOn(api, 'get').mockImplementation((path: string) => {
      if (path.includes('/run-a/')) return ++aRequests === 1 ? firstA.promise : secondA.promise
      return Promise.resolve(page([observation('obs-b', 'run-b')]))
    })
    const history = useProbeHistory()
    const pendingA = history.inspectRun(run('run-a'))
    await history.inspectRun(run('run-b'))
    expect(history.observations.items[0].id).toBe('obs-b')
    const latestA = history.inspectRun(run('run-a'))
    expect(history.observations.items).toEqual([])
    expect(history.observations.loading).toBe(true)
    firstA.resolve(page([observation('old-a', 'run-a')]))
    await pendingA
    expect(history.observations.items).toEqual([])
    expect(history.observations.loading).toBe(true)
    secondA.resolve(page([observation('new-a', 'run-a')]))
    await latestA
    expect(history.observations.items[0].id).toBe('new-a')
  })

  it('exposes inspection failures for retry and invalidates pending evidence when closed', async () => {
    const get = vi.spyOn(api, 'get').mockRejectedValueOnce(new Error('evidence unavailable'))
    const history = useProbeHistory()
    await history.inspectRun(run('run-a'))
    expect(history.observations.loaded).toBe(false)
    expect(history.observations.error).toContain('evidence unavailable')
    get.mockResolvedValueOnce(page([observation('retry', 'run-a')]))
    await history.loadObservations()
    expect(history.observations.error).toBe('')
    const pending = deferred<unknown>()
    get.mockReturnValueOnce(pending.promise)
    const loading = history.loadObservations(2)
    history.closeEvidence()
    pending.resolve(page([observation('late', 'run-a')], 51))
    await loading
    expect(history.observations.items).toEqual([])
    expect(history.evidenceOpen.value).toBe(false)
  })

  it('loads run, batch, and evidence pages beyond 100 and retains the successful page on failure', async () => {
    const get = vi.spyOn(api, 'get').mockResolvedValue(page([], 126))
    const history = useProbeHistory()
    await history.loadRuns(5)
    await history.loadBatches(5)
    await history.inspectRun(run('run-page'))
    await history.loadObservations(5)
    for (const endpoint of ['/api/v1/probes/runs', '/api/v1/probes/batches', '/api/v1/probes/runs/run-page/observations']) {
      expect(get).toHaveBeenCalledWith(endpoint, { params: { page: 5, page_size: 25 } })
    }
    get.mockRejectedValueOnce(new Error('page unavailable'))
    await history.loadRuns(6)
    expect(history.runs.page).toBe(5)
    expect(history.runs.requestedPage).toBe(6)
    expect(history.runs.total).toBe(126)
    expect(history.runs.stale).toBe(true)
  })

  it('deduplicates per-ID cancellations while leaving other rows operable and trusts server states', async () => {
    const response = deferred<unknown>()
    const post = vi.spyOn(api, 'post').mockReturnValueOnce(response.promise)
      .mockResolvedValueOnce({ run_id: 'run-other', state: 'cancelled' })
    const history = useProbeHistory()
    history.runs.items = [run('run-cancel'), run('run-other')]
    const first = history.cancelRun('run-cancel')
    await history.cancelRun('run-cancel')
    expect(post).toHaveBeenCalledTimes(1)
    expect(history.cancellingRuns.has('run-cancel')).toBe(true)
    await history.cancelRun('run-other')
    response.resolve({ run_id: 'run-cancel', state: 'succeeded' })
    await first
    expect(history.runs.items.map((item) => item.state)).toEqual(['succeeded', 'cancelled'])
    expect(history.cancellingRuns.size).toBe(0)
  })

  it('does not overwrite a confirmed cancellation with an older list response', async () => {
    const snapshot = deferred<unknown>()
    vi.spyOn(api, 'get').mockReturnValueOnce(snapshot.promise)
    vi.spyOn(api, 'post').mockResolvedValue({ run_id: 'run-cancel', state: 'cancelled' })
    const history = useProbeHistory()
    history.runs.items = [run('run-cancel')]
    const refresh = history.loadRuns()
    await history.cancelRun('run-cancel')
    snapshot.resolve(page([run('run-cancel')]))
    await refresh
    expect(history.runs.items[0].state).toBe('cancelled')
  })

  it('does not invent a cancelled state on failure or a mismatched response ID', async () => {
    const post = vi.spyOn(api, 'post').mockRejectedValueOnce(new Error('terminal_state conflict'))
      .mockResolvedValueOnce({ run_id: 'different-run', state: 'cancelled' })
    const history = useProbeHistory()
    history.runs.items = [run('run-cancel')]
    await history.cancelRun('run-cancel')
    expect(toastStore.push).toHaveBeenCalledWith(expect.objectContaining({ tone: 'error', message: expect.stringContaining('terminal_state conflict') }))
    await history.cancelRun('run-cancel')
    expect(post).toHaveBeenCalledTimes(2)
    expect(history.runs.items[0].state).toBe('running')
    expect(history.cancellingRuns.size).toBe(0)
  })

  it('deduplicates batch cancellation, keeps failures unchanged, and applies the server batch state', async () => {
    const response = deferred<unknown>()
    const post = vi.spyOn(api, 'post').mockReturnValueOnce(response.promise).mockRejectedValueOnce(new Error('batch conflict'))
    const history = useProbeHistory()
    history.batches.items = [batch('batch-cancel'), batch('batch-conflict')]
    const first = history.cancelBatch('batch-cancel')
    await history.cancelBatch('batch-cancel')
    expect(post).toHaveBeenCalledTimes(1)
    response.resolve({ ...batch('batch-cancel'), state: 'succeeded' })
    await first
    await history.cancelBatch('batch-conflict')
    expect(history.batches.items.map((item) => item.state)).toEqual(['succeeded', 'running'])
    expect(history.cancellingBatches.size).toBe(0)
  })
})
