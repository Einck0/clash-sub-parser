import { afterEach, describe, expect, it, vi } from 'vitest'
import { api } from '../../api/client'
import { useNodes } from './useNodes'
import { normalizeNode, type NodeRecord } from './nodeView'

const detail = (logicalId: string, displayName = logicalId): NodeRecord => ({
  logical_id: logicalId, display_name: displayName, protocol: 'ss', active: true,
  connection_revision: 1,
  capabilities: { speed: { verdict: 'available', throughput: 2048, observed_at: '2026-11-25T10:00:00Z' } },
})

function deferred<T>() {
  let resolve!: (value: T) => void
  const promise = new Promise<T>((done) => { resolve = done })
  return { promise, resolve }
}

function page(items: NodeRecord[], total = items.length) {
  return { items, page: 1, page_size: 100, total }
}

afterEach(() => { vi.restoreAllMocks(); vi.useRealTimers() })

describe('node ledger probe run ownership', () => {
  it.each(['failed', 'cancelled', 'expired'] as const)('tracks queued and running separately, then retains the true %s terminal state', async (terminalState) => {
    const nodes = useNodes()
    nodes.items.value = [normalizeNode(detail('node-a'))]
    vi.spyOn(api, 'post').mockResolvedValue({ run_id: 'run-a', state: 'queued' })
    const get = vi.spyOn(api, 'get').mockImplementation(async (path: string) => {
      if (path.includes('/probes/runs/')) return { state: 'running' }
      return { probing_node_ids: [], queued_node_ids: [] }
    })
    const result = await nodes.probeNodesBatch(['node-a'])
    expect(result).toMatchObject({ ok: true, state: 'queued' })
    expect(nodes.queuedNodeIds.value.has('node-a')).toBe(true)
    await nodes.syncPoolStatus()
    expect(nodes.probingNodeIds.value.has('node-a')).toBe(true)
    expect(get).toHaveBeenCalledWith('/api/v1/probes/runs/run-a')

    get.mockImplementation(async (path: string) => path.includes('/probes/runs/')
      ? { state: terminalState }
      : { probing_node_ids: [], queued_node_ids: [] })
    await nodes.syncPoolStatus()
    expect(nodes.probingNodeIds.value.has('node-a')).toBe(false)
    expect(nodes.queuedNodeIds.value.has('node-a')).toBe(false)
    expect(nodes.probeStates.value['node-a']).toBe(terminalState)
  })

  it('refreshes only completed visible pool nodes without switching the selected drawer or repeatedly rereading results', async () => {
    const nodes = useNodes()
    nodes.items.value = [normalizeNode(detail('node-a')), normalizeNode(detail('node-b'))]
    nodes.selectedNode.value = nodes.items.value[1]!
    let poolActive = true
    const get = vi.spyOn(api, 'get').mockImplementation(async (path: string) => {
      if (path === '/api/v1/probes/pool') return { probing_node_ids: poolActive ? ['node-a', 'offscreen-node'] : [], queued_node_ids: [] }
      if (path === '/api/v1/nodes/node-a') return { ...detail('node-a', 'Updated result'), connection_revision: 2 }
      throw new Error(`Unexpected request: ${path}`)
    })
    await nodes.syncPoolStatus()
    expect(nodes.items.value[0]?.probeState).toBe('probing')
    poolActive = false
    await nodes.syncPoolStatus()
    expect(nodes.items.value[0]).toMatchObject({ displayName: 'Updated result', probeState: 'idle' })
    expect(nodes.selectedNode.value?.logicalId).toBe('node-b')
    expect(get.mock.calls.filter(([path]) => path.startsWith('/api/v1/nodes/')).map(([path]) => path)).toEqual(['/api/v1/nodes/node-a'])
    await nodes.syncPoolStatus()
    expect(get.mock.calls.filter(([path]) => path === '/api/v1/nodes/node-a')).toHaveLength(1)
  })

  it('preserves pool evidence during a status failure and shares an in-flight status read', async () => {
    const nodes = useNodes()
    nodes.items.value = [normalizeNode(detail('node-a'))]
    const get = vi.spyOn(api, 'get').mockResolvedValue({ probing_node_ids: ['node-a'], queued_node_ids: [] })
    await nodes.syncPoolStatus()
    get.mockRejectedValueOnce(new Error('Fixture pool unavailable'))
    await nodes.syncPoolStatus()
    expect(nodes.probingNodeIds.value.has('node-a')).toBe(true)
    expect(nodes.poolError.value).toContain('暂时无法同步')
    const pool = deferred<{ probing_node_ids: string[]; queued_node_ids: string[] }>()
    const poolRequested = deferred<void>()
    get.mockImplementationOnce(() => {
      poolRequested.resolve()
      return pool.promise
    })
    const first = nodes.syncPoolStatus()
    const second = nodes.syncPoolStatus()
    expect(first).toBe(second)
    await poolRequested.promise
    expect(get).toHaveBeenCalledTimes(3)
    expect(get).toHaveBeenLastCalledWith('/api/v1/probes/pool')
    pool.resolve({ probing_node_ids: ['node-a'], queued_node_ids: [] })
    await Promise.all([first, second])
    expect(get).toHaveBeenCalledTimes(3)
    expect(nodes.poolError.value).toBe('')
    get.mockResolvedValueOnce({})
    await nodes.syncPoolStatus()
    expect(nodes.probingNodeIds.value.has('node-a')).toBe(true)
    expect(nodes.poolError.value).toContain('暂时无法同步')
  })

  it('shares the same run status request between bounded single-node waiting and background synchronization', async () => {
    vi.useFakeTimers()
    const nodes = useNodes()
    nodes.items.value = [normalizeNode(detail('node-a'))]
    const status = deferred<{ state: 'succeeded' }>()
    vi.spyOn(api, 'post').mockResolvedValue({ run_id: 'run-a', state: 'queued' })
    const get = vi.spyOn(api, 'get').mockImplementation(async (path: string) => {
      if (path === '/api/v1/probes/runs/run-a') return status.promise
      if (path === '/api/v1/probes/pool') return { probing_node_ids: [], queued_node_ids: [] }
      return detail('node-a')
    })
    const probe = nodes.probeSingleNode('node-a')
    await vi.advanceTimersByTimeAsync(350)
    const sync = nodes.syncPoolStatus()
    expect(get.mock.calls.filter(([path]) => path === '/api/v1/probes/runs/run-a')).toHaveLength(1)
    status.resolve({ state: 'succeeded' })
    expect(await probe).toMatchObject({ ok: true, state: 'succeeded' })
    await sync
    expect(nodes.probingNodeIds.value.has('node-a')).toBe(false)
    expect(nodes.queuedNodeIds.value.has('node-a')).toBe(false)
  })

  it('bounds automatic result refresh retries without turning a refresh failure into success, then permits explicit detail recovery', async () => {
    const nodes = useNodes()
    nodes.items.value = [normalizeNode(detail('node-a'))]
    nodes.selectedNode.value = nodes.items.value[0]!
    vi.spyOn(api, 'post').mockResolvedValue({ run_id: 'run-a', state: 'queued' })
    const get = vi.spyOn(api, 'get').mockImplementation(async (path: string) => {
      if (path.includes('/probes/runs/')) return { state: 'succeeded' }
      if (path === '/api/v1/probes/pool') return { probing_node_ids: [], queued_node_ids: [] }
      throw new Error('Fixture result unavailable')
    })
    await nodes.probeNodesBatch(['node-a'])
    for (let attempt = 0; attempt < 4; attempt++) await nodes.syncPoolStatus()
    expect(get.mock.calls.filter(([path]) => path === '/api/v1/nodes/node-a')).toHaveLength(3)
    expect(nodes.probeStates.value['node-a']).toBe('succeeded')
    expect(nodes.pendingResultRefreshIds.value.has('node-a')).toBe(false)
    expect(nodes.resultRefreshFailedIds.value.has('node-a')).toBe(true)
    expect(nodes.isDetailStale.value).toBe(true)
    get.mockResolvedValueOnce(detail('node-a', 'Recovered result'))
    await nodes.fetchNodeDetail('node-a')
    expect(nodes.resultRefreshFailedIds.value.has('node-a')).toBe(false)
    expect(nodes.isDetailStale.value).toBe(false)
    expect(nodes.selectedNode.value?.displayName).toBe('Recovered result')
  })

  it('does not release another pending submission when an earlier accepted run finishes its bounded wait', async () => {
    vi.useFakeTimers()
    const nodes = useNodes()
    const submissionB = deferred<{ run_id: string; state: 'queued' }>()
    const post = vi.spyOn(api, 'post').mockResolvedValueOnce({ run_id: 'run-a', state: 'queued' }).mockReturnValueOnce(submissionB.promise)
    vi.spyOn(api, 'get').mockResolvedValue({ state: 'running' })
    const first = nodes.probeSingleNode('node-a')
    await vi.advanceTimersByTimeAsync(0)
    const second = nodes.probeNodesBatch(['node-b'])
    expect(nodes.pendingProbeIds.value.has('node-b')).toBe(true)
    await vi.advanceTimersByTimeAsync(2100)
    expect(await first).toMatchObject({ state: 'running' })
    expect(nodes.submittingProbes.value).toBe(true)
    expect((await nodes.probeNodesBatch(['node-c'])).ok).toBe(false)
    expect(post).toHaveBeenCalledTimes(2)
    submissionB.resolve({ run_id: 'run-b', state: 'queued' })
    await second
    expect(nodes.submittingProbes.value).toBe(false)
    expect(nodes.pendingProbeIds.value.size).toBe(0)
  })

  it('rolls back pending state on submit failure and keeps unrelated detail selection', async () => {
    const nodes = useNodes()
    nodes.items.value = [normalizeNode(detail('node-a')), normalizeNode(detail('node-b'))]
    nodes.selectedNode.value = normalizeNode(detail('node-b'))
    vi.spyOn(api, 'post').mockRejectedValue(new Error('server rejected'))
    const result = await nodes.probeNodesBatch(['node-a'])
    expect(result.ok).toBe(false)
    expect(nodes.pendingProbeIds.value.has('node-a')).toBe(false)
    expect(nodes.probingNodeIds.value.has('node-a')).toBe(false)
    expect(nodes.selectedNode.value?.logicalId).toBe('node-b')
  })

  it('returns cancellation as failure even when the saved node evidence refreshes successfully', async () => {
    const nodes = useNodes()
    nodes.items.value = [normalizeNode(detail('node-a'))]
    const post = vi.spyOn(api, 'post').mockResolvedValue({ run_id: 'run-cancelled', state: 'cancelled' })
    const get = vi.spyOn(api, 'get').mockResolvedValue(detail('node-a', 'Refreshed historical result'))
    const result = await nodes.probeNodesBatch(['node-a'])
    expect(result).toMatchObject({ ok: false, state: 'cancelled', refreshed: true })
    expect(nodes.probeStates.value['node-a']).toBe('cancelled')
    expect(nodes.items.value[0]?.displayName).toBe('Refreshed historical result')
    expect(get).toHaveBeenCalledWith('/api/v1/nodes/node-a')
    expect(post).toHaveBeenCalledOnce()
  })

  it('does not turn a nonterminal single-node probe into completed bandwidth evidence', async () => {
    vi.useFakeTimers()
    const nodes = useNodes()
    const post = vi.spyOn(api, 'post').mockResolvedValue({ run_id: 'run-a', state: 'queued' })
    vi.spyOn(api, 'get').mockResolvedValue({ state: 'running' })
    const pending = nodes.probeSingleNode('node-a')
    await vi.advanceTimersByTimeAsync(2100)
    const result = await pending
    expect(result).toMatchObject({ ok: true, state: 'running' })
    expect(post).toHaveBeenCalledWith('/api/v1/probes/runs', expect.objectContaining({ kinds: ['baseline', 'streaming', 'ai', 'ip_risk', 'geo'] }), expect.anything())
    expect(result).not.toHaveProperty('refreshed', true)
  })

  it.each([
    { initialRevision: 1, savedRevision: 2 },
    { initialRevision: 1, savedRevision: 1 },
    { initialRevision: undefined, savedRevision: undefined },
  ])('keeps the accepted save after an older list resolves (revision $initialRevision → $savedRevision)', async ({ initialRevision, savedRevision }) => {
    const nodes = useNodes()
    const initial = { ...detail('node-a', 'Original name'), connection_revision: initialRevision, server: 'fixture.example', port: 443, credentials: { method: 'aes-128-gcm', password: 'fixture-password' } }
    nodes.items.value = [normalizeNode(initial)]
    nodes.selectedNode.value = nodes.items.value[0]!
    const list = deferred<ReturnType<typeof page>>()
    const listRequested = deferred<void>()
    vi.spyOn(api, 'get').mockImplementation(async (path: string) => {
      if (path === '/api/v1/nodes') {
        listRequested.resolve()
        return list.promise
      }
      if (path === '/api/v1/probes/pool') return { probing_node_ids: [], queued_node_ids: [] }
      throw new Error(`Unexpected request: ${path}`)
    })
    const saved = { ...initial, display_name: 'Saved name', connection_revision: savedRevision, server: 'new.example' }
    vi.spyOn(api, 'patch').mockResolvedValue(saved)

    const loading = nodes.load()
    await listRequested.promise
    expect((await nodes.updateNodeConnection('node-a', { displayName: 'Saved name', server: 'new.example' })).ok).toBe(true)
    list.resolve(page([initial]))
    await loading

    expect(nodes.items.value[0]).toMatchObject({ displayName: 'Saved name', connectionRevision: savedRevision, connection: { server: 'new.example' } })
    expect(nodes.selectedNode.value).toMatchObject({ displayName: 'Saved name', connectionRevision: savedRevision, connection: { server: 'new.example' } })
    expect(nodes.loading.value).toBe(false)
    expect(nodes.loadingMore.value).toBe(false)
    expect(nodes.hasMore.value).toBe(false)
  })

  it('preserves revision 3 read after saving revision 2 when an older list finally resolves', async () => {
    const nodes = useNodes()
    const initial: NodeRecord = { ...detail('node-a'), server: 'initial.example', port: 443, credentials: { method: 'aes-128-gcm', password: 'fixture-password' } }
    const saved = { ...initial, display_name: 'Saved revision 2', connection_revision: 2, server: 'saved.example' }
    const newest = { ...initial, display_name: 'Newest revision 3', connection_revision: 3, server: 'newest.example' }
    nodes.items.value = [normalizeNode(initial)]
    nodes.selectedNode.value = nodes.items.value[0]!
    const list = deferred<ReturnType<typeof page>>()
    const requested = deferred<void>()
    vi.spyOn(api, 'get').mockImplementation(async (path: string) => {
      if (path === '/api/v1/nodes') {
        requested.resolve()
        return list.promise
      }
      if (path === '/api/v1/nodes/node-a') return { node: newest }
      if (path === '/api/v1/probes/pool') return { probing_node_ids: [], queued_node_ids: [] }
      throw new Error(`Unexpected request: ${path}`)
    })
    vi.spyOn(api, 'patch').mockResolvedValue({ node: saved })

    const loading = nodes.load()
    await requested.promise
    expect((await nodes.updateNodeConnection('node-a', { server: 'saved.example' })).ok).toBe(true)
    expect(nodes.items.value[0]?.connectionRevision).toBe(2)
    expect((await nodes.fetchNodeDetail('node-a'))?.connectionRevision).toBe(3)
    list.resolve(page([initial]))
    await loading

    for (const node of [nodes.items.value[0], nodes.selectedNode.value]) {
      expect(node).toMatchObject({ displayName: 'Newest revision 3', connectionRevision: 3, connection: { server: 'newest.example' } })
    }
    expect(nodes.loading.value).toBe(false)
    expect(nodes.loadingMore.value).toBe(false)
    expect(nodes.total.value).toBe(1)
    expect(nodes.hasMore.value).toBe(false)
  })

  it.each(['trojan', 'tuic'])('normalizes omitted empty TLS fields from the real %s wire shape', async (protocol) => {
    const nodes = useNodes()
    const authentication = {
      password: 'fixture-password',
      ...(protocol === 'tuic' ? { uuid: '11111111-2222-4333-8444-555555555555' } : {}),
    }
    const current = normalizeNode({
      ...detail('node-a'), protocol, server: 'fixture.example', port: 443,
      credentials: {
        ...authentication,
        ...(protocol === 'tuic' ? { sni: 'old.example', alpn: ['h2'] } : {}),
        transport: { network: protocol === 'tuic' ? 'quic' : 'tcp', tls: 'true', sni: 'old.example', alpn: 'h2' },
      },
    })
    nodes.items.value = [current]
    expect(current.connection).toMatchObject({ sni: 'old.example', alpn: ['h2'] })
    // Go's omitempty leaves neither typed empty field on the saved HTTP DTO.
    const savedRecord: NodeRecord = JSON.parse(JSON.stringify({
      ...detail('node-a'), protocol, server: 'fixture.example', port: 443, connection_revision: 2,
      credentials: { ...authentication, transport: { network: protocol === 'tuic' ? 'quic' : 'tcp', tls: 'true' } },
    }))
    expect(savedRecord.credentials).not.toHaveProperty('sni')
    expect(savedRecord.credentials).not.toHaveProperty('alpn')
    const patch = vi.spyOn(api, 'patch').mockResolvedValue({ node: savedRecord })
    const get = vi.spyOn(api, 'get').mockResolvedValue({ node: savedRecord })
    nodes.selectedNode.value = current

    const result = await nodes.updateNodeConnection('node-a', { sni: '', alpn: [] })

    expect(result.ok).toBe(true)
    expect(patch).toHaveBeenCalledWith('/api/v1/nodes/node-a/connection', { sni: '', alpn: [] })
    expect(nodes.items.value[0]?.connection).toMatchObject({ sni: '', alpn: [] })
    expect(nodes.selectedNode.value?.connection).toMatchObject({ sni: '', alpn: [] })
    await nodes.fetchNodeDetail('node-a')
    expect(get).toHaveBeenCalledWith('/api/v1/nodes/node-a')
    expect(nodes.items.value[0]?.connection).toMatchObject({ sni: '', alpn: [] })
    expect(nodes.selectedNode.value?.connection).toMatchObject({ sni: '', alpn: [] })

    patch.mockClear()
    expect((await nodes.updateNodeConnection('node-a', { displayName: 'Only name changed' })).ok).toBe(true)
    expect(patch).toHaveBeenCalledWith('/api/v1/nodes/node-a/connection', { display_name: 'Only name changed' })
  })

  it.each(['uuid', 'password'] as const)('does not treat a cleared TUIC %s as an optional TLS clear', async (field) => {
    const nodes = useNodes()
    nodes.items.value = [normalizeNode({
      ...detail('node-a'), protocol: 'tuic', server: 'fixture.example', port: 443,
      credentials: { uuid: '11111111-2222-4333-8444-555555555555', password: 'fixture-password' },
    })]
    const patch = vi.spyOn(api, 'patch')
    const result = await nodes.updateNodeConnection('node-a', { [field]: '', sni: '', alpn: [] })
    expect(result.ok).toBe(false)
    expect(result.error).toContain(field)
    expect(patch).not.toHaveBeenCalled()
  })

  it('does not overwrite an accepted save with a result read that began while the save was pending', async () => {
    const nodes = useNodes()
    const initial: NodeRecord = { ...detail('node-a', 'Original name'), server: 'fixture.example', port: 443, credentials: { method: 'aes-128-gcm', password: 'fixture-only' } }
    nodes.items.value = [normalizeNode(initial)]
    nodes.selectedNode.value = nodes.items.value[0]!
    const patch = deferred<NodeRecord>()
    const result = deferred<NodeRecord>()
    const readRequested = deferred<void>()
    vi.spyOn(api, 'patch').mockReturnValueOnce(patch.promise)
    vi.spyOn(api, 'post').mockResolvedValue({ run_id: 'run-a', state: 'succeeded' })
    vi.spyOn(api, 'get').mockImplementation(async () => {
      readRequested.resolve()
      return result.promise
    })
    const save = nodes.updateNodeConnection('node-a', { displayName: 'Saved name' })
    const probe = nodes.probeNodesBatch(['node-a'])
    await readRequested.promise
    patch.resolve({ ...initial, display_name: 'Saved name' })
    expect((await save).ok).toBe(true)
    result.resolve(initial)
    await probe
    expect(nodes.items.value[0]?.displayName).toBe('Saved name')
    expect(nodes.selectedNode.value?.displayName).toBe('Saved name')
  })

  it('preserves connection revision and newer detail if a probe result request returns an older DTO', async () => {
    const nodes = useNodes()
    nodes.items.value = [normalizeNode({ ...detail('node-a'), connection_revision: 3, display_name: 'New name' })]
    nodes.selectedNode.value = nodes.items.value[0]!
    vi.spyOn(api, 'post').mockResolvedValue({ run_id: 'run-a', state: 'succeeded' })
    vi.spyOn(api, 'get').mockResolvedValue(detail('node-a', 'Old name'))
    const result = await nodes.probeNodesBatch(['node-a'])
    expect(result).toMatchObject({ ok: true, state: 'succeeded', refreshed: false })
    expect(nodes.items.value[0]?.displayName).toBe('New name')
    expect(nodes.selectedNode.value?.logicalId).toBe('node-a')
  })
})
