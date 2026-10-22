import { describe, expect, it } from 'vitest'
import { useGroupDraft, getSourceKey } from './useGroupDraft'
import type { PolicyGroup, MemberSource } from './policyTypes'

describe('useGroupDraft Composable', () => {
  const sampleGroup: PolicyGroup = {
    id: 'grp-test',
    name: 'Proxy Group',
    group_type: 'select',
    position: 2,
    empty_fallback_pass: true,
    inherit_global_filter: false,
    revision: 3,
    sources: [
      { operation: 'add', kind: 'manual_node', position: 0, node_logical_id: 'node-hk-01' },
      { operation: 'add', kind: 'group_ref', position: 1, target_group_id: 'grp-sub' },
    ],
  }

  it('initializes draft from PolicyGroup and tracks clean dirty state', () => {
    const draft = useGroupDraft()
    draft.initDraft(sampleGroup)

    expect(draft.name.value).toBe('Proxy Group')
    expect(draft.groupType.value).toBe('select')
    expect(draft.position.value).toBe(2)
    expect(draft.emptyFallbackPass.value).toBe(true)
    expect(draft.inheritGlobalFilter.value).toBe(false)
    expect(draft.expectedRevision.value).toBe(3)
    expect(draft.sources.value).toHaveLength(2)
    expect(draft.isDirty.value).toBe(false)
    expect(draft.hasInvalidFilterSources.value).toBe(false)
  })

  it('becomes dirty when name or group attributes change', () => {
    const draft = useGroupDraft()
    draft.initDraft(sampleGroup)

    draft.name.value = 'Renamed Group'
    expect(draft.isDirty.value).toBe(true)

    draft.name.value = 'Proxy Group'
    expect(draft.isDirty.value).toBe(false)

    draft.emptyFallbackPass.value = false
    expect(draft.isDirty.value).toBe(true)
  })

  it('handles source pipeline manipulation with normalized positions', () => {
    const draft = useGroupDraft()
    draft.initDraft(sampleGroup)

    // Add source
    draft.addSource({
      operation: 'remove',
      kind: 'regex',
      position: 2,
      pattern: '.*(过期|官网).*',
    })
    expect(draft.sources.value).toHaveLength(3)
    expect(draft.sources.value[2].position).toBe(2)
    expect(draft.isDirty.value).toBe(true)

    // Move source up
    draft.moveSourceUp(2)
    expect(draft.sources.value[1].pattern).toBe('.*(过期|官网).*')
    expect(draft.sources.value[1].position).toBe(1)
    expect(draft.sources.value[2].target_group_id).toBe('grp-sub')
    expect(draft.sources.value[2].position).toBe(2)

    // Toggle operation
    draft.toggleSourceOp(1)
    expect(draft.sources.value[1].operation).toBe('add')

    // Remove source
    draft.removeSource(0)
    expect(draft.sources.value).toHaveLength(2)
    expect(draft.sources.value[0].position).toBe(0)
    expect(draft.sources.value[1].position).toBe(1)
  })

  it('tracks invalid filter sources and blocks save payload validity', () => {
    const draft = useGroupDraft()
    draft.initDraft(sampleGroup)

    draft.addFilterSource('add')
    const filterSource = draft.sources.value[draft.sources.value.length - 1]
    const key = getSourceKey(filterSource)

    expect(draft.checkDraftValidity()).toBe(true)

    draft.setFilterInvalid(key, true)
    expect(draft.hasInvalidFilterSources.value).toBe(true)
    expect(draft.checkDraftValidity()).toBe(false)
    expect(draft.formError.value).toContain('未完成或无效')

    draft.setFilterInvalid(key, false)
    expect(draft.hasInvalidFilterSources.value).toBe(false)
    expect(draft.checkDraftValidity()).toBe(true)
  })

  it('manages 409 conflict and updates CAS expected_revision', () => {
    const draft = useGroupDraft()
    draft.initDraft(sampleGroup)

    draft.conflictError.value = { currentRevision: 5, message: 'Server updated to v5' }
    draft.resolveConflictKeepDraft(5)

    expect(draft.expectedRevision.value).toBe(5)
    expect(draft.conflictError.value).toBeNull()
  })

  it('builds atomic save payload with full sources and expected_revision', () => {
    const draft = useGroupDraft()
    draft.initDraft(sampleGroup)

    draft.name.value = 'Updated Proxy'
    const payload = draft.buildSavePayload()

    expect(payload).toEqual({
      name: 'Updated Proxy',
      type: 'select',
      position: 2,
      emptyFallbackPass: true,
      inheritGlobalFilter: false,
      expectedRevision: 3,
      sources: [
        { operation: 'add', kind: 'manual_node', position: 0, node_logical_id: 'node-hk-01' },
        { operation: 'add', kind: 'group_ref', position: 1, target_group_id: 'grp-sub' },
      ],
    })
  })
})
