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
  const error = ref('')
  const totalGroups = ref(0)

  async function loadGroups(search?: string) {
    loading.value = true
    error.value = ''
    try {
      const params: Record<string, string | number> = { page: 1, page_size: 100 }
      if (search) params.search = search
      const res = await api.get<PaginatedGroups>('/api/v1/policies/groups', { params })
      groups.value = res.items || []
      totalGroups.value = res.total || 0
    } catch (err) {
      error.value = err instanceof Error ? err.message : 'Failed to load policy groups'
    } finally {
      loading.value = false
    }
  }

  async function createGroup(name: string, groupType: GroupType, edges: GroupEdge[] = [], nodeFilter?: NodeFilterSpec | null): Promise<PolicyGroup> {
    saving.value = true
    error.value = ''
    try {
      const payload: Record<string, unknown> = {
        name: name.trim(),
        group_type: groupType,
        edges,
      }
      if (nodeFilter !== undefined) {
        payload.node_filter = nodeFilter
      }
      const created = await api.post<PolicyGroup>('/api/v1/policies/groups', payload)
      await loadGroups()
      return created
    } catch (err) {
      const msg = err instanceof Error ? err.message : 'Failed to create group'
      error.value = msg
      throw err
    } finally {
      saving.value = false
    }
  }

  async function updateGroup(id: string, name?: string, groupType?: GroupType, nodeFilter?: NodeFilterSpec | null): Promise<PolicyGroup> {
    saving.value = true
    error.value = ''
    try {
      const payload: Record<string, unknown> = {}
      if (name !== undefined) payload.name = name.trim()
      if (groupType !== undefined) payload.group_type = groupType
      if (nodeFilter !== undefined) payload.node_filter = nodeFilter
      const updated = await api.patch<PolicyGroup>(`/api/v1/policies/groups/${id}`, payload)
      const idx = groups.value.findIndex((g) => g.id === id)
      if (idx >= 0) groups.value[idx] = updated
      return updated
    } catch (err) {
      error.value = err instanceof Error ? err.message : 'Failed to update group'
      throw err
    } finally {
      saving.value = false
    }
  }

  async function deleteGroup(id: string) {
    saving.value = true
    error.value = ''
    try {
      await api.delete(`/api/v1/policies/groups/${id}`)
      groups.value = groups.value.filter((g) => g.id !== id)
    } catch (err) {
      error.value = err instanceof Error ? err.message : 'Failed to delete group'
      throw err
    } finally {
      saving.value = false
    }
  }

  async function setGroupEdges(groupId: string, edges: GroupEdge[]) {
    saving.value = true
    error.value = ''
    try {
      await api.put(`/api/v1/policies/groups/${groupId}/edges`, { edges })
      await loadGroups()
    } catch (err) {
      error.value = err instanceof Error ? err.message : 'Failed to set group edges'
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
    } catch (err) {
      error.value = err instanceof Error ? err.message : 'Failed to load rules'
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
      return created
    } catch (err) {
      error.value = err instanceof Error ? err.message : 'Failed to create admission rule'
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
      return created
    } catch (err) {
      error.value = err instanceof Error ? err.message : 'Failed to create policy rule'
      throw err
    } finally {
      saving.value = false
    }
  }

  async function validateGraph(): Promise<ValidationResult> {
    validating.value = true
    error.value = ''
    try {
      const payload = {
        groups: groups.value,
        admission_rules: admissionRules.value,
        policy_rules: policyRules.value,
      }
      const res = await api.post<ValidationResult>('/api/v1/policies/validate', payload)
      validationResult.value = res
      return res
    } catch (err) {
      error.value = err instanceof Error ? err.message : 'Failed to validate graph'
      const fallback: ValidationResult = { valid: false, errors: [error.value] }
      validationResult.value = fallback
      return fallback
    } finally {
      validating.value = false
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
      error.value = err instanceof Error ? err.message : 'Failed to load global node filter'
      return null
    } finally {
      loadingGlobalFilter.value = false
    }
  }

  async function updateGlobalFilter(spec: NodeFilterSpec): Promise<GlobalNodeFilter> {
    savingGlobalFilter.value = true
    error.value = ''
    try {
      const res = await api.put<GlobalNodeFilter>('/api/v1/policies/global-node-filter', { spec })
      globalFilter.value = res
      return res
    } catch (err) {
      const msg = err instanceof Error ? err.message : 'Failed to update global node filter'
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
    error,
    totalGroups,
    loadGroups,
    createGroup,
    updateGroup,
    deleteGroup,
    setGroupEdges,
    loadRules,
    createAdmissionRule,
    createPolicyRule,
    validateGraph,
    loadGlobalFilter,
    updateGlobalFilter,
  }
}
