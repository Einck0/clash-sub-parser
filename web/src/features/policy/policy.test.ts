import { describe, expect, it, vi, beforeEach } from 'vitest'
import {
  ALL_GROUP_TYPES,
  MODERN_RULE_CAPABILITY_MATRIX,
  groupTypeLabel,
  groupTypeSupportedTargets,
  groupTypeTone,
  ruleActionLabel,
  ruleActionTone,
  validateEdgeInput,
  validateConditionInput,
  type PolicyGroup,
  type AdmissionRule,
  type PolicyRule,
} from './policyTypes'
import { usePolicy } from './usePolicy'
import { api, ApiError } from '../../api/client'

describe('policy types and helpers', () => {
  it('formats group types into readable Chinese labels', () => {
    expect(groupTypeLabel('select')).toBe('手动选择 (select)')
    expect(groupTypeLabel('urltest')).toBe('自动测速 (url-test)')
    expect(groupTypeLabel('fallback')).toBe('故障转移 (fallback)')
    expect(groupTypeLabel('loadbalance')).toBe('负载均衡 (load-balance)')
  })

  it('maps group types and rule actions to accurate tones and Chinese labels', () => {
    expect(groupTypeTone('select')).toBe('primary')
    expect(groupTypeTone('urltest')).toBe('secondary')
    expect(groupTypeTone('fallback')).toBe('accent')
    expect(groupTypeTone('loadbalance')).toBe('info')

    expect(ruleActionTone('allow')).toBe('success')
    expect(ruleActionTone('reject')).toBe('error')
    expect(ruleActionTone('quarantine')).toBe('warning')

    expect(ruleActionLabel('allow')).toBe('允许')
    expect(ruleActionLabel('reject')).toBe('拒绝')
    expect(ruleActionLabel('quarantine')).toBe('隔离')
  })

  it('validates edge inputs to strictly prevent self-loops and empty targets', () => {
    // Empty target
    expect(validateEdgeInput({}, 'grp-parent')).toBe('必须指定子策略组或节点逻辑 ID')

    // Self loop
    expect(validateEdgeInput({ child_group_id: 'grp-parent' }, 'grp-parent')).toBe(
      '禁止自环：父策略组不能引用自身'
    )

    // Valid child group
    expect(validateEdgeInput({ child_group_id: 'grp-other', position: 0 }, 'grp-parent')).toBeNull()

    // Valid node target
    expect(validateEdgeInput({ node_logical_id: 'node-hk-1', position: 0 }, 'grp-parent')).toBeNull()
  })

  it('validates filter condition inputs across fields, operators and bounds', () => {
    // Missing field or op
    expect(validateConditionInput({})).toBe('必须选择字段')
    expect(validateConditionInput({ field: 'display_name' })).toBe('必须选择运算符')

    // display_name
    expect(validateConditionInput({ field: 'display_name', op: 'equals', value: 'foo' })).toBe(
      '显示名称仅支持"包含"或"不包含"运算符'
    )
    expect(validateConditionInput({ field: 'display_name', op: 'contains', value: '' })).toBe(
      '显示名称值不能为空'
    )
    expect(validateConditionInput({ field: 'display_name', op: 'contains', value: 'US' })).toBeNull()

    // protocol
    expect(validateConditionInput({ field: 'protocol', op: 'contains', value: 'ss' })).toBe(
      '协议类型仅支持"等于"或"不等于"运算符'
    )
    expect(validateConditionInput({ field: 'protocol', op: 'equals', value: 'ss' })).toBeNull()
    expect(validateConditionInput({ field: 'protocol', op: 'equals', value: 'wireguard' })).toBeNull()
    expect(validateConditionInput({ field: 'protocol', op: 'equals', value: 'tuic' })).toBeNull()
    expect(validateConditionInput({ field: 'protocol', op: 'equals', value: 'ssr' })).toContain(
      '不支持的协议'
    )

    // group & rule target capability boundaries (only mihomo supports groups & rules)
    expect(groupTypeSupportedTargets('select')).toEqual(['mihomo'])
    expect(groupTypeSupportedTargets('urltest')).toEqual(['mihomo'])
    expect(groupTypeSupportedTargets('fallback')).toEqual(['mihomo'])
    expect(groupTypeSupportedTargets('loadbalance')).toEqual(['mihomo'])
    expect(ALL_GROUP_TYPES).toHaveLength(4)
    expect(MODERN_RULE_CAPABILITY_MATRIX).toHaveLength(3)
    for (const band of MODERN_RULE_CAPABILITY_MATRIX) {
      expect(band.supportedTargets).toEqual(['mihomo'])
    }

    // source_subscription_ids
    expect(validateConditionInput({ field: 'source_subscription_ids', op: 'equals', value: 'sub-1' })).toBe(
      '来源订阅仅支持"包含"或"不包含"运算符'
    )
    expect(validateConditionInput({ field: 'source_subscription_ids', op: 'contains', value: 'sub-1' })).toBeNull()

    // probe_verdict
    expect(validateConditionInput({ field: 'probe_verdict', op: 'equals', value: 'available' })).toBe(
      '探针判定条件必须指定探针类型'
    )
    expect(
      validateConditionInput({
        field: 'probe_verdict',
        op: 'equals',
        value: 'available',
        probe_kind: 'baseline',
        freshness_seconds: 3600,
      })
    ).toBeNull()

    // probe_latency_ms
    expect(
      validateConditionInput({
        field: 'probe_latency_ms',
        op: 'equals',
        value: '200',
        probe_kind: 'baseline',
      })
    ).toBe('探针延迟仅支持"小于等于"运算符')
    expect(
      validateConditionInput({
        field: 'probe_latency_ms',
        op: 'lte',
        value: 'invalid',
        probe_kind: 'baseline',
      })
    ).toBe('延迟阈值必须在 0 到 60000 ms 之间')
    expect(
      validateConditionInput({
        field: 'probe_latency_ms',
        op: 'lte',
        value: '300',
        probe_kind: 'baseline',
        freshness_seconds: 600,
      })
    ).toBeNull()
  })
})

describe('usePolicy composable', () => {
  beforeEach(() => {
    vi.restoreAllMocks()
  })

  it('loads policy groups from /api/v1/policies/groups', async () => {
    const mockGroups: PolicyGroup[] = [
      {
        id: 'grp-1',
        name: 'Auto Select',
        group_type: 'urltest',
        edges: [
          { id: 'edge-1', parent_group_id: 'grp-1', node_logical_id: 'node-1', position: 0 },
        ],
      },
    ]

    vi.spyOn(api, 'get').mockResolvedValueOnce({
      items: mockGroups,
      page: 1,
      page_size: 50,
      total: 1,
    })

    const { groups, loading, loadGroups } = usePolicy()
    await loadGroups()

    expect(loading.value).toBe(false)
    expect(groups.value).toHaveLength(1)
    expect(groups.value[0].name).toBe('Auto Select')
    expect(groups.value[0].edges).toHaveLength(1)
  })

  it('creates a new policy group via POST /api/v1/policies/groups', async () => {
    const mockCreated: PolicyGroup = {
      id: 'grp-new',
      name: 'Proxy Fallback',
      group_type: 'fallback',
      edges: [],
    }

    const postSpy = vi.spyOn(api, 'post').mockResolvedValueOnce(mockCreated)
    vi.spyOn(api, 'get').mockResolvedValueOnce({
      items: [mockCreated],
      page: 1,
      page_size: 50,
      total: 1,
    })

    const { createGroup } = usePolicy()
    const result = await createGroup('Proxy Fallback', 'fallback')

    expect(postSpy).toHaveBeenCalledWith('/api/v1/policies/groups', {
      name: 'Proxy Fallback',
      group_type: 'fallback',
      edges: [],
    })
    expect(result.id).toBe('grp-new')
  })

  it('deletes a policy group via DELETE /api/v1/policies/groups/{id}', async () => {
    const deleteSpy = vi.spyOn(api, 'delete').mockResolvedValueOnce(undefined)

    const { groups, deleteGroup } = usePolicy()
    groups.value = [
      { id: 'grp-del', name: 'To Delete', group_type: 'select', edges: [] },
    ]

    await deleteGroup('grp-del')
    expect(deleteSpy).toHaveBeenCalledWith('/api/v1/policies/groups/grp-del')
    expect(groups.value).toHaveLength(0)
  })

  it('loads admission and routing rules from /api/v1/policies/rules', async () => {
    const mockAdmission: AdmissionRule[] = [
      {
        id: 'adm-1',
        revision_id: 'rev-1',
        name: 'Allow HK and JP',
        expression: 'country in ["HK", "JP"]',
        action: 'allow',
        position: 0,
      },
    ]
    const mockPolicy: PolicyRule[] = [
      {
        id: 'pol-1',
        revision_id: 'rev-1',
        target_group_id: 'grp-1',
        expression: 'domain_suffix("google.com")',
        position: 0,
      },
    ]

    vi.spyOn(api, 'get').mockResolvedValueOnce({
      revision_id: 'rev-1',
      admission_rules: mockAdmission,
      policy_rules: mockPolicy,
      total: 2,
    })

    const { admissionRules, policyRules, loadRules } = usePolicy()
    await loadRules()

    expect(admissionRules.value).toHaveLength(1)
    expect(admissionRules.value[0].action).toBe('allow')
    expect(policyRules.value).toHaveLength(1)
    expect(policyRules.value[0].target_group_id).toBe('grp-1')
  })

  it('validates persisted backend graph topology via POST /api/v1/policies/validate without client payload', async () => {
    const postSpy = vi.spyOn(api, 'post').mockResolvedValueOnce({
      valid: true,
      errors: [],
    })

    const { validateGraph, validationResult } = usePolicy()
    const result = await validateGraph()

    expect(postSpy).toHaveBeenCalledWith('/api/v1/policies/validate')
    expect(result.valid).toBe(true)
    expect(validationResult.value?.valid).toBe(true)
  })

  it('loads and updates global node filter via /api/v1/policies/global-node-filter', async () => {
    const mockGlobal = {
      spec: {
        conditions: [
          { field: 'protocol' as const, op: 'equals' as const, value: 'ss' },
        ],
      },
      updated_at: '2026-09-25T10:00:00Z',
    }

    vi.spyOn(api, 'get').mockResolvedValueOnce(mockGlobal)
    const putSpy = vi.spyOn(api, 'put').mockResolvedValueOnce(mockGlobal)

    const { globalFilter, loadGlobalFilter, updateGlobalFilter } = usePolicy()
    await loadGlobalFilter()

    expect(globalFilter.value?.spec.conditions).toHaveLength(1)
    expect(globalFilter.value?.spec.conditions[0].value).toBe('ss')

    const updated = await updateGlobalFilter({
      conditions: [{ field: 'protocol', op: 'equals', value: 'ss' }],
    })
    expect(putSpy).toHaveBeenCalledWith('/api/v1/policies/global-node-filter', {
      spec: { conditions: [{ field: 'protocol', op: 'equals', value: 'ss' }] },
    })
    expect(updated.spec.conditions).toHaveLength(1)
  })

  it('loadGlobalFilter surfaces 401, 403, and 500 errors and does not swallow them as empty filter', async () => {
    vi.spyOn(api, 'get').mockRejectedValueOnce(new ApiError(403, 'forbidden', 'Access denied to global filter'))

    const { globalFilter, error, loadGlobalFilter } = usePolicy()
    const res = await loadGlobalFilter()

    expect(res).toBeNull()
    expect(globalFilter.value).toBeNull()
    expect(error.value).toBe('Access denied to global filter')
  })

  it('loadGlobalFilter treats 404 as unconfigured filter without surfacing an error', async () => {
    vi.spyOn(api, 'get').mockRejectedValueOnce(new ApiError(404, 'not_found', 'No global filter configured'))

    const { globalFilter, error, loadGlobalFilter } = usePolicy()
    const res = await loadGlobalFilter()

    expect(res).toBeNull()
    expect(globalFilter.value).toBeNull()
    expect(error.value).toBe('')
  })

  it('updateGlobalFilter with empty conditions explicitly clears global filter', async () => {
    const putSpy = vi.spyOn(api, 'put').mockResolvedValueOnce({
      spec: { conditions: [] },
      updated_at: '2026-09-25T11:00:00Z',
    })

    const { updateGlobalFilter } = usePolicy()
    const res = await updateGlobalFilter({ conditions: [] })

    expect(putSpy).toHaveBeenCalledWith('/api/v1/policies/global-node-filter', {
      spec: { conditions: [] },
    })
    expect(res.spec.conditions).toHaveLength(0)
  })

  it('updateGroup handles omitted, explicit null, and populated node_filter semantics', async () => {
    const patchSpy = vi.spyOn(api, 'patch').mockResolvedValue({
      id: 'grp-test',
      name: 'Test Group',
      group_type: 'select',
      edges: [],
    })

    const { updateGroup } = usePolicy()

    // 1. Omitted node_filter (undefined)
    await updateGroup('grp-test', 'Renamed Group')
    expect(patchSpy).toHaveBeenLastCalledWith('/api/v1/policies/groups/grp-test', {
      name: 'Renamed Group',
    })

    // 2. Explicit null clears node_filter
    await updateGroup('grp-test', undefined, undefined, null)
    expect(patchSpy).toHaveBeenLastCalledWith('/api/v1/policies/groups/grp-test', {
      node_filter: null,
    })

    // 3. Populated node_filter
    const filterSpec = {
      conditions: [{ field: 'display_name' as const, op: 'contains' as const, value: 'premium' }],
    }
    await updateGroup('grp-test', undefined, undefined, filterSpec)
    expect(patchSpy).toHaveBeenLastCalledWith('/api/v1/policies/groups/grp-test', {
      node_filter: filterSpec,
    })
  })

  it('validates filter condition inputs against matrix bounds', () => {
    // Valid display_name
    expect(validateConditionInput({ field: 'display_name', op: 'contains', value: 'hk' })).toBeNull()
    expect(validateConditionInput({ field: 'display_name', op: 'equals', value: 'hk' })).toContain('仅支持"包含"')
    expect(validateConditionInput({ field: 'display_name', op: 'contains', value: '' })).toContain('不能为空')

    // Valid protocol
    expect(validateConditionInput({ field: 'protocol', op: 'equals', value: 'ss' })).toBeNull()
    expect(validateConditionInput({ field: 'protocol', op: 'contains', value: 'ss' })).toContain('仅支持"等于"')

    // Valid source_subscription_ids
    expect(validateConditionInput({ field: 'source_subscription_ids', op: 'contains', value: 'sub-1' })).toBeNull()

    // Probe verdict requires probe_kind
    expect(validateConditionInput({ field: 'probe_verdict', op: 'equals', value: 'available' })).toContain('必须指定探针类型')
    expect(validateConditionInput({ field: 'probe_verdict', op: 'equals', probe_kind: 'baseline', value: 'available' })).toBeNull()

    // Probe latency requires lte and bounds
    expect(validateConditionInput({ field: 'probe_latency_ms', op: 'equals', probe_kind: 'baseline', value: '200' })).toContain('仅支持"小于等于"')
    expect(validateConditionInput({ field: 'probe_latency_ms', op: 'lte', probe_kind: 'baseline', value: '-10' })).toContain('0 到 60000')
    expect(validateConditionInput({ field: 'probe_latency_ms', op: 'lte', probe_kind: 'baseline', value: '200', freshness_seconds: 700000 })).toContain('1 秒到 604800 秒')
    expect(validateConditionInput({ field: 'probe_latency_ms', op: 'lte', probe_kind: 'baseline', value: '200', freshness_seconds: 3600 })).toBeNull()
  })
})
