import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { useRules } from './useRules'
import { api } from '../../api/client'
import type { RuleCategory, PolicyRuleView, ListRulesResponse } from './rulesTypes'

describe('useRules composable', () => {
  const mockCategories: RuleCategory[] = [
    { id: 'cat-2', name: 'Streaming', sort_order: 20 },
    { id: 'cat-1', name: 'Direct/LAN', sort_order: 10 },
    { id: 'cat-3', name: 'Final Match', sort_order: 100 },
  ]

  const mockRules: PolicyRuleView[] = [
    {
      id: 'rule-1',
      revision_id: 'rev-1',
      category_id: 'cat-1',
      category: 'Direct/LAN',
      name: 'LAN IP',
      builtin_target: 'DIRECT',
      expression: 'IP-CIDR,192.168.0.0/16,no-resolve',
      position: 1,
      enabled: true,
    },
    {
      id: 'rule-2',
      revision_id: 'rev-1',
      category_id: 'cat-2',
      category: 'Streaming',
      name: 'YouTube',
      target_group_id: 'grp-media',
      expression: 'DOMAIN-SUFFIX,youtube.com',
      position: 2,
      enabled: true,
    },
    {
      id: 'rule-3',
      revision_id: 'rev-1',
      category_id: 'cat-3',
      category: 'Final Match',
      name: 'Final Catch-all',
      target_group_id: 'grp-proxy',
      expression: 'MATCH',
      position: 3,
      enabled: true,
    },
  ]

  const mockGroups = [
    { id: 'grp-media', name: 'Media Group', group_type: 'select' },
    { id: 'grp-proxy', name: 'Proxy Group', group_type: 'urltest' },
  ]

  beforeEach(() => {
    vi.spyOn(api, 'get').mockImplementation(async (path: string) => {
      if (path === '/api/v1/policies/categories') {
        return [...mockCategories]
      }
      if (path === '/api/v1/policies/rules') {
        const resp: ListRulesResponse = {
          revision_id: 'rev-1',
          policy_rules: [...mockRules],
          total: mockRules.length,
        }
        return resp
      }
      if (path === '/api/v1/policies/groups') {
        return { items: [...mockGroups] }
      }
      if (path === '/api/v1/policies/validate') {
        return { valid: true, revision_id: 'rev-1', issues: [] }
      }
      return []
    })

    vi.spyOn(api, 'post').mockImplementation(async (path: string, body?: any) => {
      if (path === '/api/v1/policies/categories') {
        return { id: 'cat-new', ...body }
      }
      if (path === '/api/v1/policies/rules') {
        return { id: 'rule-new', ...body }
      }
      if (path === '/api/v1/policies/rules/backfill') {
        return { count: 470, status: 'backfilled' }
      }
      return {}
    })

    vi.spyOn(api, 'put').mockImplementation(async (_path: string, body?: any) => {
      return body || {}
    })

    vi.spyOn(api, 'delete').mockImplementation(async () => {
      return {}
    })
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('loads categories, rules, and groups, sorting categories by sort_order ASC', async () => {
    const rulesState = useRules()
    await rulesState.loadData()

    expect(rulesState.totalCategoriesCount.value).toBe(3)
    expect(rulesState.totalRulesCount.value).toBe(3)

    // Sorted categories should be cat-1 (10), cat-2 (20), cat-3 (100)
    expect(rulesState.sortedCategories.value.map((c) => c.id)).toEqual(['cat-1', 'cat-2', 'cat-3'])

    // Rules by category map
    const cat1Rules = rulesState.rulesByCategory.value.map.get('cat-1')
    expect(cat1Rules).toBeDefined()
    expect(cat1Rules!.length).toBe(1)
    expect(cat1Rules![0].id).toBe('rule-1')

    // Final match in cat-3
    const cat3Rules = rulesState.rulesByCategory.value.map.get('cat-3')
    expect(cat3Rules).toBeDefined()
    expect(cat3Rules![0].expression).toBe('MATCH')
  })

  it('filters rules by search query across expression, name, and builtin target', async () => {
    const rulesState = useRules()
    await rulesState.loadData()

    rulesState.searchQuery.value = 'youtube'
    expect(rulesState.filteredRules.value.length).toBe(1)
    expect(rulesState.filteredRules.value[0].id).toBe('rule-2')

    rulesState.searchQuery.value = 'DIRECT'
    expect(rulesState.filteredRules.value.length).toBe(1)
    expect(rulesState.filteredRules.value[0].id).toBe('rule-1')

    rulesState.searchQuery.value = 'nonexistent'
    expect(rulesState.filteredRules.value.length).toBe(0)
  })

  it('creates, updates, and deletes categories', async () => {
    const rulesState = useRules()
    await rulesState.loadData()

    await rulesState.createCategory({ name: 'Gaming', sort_order: 30 })
    expect(api.post).toHaveBeenCalledWith('/api/v1/policies/categories', {
      name: 'Gaming',
      sort_order: 30,
    })

    await rulesState.updateCategory('cat-1', { name: 'Direct Only', sort_order: 15 })
    expect(api.put).toHaveBeenCalledWith('/api/v1/policies/categories/cat-1', {
      name: 'Direct Only',
      sort_order: 15,
    })

    await rulesState.deleteCategory('cat-2')
    expect(api.delete).toHaveBeenCalledWith('/api/v1/policies/categories/cat-2')
  })

  it('moves category up and down and triggers reorder API', async () => {
    const rulesState = useRules()
    await rulesState.loadData()

    // Move cat-2 (index 1) UP -> should become first (cat-2, cat-1, cat-3)
    await rulesState.moveCategory('cat-2', 'up')
    expect(api.put).toHaveBeenCalledWith('/api/v1/policies/categories/reorder', {
      order: ['cat-2', 'cat-1', 'cat-3'],
    })
  })

  it('creates, updates, toggles, and deletes rules', async () => {
    const rulesState = useRules()
    await rulesState.loadData()

    await rulesState.createRule({
      expression: 'DOMAIN,example.com',
      builtin_target: 'REJECT',
      category_id: 'cat-1',
    })
    expect(api.post).toHaveBeenCalledWith(
      '/api/v1/policies/rules',
      expect.objectContaining({
        expression: 'DOMAIN,example.com',
        builtin_target: 'REJECT',
        kind: 'policy',
      })
    )

    await rulesState.toggleRuleEnabled(mockRules[0])
    expect(api.put).toHaveBeenCalledWith(
      '/api/v1/policies/rules/rule-1',
      expect.objectContaining({ enabled: false })
    )

    await rulesState.deleteRule('rule-1')
    expect(api.delete).toHaveBeenCalledWith(
      '/api/v1/policies/rules/rule-1',
      expect.objectContaining({ params: { kind: 'policy' } })
    )
  })

  it('executes backfill and validate endpoints successfully', async () => {
    const rulesState = useRules()
    await rulesState.loadData()

    const backfillRes = await rulesState.backfillRules()
    expect(backfillRes.count).toBe(470)

    const valRes = await rulesState.validateRules()
    expect(valRes.valid).toBe(true)
  })

  it('fetches multi-page rules and groups robustly when total exceeds page_size', async () => {
    vi.spyOn(api, 'get').mockImplementation(async (path: string, options?: any) => {
      if (path === '/api/v1/policies/categories') {
        return [...mockCategories]
      }
      if (path === '/api/v1/policies/rules') {
        const page = options?.params?.page || 1
        if (page === 1) {
          return {
            revision_id: 'rev-multi',
            policy_rules: [mockRules[0], mockRules[1]],
            total: 3,
          }
        } else {
          return {
            revision_id: 'rev-multi',
            policy_rules: [mockRules[2]],
            total: 3,
          }
        }
      }
      if (path === '/api/v1/policies/groups') {
        const page = options?.params?.page || 1
        if (page === 1) {
          return {
            items: [mockGroups[0]],
            total: 2,
          }
        } else {
          return {
            items: [mockGroups[1]],
            total: 2,
          }
        }
      }
      return []
    })

    const rulesState = useRules()
    await rulesState.loadData()

    expect(rulesState.rules.value.length).toBe(3)
    expect(rulesState.groups.value.length).toBe(2)
  })

  it('moves rules respecting real unfiltered category order, even when search query is active', async () => {
    const multiCatRules: PolicyRuleView[] = [
      {
        id: 'rule-A',
        revision_id: 'rev-1',
        category_id: 'cat-lan',
        expression: 'DOMAIN,a.com',
        position: 1,
        enabled: true,
      },
      {
        id: 'rule-B',
        revision_id: 'rev-1',
        category_id: 'cat-lan',
        expression: 'DOMAIN,b.com',
        position: 2,
        enabled: true,
      },
      {
        id: 'rule-C',
        revision_id: 'rev-1',
        category_id: 'cat-lan',
        expression: 'DOMAIN,c.com',
        position: 3,
        enabled: true,
      },
    ]

    vi.spyOn(api, 'get').mockImplementation(async (path: string) => {
      if (path === '/api/v1/policies/categories') {
        return [{ id: 'cat-lan', name: 'LAN', sort_order: 10 }]
      }
      if (path === '/api/v1/policies/rules') {
        return {
          revision_id: 'rev-1',
          policy_rules: multiCatRules,
          total: 3,
        }
      }
      return []
    })

    const rulesState = useRules()
    await rulesState.loadData()

    // Filter by 'c.com'
    rulesState.searchQuery.value = 'c.com'
    expect(rulesState.filteredRules.value.length).toBe(1)
    expect(rulesState.filteredRules.value[0].id).toBe('rule-C')

    // Move rule-C UP in the category
    // In real order: rule-A (0), rule-B (1), rule-C (2)
    // Moving rule-C UP must swap it with rule-B, resulting in [rule-A, rule-C, rule-B]
    await rulesState.moveRule('rule-C', 'up')

    expect(api.put).toHaveBeenCalledWith('/api/v1/policies/rules/reorder', {
      category_id: 'cat-lan',
      order: ['rule-A', 'rule-C', 'rule-B'],
      revision_id: 'rev-1',
    })
  })

  it('strictly disallows moving terminal MATCH rule ahead', async () => {
    const terminalRules: PolicyRuleView[] = [
      {
        id: 'rule-regular',
        revision_id: 'rev-1',
        category_id: 'cat-match',
        expression: 'DOMAIN,example.com',
        position: 1,
        enabled: true,
      },
      {
        id: 'rule-match',
        revision_id: 'rev-1',
        category_id: 'cat-match',
        expression: 'MATCH',
        position: 2,
        enabled: true,
      },
    ]

    vi.spyOn(api, 'get').mockImplementation(async (path: string) => {
      if (path === '/api/v1/policies/categories') {
        return [{ id: 'cat-match', name: 'Match Category', sort_order: 999 }]
      }
      if (path === '/api/v1/policies/rules') {
        return {
          revision_id: 'rev-1',
          policy_rules: terminalRules,
          total: 2,
        }
      }
      return []
    })

    const rulesState = useRules()
    await rulesState.loadData()

    const reorderSpy = vi.spyOn(api, 'put')
    reorderSpy.mockClear()

    // Try moving terminal MATCH rule UP
    await rulesState.moveRule('rule-match', 'up')
    expect(reorderSpy).not.toHaveBeenCalled()

    // Try moving rule-regular DOWN past terminal MATCH
    await rulesState.moveRule('rule-regular', 'down')
    expect(reorderSpy).not.toHaveBeenCalled()
  })

  it('retains existing rules on refresh error', async () => {
    const rulesState = useRules()
    await rulesState.loadData()
    expect(rulesState.rules.value.length).toBe(3)

    // Simulate network error on subsequent fetchRules
    vi.spyOn(api, 'get').mockRejectedValueOnce(new Error('Network disconnected'))

    await expect(rulesState.fetchRules()).rejects.toThrow('Network disconnected')
    // Rules must NOT be wiped!
    expect(rulesState.rules.value.length).toBe(3)
    expect(rulesState.error.value).toBe('Network disconnected')
  })
})
