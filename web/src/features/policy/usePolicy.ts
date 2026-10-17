import { ref } from 'vue'
import { api, ApiError } from '../../api/client'
import type {
  AdmissionRule,
  GlobalNodeFilter,
  GroupDraftPreviewPayload,
  GroupDraftPreviewResponse,
  GroupEdge,
  GroupType,
  MemberSource,
  NodeFilterSpec,
  PolicyGroup,
  PolicyRule,
  RuleAction,
  ValidationResult,
} from './policyTypes'
import { normalizeGroupSources } from './policyTypes'
import type { PreviewResult } from '../publications/publicationTypes'

interface PaginatedGroups {
  items?: PolicyGroup[]
  groups?: PolicyGroup[]
  page?: number
  page_size?: number
  total?: number
}

interface RulesResult {
  revision_id: string
  admission_rules: AdmissionRule[]
  policy_rules: PolicyRule[]
  total: number
}

export interface CreateGroupInput {
  name: string
  group_type: GroupType
  position?: number
  sources?: MemberSource[]
  edges?: GroupEdge[]
  node_filter?: NodeFilterSpec | null
  empty_fallback_pass?: boolean
  inherit_global_filter?: boolean
}

export interface UpdateGroupInput {
  name?: string
  group_type?: GroupType
  position?: number
  sources?: MemberSource[]
  empty_fallback_pass?: boolean
  inherit_global_filter?: boolean
  expected_revision?: number
  node_filter?: NodeFilterSpec | null
  edges?: GroupEdge[]
}

export function usePolicy() {
  const groups = ref<PolicyGroup[]>([])
  const globalFilter = ref<GlobalNodeFilter | null>(null)
  const admissionRules = ref<AdmissionRule[]>([])
  const policyRules = ref<PolicyRule[]>([])
  const loading = ref(false)
  const loadingGlobalFilter = ref(false)
  const saving = ref(false)
  const savingGlobalFilter = ref(false)
  const validating = ref(false)
  const validationResult = ref<ValidationResult | null>(null)
  const validationState = ref<'idle' | 'validating' | 'success' | 'warning' | 'error' | 'incomplete'>('idle')
  const validationStale = ref(false)
  const validationError = ref('')
  const latestRevisionId = ref('')
  const error = ref('')
  const page = ref(1)
  const pageSize = ref(50)
  const searchQuery = ref('')
  const totalGroups = ref(0)
  const allTopologyGroups = ref<PolicyGroup[]>([])

  let validateSeq = 0
  let validateAbortController: AbortController | null = null
  let loadGroupsSeq = 0

  async function loadGroups(p = page.value, ps = pageSize.value, search?: string) {
    const seq = ++loadGroupsSeq
    loading.value = true
    error.value = ''
    try {
      const targetPage = Math.max(1, p)
      const targetPageSize = Math.min(100, Math.max(1, ps || 50))
      const params: Record<string, string | number> = { page: targetPage, page_size: targetPageSize }
      const effectiveSearch = search !== undefined ? search : searchQuery.value
      if (effectiveSearch && effectiveSearch.trim()) {
        params.search = effectiveSearch.trim()
      }
      const res = await api.get<PaginatedGroups>('/api/v1/policies/groups', { params })
      if (seq !== loadGroupsSeq) return true

      const rawItems = res.items || res.groups || []
      const normalized = rawItems.map((g) => ({
        ...g,
        sources: normalizeGroupSources(g),
      }))
      groups.value = normalized
      totalGroups.value = res.total ?? normalized.length
      page.value = res.page || targetPage
      pageSize.value = res.page_size || targetPageSize
      if (search !== undefined) {
        searchQuery.value = search
      }
      return true
    } catch (err) {
      if (seq === loadGroupsSeq) {
        error.value = err instanceof Error ? err.message : '加载策略组失败'
      }
      return false
    } finally {
      if (seq === loadGroupsSeq) {
        loading.value = false
      }
    }
  }

  async function loadAllTopologyGroups(force = false): Promise<PolicyGroup[]> {
    if (!force && allTopologyGroups.value.length > 0 && totalGroups.value > 0 && allTopologyGroups.value.length >= totalGroups.value) {
      return allTopologyGroups.value
    }
    if (!force && !searchQuery.value && page.value === 1 && totalGroups.value > 0 && groups.value.length >= totalGroups.value) {
      allTopologyGroups.value = [...groups.value]
      return allTopologyGroups.value
    }
    try {
      const all: PolicyGroup[] = []
      let p = 1
      const ps = 100
      while (true) {
        const res = await api.get<PaginatedGroups>('/api/v1/policies/groups', {
          params: { page: p, page_size: ps },
        })
        const items = (res.items || res.groups || []).map((g) => ({
          ...g,
          sources: normalizeGroupSources(g),
        }))
        all.push(...items)
        if (items.length === 0 || all.length >= (res.total || 0) || items.length < ps) {
          break
        }
        p++
      }
      allTopologyGroups.value = all
      return all
    } catch {
      allTopologyGroups.value = groups.value
      return groups.value
    }
  }

  async function createGroup(
    dataOrName: CreateGroupInput | string,
    groupTypeLegacy?: GroupType,
    edgesLegacy: GroupEdge[] = [],
    nodeFilterLegacy?: NodeFilterSpec | null,
    emptyFallbackPassLegacy = false
  ): Promise<PolicyGroup> {
    saving.value = true
    validationStale.value = true
    error.value = ''
    try {
      let name = ''
      let groupType: GroupType = 'select'
      let emptyFallbackPass = false
      let inheritGlobalFilter = true
      let sources: MemberSource[] = []

      if (typeof dataOrName === 'string') {
        name = dataOrName.trim()
        groupType = groupTypeLegacy || 'select'
        emptyFallbackPass = emptyFallbackPassLegacy
        sources = normalizeGroupSources({ edges: edgesLegacy, node_filter: nodeFilterLegacy })
      } else {
        name = dataOrName.name.trim()
        groupType = dataOrName.group_type
        emptyFallbackPass = Boolean(dataOrName.empty_fallback_pass)
        if (dataOrName.inherit_global_filter !== undefined) {
          inheritGlobalFilter = Boolean(dataOrName.inherit_global_filter)
        }
        if (Array.isArray(dataOrName.sources)) {
          sources = dataOrName.sources.map((s, idx) => ({ ...s, position: idx }))
        } else {
          sources = normalizeGroupSources({ edges: dataOrName.edges, node_filter: dataOrName.node_filter })
        }
      }

      const payload: Record<string, unknown> = {
        name,
        group_type: groupType,
        empty_fallback_pass: emptyFallbackPass,
        inherit_global_filter: inheritGlobalFilter,
      }
      if (typeof dataOrName === 'object' && dataOrName !== null && dataOrName.position !== undefined) {
        payload.position = dataOrName.position
      }
      payload.sources = sources
      const created = await api.post<PolicyGroup>('/api/v1/policies/groups', payload)
      const normalized: PolicyGroup = {
        ...created,
        sources: normalizeGroupSources(created),
      }
      await reload()
      return normalized
    } catch (err) {
      const msg = err instanceof Error ? err.message : '创建策略组失败'
      error.value = msg
      throw err
    } finally {
      saving.value = false
    }
  }

  async function updateGroup(
    id: string,
    dataOrName?: UpdateGroupInput | string,
    groupTypeLegacy?: GroupType,
    nodeFilterLegacy?: NodeFilterSpec | null,
    emptyFallbackPassLegacy?: boolean
  ): Promise<PolicyGroup> {
    saving.value = true
    validationStale.value = true
    error.value = ''
    try {
      const payload: Record<string, unknown> = {}
      if (typeof dataOrName === 'object' && dataOrName !== null) {
        if (dataOrName.name !== undefined) payload.name = dataOrName.name.trim()
        if (dataOrName.group_type !== undefined) payload.group_type = dataOrName.group_type
        if (dataOrName.position !== undefined) payload.position = dataOrName.position
        if (dataOrName.empty_fallback_pass !== undefined) payload.empty_fallback_pass = dataOrName.empty_fallback_pass
        if (dataOrName.inherit_global_filter !== undefined) payload.inherit_global_filter = dataOrName.inherit_global_filter
        if (dataOrName.expected_revision !== undefined) payload.expected_revision = dataOrName.expected_revision
        if (Array.isArray(dataOrName.sources)) {
          payload.sources = dataOrName.sources.map((s, idx) => ({ ...s, position: idx }))
        } else if (dataOrName.edges !== undefined || dataOrName.node_filter !== undefined) {
          payload.sources = normalizeGroupSources({ edges: dataOrName.edges, node_filter: dataOrName.node_filter })
        }
      } else {
        if (typeof dataOrName === 'string') payload.name = dataOrName.trim()
        if (groupTypeLegacy !== undefined) payload.group_type = groupTypeLegacy
        if (nodeFilterLegacy !== undefined) payload.node_filter = nodeFilterLegacy
        if (emptyFallbackPassLegacy !== undefined) payload.empty_fallback_pass = emptyFallbackPassLegacy
      }

      let updated: PolicyGroup
      if (typeof dataOrName === 'object' && dataOrName !== null) {
        try {
          updated = await api.put<PolicyGroup>(`/api/v1/policies/groups/${encodeURIComponent(id)}`, payload)
        } catch (putErr: any) {
          if ((putErr && !(putErr instanceof ApiError)) || putErr?.status === 405 || putErr?.status === 404) {
            try {
              updated = await api.patch<PolicyGroup>(`/api/v1/policies/groups/${encodeURIComponent(id)}`, payload)
            } catch (patchErr) {
              throw patchErr
            }
          } else {
            throw putErr
          }
        }
      } else {
        updated = await api.patch<PolicyGroup>(`/api/v1/policies/groups/${encodeURIComponent(id)}`, payload)
      }

      const normalized: PolicyGroup = {
        ...updated,
        sources: normalizeGroupSources(updated),
      }
      const idx = groups.value.findIndex((g) => g.id === id)
      if (idx >= 0) groups.value[idx] = normalized
      const topoIdx = allTopologyGroups.value.findIndex((g) => g.id === id)
      if (topoIdx >= 0) allTopologyGroups.value[topoIdx] = normalized
      else allTopologyGroups.value.push(normalized)
      await reload()
      return normalized
    } catch (err) {
      error.value = err instanceof Error ? err.message : '更新策略组失败'
      throw err
    } finally {
      saving.value = false
    }
  }

  async function deleteGroup(id: string) {
    saving.value = true
    validationStale.value = true
    error.value = ''
    try {
      await api.delete(`/api/v1/policies/groups/${encodeURIComponent(id)}`)
      groups.value = groups.value.filter((g) => g.id !== id)
      allTopologyGroups.value = allTopologyGroups.value.filter((g) => g.id !== id)
      await reload()
    } catch (err) {
      error.value = err instanceof Error ? err.message : '删除策略组失败'
      throw err
    } finally {
      saving.value = false
    }
  }

  async function setGroupEdges(groupId: string, edges: GroupEdge[]) {
    saving.value = true
    validationStale.value = true
    error.value = ''
    try {
      await api.put(`/api/v1/policies/groups/${encodeURIComponent(groupId)}/edges`, { edges })
      const grp = groups.value.find((g) => g.id === groupId)
      if (grp) grp.edges = edges
      const topoGrp = allTopologyGroups.value.find((g) => g.id === groupId)
      if (topoGrp) topoGrp.edges = edges
      await reload()
    } catch (err) {
      error.value = err instanceof Error ? err.message : '保存策略组来源失败'
      throw err
    } finally {
      saving.value = false
    }
  }

  async function loadRules(revisionId?: string) {
    loading.value = true
    error.value = ''
    try {
      const params: Record<string, string> = {}
      if (revisionId) params.revision_id = revisionId
      const res = await api.get<RulesResult>('/api/v1/policies/rules', { params })
      admissionRules.value = res.admission_rules || []
      policyRules.value = res.policy_rules || []
      latestRevisionId.value = res.revision_id || ''
      return true
    } catch (err) {
      error.value = err instanceof Error ? err.message : '加载规则失败'
      return false
    } finally {
      loading.value = false
    }
  }

  async function createAdmissionRule(rule: {
    name: string
    expression: string
    action: RuleAction
    position?: number
    revision_id?: string
  }) {
    saving.value = true
    validationStale.value = true
    error.value = ''
    try {
      const payload = {
        kind: 'admission',
        name: rule.name.trim(),
        expression: rule.expression.trim(),
        action: rule.action,
        position: rule.position ?? admissionRules.value.length,
        revision_id: rule.revision_id || '',
      }
      const created = await api.post<AdmissionRule>('/api/v1/policies/rules', payload)
      admissionRules.value.push(created)
      if (created.revision_id) {
        latestRevisionId.value = created.revision_id
      }
      void validateGraph()
      return created
    } catch (err) {
      error.value = err instanceof Error ? err.message : '创建准入规则失败'
      throw err
    } finally {
      saving.value = false
    }
  }

  async function createPolicyRule(rule: {
    target_group_id: string
    expression: string
    position?: number
    revision_id?: string
  }) {
    saving.value = true
    validationStale.value = true
    error.value = ''
    try {
      const payload = {
        kind: 'policy',
        target_group_id: rule.target_group_id,
        expression: rule.expression.trim(),
        position: rule.position ?? policyRules.value.length,
        revision_id: rule.revision_id || '',
      }
      const created = await api.post<PolicyRule>('/api/v1/policies/rules', payload)
      policyRules.value.push(created)
      if (created.revision_id) {
        latestRevisionId.value = created.revision_id
      }
      void validateGraph()
      return created
    } catch (err) {
      error.value = err instanceof Error ? err.message : '创建分流规则失败'
      throw err
    } finally {
      saving.value = false
    }
  }

  async function deleteRule(id: string): Promise<void> {
    saving.value = true
    validationStale.value = true
    error.value = ''
    try {
      await api.delete(`/api/v1/policies/rules/${encodeURIComponent(id)}`)
      admissionRules.value = admissionRules.value.filter((r) => r.id !== id)
      policyRules.value = policyRules.value.filter((r) => r.id !== id)
      void validateGraph()
    } catch (err) {
      error.value = err instanceof Error ? err.message : '删除规则失败'
      throw err
    } finally {
      saving.value = false
    }
  }

  let reloadSeq = 0
  async function reload() {
    const seq = ++reloadSeq
    abortValidation()
    validationStale.value = true
    validationState.value = 'incomplete'
    // Sequential loads keep the first failure visible; validate only the
    // successfully loaded current revision, never a half-loaded screen.
    if (!await loadGroups(page.value, pageSize.value, searchQuery.value) || seq !== reloadSeq) return
    if (!await loadRules() || seq !== reloadSeq) return
    await loadGlobalFilter()
    if (error.value || seq !== reloadSeq) return
    await loadAllTopologyGroups()
    if (seq !== reloadSeq) return
    await validateGraph()
  }

  function abortValidation() {
    ++validateSeq
    validating.value = false
    if (validateAbortController) {
      validateAbortController.abort()
      validateAbortController = null
    }
  }

  async function validateGraph(): Promise<ValidationResult> {
    if (validateAbortController) {
      validateAbortController.abort()
      validateAbortController = null
    }

    const currentSeq = ++validateSeq
    const controller = new AbortController()
    validateAbortController = controller

    validating.value = true
    validationState.value = 'validating'
    validationError.value = ''

    try {
      const res = await api.post<ValidationResult>('/api/v1/policies/validate', undefined, {
        signal: controller.signal,
      })

      if (currentSeq !== validateSeq) {
        return res
      }

      // Revision match guard: if response has a revision_id and we have a tracked latestRevisionId,
      // ensure they match; otherwise response is from a pre-mutation revision
      if (res.revision_id && latestRevisionId.value && res.revision_id !== latestRevisionId.value) {
        validationStale.value = true
        validationState.value = 'incomplete'
        return res
      }

      if (res.revision_id) {
        latestRevisionId.value = res.revision_id
      }

      validationResult.value = res
      validationStale.value = false
      const hasWarningOrError = res.issues?.some(
        (issue: any) => issue.severity === 'warning' || issue.severity === 'error'
      )
      if (!res.valid) {
        validationState.value = 'error'
      } else if (hasWarningOrError) {
        validationState.value = 'warning'
      } else {
        validationState.value = 'success'
      }
      return res
    } catch (err: any) {
      if (err?.name === 'AbortError') {
        const fallback: ValidationResult = validationResult.value || { valid: false, errors: [] }
        return fallback
      }
      if (currentSeq !== validateSeq) {
        return validationResult.value || { valid: false, errors: [] }
      }

      const msg = err instanceof Error ? err.message : '校验未完成'
      validationError.value = msg
      validationState.value = 'incomplete'
      const fallback: ValidationResult = { valid: false, errors: [msg] }
      validationResult.value = fallback
      return fallback
    } finally {
      if (currentSeq === validateSeq) {
        validating.value = false
      }
    }
  }

  async function loadGlobalFilter(): Promise<GlobalNodeFilter | null> {
    loadingGlobalFilter.value = true
    error.value = ''
    try {
      const res = await api.get<GlobalNodeFilter>('/api/v1/policies/global-node-filter')
      globalFilter.value = res
      return res
    } catch (err) {
      if (err instanceof ApiError && err.status === 404) {
        globalFilter.value = null
        return null
      }
      error.value = err instanceof Error ? err.message : '加载全局节点筛选失败'
      return null
    } finally {
      loadingGlobalFilter.value = false
    }
  }

  async function updateGlobalFilter(spec: NodeFilterSpec): Promise<GlobalNodeFilter> {
    validationStale.value = true
    savingGlobalFilter.value = true
    error.value = ''
    try {
      const res = await api.put<GlobalNodeFilter>('/api/v1/policies/global-node-filter', { spec })
      globalFilter.value = res
      return res
    } catch (err) {
      const msg = err instanceof Error ? err.message : '更新全局节点筛选失败'
      error.value = msg
      throw err
    } finally {
      savingGlobalFilter.value = false
    }
  }

  async function fetchGroupSnapshotPreview(
    payload: GroupDraftPreviewPayload
  ): Promise<GroupDraftPreviewResponse> {
    const body = {
      group_id: payload.group_id,
      expected_revision: payload.expected_revision,
      name: payload.name,
      group_type: payload.group_type,
      sources: (payload.sources || []).map((s, idx) => ({
        ...s,
        position: s.position !== undefined ? s.position : idx,
      })),
      target: payload.target || 'mihomo',
    }
    return await api.post<GroupDraftPreviewResponse>('/api/v1/policies/groups/preview', body)
  }

  async function moveGroup(groupId: string, direction: 'up' | 'down'): Promise<boolean> {
    const list = [...groups.value]
    const idx = list.findIndex((g) => g.id === groupId)
    if (idx < 0) return false
    const targetIdx = direction === 'up' ? idx - 1 : idx + 1
    if (targetIdx < 0 || targetIdx >= list.length) return false

    const current = list[idx]
    const target = list[targetIdx]

    saving.value = true
    try {
      const targetPos = target.position !== undefined ? target.position : targetIdx
      const currentPos = current.position !== undefined ? current.position : idx

      let newCurrentPos = targetPos
      let newTargetPos = currentPos
      if (newCurrentPos === newTargetPos) {
        newCurrentPos = direction === 'up' ? targetPos : targetPos + 1
        newTargetPos = direction === 'up' ? targetPos + 1 : targetPos
      }

      await api.patch(`/api/v1/policies/groups/${encodeURIComponent(current.id)}`, {
        position: newCurrentPos,
        expected_revision: current.revision,
      })
      await api.patch(`/api/v1/policies/groups/${encodeURIComponent(target.id)}`, {
        position: newTargetPos,
        expected_revision: target.revision,
      })
      await loadGroups()
      return true
    } catch (err) {
      error.value = err instanceof Error ? err.message : '调整策略组顺序失败'
      return false
    } finally {
      saving.value = false
    }
  }

  return {
    groups,
    globalFilter,
    admissionRules,
    policyRules,
    loading,
    loadingGlobalFilter,
    saving,
    savingGlobalFilter,
    validating,
    validationResult,
    validationState,
    validationStale,
    validationError,
    latestRevisionId,
    abortValidation,
    error,
    page,
    pageSize,
    searchQuery,
    totalGroups,
    allTopologyGroups,
    loadGroups,
    loadAllTopologyGroups,
    reload,
    createGroup,
    updateGroup,
    moveGroup,
    deleteGroup,
    setGroupEdges,
    loadRules,
    createAdmissionRule,
    createPolicyRule,
    deleteRule,
    validateGraph,
    loadGlobalFilter,
    updateGlobalFilter,
    fetchGroupSnapshotPreview,
  }
}
