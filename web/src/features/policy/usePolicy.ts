import { ref } from 'vue'
import { api, ApiError } from '../../api/client'
import type {
  AdmissionRule,
  GlobalNodeFilter,
  GroupEdge,
  GroupType,
  NodeFilterSpec,
  PolicyGroup,
  PolicyRule,
  RuleAction,
  ValidationResult,
} from './policyTypes'

interface PaginatedGroups {
  items: PolicyGroup[]
  page: number
  page_size: number
  total: number
}

interface RulesResult {
  revision_id: string
  admission_rules: AdmissionRule[]
  policy_rules: PolicyRule[]
  total: number
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
  const totalGroups = ref(0)

  let validateSeq = 0
  let validateAbortController: AbortController | null = null

  async function loadGroups(search?: string) {
    loading.value = true
    error.value = ''
    try {
      const params: Record<string, string | number> = { page: 1, page_size: 100 }
      if (search) params.search = search
      const res = await api.get<PaginatedGroups>('/api/v1/policies/groups', { params })
      groups.value = res.items || []
      totalGroups.value = res.total || 0
      return true
    } catch (err) {
      error.value = err instanceof Error ? err.message : '加载策略组失败'
      return false
    } finally {
      loading.value = false
    }
  }

  async function createGroup(name: string, groupType: GroupType, edges: GroupEdge[] = [], nodeFilter?: NodeFilterSpec | null, emptyFallbackPass = false): Promise<PolicyGroup> {
    saving.value = true
    validationStale.value = true
    error.value = ''
    try {
      const payload: Record<string, unknown> = {
        name: name.trim(),
        group_type: groupType,
        empty_fallback_pass: emptyFallbackPass,
        edges,
      }
      if (nodeFilter !== undefined) {
        payload.node_filter = nodeFilter
      }
      const created = await api.post<PolicyGroup>('/api/v1/policies/groups', payload)
      await reload()
      return created
    } catch (err) {
      const msg = err instanceof Error ? err.message : '创建策略组失败'
      error.value = msg
      throw err
    } finally {
      saving.value = false
    }
  }

  async function updateGroup(id: string, name?: string, groupType?: GroupType, nodeFilter?: NodeFilterSpec | null, emptyFallbackPass?: boolean): Promise<PolicyGroup> {
    saving.value = true
    validationStale.value = true
    error.value = ''
    try {
      const payload: Record<string, unknown> = {}
      if (emptyFallbackPass !== undefined) payload.empty_fallback_pass = emptyFallbackPass
      if (name !== undefined) payload.name = name.trim()
      if (groupType !== undefined) payload.group_type = groupType
      if (nodeFilter !== undefined) payload.node_filter = nodeFilter
      const updated = await api.patch<PolicyGroup>(`/api/v1/policies/groups/${encodeURIComponent(id)}`, payload)
      const idx = groups.value.findIndex((g) => g.id === id)
      if (idx >= 0) groups.value[idx] = updated
      await reload()
      return updated
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
      await reload()
    } catch (err) {
      error.value = err instanceof Error ? err.message : '保存策略组连接边失败'
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
    if (!await loadGroups() || seq !== reloadSeq) return
    if (!await loadRules() || seq !== reloadSeq) return
    await loadGlobalFilter()
    if (error.value || seq !== reloadSeq) return
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
      if (!res.valid) {
        validationState.value = 'error'
      } else if (res.issues && res.issues.length > 0) {
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
    totalGroups,
    loadGroups,
    reload,
    createGroup,
    updateGroup,
    deleteGroup,
    setGroupEdges,
    loadRules,
    createAdmissionRule,
    createPolicyRule,
    deleteRule,
    validateGraph,
    loadGlobalFilter,
    updateGlobalFilter,
  }
}
