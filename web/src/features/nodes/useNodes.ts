import { computed, ref } from 'vue'
import { api } from '../../api/client'
import type { NodeSourceHistoryData } from './sourceHistoryTypes'
import {
  generateIdempotencyKey,
  type CreateProbeRunResponse,
  type ProbeKind,
  type ProbePoolStatus,
  type ProbeRun,
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

export function useNodes() {
  const items = ref<NormalizedNode[]>([])
  const loading = ref(false)
  const loadingMore = ref(false)
  const loadingDetail = ref(false)
  const savingConnection = ref(false)
  const probingNodeId = ref<string | null>(null)
  const probingNodeIds = ref<Set<string>>(new Set())
  const queuedNodeIds = ref<Set<string>>(new Set())
  const error = ref('')
  const page = ref(0)
  const pageSize = 100
  const total = ref(0)
  const protocolFilter = ref<string>('all')
  const healthFilter = ref<HealthFilterType>('all')
  const searchQuery = ref<string>('')
  const selectedNode = ref<NormalizedNode | null>(null)
  const requestGeneration = ref(0)
  const hasMore = computed(() => items.value.length < total.value)

  function applyPoolStateToNode(node: NormalizedNode): NormalizedNode {
    if (probingNodeIds.value.has(node.logicalId) || probingNodeId.value === node.logicalId) {
      return { ...node, probeState: 'probing' }
    }
    if (queuedNodeIds.value.has(node.logicalId)) {
      return { ...node, probeState: 'queued' }
    }
    if (node.healthStatus === 'probing') {
      return { ...node, probeState: 'probing' }
    }
    if (node.healthStatus === 'queued') {
      return { ...node, probeState: 'queued' }
    }
    return { ...node, probeState: node.probeState ?? 'idle' }
  }

  async function syncPoolStatus(): Promise<void> {
    try {
      const res = await api.get<ProbePoolStatus>('/api/v1/probes/pool')
      if (
        res &&
        typeof res === 'object' &&
        (Array.isArray(res.probing_node_ids) || Array.isArray(res.queued_node_ids))
      ) {
        const nextProbing = new Set<string>(
          Array.isArray(res.probing_node_ids) ? res.probing_node_ids : []
        )
        const nextQueued = new Set<string>(
          (Array.isArray(res.queued_node_ids) ? res.queued_node_ids : []).filter(
            (id) => !nextProbing.has(id)
          )
        )
        probingNodeIds.value = nextProbing
        queuedNodeIds.value = nextQueued
        if (items.value.length > 0) {
          items.value = items.value.map(applyPoolStateToNode)
        }
        if (selectedNode.value) {
          selectedNode.value = applyPoolStateToNode(selectedNode.value)
        }
      }
    } catch {
      // Non-blocking if /api/v1/probes/pool is unavailable in isolated unit tests
    }
  }

  async function loadPage(nextPage: number, append = false, generation = requestGeneration.value) {
    if (append && (loadingMore.value || loading.value)) return
    if (append) loadingMore.value = true
    else loading.value = true
    error.value = ''
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
      if (searchQuery.value.trim()) {
        params.search = searchQuery.value.trim()
      }
      if (healthFilter.value && healthFilter.value !== 'all' && healthFilter.value !== 'probing') {
        if (healthFilter.value === 'unknown') {
          params.health_status = 'undetermined,untested'
        } else {
          params.health_status = healthFilter.value
        }
      }
      const [result] = await Promise.all([
        api.get<NodePage>('/api/v1/nodes', { params }),
        syncPoolStatus(),
      ])
      if (generation !== requestGeneration.value) return
      const normalized = (result.items || []).map((raw) => applyPoolStateToNode(normalizeNode(raw)))
      items.value = append ? [...items.value, ...normalized] : normalized
      page.value = result.page
      total.value = result.total
    } catch (cause) {
      if (generation === requestGeneration.value) {
        error.value = cause instanceof Error ? cause.message : '加载节点列表失败'
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
    if (loadingMore.value) loadingMore.value = false
    await loadPage(1, false, requestGeneration.value)
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

  async function fetchNodeDetail(logicalId: string): Promise<NormalizedNode | null> {
    const existing = items.value.find((n) => n.logicalId === logicalId) ?? null
    if (existing) {
      selectedNode.value = existing
    }
    loadingDetail.value = true
    try {
      const res = await api.get<NodeDetailResponse | NodeRecord>(`/api/v1/nodes/${encodeURIComponent(logicalId)}`)
      const rawNode = extractNodeRecord(res)
      if (rawNode) {
        const detailed = normalizeNode(rawNode)
        selectedNode.value = detailed
        const idx = items.value.findIndex((n) => n.logicalId === logicalId)
        if (idx >= 0) {
          items.value[idx] = detailed
        }
        return detailed
      }
      return existing
    } catch {
      return existing
    } finally {
      loadingDetail.value = false
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

  async function probeSingleNode(
    logicalId: string,
    kinds: ProbeKind[] = ['baseline', 'streaming', 'ai', 'ip_risk', 'geo']
  ): Promise<{ ok: boolean; runId?: string; error?: string }> {
    probingNodeId.value = logicalId
    const idx = items.value.findIndex((n) => n.logicalId === logicalId)
    if (idx >= 0) {
      items.value[idx] = { ...items.value[idx], probeState: 'probing' }
    }
    if (selectedNode.value?.logicalId === logicalId) {
      selectedNode.value = { ...selectedNode.value, probeState: 'probing' }
    }
    try {
      const res = await api.post<CreateProbeRunResponse>(
        '/api/v1/probes/runs',
        {
          config_revision: '',
          node_logical_ids: [logicalId],
          kinds,
        },
        {
          headers: {
            'Idempotency-Key': generateIdempotencyKey(),
          },
        }
      )
      // Poll briefly for completion so single-node probe feels instant
      if (res?.run_id) {
        for (let attempt = 0; attempt < 6; attempt++) {
          await new Promise((r) => setTimeout(r, 350))
          try {
            const run = await api.get<ProbeRun>(`/api/v1/probes/runs/${encodeURIComponent(res.run_id)}`)
            if (run && ['succeeded', 'failed', 'cancelled', 'expired'].includes(run.state)) {
              break
            }
          } catch {
            break
          }
        }
      }
      await fetchNodeDetail(logicalId)
      return { ok: true, runId: res?.run_id }
    } catch (cause) {
      const msg = cause instanceof Error ? cause.message : '发起节点测速失败'
      return { ok: false, error: msg }
    } finally {
      probingNodeId.value = null
      const currentIdx = items.value.findIndex((n) => n.logicalId === logicalId)
      if (
        currentIdx >= 0 &&
        items.value[currentIdx].probeState === 'probing' &&
        items.value[currentIdx].healthStatus !== 'probing'
      ) {
        items.value[currentIdx] = { ...items.value[currentIdx], probeState: 'idle' }
      }
      if (
        selectedNode.value?.logicalId === logicalId &&
        selectedNode.value.probeState === 'probing' &&
        selectedNode.value.healthStatus !== 'probing'
      ) {
        selectedNode.value = { ...selectedNode.value, probeState: 'idle' }
      }
    }
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
      return { ok: false, error: '未找到目标节点' }
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
      if (draft.alpn !== undefined && draft.alpn.length > 0) patchBody.alpn = draft.alpn
      if (draft.sni !== undefined) patchBody.sni = draft.sni.trim()
      if (draft.disableSni !== undefined) patchBody.disable_sni = draft.disableSni
    } else {
      if (draft.uuid !== undefined && draft.uuid.trim()) patchBody.uuid = draft.uuid.trim()
      if (draft.method !== undefined && draft.method.trim()) patchBody.method = draft.method.trim()
      if (draft.password !== undefined && draft.password.trim()) patchBody.password = draft.password.trim()
      if (draft.sni !== undefined && draft.sni.trim()) patchBody.sni = draft.sni.trim()
      if (draft.alpn !== undefined && draft.alpn.length > 0) patchBody.alpn = draft.alpn
      if (draft.disableSni !== undefined) patchBody.disable_sni = draft.disableSni
    }

    savingConnection.value = true
    try {
      const res = await api.patch<NodeDetailResponse | NodeRecord>(
        `/api/v1/nodes/${encodeURIComponent(logicalId)}/connection`,
        patchBody
      )
      const rawNode = extractNodeRecord(res)
      if (!rawNode) {
        return { ok: false, error: '服务器未返回更新后的节点详情' }
      }
      const updatedNode = normalizeNode(rawNode)
      const idx = items.value.findIndex((n) => n.logicalId === logicalId)
      if (idx >= 0) {
        items.value[idx] = updatedNode
      }
      if (selectedNode.value?.logicalId === logicalId) {
        selectedNode.value = updatedNode
      }
      return { ok: true, node: updatedNode }
    } catch (cause) {
      const msg = cause instanceof Error ? cause.message : '保存节点连接参数失败'
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
    probingNodeIds,
    queuedNodeIds,
    error,
    total,
    hasMore,
    protocolFilter,
    healthFilter,
    searchQuery,
    selectedNode,
    load,
    loadMore,
    syncPoolStatus,
    fetchNodeDetail,
    fetchNodeSourceHistory,
    probeSingleNode,
    updateNodeConnection,
  }
}
