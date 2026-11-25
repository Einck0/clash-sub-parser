// @vitest-environment jsdom
import { afterEach, describe, expect, it, vi } from 'vitest'
import { api } from '../../api/client'
import { useChains } from './useChains'
import { getTargetDisplayName, getDialerDisplayName } from './types'

describe('Chain recovery reads', () => {
  afterEach(() => vi.restoreAllMocks())

  it('retains confirmed options and exposes an error rather than replacing them with empty options', async () => {
    const get = vi.spyOn(api, 'get').mockResolvedValueOnce({
      nodes: [{ id: 'node-one', name: 'Known node' }], groups: [], subscriptions: [],
    })
    const state = useChains()
    await state.loadOptions()
    get.mockRejectedValueOnce(new Error('Options unavailable'))
    await state.loadOptions()
    expect(state.options.value.nodes[0].name).toBe('Known node')
    expect(state.optionsError.value).toBe('Options unavailable')
    expect(state.optionsLoading.value).toBe(false)
    expect(state.optionsLoaded.value).toBe(true)
  })

  it('distinguishes an unverified option lookup from a confirmed missing reference', () => {
    const options = { nodes: [], groups: [], subscriptions: [] }
    expect(getTargetDisplayName('node', 'node-lost', options, false)).toEqual({
      name: '目标待确认',
      isMissing: false,
      isUnverified: true,
    })
    expect(getTargetDisplayName('node', 'node-lost', options, true).isMissing).toBe(true)
    expect(getDialerDisplayName('node', 'node-lost', options, false).isMissing).toBe(false)
  })

  it('retains the last confirmed effects when a refresh fails instead of showing zero counts', async () => {
    const get = vi.spyOn(api, 'get').mockResolvedValueOnce({
      version: 'effects-version',
      effects: [{ binding_id: 'binding-1', target_count: 2, chain_count: 1, skip_count: 1, overridden_count: 0 }],
      nodes: [{ logical_id: 'node-1', display_name: 'Node 1', dialer_proxy: 'dialer-1' }],
    })
    const state = useChains()
    expect(await state.loadEffects()).toBe(true)
    get.mockRejectedValueOnce(new Error('Effects unavailable'))
    expect(await state.loadEffects()).toBe(false)
    expect(state.effects.value).toHaveLength(1)
    expect(state.nodes.value).toHaveLength(1)
    expect(state.effectsLoaded.value).toBe(true)
    expect(state.effectsError.value).toBe('Effects unavailable')
  })
  it('does not claim an initial effects read failure is a verified empty result', async () => {
    vi.spyOn(api, 'get').mockRejectedValue(new Error('Effects offline'))
    const state = useChains()
    expect(await state.loadEffects()).toBe(false)
    expect(state.effectsLoaded.value).toBe(false)
    expect(state.effectsError.value).toBe('Effects offline')
  })

  it('reports a failed binding reload and retains the last confirmed version', async () => {
    const get = vi.spyOn(api, 'get').mockResolvedValueOnce({ bindings: [], version: 'confirmed-version' })
    const state = useChains()
    expect(await state.loadBindings()).toBe(true)
    get.mockRejectedValueOnce(new Error('Read unavailable'))
    expect(await state.loadBindings()).toBe(false)
    expect(state.configVersion.value).toBe('confirmed-version')
    expect(state.error.value).toBe('Read unavailable')
  })
})
