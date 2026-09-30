import { computed, ref } from 'vue'
import { api } from '../../api/client'
import { normalizeNode, nodeUnderlyingHealthCategory, type NodeRecord, type NormalizedNode } from '../nodes/nodeView'
import {
  generateIdempotencyKey,
  type CreateProbeRunInput,
  type CreateProbeRunResponse,
  type ProbeBatch,
  type ProbeKind,
  type ProbeObservation,
  type ProbePoolStatus,
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

interface SubscriptionLite {
  id: string
  name: string
  enabled?: boolean
}

export function useProbes() {
  const runs = ref<ProbeRun[]>([])
  const activeRun = ref<ProbeRun | null>(null)
  const observations = ref<ProbeObservation[]>([])
  const nodeObservations = ref<ProbeObservation[]>([])
  const schedule = ref<ProbeSchedule | null>(null)
  const batches = ref<ProbeBatch[]>([])
  const probeNodes = ref<NormalizedNode[]>([])
  const subscriptions = ref<SubscriptionLite[]>([])
  const probingNodeIds = ref<Set<string>>(new Set())
  const queuedNodeIds = ref<Set<string>>(new Set())
  const serverPoolStatus = ref<ProbePoolStatus | null>(null)
  const loadingPoolStatus = ref(false)
  const triggeringSchedule = ref(false)

  const loadingRuns = ref(false)
  const loadingObservations = ref(false)
  const loadingNodeObservations = ref(false)
  const loadingSchedule = ref(false)
  const loadingBatches = ref(false)
  const loadingNodes = ref(false)
  const savingSchedule = ref(false)
  const cancellingBatch = ref(false)
  const submitting = ref(false)
  const cancelling = ref(false)
  const error = ref('')
  const totalRuns = ref(0)
  const totalBatches = ref(0)
  const totalNodes = ref(0)

  const nodeMap = computed<Record<string, NormalizedNode>>(() => {
    const map: Record<string, NormalizedNode> = {}
    for (const node of probeNodes.value) {
      map[node.logicalId] = node
    }
    return map
  })

  const subscriptionNameMap = computed<Record<string, string>>(() => {
    const map: Record<string, string> = {}
    for (const sub of subscriptions.value) {
      map[sub.id] = sub.name || sub.id
    }
    return map
  })

  async function loadProbeNodes() {
    loadingNodes.value = true
    try {
      const collected: NormalizedNode[] = []
      let page = 1
      const pageSize = 100
      let expectedTotal = 0

      while (page <= 20) {
        const res = await api.get<PaginatedResult<NodeRecord>>('/api/v1/nodes', {
          params: {
            page,
            page_size: pageSize,
            active_only: 'true',
            sort_by: 'display_name',
            sort_order: 'asc',
          },
        })
        const items = Array.isArray(res?.items) ? res.items : []
        expectedTotal = typeof res?.total === 'number' ? res.total : items.length
        for (const raw of items) {
          const normalized = normalizeNode(raw)
          if (normalized.probeState === 'idle') {
            if (probingNodeIds.value.has(normalized.logicalId)) {
              normalized.probeState = 'probing'
            } else if (queuedNodeIds.value.has(normalized.logicalId)) {
              normalized.probeState = 'queued'
            }
          }
          collected.push(normalized)
        }
        if (items.length === 0 || collected.length >= expectedTotal || items.length < pageSize) {
          break
        }
        page += 1
      }

      probeNodes.value = collected
      totalNodes.value = expectedTotal || collected.length
      return collected
    } catch {
      // Keep workbench resilient if /api/v1/nodes is not mocked in isolated unit tests
      return probeNodes.value
    } finally {
      loadingNodes.value = false
    }
  }

  function isValidPoolStatus(res: unknown): res is ProbePoolStatus {
    if (!res || typeof res !== 'object') return false
    const candidate = res as Record<string, unknown>
    return (
      typeof candidate.total_count === 'number' &&
      typeof candidate.queue_nodes_count === 'number' &&
      typeof candidate.available_count === 'number' &&
      typeof candidate.unavailable_count === 'number' &&
      typeof candidate.untested_count === 'number'
    )
  }

  function derivePoolStatusFromLocal(): ProbePoolStatus {
    const nodes = probeNodes.value
    const total = nodes.length
    const probingSet = new Set<string>(probingNodeIds.value)
    const queuedSet = new Set<string>(queuedNodeIds.value)
    let healthy = 0
    let degraded = 0
    let unhealthy = 0
    let untested = 0

    for (const node of nodes) {
      if (node.probeState === 'probing' || node.healthStatus === 'probing') {
        probingSet.add(node.logicalId)
      } else if (node.probeState === 'queued' || node.healthStatus === 'queued') {
        queuedSet.add(node.logicalId)
      }
      const cat = nodeUnderlyingHealthCategory(node)
      if (cat === 'healthy') healthy += 1
      else if (cat === 'degraded') degraded += 1
      else if (cat === 'unhealthy') unhealthy += 1
      else untested += 1
    }

    for (const id of probingSet) {
      queuedSet.delete(id)
    }

    const probingIds = Array.from(probingSet)
    const queuedIds = Array.from(queuedSet)
    const probingCount = probingIds.length
    const queuedWaitingCount = queuedIds.length

    return {
      queue_nodes_count: probingCount + queuedWaitingCount,
      probing_count: probingCount,
      queued_waiting_count: queuedWaitingCount,
      untested_count: untested,
      total_count: total,
      unavailable_count: unhealthy,
      available_count: healthy + degraded,
      healthy_count: healthy,
      degraded_count: degraded,
      probing_node_ids: probingIds,
      queued_node_ids: queuedIds,
      updated_at: new Date().toISOString(),
    }
  }

  const poolStatus = computed<ProbePoolStatus>({
    get: () => serverPoolStatus.value ?? derivePoolStatusFromLocal(),
    set: (val) => {
      serverPoolStatus.value = val
    },
  })

  function applyServerPoolStatus(res: ProbePoolStatus): ProbePoolStatus {
    const probingIds = Array.isArray(res.probing_node_ids) ? res.probing_node_ids : []
    const queuedIds = Array.isArray(res.queued_node_ids) ? res.queued_node_ids : []
    const nextProbing = new Set<string>(probingIds)
    const nextQueued = new Set<string>(queuedIds.filter((id) => !nextProbing.has(id)))
    probingNodeIds.value = nextProbing
    queuedNodeIds.value = nextQueued

    const normalized: ProbePoolStatus = {
      queue_nodes_count:
        typeof res.queue_nodes_count === 'number'
          ? res.queue_nodes_count
          : nextProbing.size + nextQueued.size,
      probing_count:
        typeof res.probing_count === 'number' ? res.probing_count : nextProbing.size,
      queued_waiting_count:
        typeof res.queued_waiting_count === 'number'
          ? res.queued_waiting_count
          : nextQueued.size,
      untested_count: res.untested_count ?? 0,
      total_count: res.total_count ?? 0,
      unavailable_count: res.unavailable_count ?? 0,
      available_count: res.available_count ?? 0,
      healthy_count: res.healthy_count ?? 0,
      degraded_count: res.degraded_count ?? 0,
      probing_node_ids: Array.from(nextProbing),
      queued_node_ids: Array.from(nextQueued),
      updated_at: res.updated_at || new Date().toISOString(),
      ...(res.batch_id !== undefined ? { batch_id: res.batch_id } : {}),
      ...(res.batch_state !== undefined ? { batch_state: res.batch_state } : {}),
      ...(res.dispatched_runs !== undefined ? { dispatched_runs: res.dispatched_runs } : {}),
      ...(res.scheduled_nodes !== undefined ? { scheduled_nodes: res.scheduled_nodes } : {}),
      ...(res.scheduled_tasks !== undefined ? { scheduled_tasks: res.scheduled_tasks } : {}),
      ...(res.skipped_nodes !== undefined ? { skipped_nodes: res.skipped_nodes } : {}),
      ...(res.no_due_tasks !== undefined ? { no_due_tasks: res.no_due_tasks } : {}),
      ...(res.run_ids !== undefined ? { run_ids: res.run_ids } : {}),
    }

    serverPoolStatus.value = normalized

    if (probeNodes.value.length > 0) {
      probeNodes.value = probeNodes.value.map((n) => {
        if (nextProbing.has(n.logicalId)) {
          return { ...n, probeState: 'probing' as const }
        }
        if (nextQueued.has(n.logicalId)) {
          return { ...n, probeState: 'queued' as const }
        }
        if (n.probeState !== 'idle' && n.healthStatus !== 'probing' && n.healthStatus !== 'queued') {
          return { ...n, probeState: 'idle' as const }
        }
        return n
      })
    }

    return normalized
  }

  async function loadPoolStatus(): Promise<ProbePoolStatus> {
    loadingPoolStatus.value = true
    try {
      const res = await api.get<ProbePoolStatus>('/api/v1/probes/pool')
      if (isValidPoolStatus(res)) {
        return applyServerPoolStatus(res)
      }
      serverPoolStatus.value = null
      return derivePoolStatusFromLocal()
    } catch {
      serverPoolStatus.value = null
      return derivePoolStatusFromLocal()
    } finally {
      loadingPoolStatus.value = false
    }
  }

  async function triggerPeriodicPoolEnqueue(): Promise<ProbePoolStatus | null> {
    triggeringSchedule.value = true
    error.value = ''
    try {
      await Promise.allSettled([
        loadPoolStatus(),
        loadProbeNodes(),
        loadSchedule(),
      ])
      const res = await api.post<ProbePoolStatus>('/api/v1/probes/schedule/trigger')
      if (isValidPoolStatus(res)) {
        const applied = applyServerPoolStatus(res)
        await Promise.allSettled([
          loadProbeNodes(),
          loadBatches(),
          loadRuns(),
          loadSchedule(),
        ])
        return applied
      }
      await Promise.allSettled([
        loadPoolStatus(),
        loadProbeNodes(),
        loadBatches(),
        loadRuns(),
        loadSchedule(),
      ])
      return poolStatus.value
    } catch (err) {
      error.value = err instanceof Error ? err.message : '刷新状态并触发按需检测失败'
      return null
    } finally {
      triggeringSchedule.value = false
    }
  }

  const triggerScheduleNow = triggerPeriodicPoolEnqueue

  async function loadSubscriptions() {
    try {
      const res = await api.get<PaginatedResult<SubscriptionLite>>('/api/v1/subscriptions', {
        params: { page: 1, page_size: 100 },
      })
      subscriptions.value = Array.isArray(res?.items) ? res.items : []
    } catch {
      // Non-blocking if subscriptions endpoint is not mocked
    }
  }

  function synthesizeObservationsFromNode(logicalId: string): ProbeObservation[] {
    const node = nodeMap.value[logicalId]
    if (!node || !node.capabilityDetails) return []
    const synthesized: ProbeObservation[] = []
    for (const [kind, detail] of Object.entries(node.capabilityDetails)) {
      if (!detail || detail.verdict === 'missing') continue
      if (detail.verdict === 'unknown' && !detail.summary && !detail.observed_at) continue
      synthesized.push({
        id: `cap-${logicalId}-${kind}`,
        probe_run_id: 'latest',
        node_logical_id: logicalId,
        kind: kind as ProbeKind,
        verdict: detail.verdict as ProbeObservation['verdict'],
        evidence_digest: '',
        observed_at: detail.observed_at || node.lastProbedAt || '',
        latency_ms:
          detail.verdict === 'error'
            ? -1
            : typeof detail.latency_ms === 'number' && detail.latency_ms > 0
            ? detail.latency_ms
            : kind === 'baseline' && typeof node.latencyMs === 'number' && node.latencyMs > 0
            ? node.latencyMs
            : -1,
        redacted_summary: detail.summary || '',
      })
    }
    return synthesized
  }

  async function loadNodeObservations(logicalId: string) {
    loadingNodeObservations.value = true
    error.value = ''
    try {
      const res = await api.get<PaginatedResult<ProbeObservation>>(
        `/api/v1/nodes/${encodeURIComponent(logicalId)}/observations`,
        {
          params: { page: 1, page_size: 100 },
        }
      )
      const items = res?.items || []
      nodeObservations.value = items.length > 0 ? items : synthesizeObservationsFromNode(logicalId)
      return nodeObservations.value
    } catch (err) {
      const fallback = synthesizeObservationsFromNode(logicalId)
      if (fallback.length > 0) {
        nodeObservations.value = fallback
        return fallback
      }
      error.value = err instanceof Error ? err.message : '加载节点测速历史失败'
      nodeObservations.value = []
      return []
    } finally {
      loadingNodeObservations.value = false
    }
  }

  async function loadSchedule() {
    loadingSchedule.value = true
    error.value = ''
    try {
      const res = await api.get<ProbeSchedule>('/api/v1/probes/schedule')
      schedule.value = res
      return res
    } catch (err) {
      error.value = err instanceof Error ? err.message : '加载探针周期计划失败'
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
      const msg = err instanceof Error ? err.message : '更新探针周期计划失败'
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
      error.value = err instanceof Error ? err.message : '加载探针执行批次失败'
      return null
    } finally {
      loadingBatches.value = false
    }
  }

  async function getBatch(batchId: string): Promise<ProbeBatch | null> {
    try {
      return await api.get<ProbeBatch>(`/api/v1/probes/batches/${batchId}`)
    } catch (err) {
      const msg = err instanceof Error ? err.message : '获取探针批次详情失败'
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
      const msg = err instanceof Error ? err.message : '取消探针批次失败'
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
      error.value = err instanceof Error ? err.message : '加载探针执行记录失败'
    } finally {
      loadingRuns.value = false
    }
  }

  async function createRun(input: CreateProbeRunInput): Promise<CreateProbeRunResponse> {
    submitting.value = true
    error.value = ''
    const targetIds = input.node_logical_ids || []
    const idsToMark = targetIds.length > 0 ? targetIds : probeNodes.value.map((n) => n.logicalId)

    if (idsToMark.length > 0) {
      const nextProbing = new Set(probingNodeIds.value)
      const nextQueued = new Set(queuedNodeIds.value)
      for (const id of idsToMark) {
        nextProbing.add(id)
        nextQueued.delete(id)
      }
      probingNodeIds.value = nextProbing
      queuedNodeIds.value = nextQueued
      probeNodes.value = probeNodes.value.map((n) =>
        idsToMark.includes(n.logicalId) ? { ...n, probeState: 'probing' as const } : n
      )
      if (serverPoolStatus.value) {
        const mergedProbing = Array.from(new Set([...serverPoolStatus.value.probing_node_ids, ...idsToMark]))
        const mergedQueued = serverPoolStatus.value.queued_node_ids.filter((id) => !mergedProbing.includes(id))
        serverPoolStatus.value = {
          ...serverPoolStatus.value,
          probing_node_ids: mergedProbing,
          queued_node_ids: mergedQueued,
          probing_count: mergedProbing.length,
          queued_waiting_count: mergedQueued.length,
          queue_nodes_count: mergedProbing.length + mergedQueued.length,
        }
      }
    }

    try {
      const idempotencyKey = generateIdempotencyKey()
      const body: Record<string, unknown> = {
        config_revision: input.config_revision || '',
        node_logical_ids: targetIds,
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
      if (res && isValidPoolStatus((res as any).pool_status)) {
        applyServerPoolStatus((res as any).pool_status)
      } else if (res && ['succeeded', 'failed', 'cancelled', 'expired'].includes(res.state)) {
        const nextProbing = new Set(probingNodeIds.value)
        for (const id of idsToMark) nextProbing.delete(id)
        probingNodeIds.value = nextProbing
      }
      await loadRuns()
      return res
    } catch (err) {
      if (idsToMark.length > 0) {
        const nextProbing = new Set(probingNodeIds.value)
        for (const id of idsToMark) nextProbing.delete(id)
        probingNodeIds.value = nextProbing
        probeNodes.value = probeNodes.value.map((n) =>
          idsToMark.includes(n.logicalId) && n.healthStatus !== 'probing'
            ? { ...n, probeState: 'idle' as const }
            : n
        )
      }
      const msg = err instanceof Error ? err.message : '发起探针任务失败'
      error.value = msg
      throw err
    } finally {
      submitting.value = false
    }
  }

  async function triggerQuickProbe(options: {
    kinds: ProbeKind[]
    nodeLogicalIds?: string[]
  }): Promise<CreateProbeRunResponse | null> {
    const kinds: ProbeKind[] = options.kinds.length > 0 ? options.kinds : ['baseline']
    const res = await createRun({
      kinds,
      node_logical_ids: options.nodeLogicalIds ?? [],
    })
    await Promise.all([loadProbeNodes(), loadPoolStatus()])
    return res
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
      error.value = err instanceof Error ? err.message : '取消探针任务失败'
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
      error.value = err instanceof Error ? err.message : '加载探测观测证据失败'
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
    } catch {
      return null
    }
  }

  return {
    runs,
    activeRun,
    observations,
    nodeObservations,
    schedule,
    batches,
    probeNodes,
    subscriptions,
    nodeMap,
    subscriptionNameMap,
    probingNodeIds,
    queuedNodeIds,
    poolStatus,
    loadingPoolStatus,
    triggeringSchedule,
    loadingRuns,
    loadingObservations,
    loadingNodeObservations,
    loadingSchedule,
    loadingBatches,
    loadingNodes,
    savingSchedule,
    cancellingBatch,
    submitting,
    cancelling,
    error,
    totalRuns,
    totalBatches,
    totalNodes,
    loadProbeNodes,
    loadSubscriptions,
    loadNodeObservations,
    loadRuns,
    createRun,
    triggerQuickProbe,
    cancelRun,
    loadObservations,
    fetchRunDetail,
    loadSchedule,
    updateSchedule,
    loadBatches,
    getBatch,
    cancelBatch,
    loadPoolStatus,
    triggerPeriodicPoolEnqueue,
    triggerScheduleNow,
  }
}
