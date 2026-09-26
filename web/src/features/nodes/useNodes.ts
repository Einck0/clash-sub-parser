import { computed, ref } from 'vue'
import { api } from '../../api/client'
import {
  normalizeNode,
  validateNodeConnectionProfile,
  type NodeRecord,
  type NodeSourceRecord,
  type NormalizedNode,
  type SafeNodeConnectionProfile,
} from './nodeView'

interface NodePage {
  items: NodeRecord[]
  page: number
  page_size: number
  total: number
}

interface NodeDetailResponse {
  node?: NodeRecord
  sources?: NodeSourceRecord[]
}

export function useNodes() {
  const items = ref<NormalizedNode[]>([])
  const loading = ref(false)
  const loadingMore = ref(false)
  const loadingDetail = ref(false)
  const savingConnection = ref(false)
  const error = ref('')
  const page = ref(0)
  const pageSize = 100
  const total = ref(0)
  const protocolFilter = ref<string>('all')
  const searchQuery = ref<string>('')
  const selectedNode = ref<NormalizedNode | null>(null)
  const requestGeneration = ref(0)
  const hasMore = computed(() => items.value.length < total.value)

  async function loadPage(nextPage: number, append = false, generation = requestGeneration.value) {
    if (append && (loadingMore.value || loading.value)) return
    if (append) loadingMore.value = true
    else loading.value = true
    error.value = ''
    try {
      const params: Record<string, string | number> = {
        page: nextPage,
        page_size: pageSize,
        sort_by: 'display_name',
        sort_order: 'asc',
      }
      if (protocolFilter.value && protocolFilter.value !== 'all') {
        params.protocol = protocolFilter.value
      }
      if (searchQuery.value.trim()) {
        params.search = searchQuery.value.trim()
      }
      const result = await api.get<NodePage>('/api/v1/nodes', { params })
      if (generation !== requestGeneration.value) return
      const normalized = (result.items || []).map(normalizeNode)
      items.value = append ? [...items.value, ...normalized] : normalized
      page.value = result.page
      total.value = result.total
    } catch (cause) {
      if (generation === requestGeneration.value) {
        error.value = cause instanceof Error ? cause.message : 'Unable to load nodes'
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

  async function updateNodeConnection(
    logicalId: string,
    draft: Partial<SafeNodeConnectionProfile> & {
      displayName?: string
      expectedCredentialVersion?: number
      privateKeyInput?: string
      preSharedKeyInput?: string
      passwordInput?: string
    }
  ): Promise<{ ok: boolean; error?: string; node?: NormalizedNode }> {
    const target =
      (selectedNode.value?.logicalId === logicalId ? selectedNode.value : null) ??
      items.value.find((n) => n.logicalId === logicalId) ??
      null
    if (!target) {
      return { ok: false, error: 'Node not found' }
    }
    if (!target.connection.available) {
      return {
        ok: false,
        error: `Node credentials are unavailable (${target.connection.unavailableReason || 'credential_unavailable'})`,
      }
    }

    const candidateConn: SafeNodeConnectionProfile = {
      ...target.connection,
      ...draft,
      privateKeyMasked: '***',
      preSharedKeyMasked: '***',
      passwordMasked: '***',
      hasPrivateKey: Boolean(target.connection.hasPrivateKey || Boolean(draft.privateKeyInput?.trim())),
      hasPreSharedKey: Boolean(target.connection.hasPreSharedKey || Boolean(draft.preSharedKeyInput?.trim())),
      hasPassword: Boolean(target.connection.hasPassword || Boolean(draft.passwordInput?.trim())),
    }

    const validationErr = validateNodeConnectionProfile(target.protocol, {
      ...candidateConn,
      privateKeyInput: draft.privateKeyInput,
      preSharedKeyInput: draft.preSharedKeyInput,
      passwordInput: draft.passwordInput,
    })
    if (validationErr) {
      return { ok: false, error: validationErr }
    }

    const expectedVersion = draft.expectedCredentialVersion ?? target.credentialVersion ?? 1
    const patchBody: Record<string, unknown> = {
      expected_credential_version: expectedVersion,
    }
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
      if (draft.reserved !== undefined && draft.reserved.length > 0) patchBody.reserved = draft.reserved
      if (draft.mtu !== undefined) patchBody.mtu = draft.mtu
      if (draft.dns !== undefined) patchBody.dns = draft.dns
    } else if (proto === 'tuic') {
      if (draft.uuid !== undefined) patchBody.uuid = draft.uuid.trim()
      if (draft.congestionControl !== undefined) patchBody.congestion_control = draft.congestionControl.trim()
      if (draft.udpRelayMode !== undefined) patchBody.udp_relay_mode = draft.udpRelayMode.trim()
      if (draft.alpn !== undefined && draft.alpn.length > 0) patchBody.alpn = draft.alpn
      if (draft.sni !== undefined) patchBody.sni = draft.sni.trim()
      if (draft.disableSni !== undefined) patchBody.disable_sni = draft.disableSni
    } else {
      if (draft.uuid !== undefined && draft.uuid.trim()) patchBody.uuid = draft.uuid.trim()
      if (draft.method !== undefined && draft.method.trim()) patchBody.method = draft.method.trim()
    }

    if (draft.privateKeyInput && draft.privateKeyInput.trim()) {
      patchBody.private_key_input = draft.privateKeyInput.trim()
    }
    if (draft.preSharedKeyInput && draft.preSharedKeyInput.trim()) {
      patchBody.pre_shared_key_input = draft.preSharedKeyInput.trim()
    }
    if (draft.passwordInput && draft.passwordInput.trim()) {
      patchBody.password_input = draft.passwordInput.trim()
    }

    savingConnection.value = true
    try {
      const res = await api.patch<NodeDetailResponse | NodeRecord>(
        `/api/v1/nodes/${encodeURIComponent(logicalId)}/connection`,
        patchBody
      )
      const rawNode = extractNodeRecord(res)
      if (!rawNode) {
        return { ok: false, error: 'Server did not return updated node detail' }
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
      const msg = cause instanceof Error ? cause.message : 'Failed to persist node connection changes'
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
    error,
    total,
    hasMore,
    protocolFilter,
    searchQuery,
    selectedNode,
    load,
    loadMore,
    fetchNodeDetail,
    updateNodeConnection,
  }
}
