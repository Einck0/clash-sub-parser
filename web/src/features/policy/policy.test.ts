import { describe, expect, it, vi, beforeEach } from 'vitest'
import {
  groupTypeLabel,
  groupTypeTone,
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
  it('formats group types into readable labels', () => {
    expect(groupTypeLabel('select')).toBe('Select')
    expect(groupTypeLabel('urltest')).toBe('URL Test')
    expect(groupTypeLabel('fallback')).toBe('Fallback')
    expect(groupTypeLabel('loadbalance')).toBe('Load Balance')
  })

  it('maps group types and rule actions to accurate tones', () => {
    expect(groupTypeTone('select')).toBe('primary')
    expect(groupTypeTone('urltest')).toBe('secondary')
    expect(groupTypeTone('fallback')).toBe('accent')
    expect(groupTypeTone('loadbalance')).toBe('info')

    expect(ruleActionTone('allow')).toBe('success')
    expect(ruleActionTone('reject')).toBe('error')
    expect(ruleActionTone('quarantine')).toBe('warning')
  })

  it('validates edge inputs to strictly prevent self-loops and empty targets', () => {
    // Empty target
    expect(validateEdgeInput({}, 'grp-parent')).toBe('Must specify either child group or node logical ID')

    // Self loop
    expect(validateEdgeInput({ child_group_id: 'grp-parent' }, 'grp-parent')).toBe(
      'Self-loop forbidden: parent group cannot reference itself'
    )

    // Valid child group
    expect(validateEdgeInput({ child_group_id: 'grp-other', position: 0 }, 'grp-parent')).toBeNull()

    // Valid node target
    expect(validateEdgeInput({ node_logical_id: 'node-hk-1', position: 0 }, 'grp-parent')).toBeNull()
  })

  it('validates filter condition inputs across fields, operators and bounds', () => {
    // Missing field or op
    expect(validateConditionInput({})).toBe('Field is required')
    expect(validateConditionInput({ field: 'display_name' })).toBe('Operator is required')

    // display_name
    expect(validateConditionInput({ field: 'display_name', op: 'equals', value: 'foo' })).toBe(
      'Display name only supports "contains" or "not_contains"'
    )
    expect(validateConditionInput({ field: 'display_name', op: 'contains', value: '' })).toBe(
      'Display name value cannot be empty'
    )
    expect(validateConditionInput({ field: 'display_name', op: 'contains', value: 'US' })).toBeNull()

    // protocol
    expect(validateConditionInput({ field: 'protocol', op: 'contains', value: 'ss' })).toBe(
      'Protocol only supports "equals" or "not_equals"'
    )
    expect(validateConditionInput({ field: 'protocol', op: 'equals', value: 'ss' })).toBeNull()

    // source_subscription_ids
    expect(validateConditionInput({ field: 'source_subscription_ids', op: 'equals', value: 'sub-1' })).toBe(
      'Source subscription only supports "contains" or "not_contains"'
    )
    expect(validateConditionInput({ field: 'source_subscription_ids', op: 'contains', value: 'sub-1' })).toBeNull()

    // probe_verdict
    expect(validateConditionInput({ field: 'probe_verdict', op: 'equals', value: 'available' })).toBe(
      'Probe kind is required for probe verdict condition'
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
    ).toBe('Probe latency only supports "<=" (lte)')
    expect(
      validateConditionInput({
        field: 'probe_latency_ms',
        op: 'lte',
        value: 'invalid',
        probe_kind: 'baseline',
      })
    ).toBe('Latency threshold must be a number between 0 and 60000 ms')
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

  it('validates graph topology via POST /api/v1/policies/validate', async () => {
    const postSpy = vi.spyOn(api, 'post').mockResolvedValueOnce({
      valid: true,
      errors: [],
    })

    const { validateGraph, validationResult } = usePolicy()
    const result = await validateGraph()

    expect(postSpy).toHaveBeenCalledWith('/api/v1/policies/validate', expect.any(Object))
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
    expect(validateConditionInput({ field: 'display_name', op: 'equals', value: 'hk' })).toContain('only supports "contains"')
    expect(validateConditionInput({ field: 'display_name', op: 'contains', value: '' })).toContain('cannot be empty')

    // Valid protocol
    expect(validateConditionInput({ field: 'protocol', op: 'equals', value: 'ss' })).toBeNull()
    expect(validateConditionInput({ field: 'protocol', op: 'contains', value: 'ss' })).toContain('only supports "equals"')

    // Valid source_subscription_ids
    expect(validateConditionInput({ field: 'source_subscription_ids', op: 'contains', value: 'sub-1' })).toBeNull()

    // Probe verdict requires probe_kind
    expect(validateConditionInput({ field: 'probe_verdict', op: 'equals', value: 'available' })).toContain('Probe kind is required')
    expect(validateConditionInput({ field: 'probe_verdict', op: 'equals', probe_kind: 'baseline', value: 'available' })).toBeNull()

    // Probe latency requires lte and bounds
    expect(validateConditionInput({ field: 'probe_latency_ms', op: 'equals', probe_kind: 'baseline', value: '200' })).toContain('only supports "<="')
    expect(validateConditionInput({ field: 'probe_latency_ms', op: 'lte', probe_kind: 'baseline', value: '-10' })).toContain('between 0 and 60000')
    expect(validateConditionInput({ field: 'probe_latency_ms', op: 'lte', probe_kind: 'baseline', value: '200', freshness_seconds: 700000 })).toContain('Freshness must be between 1s and 604800s')
    expect(validateConditionInput({ field: 'probe_latency_ms', op: 'lte', probe_kind: 'baseline', value: '200', freshness_seconds: 3600 })).toBeNull()
  })
})
