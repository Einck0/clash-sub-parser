// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { api, ApiError } from '../../api/client'
import { toastStore } from '../../ui/toast'
import { useChains } from './useChains'
import type { ProxyChainBinding } from './types'

describe('useChains composable', () => {
  beforeEach(() => {
    vi.restoreAllMocks()
    toastStore.clear()
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('loads bindings, options, and effects concurrently', async () => {
    vi.spyOn(api, 'get').mockImplementation(async (path: string) => {
      if (path === '/api/v1/proxy-chains') {
        return {
          bindings: [
            {
              id: 'chain-1',
              target_type: 'subscription',
              target_id: 'sub-1',
              dialer_type: 'node_group',
              dialer_id: 'group-1',
              enabled: true,
              sort_order: 10,
              note: 'Test chain',
              revision: 1,
            },
          ],
          version: 'ver-alpha',
        }
      }
      if (path === '/api/v1/proxy-chains/options') {
        return {
          nodes: [{ id: 'n-1', name: 'Node 1' }],
          groups: [{ id: 'group-1', name: 'Group 1' }],
          subscriptions: [{ id: 'sub-1', name: 'Sub 1' }],
        }
      }
      if (path === '/api/v1/proxy-chains/effects') {
        return {
          version: 'ver-alpha',
          effects: [{ binding_id: 'chain-1', target_count: 1, chain_count: 1, skip_count: 0, overridden_count: 0 }],
          nodes: [{ logical_id: 'n-1', display_name: 'Node 1', dialer_proxy: 'group-1' }],
        }
      }
      return {}
    })

    const chains = useChains()
    await chains.loadAll()

    expect(chains.bindings.value.length).toBe(1)
    expect(chains.bindings.value[0]?.id).toBe('chain-1')
    expect(chains.configVersion.value).toBe('ver-alpha')
    expect(chains.options.value.groups.length).toBe(1)
    expect(chains.effects.value.length).toBe(1)
    expect(chains.nodes.value.length).toBe(1)
  })

  it('toggles enabled state via PUT and does not fake success on API error', async () => {
    const putSpy = vi.spyOn(api, 'put').mockRejectedValue(
      new ApiError(409, 'conflict', 'Concurrent modification on binding revision')
    )

    const chains = useChains()
    const binding: ProxyChainBinding = {
      id: 'chain-1',
      target_type: 'node',
      target_id: 'node-1',
      dialer_type: 'node',
      dialer_id: 'node-2',
      enabled: true,
      sort_order: 10,
      note: 'Note',
      revision: 1,
    }
    chains.bindings.value = [{ ...binding }]
    chains.configVersion.value = 'ver-initial'

    await chains.toggleEnabled(binding)

    expect(putSpy).toHaveBeenCalledWith('/api/v1/proxy-chains/chain-1', {
      binding: expect.objectContaining({
        id: 'chain-1',
        enabled: false,
      }),
      config_version: 'ver-initial',
      expected_revision: 1,
    })

    // Local binding must NOT be optimistically toggled to false
    expect(chains.bindings.value[0]?.enabled).toBe(true)
    expect(chains.rowError.value['chain-1']).toContain('Concurrent modification')
    expect(toastStore.items.value.some((t) => t.tone === 'error')).toBe(true)
  })

  it('deletes binding with confirmation, preserving 422 errors if lower binding becomes unsafe', async () => {
    const deleteSpy = vi.spyOn(api, 'delete').mockRejectedValue(
      new ApiError(422, 'chain_cycle', 'Deleting binding exposes an unsafe cycle in lower-priority bindings')
    )

    const chains = useChains()
    const binding: ProxyChainBinding = {
      id: 'chain-to-del',
      target_type: 'node_group',
      target_id: 'g-1',
      dialer_type: 'node',
      dialer_id: 'n-2',
      enabled: true,
      sort_order: 10,
      note: '',
      revision: 3,
    }
    chains.bindings.value = [binding]
    chains.configVersion.value = 'ver-del'

    chains.openDeleteConfirm(binding)
    expect(chains.isDeleteModalOpen.value).toBe(true)

    const success = await chains.confirmDelete()
    expect(success).toBe(false)
    expect(deleteSpy).toHaveBeenCalledWith('/api/v1/proxy-chains/chain-to-del', {
      body: {
        config_version: 'ver-del',
        expected_revision: 3,
      },
    })

    // Binding remains in list
    expect(chains.bindings.value.length).toBe(1)
    // Modal stays open with 422 error preserved
    expect(chains.isDeleteModalOpen.value).toBe(true)
    expect(chains.deleteError.value).toContain('unsafe cycle')
  })
})
