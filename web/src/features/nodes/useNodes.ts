import { computed, ref } from 'vue'
import { displayText } from '../../utils/displayNames'
import { api, ApiError } from '../../api/client'
import { uiText } from '../../locales/redesign'
import type { NodeSourceHistoryData } from './sourceHistoryTypes'
import {
  generateIdempotencyKey,
  type CreateProbeRunResponse,
  type ProbeKind,
  type ProbePoolStatus,
  type ProbeRun,
  type ProbeRunState,
} from '../probes/probeTypes'
import {
  normalizeNode,
  validateNodeConnectionProfile,
  type IPRiskSummaryRecord,
  type NodeConnectionProfile,
  type NodeRecord,
  type NodeSourceRecord,
  type NormalizedNode,
} from './nodeView'

export interface NodeDetailError {
  logicalId: string
  status: number
  message: string
  isStale: boolean
}

interface NodePage {
  items: NodeRecord[]
  page: number
  page_size: number
  total: number
  scope?: string
}

interface NodeDetailResponse {
  node?: NodeRecord
  sources?: NodeSourceRecord[]
  ip_risk_summary?: IPRiskSummaryRecord
}

export type HealthFilterType =
  | 'all'
  | 'probing'
  | 'healthy'
  | 'degraded'
  | 'unhealthy'
  | 'undetermined'
  | 'untested'
  | 'unknown'

// Match the API's unsigned decimal grammar; hex, Infinity and non-strings
// (including repeated URL parameters) must not become applied thresholds.
export function parseSpeedMinimum(value: unknown): string {
  if (typeof value !== 'string') return ''
  const decimal = /^([0-9]+(?:\.[0-9]*)?|\.[0-9]+)(?:[eE][+-]?[0-9]+)?$/.exec(value)
  if (!decimal) return ''
  const speed = Number(value)
  if (speed === 0 && /[1-9]/.test(decimal[1])) return ''
  return Number.isFinite(speed) ? String(speed) : ''
}

export function useNodes() {
  const items = ref<NormalizedNode[]>([])
  const loading = ref(false)
  const loadingMore = ref(false)
  const loadingDetail = ref(false)
  const savingConnection = ref(false)
  const probingNodeId = ref<string | null>(null)
  const speedingNodeId = ref<string | null>(null)
  const poolProbingIds = ref<Set<string>>(new Set())
  const poolQueuedIds = ref<Set<string>>(new Set())
  const submittingProbes = ref(false)
  const pendingProbeIds = ref<Set<string>>(new Set())
  const activeRuns = ref(new Map<string, { ids: string[]; state: ProbeRunState }>())
  const runReads = new Map<string, Promise<ProbeRun>>()
  const probeStates = ref<Record<string, ProbeRunState>>({})
  const resultReadSeq = new Map<string, number>()
  const connectionUpdates = new Map<string, { sequence: number; node: NormalizedNode }>()
  let connectionUpdateSequence = 0
  const resultsToRefresh = ref(new Set<string>())
  const resultRefreshFailedIds = ref(new Set<string>())
  const poolError = ref('')
  const refreshAttempts = new Map<string, number>()
  const pendingResultRefreshIds = computed(() => {
    const ids = new Set(items.value.map((node) => node.logicalId))
    if (selectedNode.value) ids.add(selectedNode.value.logicalId)
    return new Set([...resultsToRefresh.value].filter((id) => ids.has(id)))
  })
  const probePendingIds = computed(() => pendingProbeIds.value)
  const probingNodeIds = computed(() => new Set([
    ...poolProbingIds.value,
    ...[...activeRuns.value.values()].filter((run) => run.state === 'running').flatMap((run) => run.ids),
  ]))
  const queuedNodeIds = computed(() => new Set([
    ...poolQueuedIds.value,
    ...[...activeRuns.value.values()].filter((run) => run.state === 'queued').flatMap((run) => run.ids),
  ].filter((id) => !probingNodeIds.value.has(id))))
  const error = ref('')
  const appendError = ref('')
  const failureMode = ref<'initial' | 'refresh' | 'append' | null>(null)
  let poolSync: Promise<void> | null = null
  const page = ref(0)
  const pageSize = 100
  const total = ref(0)
  const protocolFilter = ref<string>('all')
  const healthFilter = ref<HealthFilterType>('all')
  const riskBandFilter = ref('all')
  const riskScoreMax = ref('')
  const speedMinKiBS = ref('')
  const searchQuery = ref<string>('')
  const selectedNode = ref<NormalizedNode | null>(null)
  const requestGeneration = ref(0)
  const globalProbeKinds = ref<ProbeKind[]>(['baseline', 'streaming', 'ai', 'ip_risk', 'geo'])
  const hasMore = computed(() => items.value.length < total.value)
  const initialLoaded = ref(false)
  const currentDetailId = ref('')
  const nodeDetailError = ref<NodeDetailError | null>(null)
  let nodeDetailSeq = 0

  const isDetailStale = computed(() => {
    return Boolean(
      nodeDetailError.value?.isStale &&
      nodeDetailError.value?.logicalId === selectedNode.value?.logicalId
    )
  })

  const detailErrorMessage = computed(() => {
    if (!nodeDetailError.value || nodeDetailError.value.logicalId !== selectedNode.value?.logicalId) {
      return ''
    }
    return nodeDetailError.value.message
  })

  async function fetchGlobalScheduleKinds(): Promise<ProbeKind[]> {
    try {
      const res = await api.get<{ kinds?: ProbeKind[] }>('/api/v1/probes/schedule')
      if (res && Array.isArray(res.kinds) && res.kinds.length > 0) {
        globalProbeKinds.value = res.kinds.filter((kind) => kind !== 'speed')
      }
    } catch {
      // Non-blocking fallback to default kinds
    }
    return globalProbeKinds.value
  }

  function applyPoolStateToNode(node: NormalizedNode): NormalizedNode {
    const probeState = probingNodeIds.value.has(node.logicalId) ? 'probing'
      : queuedNodeIds.value.has(node.logicalId) ? 'queued' : 'idle'
    // Pool/run evidence owns activity; a cached DTO must not keep a completed probe spinning
    const healthStatus = node.healthStatus === 'probing' || node.healthStatus === 'queued'
      ? 'unknown' : node.healthStatus
    return { ...node, healthStatus, probeState }
  }

  function projectProbeState() {
    items.value = items.value.map(applyPoolStateToNode)
    if (selectedNode.value) selectedNode.value = applyPoolStateToNode(selectedNode.value)
  }

  function nextResultRead(logicalId: string) {
    const seq = (resultReadSeq.get(logicalId) ?? 0) + 1
    resultReadSeq.set(logicalId, seq)
    return seq
  }

  function recordRunState(ids: string[], state: ProbeRunState) {
    for (const id of ids) probeStates.value[id] = state
  }

  async function refreshNodeResult(logicalId: string): Promise<NormalizedNode | null> {
    const seq = nextResultRead(logicalId)
    const response = await api.get<NodeDetailResponse | NodeRecord>(`/api/v1/nodes/${encodeURIComponent(logicalId)}`)
    if (resultReadSeq.get(logicalId) !== seq) return null
    const raw = extractNodeRecord(response)
    if (!raw || raw.logical_id !== logicalId) throw new Error(uiText('节点结果无效，请刷新重试', 'Invalid node result. Refresh and retry.'))
    const refreshed = applyPoolStateToNode(normalizeNode(raw))
    const current = items.value.find((node) => node.logicalId === logicalId)
      ?? (selectedNode.value?.logicalId === logicalId ? selectedNode.value : null)
    if (current?.connectionRevision != null && refreshed.connectionRevision != null && refreshed.connectionRevision < current.connectionRevision) return null
    const index = items.value.findIndex((node) => node.logicalId === logicalId)
    if (index >= 0) items.value[index] = refreshed
    // Background refresh must never navigate the detail drawer
    if (selectedNode.value?.logicalId === logicalId) {
      selectedNode.value = refreshed
      nodeDetailError.value = null
    }
    refreshAttempts.delete(logicalId)
    resultsToRefresh.value.delete(logicalId)
    resultRefreshFailedIds.value.delete(logicalId)
    return refreshed
  }

  function isTerminal(state: string) {
    return ['succeeded', 'failed', 'cancelled', 'expired'].includes(state)
  }

  function readRunStatus(runId: string): Promise<ProbeRun> {
    const pending = runReads.get(runId)
    if (pending) return pending
    const request = api.get<ProbeRun>(`/api/v1/probes/runs/${encodeURIComponent(runId)}`)
      .finally(() => { runReads.delete(runId) })
    runReads.set(runId, request)
    return request
  }

  function syncPoolStatus(): Promise<void> {
    if (poolSync) return poolSync
    poolSync = readPoolStatus().finally(() => { poolSync = null })
    return poolSync
  }

  async function readPoolStatus(): Promise<void> {
    let statusUnavailable = false
    const previouslyActive = new Set([...probingNodeIds.value, ...queuedNodeIds.value])
    const finishedIds = resultsToRefresh.value
    const runs = [...activeRuns.value.entries()]
    await Promise.all(runs.map(async ([id, tracked]) => {
      try {
        const run = await readRunStatus(id)
        if (!run?.state) { statusUnavailable = true; return }
        recordRunState(tracked.ids, run.state)
        if (isTerminal(run.state)) {
          const nextRuns = new Map(activeRuns.value)
          nextRuns.delete(id)
          activeRuns.value = nextRuns
          tracked.ids.forEach((nodeId) => finishedIds.add(nodeId))
        } else {
          const nextRuns = new Map(activeRuns.value)
          nextRuns.set(id, { ...tracked, state: run.state })
          activeRuns.value = nextRuns
        }
      } catch {
        statusUnavailable = true
        // Preserve the last server evidence when the status endpoint is unavailable
      }
    }))
    try {
      const res = await api.get<ProbePoolStatus>('/api/v1/probes/pool')
      if (res && (Array.isArray(res.probing_node_ids) || Array.isArray(res.queued_node_ids))) {
        poolProbingIds.value = new Set(res.probing_node_ids ?? [])
        poolQueuedIds.value = new Set((res.queued_node_ids ?? []).filter((id) => !poolProbingIds.value.has(id)))
        for (const id of previouslyActive) {
          if (!probingNodeIds.value.has(id) && !queuedNodeIds.value.has(id)) finishedIds.add(id)
        }
      } else {
        statusUnavailable = true
      }
    } catch {
      statusUnavailable = true
      // A pool failure is not completion evidence
    }
    poolError.value = statusUnavailable
      ? uiText('检测状态暂时无法同步，当前保留最近一次状态；可刷新重试。', 'Probe status could not synchronize. The last known state is retained; refresh to retry.')
      : ''
    projectProbeState()
    const visibleIds = new Set(items.value.map((node) => node.logicalId))
    if (selectedNode.value) visibleIds.add(selectedNode.value.logicalId)
    await Promise.all([...finishedIds].filter((id) => visibleIds.has(id)).map(async (id) => {
      try {
        if (await refreshNodeResult(id)) return
      } catch {
        if (selectedNode.value?.logicalId === id) {
          nodeDetailError.value = { logicalId: id, status: 0, isStale: true, message: uiText('检测已结束，但结果刷新失败，请重试详情', 'Probe ended, but results could not refresh. Retry node details.') }
        }
      }
      const attempts = (refreshAttempts.get(id) ?? 0) + 1
      refreshAttempts.set(id, attempts)
      if (attempts >= 3) {
        finishedIds.delete(id)
        resultRefreshFailedIds.value.add(id)
      }
    }))
  }

  function resolveListedNode(node: NormalizedNode, connectionSequence: number, listedNode?: NormalizedNode): NormalizedNode {
    const saved = connectionUpdates.get(node.logicalId)
    const candidates = [
      saved?.node,
      listedNode,
      selectedNode.value?.logicalId === node.logicalId ? selectedNode.value : null,
    ]
    const current = candidates.reduce<NormalizedNode | null>((latest, candidate) => {
      if (!candidate) return latest
      return !latest || (candidate.connectionRevision ?? 0) >= (latest.connectionRevision ?? 0) ? candidate : latest
    }, null)
    const nodeRevision = node.connectionRevision
    const currentRevision = current?.connectionRevision
    const olderRevision = current && nodeRevision != null && currentRevision != null && nodeRevision < currentRevision
    const savedSinceRead = saved && saved.sequence > connectionSequence
      && (nodeRevision == null || saved.node.connectionRevision == null || nodeRevision <= saved.node.connectionRevision)
    // Keep page membership and metadata, but never roll an accepted connection back with an older read.
    if (savedSinceRead) {
      if (current && currentRevision != null
        && (saved.node.connectionRevision == null || currentRevision > saved.node.connectionRevision)) {
        return applyPoolStateToNode(current)
      }
      return applyPoolStateToNode(saved.node)
    }
    return applyPoolStateToNode(current && olderRevision ? current : node)
  }

  async function loadPage(nextPage: number, append = false, generation = requestGeneration.value) {
    if (append && (loadingMore.value || loading.value)) return
    if (append) loadingMore.value = true
    else loading.value = true
    if (append) appendError.value = ''
    else error.value = ''
    const connectionSequence = connectionUpdateSequence
    try {
      const params: Record<string, string | number> = {
        page: nextPage,
        page_size: pageSize,
        scope: 'enabled_subscriptions',
        sort_by: 'display_name',
        sort_order: 'asc',
      }
      if (protocolFilter.value && protocolFilter.value !== 'all') {
        params.protocol = protocolFilter.value
      }
      if (riskBandFilter.value !== 'all') params.ip_risk_band = riskBandFilter.value
      if (riskScoreMax.value !== '') params.ip_risk_score_max = riskScoreMax.value
      if (speedMinKiBS.value !== '') params.speed_min_kib_s = speedMinKiBS.value
      if (searchQuery.value.trim()) {
        params.search = searchQuery.value.trim()
      }
      if (healthFilter.value === 'probing') {
        params.probe_state = 'active'
      } else if (healthFilter.value && healthFilter.value !== 'all') {
        if (healthFilter.value === 'unknown' || healthFilter.value === 'untested' || healthFilter.value === 'undetermined') {
          params.health_status = 'unknown'
        } else if (healthFilter.value === 'degraded') {
          params.health_status = 'healthy'
        } else {
          params.health_status = healthFilter.value
        }
      }
      const [result] = await Promise.all([
        api.get<NodePage>('/api/v1/nodes', { params }),
        syncPoolStatus(),
      ])
      if (generation !== requestGeneration.value) return
      const currentNodes = new Map(items.value.map((node) => [node.logicalId, node]))
      const normalized = (result.items || []).map((raw) => {
        const node = normalizeNode(raw)
        return resolveListedNode(node, connectionSequence, currentNodes.get(node.logicalId))
      })
      items.value = append ? [...new Map([...items.value, ...normalized].map((node) => [node.logicalId, node])).values()] : normalized
      page.value = result.page
      total.value = result.total
      initialLoaded.value = true
      failureMode.value = null
    } catch (cause) {
      if (generation === requestGeneration.value) {
        const message = displayText(cause instanceof Error ? cause.message : uiText('加载节点列表失败', 'Could not load nodes.'))
        failureMode.value = append ? 'append' : initialLoaded.value ? 'refresh' : 'initial'
        if (append) appendError.value = message
        else error.value = message
      }
    } finally {
      if (generation === requestGeneration.value) {
        loading.value = false
        loadingMore.value = false
      }
    }
  }

  async function load() {
    requestGeneration.value += 1
    const currentGen = requestGeneration.value
    if (loadingMore.value) loadingMore.value = false
    appendError.value = ''
    await loadPage(1, false, currentGen)
  }

  async function loadMore() {
    if (hasMore.value) await loadPage(page.value + 1, true, requestGeneration.value)
  }

  function extractNodeRecord(res: NodeDetailResponse | NodeRecord | null | undefined): NodeRecord | undefined {
    if (!res || typeof res !== 'object') return undefined
    if ('node' in res && res.node) {
      return {
        ...res.node,
        sources: Array.isArray(res.sources) ? res.sources : res.node.sources,
        ip_risk_summary: res.ip_risk_summary ?? res.node.ip_risk_summary,
      }
    }
    if ('logical_id' in res || 'logicalId' in (res as any)) {
      return res as NodeRecord
    }
    return undefined
  }

  function invalidateNodeDetail() {
    nodeDetailSeq++
    currentDetailId.value = ''
    loadingDetail.value = false
  }

  async function fetchNodeDetail(logicalId: string): Promise<NormalizedNode | null> {
    const seq = ++nodeDetailSeq
    const resultSeq = nextResultRead(logicalId)
    const existing = items.value.find((n) => n.logicalId === logicalId) ?? null
    if (existing) {
      selectedNode.value = existing
    }
    loadingDetail.value = true
    currentDetailId.value = logicalId
    if (nodeDetailError.value?.logicalId === logicalId) {
      nodeDetailError.value = null
    }
    try {
      const res = await api.get<NodeDetailResponse | NodeRecord>(`/api/v1/nodes/${encodeURIComponent(logicalId)}`)
      if (seq !== nodeDetailSeq || currentDetailId.value !== logicalId || resultReadSeq.get(logicalId) !== resultSeq) return null
      const rawNode = extractNodeRecord(res)
      if (rawNode) {
        const detailed = applyPoolStateToNode(normalizeNode(rawNode))
        selectedNode.value = detailed
        nodeDetailError.value = null
        resultsToRefresh.value.delete(logicalId)
        resultRefreshFailedIds.value.delete(logicalId)
        refreshAttempts.delete(logicalId)
        const idx = items.value.findIndex((n) => n.logicalId === logicalId)
        if (idx >= 0) {
          items.value[idx] = detailed
        }
        return detailed
      }
      return existing
    } catch (cause) {
      if (seq !== nodeDetailSeq || currentDetailId.value !== logicalId || resultReadSeq.get(logicalId) !== resultSeq) return null
      let status = 0
      let message = uiText('网络连接失败，请检查网络后重试', 'Network connection failed. Check your connection and retry.')
      if (cause instanceof ApiError) {
        status = cause.status
        message = cause.status === 404
          ? uiText('未找到该节点（可能已移除），请刷新台账 (404)', 'Node not found (it may have been removed). Refresh the ledger (404)')
          : uiText(`获取节点详情失败 (${cause.status})`, `Failed to load node details (${cause.status})`)
      } else if (cause instanceof Error && cause.message) {
        message = displayText(cause.message)
      }
      nodeDetailError.value = {
        logicalId,
        status,
        message,
        isStale: Boolean(existing),
      }
      return existing
    } finally {
      if (seq === nodeDetailSeq && currentDetailId.value === logicalId) {
        loadingDetail.value = false
      }
    }
  }

  async function fetchNodeSourceHistory(logicalId: string): Promise<NodeSourceHistoryData | null> {
    try {
      const res = await api.get<NodeSourceHistoryData | { data: NodeSourceHistoryData }>(
        `/api/v1/nodes/${encodeURIComponent(logicalId)}/source-history`
      )
      if (res && typeof res === 'object' && 'data' in res && (res as any).data) {
        return (res as any).data as NodeSourceHistoryData
      }
      if (
        res &&
        typeof res === 'object' &&
        (Array.isArray((res as any).current_sources) ||
          Array.isArray((res as any).history) ||
          typeof (res as any).attribution_status === 'string')
      ) {
        return res as NodeSourceHistoryData
      }
      return null
    } catch (err) {
      throw err
    }
  }

  async function submitProbe(
    logicalIds: string[],
    kinds?: ProbeKind[],
    waitBriefly = false
  ): Promise<{ ok: boolean; runId?: string; state?: ProbeRunState; error?: string; refreshed?: boolean }> {
    const ids = logicalIds.length ? [...new Set(logicalIds)] : items.value.map((node) => node.logicalId)
    if (submittingProbes.value || ids.some((id) => pendingProbeIds.value.has(id))) {
      return { ok: false, error: uiText('检测请求正在提交，请勿重复操作', 'A probe request is being submitted. Please wait.') }
    }
    submittingProbes.value = true
    let submissionPending = true
    ids.forEach((id) => { pendingProbeIds.value.add(id); delete probeStates.value[id] })
    const effectiveKinds = (kinds?.length ? kinds : globalProbeKinds.value).filter((kind) => kind !== 'speed')
    try {
      const res = await api.post<CreateProbeRunResponse>('/api/v1/probes/runs', {
        config_revision: '', node_logical_ids: logicalIds, kinds: effectiveKinds.length ? effectiveKinds : ['baseline'],
      }, { headers: { 'Idempotency-Key': generateIdempotencyKey() } })
      if (!res?.run_id || !['queued', 'running', 'succeeded', 'failed', 'cancelled', 'expired'].includes(res.state)) {
        throw new Error(uiText('未获得有效检测任务，请重试', 'No valid probe run was returned. Please retry.'))
      }
      let state = res.state
      recordRunState(ids, state)
      ids.forEach((id) => {
        resultsToRefresh.value.delete(id)
        resultRefreshFailedIds.value.delete(id)
        refreshAttempts.delete(id)
      })
      submittingProbes.value = false
      submissionPending = false
      ids.forEach((id) => pendingProbeIds.value.delete(id))
      let refreshed = false
      if (!isTerminal(state)) {
        const nextRuns = new Map(activeRuns.value)
        nextRuns.set(res.run_id, { ids, state })
        activeRuns.value = nextRuns
      }
      projectProbeState()
      for (let attempt = 0; waitBriefly && attempt < 6 && !isTerminal(state); attempt++) {
        await new Promise((resolve) => setTimeout(resolve, 350))
        try {
          const run = await readRunStatus(res.run_id)
          if (!run?.state) break
          state = run.state
          recordRunState(ids, state)
          const nextRuns = new Map(activeRuns.value)
          if (isTerminal(state)) nextRuns.delete(res.run_id)
          else nextRuns.set(res.run_id, { ids, state })
          activeRuns.value = nextRuns
          projectProbeState()
        } catch { break }
      }
      if (isTerminal(state)) {
        const nextRuns = new Map(activeRuns.value)
        nextRuns.delete(res.run_id)
        activeRuns.value = nextRuns
        ids.forEach((id) => { poolProbingIds.value.delete(id); poolQueuedIds.value.delete(id) })
        projectProbeState()
        const results = await Promise.all(ids.map(async (id) => {
          try {
            const updated = Boolean(await refreshNodeResult(id))
            if (!updated) resultsToRefresh.value.add(id)
            return updated
          } catch { resultsToRefresh.value.add(id); return false }
        }))
        refreshed = results.every(Boolean)
      }
      return {
        ok: state === 'queued' || state === 'running' || state === 'succeeded', runId: res.run_id, state, refreshed,
        error: state === 'failed' ? uiText('检测失败，请查看检测历史', 'Probe failed. See probe history.')
          : state === 'cancelled' ? uiText('检测已取消', 'Probe cancelled.')
          : state === 'expired' ? uiText('检测已超时', 'Probe expired.') : undefined,
      }
    } catch (cause) {
      return { ok: false, error: displayText(cause instanceof Error ? cause.message : uiText('提交节点检测失败', 'Could not submit node probe.')) }
    } finally {
      if (submissionPending) {
        submittingProbes.value = false
        ids.forEach((id) => pendingProbeIds.value.delete(id))
      }
      projectProbeState()
    }
  }

  async function probeSingleNode(logicalId: string, kinds?: ProbeKind[]) {
    probingNodeId.value = logicalId
    try { return await submitProbe([logicalId], kinds, true) }
    finally { if (probingNodeId.value === logicalId) probingNodeId.value = null }
  }

  // A separate explicit action: never inherits schedule kinds or expands to the fleet.
  async function measureNodeSpeed(logicalId: string): Promise<{ ok: boolean; error?: string }> {
    if (speedingNodeId.value) return { ok: false, error: uiText('已有测速正在执行，请等待完成。', 'A bandwidth measurement is already running. Please wait.') }
    if (!logicalId || logicalId.trim() !== logicalId) return { ok: false, error: uiText('请选择有效节点。', 'Select a valid node.') }
    speedingNodeId.value = logicalId
    const target = (selectedNode.value?.logicalId === logicalId ? selectedNode.value : null)
      ?? items.value.find((node) => node.logicalId === logicalId)
    const previousObservedAt = target?.capabilityDetails.speed?.observed_at
    const connectionRevision = target?.connectionRevision
    try {
      const created = await api.post<CreateProbeRunResponse>('/api/v1/probes/runs', {
        manual_speed: true,
        node_logical_ids: [logicalId],
        kinds: ['speed'],
      }, { headers: { 'Idempotency-Key': generateIdempotencyKey() } })
      if (!created?.run_id) return { ok: false, error: uiText('未能创建测速任务，请重试。', 'Could not create bandwidth measurement. Retry.') }
      let state = created.state
      for (let attempt = 0; attempt < 60 && !isTerminal(state); attempt++) {
        await new Promise((resolve) => setTimeout(resolve, 1000))
        const run = await readRunStatus(created.run_id)
        state = run.state
      }
      const refreshed = await refreshNodeResult(logicalId)
      if (!isTerminal(state)) {
        return { ok: false, error: uiText('测速仍在执行，暂未获得本次结果；稍后刷新节点状态查看。', 'Bandwidth measurement is still running. Refresh node results later.') }
      }
      if (state !== 'succeeded') return { ok: false, error: state === 'cancelled'
        ? uiText('本次测速已取消，历史结果不代表本次成功。', 'Bandwidth measurement was cancelled. Previous results do not indicate success.')
        : uiText('本次测速未完成，历史结果不代表本次成功；请重试。', 'Bandwidth measurement did not succeed. Previous results do not indicate success; retry.') }
      if (!refreshed || (connectionRevision != null && refreshed.connectionRevision !== connectionRevision)) {
        return { ok: false, error: uiText('节点连接已变更或结果被更新，请重新测速。', 'The connection or result changed during measurement. Measure again.') }
      }
      const speed = refreshed.capabilityDetails.speed
      const observedAt = Date.parse(speed?.observed_at ?? '')
      const previousAt = Date.parse(previousObservedAt ?? '')
      // Compare server observations with each other, not the browser clock (which can drift).
      if (!Number.isFinite(observedAt) || (Number.isFinite(previousAt) && observedAt <= previousAt)) {
        return { ok: false, error: uiText('任务已结束，但未取得新的测速证据；请刷新详情后重试。', 'The task ended without new speed evidence. Refresh details and retry.') }
      }
      if (!speed || speed.stale || speed.verdict !== 'available' || !Number.isFinite(speed.throughput) || (speed.throughput ?? 0) <= 0) {
        return { ok: false, error: speed?.stale
          ? uiText('本次结果已失效，请重新测速。', 'This result is stale. Measure again.')
          : uiText('本次测速未获得可用吞吐结果，请重试。', 'No usable bandwidth result was obtained. Retry.') }
      }
      return { ok: true }
    } catch {
      return { ok: false, error: uiText('本次测速请求或结果刷新失败，历史结果不代表本次成功；请重试。', 'Bandwidth request or result refresh failed. Previous results do not indicate success; retry.') }
    } finally {
      speedingNodeId.value = null
    }
  }

  async function probeNodesBatch(logicalIds: string[], kinds?: ProbeKind[]) {
    return submitProbe(logicalIds, kinds)
  }

  async function updateNodeConnection(
    logicalId: string,
    draft: Partial<NodeConnectionProfile> & {
      displayName?: string
    }
  ): Promise<{ ok: boolean; error?: string; node?: NormalizedNode }> {
    const target =
      (selectedNode.value?.logicalId === logicalId ? selectedNode.value : null) ??
      items.value.find((n) => n.logicalId === logicalId) ??
      null
    if (!target) {
      return { ok: false, error: uiText('未找到目标节点', 'Target node not found') }
    }

    const candidateConn: NodeConnectionProfile = {
      ...target.connection,
      ...draft,
    }

    const validationErr = validateNodeConnectionProfile(target.protocol, candidateConn)
    if (validationErr) {
      return { ok: false, error: validationErr }
    }

    const patchBody: Record<string, unknown> = {}
    if (draft.displayName !== undefined) {
      patchBody.display_name = draft.displayName.trim()
    }
    if (draft.server !== undefined) {
      patchBody.server = draft.server.trim()
    }
    if (draft.port !== undefined) {
      patchBody.port = draft.port
    }
    const proto = target.protocol.trim().toLowerCase()
    if (proto === 'wireguard') {
      if (draft.localAddress !== undefined) patchBody.local_address = draft.localAddress
      if (draft.publicKey !== undefined) patchBody.public_key = draft.publicKey.trim()
      if (draft.privateKey !== undefined) patchBody.private_key = draft.privateKey.trim()
      if (draft.preSharedKey !== undefined) patchBody.pre_shared_key = draft.preSharedKey.trim()
      if (draft.reserved !== undefined && draft.reserved.length > 0) patchBody.reserved = draft.reserved
      if (draft.mtu !== undefined) patchBody.mtu = draft.mtu
      if (draft.dns !== undefined) patchBody.dns = draft.dns
    } else if (proto === 'tuic') {
      if (draft.uuid !== undefined) patchBody.uuid = draft.uuid.trim()
      if (draft.password !== undefined) patchBody.password = draft.password.trim()
      if (draft.congestionControl !== undefined) patchBody.congestion_control = draft.congestionControl.trim()
      if (draft.udpRelayMode !== undefined) patchBody.udp_relay_mode = draft.udpRelayMode.trim()
      if (draft.alpn !== undefined) patchBody.alpn = draft.alpn
      if (draft.sni !== undefined) patchBody.sni = draft.sni.trim()
      if (draft.disableSni !== undefined) patchBody.disable_sni = draft.disableSni
    } else {
      if (draft.uuid !== undefined && draft.uuid.trim()) patchBody.uuid = draft.uuid.trim()
      if (draft.method !== undefined && draft.method.trim()) patchBody.method = draft.method.trim()
      if (draft.password !== undefined && draft.password.trim()) patchBody.password = draft.password.trim()
      if (draft.sni !== undefined) patchBody.sni = draft.sni.trim()
      if (draft.alpn !== undefined) patchBody.alpn = draft.alpn
      if (draft.disableSni !== undefined) patchBody.disable_sni = draft.disableSni
    }

    savingConnection.value = true
    nextResultRead(logicalId)
    try {
      const res = await api.patch<NodeDetailResponse | NodeRecord>(
        `/api/v1/nodes/${encodeURIComponent(logicalId)}/connection`,
        patchBody
      )
      const rawNode = extractNodeRecord(res)
      if (!rawNode) {
        return { ok: false, error: uiText('服务器未返回更新后的节点详情', 'The server did not return updated node details') }
      }
      const updatedNode = applyPoolStateToNode(normalizeNode(rawNode))
      // 保存成功后失效化较早的节点读取，避免旧响应覆盖已接受的连接更新
      nextResultRead(logicalId)
      connectionUpdates.set(logicalId, { sequence: ++connectionUpdateSequence, node: updatedNode })
      const idx = items.value.findIndex((n) => n.logicalId === logicalId)
      if (idx >= 0) {
        items.value[idx] = updatedNode
      }
      if (selectedNode.value?.logicalId === logicalId) {
        selectedNode.value = updatedNode
      }
      return { ok: true, node: updatedNode }
    } catch (cause) {
      const msg = displayText(cause instanceof Error ? cause.message : uiText('保存节点连接参数失败', 'Failed to save node connection parameters'))
      return { ok: false, error: msg }
    } finally {
      savingConnection.value = false
    }
  }

  return {
    items,
    loading,
    loadingMore,
    loadingDetail,
    savingConnection,
    probingNodeId,
    speedingNodeId,
    measureNodeSpeed,
    probingNodeIds,
    queuedNodeIds,
    pendingProbeIds,
    probePendingIds,
    probeStates,
    submittingProbes,
    pendingResultRefreshIds,
    resultRefreshFailedIds,
    poolError,
    error,
    appendError,
    failureMode,
    total,
    hasMore,
    initialLoaded,
    protocolFilter,
    healthFilter,
    riskBandFilter,
    riskScoreMax,
    speedMinKiBS,
    searchQuery,
    selectedNode,
    nodeDetailError,
    isDetailStale,
    detailErrorMessage,
    globalProbeKinds,
    fetchGlobalScheduleKinds,
    load,
    loadMore,
    syncPoolStatus,
    fetchNodeDetail,
    invalidateNodeDetail,
    fetchNodeSourceHistory,
    probeSingleNode,
    probeNodesBatch,
    updateNodeConnection,
  }
}
