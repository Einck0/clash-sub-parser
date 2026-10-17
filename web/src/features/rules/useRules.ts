import { ref, computed } from 'vue'
import { api, ApiError } from '../../api/client'
import type {
  RuleCategory,
  PolicyRuleView,
  ListRulesResponse,
  CreateRulePayload,
  UpdateRulePayload,
  CreateCategoryPayload,
  UpdateCategoryPayload,
  RuleValidationResult,
} from './rulesTypes'

export interface PolicyGroupSummary {
  id: string
  name: string
  group_type?: string
}

export function useRules() {
  const categories = ref<RuleCategory[]>([])
  const rules = ref<PolicyRuleView[]>([])
  const groups = ref<PolicyGroupSummary[]>([])
  const activeRevisionId = ref<string>('')

  const loading = ref(false)
  const saving = ref(false)
  const validating = ref(false)
  const error = ref<string | null>(null)

  const searchQuery = ref('')
  const selectedCategoryId = ref<string | null>(null)
  const validationResult = ref<RuleValidationResult | null>(null)

  // Sorted categories by sort_order ascending
  const sortedCategories = computed(() => {
    return [...categories.value].sort((a, b) => a.sort_order - b.sort_order)
  })

  // Map of group ID -> group name
  const groupNameMap = computed(() => {
    const map = new Map<string, string>()
    for (const g of groups.value) {
      map.set(g.id, g.name)
    }
    return map
  })

  // Filtered rules by search query
  const filteredRules = computed(() => {
    const q = searchQuery.value.trim().toLowerCase()
    if (!q) {
      return rules.value
    }
    return rules.value.filter((r) => {
      const expr = (r.expression || '').toLowerCase()
      const name = (r.name || '').toLowerCase()
      const builtin = (r.builtin_target || '').toLowerCase()
      const cat = (r.category || '').toLowerCase()
      const groupName = r.target_group_id ? (groupNameMap.value.get(r.target_group_id) || '').toLowerCase() : ''
      return (
        expr.includes(q) ||
        name.includes(q) ||
        builtin.includes(q) ||
        cat.includes(q) ||
        groupName.includes(q)
      )
    })
  })

  // Rules grouped by category ID (preserving position ASC)
  const rulesByCategory = computed(() => {
    const map = new Map<string, PolicyRuleView[]>()
    // initialize map with all category IDs
    for (const cat of categories.value) {
      map.set(cat.id, [])
    }
    const uncategorized: PolicyRuleView[] = []

    for (const rule of filteredRules.value) {
      if (rule.category_id && map.has(rule.category_id)) {
        map.get(rule.category_id)!.push(rule)
      } else if (rule.category) {
        // Fallback matching by category name if category_id not linked
        const matchedCat = categories.value.find((c) => c.name.toLowerCase() === rule.category?.toLowerCase())
        if (matchedCat && map.has(matchedCat.id)) {
          map.get(matchedCat.id)!.push(rule)
        } else {
          uncategorized.push(rule)
        }
      } else {
        uncategorized.push(rule)
      }
    }

    // Sort rules inside each category by position ascending
    for (const [catId, catRules] of map.entries()) {
      catRules.sort((a, b) => a.position - b.position)
    }
    uncategorized.sort((a, b) => a.position - b.position)

    return { map, uncategorized }
  })

  const totalRulesCount = computed(() => rules.value.length)
  const totalCategoriesCount = computed(() => categories.value.length)

  // Fetch all categories
  async function fetchCategories(): Promise<void> {
    try {
      const res = await api.get<RuleCategory[]>('/api/v1/policies/categories')
      categories.value = Array.isArray(res) ? res : []
    } catch (err: unknown) {
      error.value = err instanceof Error ? err.message : 'Failed to fetch rule categories'
      throw err
    }
  }

  // Fetch all rules
  async function fetchRules(): Promise<void> {
    try {
      let page = 1
      const pageSize = 1000
      let allRules: PolicyRuleView[] = []
      let revId = ''

      while (true) {
        const res = await api.get<ListRulesResponse>('/api/v1/policies/rules', {
          params: { kind: 'policy', page, page_size: pageSize },
        })

        if (!res) break

        if (res.revision_id) {
          revId = res.revision_id
        }

        const currentRules = Array.isArray(res.policy_rules) ? res.policy_rules : []
        allRules = allRules.concat(currentRules)

        const hasMore = typeof res.total === 'number'
          ? allRules.length < res.total
          : currentRules.length === pageSize
        if (!hasMore || currentRules.length === 0) {
          break
        }

        page++
      }

      rules.value = allRules
      if (revId) {
        activeRevisionId.value = revId
      }
    } catch (err: unknown) {
      error.value = err instanceof Error ? err.message : 'Failed to fetch rules'
      throw err
    }
  }

  // Fetch groups for target selector
  async function fetchGroups(): Promise<void> {
    try {
      let page = 1
      const pageSize = 200
      let allGroups: PolicyGroupSummary[] = []

      while (true) {
        const res = await api.get<{
          items?: PolicyGroupSummary[]
          groups?: PolicyGroupSummary[]
          total?: number
          page_size?: number
        }>('/api/v1/policies/groups', {
          params: { page, page_size: pageSize },
        })

        if (!res) break

        const items: PolicyGroupSummary[] = Array.isArray(res.items)
          ? res.items
          : Array.isArray(res.groups)
          ? res.groups
          : []

        allGroups = allGroups.concat(items)

        const hasMore = typeof res.total === 'number'
          ? allGroups.length < res.total
          : items.length === pageSize
        if (!hasMore || items.length === 0) {
          break
        }

        page++
      }

      groups.value = allGroups
    } catch (err: unknown) {
      // Non-fatal if groups fail
      console.warn('Failed to fetch policy groups:', err)
    }
  }

  // Load all initial data
  async function loadData(): Promise<void> {
    loading.value = true
    error.value = null
    try {
      await Promise.all([fetchCategories(), fetchRules(), fetchGroups()])
    } finally {
      loading.value = false
    }
  }

  // Category CRUD
  async function createCategory(payload: CreateCategoryPayload): Promise<RuleCategory> {
    saving.value = true
    try {
      const created = await api.post<RuleCategory>('/api/v1/policies/categories', payload)
      await fetchCategories()
      return created
    } finally {
      saving.value = false
    }
  }

  async function updateCategory(id: string, payload: UpdateCategoryPayload): Promise<RuleCategory> {
    saving.value = true
    try {
      const updated = await api.put<RuleCategory>(`/api/v1/policies/categories/${encodeURIComponent(id)}`, payload)
      await fetchCategories()
      return updated
    } finally {
      saving.value = false
    }
  }

  async function deleteCategory(id: string): Promise<void> {
    saving.value = true
    try {
      await api.delete(`/api/v1/policies/categories/${encodeURIComponent(id)}`)
      await Promise.all([fetchCategories(), fetchRules()])
    } finally {
      saving.value = false
    }
  }

  async function reorderCategories(orderedIds: string[]): Promise<void> {
    saving.value = true
    try {
      await api.put('/api/v1/policies/categories/reorder', { order: orderedIds })
      await fetchCategories()
    } finally {
      saving.value = false
    }
  }

  async function moveCategory(categoryId: string, direction: 'up' | 'down'): Promise<void> {
    const currentList = sortedCategories.value
    const idx = currentList.findIndex((c) => c.id === categoryId)
    if (idx === -1) return
    const targetIdx = direction === 'up' ? idx - 1 : idx + 1
    if (targetIdx < 0 || targetIdx >= currentList.length) return

    const newOrder = [...currentList]
    const [moved] = newOrder.splice(idx, 1)
    newOrder.splice(targetIdx, 0, moved)

    await reorderCategories(newOrder.map((c) => c.id))
  }

  // Rule CRUD
  async function createRule(payload: CreateRulePayload): Promise<PolicyRuleView> {
    saving.value = true
    try {
      const created = await api.post<PolicyRuleView>('/api/v1/policies/rules', {
        ...payload,
        kind: 'policy',
        revision_id: payload.revision_id || activeRevisionId.value,
      })
      await fetchRules()
      return created
    } finally {
      saving.value = false
    }
  }

  async function updateRule(id: string, payload: UpdateRulePayload): Promise<PolicyRuleView> {
    saving.value = true
    try {
      const updated = await api.put<PolicyRuleView>(`/api/v1/policies/rules/${encodeURIComponent(id)}`, payload)
      await fetchRules()
      return updated
    } finally {
      saving.value = false
    }
  }

  async function deleteRule(id: string): Promise<void> {
    saving.value = true
    try {
      await api.delete(`/api/v1/policies/rules/${encodeURIComponent(id)}`, {
        params: { kind: 'policy' },
      })
      await fetchRules()
    } finally {
      saving.value = false
    }
  }

  async function toggleRuleEnabled(rule: PolicyRuleView): Promise<void> {
    const nextState = !rule.enabled
    await updateRule(rule.id, { enabled: nextState })
  }

  async function reorderRules(categoryId: string, orderedIds: string[]): Promise<void> {
    saving.value = true
    try {
      await api.put('/api/v1/policies/rules/reorder', {
        category_id: categoryId,
        order: orderedIds,
        revision_id: activeRevisionId.value,
      })
      await fetchRules()
    } finally {
      saving.value = false
    }
  }

  async function moveRule(ruleId: string, direction: 'up' | 'down'): Promise<void> {
    const rule = rules.value.find((r) => r.id === ruleId)
    if (!rule) return

    // Find category rules
    const catId = rule.category_id || ''
    const catRules = (rulesByCategory.value.map.get(catId) || rulesByCategory.value.uncategorized).slice()
    const idx = catRules.findIndex((r) => r.id === ruleId)
    if (idx === -1) return
    const targetIdx = direction === 'up' ? idx - 1 : idx + 1
    if (targetIdx < 0 || targetIdx >= catRules.length) return

    const [moved] = catRules.splice(idx, 1)
    catRules.splice(targetIdx, 0, moved)

    await reorderRules(catId, catRules.map((r) => r.id))
  }

  async function backfillRules(): Promise<{ count: number; status: string }> {
    saving.value = true
    try {
      const res = await api.post<{ count: number; status: string }>('/api/v1/policies/rules/backfill', undefined, {
        params: activeRevisionId.value ? { revision_id: activeRevisionId.value } : undefined,
      })
      await Promise.all([fetchCategories(), fetchRules()])
      return res
    } finally {
      saving.value = false
    }
  }

  async function validateRules(): Promise<RuleValidationResult> {
    validating.value = true
    try {
      const res = await api.get<RuleValidationResult>('/api/v1/policies/validate')
      validationResult.value = res
      return res
    } finally {
      validating.value = false
    }
  }

  return {
    categories,
    sortedCategories,
    rules,
    filteredRules,
    rulesByCategory,
    groups,
    groupNameMap,
    activeRevisionId,
    loading,
    saving,
    validating,
    error,
    searchQuery,
    selectedCategoryId,
    validationResult,
    totalRulesCount,
    totalCategoriesCount,
    loadData,
    fetchCategories,
    fetchRules,
    fetchGroups,
    createCategory,
    updateCategory,
    deleteCategory,
    reorderCategories,
    moveCategory,
    createRule,
    updateRule,
    deleteRule,
    toggleRuleEnabled,
    reorderRules,
    moveRule,
    backfillRules,
    validateRules,
  }
}
