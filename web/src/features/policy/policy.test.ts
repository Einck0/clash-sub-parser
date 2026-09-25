import { describe, expect, it, vi, beforeEach } from 'vitest'
import {
  groupTypeLabel,
  groupTypeTone,
  ruleActionTone,
  validateEdgeInput,
  type PolicyGroup,
  type AdmissionRule,
  type PolicyRule,
} from './policyTypes'
import { usePolicy } from './usePolicy'
import { api } from '../../api/client'

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
})
