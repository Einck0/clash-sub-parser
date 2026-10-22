import { ref, computed } from 'vue'
import { api } from '../../api/client'
import {
  type RuleCategory,
  type PolicyRuleView,
  type ListRulesResponse,
  type CreateRulePayload,
  type UpdateRulePayload,
  type CreateCategoryPayload,
  type UpdateCategoryPayload,
  type RuleValidationResult,
  parseRuleExpression,
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

  let fetchRulesSeq = 0

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

  // Total counts
  const totalRulesCount = computed(() => rules.value.length)
  const totalCategoriesCount = computed(() => categories.value.length)

  // Unfiltered count of rules per category
  const categoryRulesCountMap = computed(() => {
    const map = new Map<string, number>()
    for (const r of rules.value) {
      const catId = r.category_id || 'uncategorized'
      map.set(catId, (map.get(catId) || 0) + 1)
    }
    return map
  })

  // Filtered rules by search query or selected category
  const filteredRules = computed(() => {
    const q = searchQuery.value.trim().toLowerCase()
    if (q) {
      // Cross-category search across all rules
      return rules.value.filter((r) => {
        const expr = (r.expression || '').toLowerCase()
        const name = (r.name || '').toLowerCase()
        const builtin = (r.builtin_target || '').toLowerCase()
        const cat = (r.category || '').toLowerCase()
        const groupName = r.target_group_id
          ? (groupNameMap.value.get(r.target_group_id) || '').toLowerCase()
          : ''
        return (
          expr.includes(q) ||
          name.includes(q) ||
          builtin.includes(q) ||
          cat.includes(q) ||
          groupName.includes(q)
        )
      })
    }

    // When not searching, filter by selected category if one is picked
    if (selectedCategoryId.value && selectedCategoryId.value !== 'all') {
      const targetCatId = selectedCategoryId.value
      const matchedCat = categories.value.find((c) => c.id === targetCatId)
      const matchedName = matchedCat ? matchedCat.name.toLowerCase() : ''

      return rules.value.filter((r) => {
        if (targetCatId === 'uncategorized') {
          return !r.category_id && !r.category
        }
        return (
          r.category_id === targetCatId ||
          (matchedName && r.category && r.category.toLowerCase() === matchedName)
        )
      })
    }

    return rules.value
  })

  // Rules grouped by category ID (preserving position ASC)
  const rulesByCategory = computed(() => {
    const map = new Map<string, PolicyRuleView[]>()
    for (const cat of categories.value) {
      map.set(cat.id, [])
    }
    const uncategorized: PolicyRuleView[] = []

    // When searching, group the filtered results; otherwise group all rules
    const sourceRules = searchQuery.value.trim() ? filteredRules.value : rules.value

    for (const rule of sourceRules) {
      if (rule.category_id && map.has(rule.category_id)) {
        map.get(rule.category_id)!.push(rule)
      } else if (rule.category) {
        const matchedCat = categories.value.find(
          (c) => c.name.toLowerCase() === rule.category?.toLowerCase()
        )
        if (matchedCat && map.has(matchedCat.id)) {
          map.get(matchedCat.id)!.push(rule)
        } else {
          uncategorized.push(rule)
        }
      } else {
        uncategorized.push(rule)
      }
    }

    for (const [, catRules] of map.entries()) {
      catRules.sort((a, b) => a.position - b.position)
    }
    uncategorized.sort((a, b) => a.position - b.position)

    return { map, uncategorized }
  })

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

  // Fetch all rules with pagination & stale response protection
  async function fetchRules(): Promise<void> {
    const currentSeq = ++fetchRulesSeq
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

        const hasMore =
          typeof res.total === 'number'
            ? allRules.length < res.total
            : currentRules.length === pageSize
        if (!hasMore || currentRules.length === 0) {
          break
        }

        page++
      }

      // Stale response guard
      if (currentSeq !== fetchRulesSeq) {
        return
      }

      rules.value = allRules
      if (revId) {
        activeRevisionId.value = revId
      }
    } catch (err: unknown) {
      // Retain existing rules on refresh error
      error.value = err instanceof Error ? err.message : 'Failed to fetch rules'
      throw err
    }
  }

  // Fetch policy groups for target selection
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

        const hasMore =
          typeof res.total === 'number'
            ? allGroups.length < res.total
            : items.length === pageSize
        if (!hasMore || items.length === 0) {
          break
        }

        page++
      }

      groups.value = allGroups
    } catch (err: unknown) {
      console.warn('Failed to fetch policy groups:', err)
    }
  }

  // Load all initial data
  async function loadData(): Promise<void> {
    loading.value = true
    error.value = null
    try {
      await Promise.all([fetchCategories(), fetchRules(), fetchGroups()])
      syncFromUrl()
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
      const updated = await api.put<RuleCategory>(
        `/api/v1/policies/categories/${encodeURIComponent(id)}`,
        payload
      )
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
      const updated = await api.put<PolicyRuleView>(
        `/api/v1/policies/rules/${encodeURIComponent(id)}`,
        payload
      )
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

  // Move rule respecting REAL category order and terminal MATCH constraints
  async function moveRule(ruleId: string, direction: 'up' | 'down'): Promise<void> {
    const rule = rules.value.find((r) => r.id === ruleId)
    if (!rule) return

    const parsed = parseRuleExpression(rule.expression)
    if (parsed.isTerminal && direction === 'up') {
      // Terminal MATCH/FINAL rule cannot be moved up!
      return
    }

    // Get the UNFILTERED list of rules in this category, sorted by position ASC
    const catId = rule.category_id || ''
    const catRules = rules.value
      .filter((r) => (r.category_id || '') === catId)
      .sort((a, b) => a.position - b.position)

    const idx = catRules.findIndex((r) => r.id === ruleId)
    if (idx === -1) return

    const targetIdx = direction === 'up' ? idx - 1 : idx + 1
    if (targetIdx < 0 || targetIdx >= catRules.length) return

    // Regular rule cannot be moved down past terminal MATCH rule
    const targetRule = catRules[targetIdx]
    const targetParsed = parseRuleExpression(targetRule.expression)
    if (targetParsed.isTerminal && direction === 'down') {
      return
    }

    const newOrder = [...catRules]
    const [moved] = newOrder.splice(idx, 1)
    newOrder.splice(targetIdx, 0, moved)

    await reorderRules(catId, newOrder.map((r) => r.id))
  }

  async function backfillRules(): Promise<{ count: number; status: string }> {
    saving.value = true
    try {
      const res = await api.post<{ count: number; status: string }>(
        '/api/v1/policies/rules/backfill',
        undefined,
        {
          params: activeRevisionId.value ? { revision_id: activeRevisionId.value } : undefined,
        }
      )
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

  // Non-sensitive URL query param sync
  function syncFromUrl(): void {
    if (typeof window === 'undefined') return
    try {
      const params = new URLSearchParams(window.location.search)
      const q = params.get('q')
      if (q) {
        searchQuery.value = q
        selectedCategoryId.value = null
      } else {
        const cat = params.get('category')
        if (cat) {
          selectedCategoryId.value = cat
        }
      }
    } catch {
      // ignore
    }
  }

  function syncToUrl(): void {
    if (typeof window === 'undefined') return
    try {
      const url = new URL(window.location.href)
      if (searchQuery.value.trim()) {
        url.searchParams.set('q', searchQuery.value.trim())
        url.searchParams.delete('category')
      } else {
        url.searchParams.delete('q')
        if (selectedCategoryId.value && selectedCategoryId.value !== 'all') {
          url.searchParams.set('category', selectedCategoryId.value)
        } else {
          url.searchParams.delete('category')
        }
      }
      window.history.replaceState({}, '', url.toString())
    } catch {
      // ignore
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
    categoryRulesCountMap,
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
    syncFromUrl,
    syncToUrl,
  }
}
