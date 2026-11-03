// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { api, ApiError } from '../../api/client'
import { useChainDraft } from './useChainDraft'
import type { OptionsResult, ProxyChainBinding } from './types'

describe('useChainDraft composable', () => {
  let mockOptions: OptionsResult
  let currentConfigVersion: string

  beforeEach(() => {
    vi.useFakeTimers()
    currentConfigVersion = 'test-version-v1'
    mockOptions = {
      subscriptions: [
        { id: 'sub-1', name: 'Primary Sub' },
        { id: 'sub-2', name: 'Secondary Sub' },
      ],
      groups: [
        { id: 'group-hk', name: 'Hong Kong Group' },
        { id: 'group-jp', name: 'Japan Group' },
      ],
      nodes: [
        { id: 'node-a', name: 'Node Alpha' },
        { id: 'node-b', name: 'Node Beta' },
      ],
    }
  })

  afterEach(() => {
    vi.clearAllTimers()
    vi.useRealTimers()
    vi.restoreAllMocks()
  })

  function createDraftManager(onSaved?: (binding: ProxyChainBinding, newConfigVersion: string) => void) {
    return useChainDraft({
      getConfigVersion: () => currentConfigVersion,
      getOptions: () => mockOptions,
      onSaved,
    })
  }

  it('initializes draft on openCreate with safe defaults and triggers immediate preview', async () => {
    const postSpy = vi.spyOn(api, 'post').mockResolvedValue({
      version: 'test-version-v1',
      effects: [
        {
          binding_id: 'temp-id',
          target_count: 5,
          chain_count: 4,
          skip_count: 1,
          overridden_count: 0,
          chain_samples: ['Node Alpha'],
          skip_samples: ['Node Beta'],
        },
      ],
      nodes: [],
    })

    const mgr = createDraftManager()
    mgr.openCreate()

    expect(mgr.isOpen.value).toBe(true)
    expect(mgr.isEditing.value).toBe(false)
    expect(mgr.draft.value.target_type).toBe('subscription')
    expect(mgr.draft.value.target_id).toBe('sub-1')
    expect(mgr.draft.value.dialer_type).toBe('node_group')
    expect(mgr.draft.value.dialer_id).toBe('group-hk')
    expect(mgr.draft.value.enabled).toBe(true)
    expect(mgr.isDirty.value).toBe(false)

    // Immediate preview was called
    expect(postSpy).toHaveBeenCalledWith('/api/v1/proxy-chains/preview', {
      binding: expect.objectContaining({
        target_type: 'subscription',
        target_id: 'sub-1',
        dialer_type: 'node_group',
        dialer_id: 'group-hk',
      }),
      config_version: 'test-version-v1',
    })
  })

  it('validates client side constraints preventing self-referencing chains', () => {
    const mgr = createDraftManager()
    mgr.openCreate()

    // Self-referencing node
    mgr.setTargetType('node')
    mgr.setTargetId('node-a')
    mgr.setDialerType('node')
    mgr.setDialerId('node-a')

    const err = mgr.clientValidate()
    expect(err).toContain('单节点不能将自身作为跳板')

    // Self-referencing group
    mgr.setTargetType('node_group')
    mgr.setTargetId('group-hk')
    mgr.setDialerType('node_group')
    mgr.setDialerId('group-hk')

    const groupErr = mgr.clientValidate()
    expect(groupErr).toContain('策略组不能将自身作为跳板')
  })

  it('debounces live preview by 250ms and prevents out-of-order race conditions via generation tracking', async () => {
    let callCount = 0
    let resolveFirst: (val: any) => void
    const firstPromise = new Promise((resolve) => {
      resolveFirst = resolve
    })

    vi.spyOn(api, 'post').mockImplementation(async (_path, body: any) => {
      callCount++
      if (body.binding.sort_order === 20) {
        // Slow first response
        await firstPromise
        return {
          version: 'v1',
          effects: [{ binding_id: 'slow', target_count: 1, chain_count: 1, skip_count: 0, overridden_count: 0 }],
          nodes: [],
        }
      }
      // Fast second response
      return {
        version: 'v2',
        effects: [{ binding_id: 'fast', target_count: 2, chain_count: 2, skip_count: 0, overridden_count: 0 }],
        nodes: [],
      }
    })

    const mgr = createDraftManager()
    mgr.openCreate()

    // Edit sort order to 20
    mgr.setSortOrder(20)
    // Advance timers 250ms to fire first preview
    vi.advanceTimersByTime(250)

    // Edit sort order to 30 quickly
    mgr.setSortOrder(30)
    // Advance timers 250ms to fire second preview
    vi.advanceTimersByTime(250)
    await Promise.resolve()

    // Second preview finished first
    expect(mgr.previewEffects.value[0]?.binding_id).toBe('fast')

    // Now let the slow first preview resolve
    resolveFirst!({})
    await Promise.resolve()

    // Generation tracking must ensure the older response did NOT overwrite the fast newer response
    expect(mgr.previewEffects.value[0]?.binding_id).toBe('fast')
  })

  it('preserves exact draft on 409 Conflict during save, offering rebase without auto-retrying', async () => {
    vi.spyOn(api, 'post').mockRejectedValue(
      new ApiError(409, 'conflict', 'Proxy chain configuration version conflict')
    )

    const mgr = createDraftManager()
    mgr.openCreate()
    mgr.setNote('Draft with custom user note')

    const success = await mgr.save()
    expect(success).toBe(false)
    expect(mgr.isOpen.value).toBe(true)
    // User draft is exactly preserved
    expect(mgr.draft.value.note).toBe('Draft with custom user note')
    // 409 conflict state is active
    expect(mgr.conflictState.value).not.toBeNull()
    expect(mgr.conflictState.value?.message).toContain('conflict')

    // Preview spy for rebase
    const previewSpy = vi.spyOn(api, 'post').mockResolvedValue({
      version: 'new-version-v2',
      effects: [],
      nodes: [],
    })

    // User explicitly clicks rebase
    mgr.rebaseAndKeepDraft('new-version-v2')

    // Conflict cleared, draft fields preserved, preview re-triggered
    expect(mgr.conflictState.value).toBeNull()
    expect(mgr.draft.value.note).toBe('Draft with custom user note')
    expect(previewSpy).toHaveBeenCalledWith('/api/v1/proxy-chains/preview', expect.objectContaining({
      config_version: 'new-version-v2',
    }))
  })

  it('performs create via POST /api/v1/proxy-chains and edit via PUT /api/v1/proxy-chains/{id}', async () => {
    const postSpy = vi.spyOn(api, 'post').mockResolvedValue({
      binding: {
        id: 'server-id-1',
        target_type: 'subscription',
        target_id: 'sub-1',
        dialer_type: 'node_group',
        dialer_id: 'group-hk',
        enabled: true,
        sort_order: 10,
        note: 'New binding',
        revision: 1,
      },
      version: 'ver-2',
    })

    const putSpy = vi.spyOn(api, 'put').mockResolvedValue({
      binding: {
        id: 'server-id-1',
        target_type: 'subscription',
        target_id: 'sub-1',
        dialer_type: 'node_group',
        dialer_id: 'group-hk',
        enabled: false,
        sort_order: 15,
        note: 'Updated note',
        revision: 2,
      },
      version: 'ver-3',
    })

    const onSaved = vi.fn()
    const mgr = createDraftManager(onSaved)

    // 1. Create
    mgr.openCreate()
    mgr.setNote('New binding')
    const createSuccess = await mgr.save()
    expect(createSuccess).toBe(true)
    expect(postSpy).toHaveBeenCalledWith('/api/v1/proxy-chains', {
      binding: expect.objectContaining({
        target_type: 'subscription',
        target_id: 'sub-1',
        note: 'New binding',
      }),
      config_version: 'test-version-v1',
    })
    expect(onSaved).toHaveBeenCalledWith(expect.objectContaining({ id: 'server-id-1' }), 'ver-2')

    // 2. Edit
    currentConfigVersion = 'ver-2'
    mgr.openEdit({
      id: 'server-id-1',
      target_type: 'subscription',
      target_id: 'sub-1',
      dialer_type: 'node_group',
      dialer_id: 'group-hk',
      enabled: true,
      sort_order: 10,
      note: 'New binding',
      revision: 1,
    })

    mgr.setEnabled(false)
    mgr.setSortOrder(15)
    mgr.setNote('Updated note')
    expect(mgr.isDirty.value).toBe(true)

    const updateSuccess = await mgr.save()
    expect(updateSuccess).toBe(true)
    expect(putSpy).toHaveBeenCalledWith('/api/v1/proxy-chains/server-id-1', {
      binding: expect.objectContaining({
        id: 'server-id-1',
        enabled: false,
        sort_order: 15,
        note: 'Updated note',
        revision: 1,
      }),
      config_version: 'ver-2',
      expected_revision: 1,
    })
  })
})
