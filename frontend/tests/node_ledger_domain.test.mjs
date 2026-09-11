import assert from 'node:assert/strict'
import test from 'node:test'
import {
  applyMetricShortcut,
  buildEffectiveBatchTargets,
  calculateRemainingSeconds,
  chunkItems,
  computeFilterOptions,
  createDefaultFacetFilterState,
  createDrawerDetailSession,
  filterAndSortNodes,
  formatCountdown,
  getMediaSemanticPresentation,
  getProbeForNode,
  isMediaFullUnlocked,
  MEDIA_PLATFORMS,
  mergeNodeLedgerProbePages,
  nodeMatchesStatus,
  normalizeFacetFilterState,
  normalizeNodeLedgerMap,
  planDialerReplacement,
  replaceNodeDialerProxy,
  resolveDrawerDetailFetch,
  runChunkedBatchProbe,
  runSlidingWorkerPool,
  startDrawerDetailFetch,
} from '../src/views/nodeLedgerDomain.ts'

const mockNodes = [
  {
    name: 'HK-01',
    node_key: 'key-hk',
    type: 'vmess',
    server: 'hk.example.com',
    port: 443,
    subscription_name: 'Sub-A',
    dialer_proxy: null,
    chain_source: null,
    group_names: ['HK-Group'],
  },
  {
    name: 'US-01',
    node_key: 'key-us',
    type: 'ss',
    server: 'us.example.com',
    port: 8388,
    subscription_name: 'Sub-B',
    dialer_proxy: 'HK-01',
    chain_source: 'node',
    group_names: ['US-Group'],
  },
  {
    name: 'JP-01',
    node_key: 'key-jp',
    type: 'trojan',
    server: 'jp.example.com',
    port: 443,
    subscription_name: 'Sub-A',
    dialer_proxy: 'Relay-Group',
    chain_source: 'node_group',
    group_names: ['JP-Group'],
  },
]

const mockProbes = {
  'key-hk': {
    name: 'HK-01',
    node_key: 'key-hk',
    status: 'ok',
    latency_ms: 45,
    speed_mbps: 35.5,
    ip: '1.1.1.1',
    country: 'HK',
    media: {
      youtube: { status: 'ok', unlocked: true },
      netflix: { status: 'ok', unlocked: true },
      chatgpt: { status: 'ok', unlocked: true },
    },
    checked_at: 1000,
  },
  'key-us': {
    name: 'US-01',
    node_key: 'key-us',
    status: 'ok',
    latency_ms: 180,
    speed_mbps: 15.0,
    ip: '2.2.2.2',
    country: 'US',
    media: {
      youtube: { status: 'ok', unlocked: true },
      netflix: { status: 'blocked', unlocked: false },
      chatgpt: { status: 'ok', unlocked: true },
    },
    checked_at: 2000,
  },
  'key-jp': {
    name: 'JP-01',
    node_key: 'key-jp',
    status: 'fail',
    error: 'connection refused',
    checked_at: 1500,
  },
}

test('1.1 node-to-probe matching, multi-field search, multi-media AND filtering, min-speed filtering, metric shortcuts, and sorting', () => {
  const rawList = [
    { name: 'HK-01', node_key: 'key-hk', latency_ms: 45 },
    { node_key: 'key-only', latency_ms: 100 },
  ]
  const map = normalizeNodeLedgerMap(rawList)
  assert.equal(map['key-hk']?.latency_ms, 45)
  assert.equal(map['key-only']?.latency_ms, 100)

  const baseFilters = {
    keyword: 'hk.example.com',
    subscription: '',
    protocols: [],
    statuses: [],
    countries: [],
    chain: 'all',
    minSpeed: 0,
    mediaPlatforms: [],
    sortBy: 'default',
  }
  const res1 = filterAndSortNodes(mockNodes, mockProbes, baseFilters)
  assert.equal(res1.length, 1)
  assert.equal(res1[0].name, 'HK-01')

  const res2 = filterAndSortNodes(mockNodes, mockProbes, {
    ...baseFilters,
    keyword: '',
    protocols: ['ss'],
  })
  assert.equal(res2.length, 1)
  assert.equal(res2[0].name, 'US-01')

  // Multi-media AND logic and speed threshold
  const mediaFilters = {
    ...baseFilters,
    keyword: '',
    minSpeed: 20,
    mediaPlatforms: ['youtube', 'netflix'],
  }
  const matched = filterAndSortNodes(mockNodes, mockProbes, mediaFilters)
  assert.equal(matched.length, 1)
  assert.equal(matched[0].name, 'HK-01')

  // Metric shortcuts
  const shortcutHealthy = applyMetricShortcut(baseFilters, 'healthy')
  assert.deepEqual(shortcutHealthy.statuses, ['ok'])
  const shortcutFast = applyMetricShortcut(baseFilters, 'fast')
  assert.equal(shortcutFast.minSpeed, 10)
  const shortcutChained = applyMetricShortcut(baseFilters, 'chained')
  assert.equal(shortcutChained.chain, 'chained')
  const shortcutReset = applyMetricShortcut(shortcutHealthy, 'all')
  assert.deepEqual(shortcutReset.statuses, [])

  // Sorting
  const sortedLatencyAsc = filterAndSortNodes(mockNodes, mockProbes, { ...baseFilters, keyword: '', sortBy: 'latency_asc' })
  assert.deepEqual(sortedLatencyAsc.map((n) => n.name), ['HK-01', 'US-01', 'JP-01'])

  const sortedSpeedDesc = filterAndSortNodes(mockNodes, mockProbes, { ...baseFilters, keyword: '', sortBy: 'speed_desc' })
  assert.deepEqual(sortedSpeedDesc.map((n) => n.name), ['HK-01', 'US-01', 'JP-01'])
})

test('1.1-facet: normalizeFacetFilterState produces canonical, deduplicated, uppercase-country, lowercase-protocol state', () => {
  const defaults = createDefaultFacetFilterState()
  assert.deepEqual(defaults, {
    keyword: '',
    subscription: '',
    protocols: [],
    statuses: [],
    countries: [],
    chain: 'all',
    minSpeed: 0,
    mediaPlatforms: [],
    sortBy: 'default',
  })

  const raw = {
    keyword: '  my-node  ',
    subscription: '  Sub-Alpha  ',
    protocols: [' VMESS ', 'ss', 'vmess', '', '  TROJAN  '],
    statuses: [' OK ', 'fail', 'ok', 'ALL', '  fast '],
    countries: [' hk ', 'us', 'HK', '  ', 'jp '],
    chain: 'chained',
    minSpeed: -5,
    mediaPlatforms: [' NETFLIX ', 'chatgpt', 'netflix', ' '],
    sortBy: '  latency_asc  ',
  }

  const normalized = normalizeFacetFilterState(raw)
  assert.equal(normalized.keyword, 'my-node')
  assert.equal(normalized.subscription, 'Sub-Alpha')
  assert.deepEqual(normalized.protocols, ['vmess', 'ss', 'trojan'])
  assert.deepEqual(normalized.statuses, ['ok', 'fail', 'fast'])
  assert.deepEqual(normalized.countries, ['HK', 'US', 'JP'])
  assert.equal(normalized.chain, 'chained')
  assert.equal(normalized.minSpeed, 0)
  assert.deepEqual(normalized.mediaPlatforms, ['netflix', 'chatgpt'])
  assert.equal(normalized.sortBy, 'latency_asc')

  // Defensive against null/undefined
  const emptyNorm = normalizeFacetFilterState(null)
  assert.deepEqual(emptyNorm, defaults)
})

test('1.1-facet: FacetFilterState composable multi-selection (OR within facet, AND across facets, media verified-full AND)', () => {
  const baseFacet = createDefaultFacetFilterState()

  // 1. Empty facets: no constraints, all 3 nodes returned
  const allNodes = filterAndSortNodes(mockNodes, mockProbes, baseFacet)
  assert.equal(allNodes.length, 3)

  // 2. Protocols OR: ['vmess', 'ss'] matches HK-01 and US-01, excludes JP-01
  const protoOr = filterAndSortNodes(mockNodes, mockProbes, {
    ...baseFacet,
    protocols: ['vmess', 'ss'],
  })
  assert.equal(protoOr.length, 2)
  assert.deepEqual(protoOr.map((n) => n.name), ['HK-01', 'US-01'])

  // 3. Countries OR: ['HK', 'JP'] matches HK-01 and JP-01, excludes US-01
  const countryOr = filterAndSortNodes(mockNodes, mockProbes, {
    ...baseFacet,
    countries: ['HK', 'JP'],
  })
  assert.equal(countryOr.length, 2)
  assert.deepEqual(countryOr.map((n) => n.name), ['HK-01', 'JP-01'])

  // 4. Statuses OR: ['fail'] matches only JP-01
  const statusFail = filterAndSortNodes(mockNodes, mockProbes, {
    ...baseFacet,
    statuses: ['fail'],
  })
  assert.equal(statusFail.length, 1)
  assert.equal(statusFail[0].name, 'JP-01')

  // Statuses OR: ['ok', 'fail'] matches HK-01, US-01, JP-01
  const statusOkFail = filterAndSortNodes(mockNodes, mockProbes, {
    ...baseFacet,
    statuses: ['ok', 'fail'],
  })
  assert.equal(statusOkFail.length, 3)

  // Status fast: HK-01 (45ms) and US-01 (180ms) both have latency <= 300ms
  const statusFast = filterAndSortNodes(mockNodes, mockProbes, {
    ...baseFacet,
    statuses: ['fast'],
  })
  assert.equal(statusFast.length, 2)
  assert.deepEqual(statusFast.map((n) => n.name), ['HK-01', 'US-01'])

  // 5. Cross-facet AND: Countries ['HK', 'US'] AND Protocols ['ss'] -> Only US-01
  const crossAnd = filterAndSortNodes(mockNodes, mockProbes, {
    ...baseFacet,
    countries: ['HK', 'US'],
    protocols: ['ss'],
  })
  assert.equal(crossAnd.length, 1)
  assert.equal(crossAnd[0].name, 'US-01')

  // 6. Cross-facet AND with media full unlock:
  // Both HK-01 and US-01 have chatgpt: full
  // But only HK-01 has netflix: full
  const mediaAnd = filterAndSortNodes(mockNodes, mockProbes, {
    ...baseFacet,
    countries: ['HK', 'US'],
    mediaPlatforms: ['chatgpt', 'netflix'],
  })
  assert.equal(mediaAnd.length, 1)
  assert.equal(mediaAnd[0].name, 'HK-01')

  // 7. Metric shortcut with FacetFilterState
  const shortcutHealthy = applyMetricShortcut(baseFacet, 'healthy')
  assert.deepEqual(shortcutHealthy.statuses, ['ok'])

  const shortcutHealthyToggle = applyMetricShortcut(shortcutHealthy, 'healthy')
  assert.deepEqual(shortcutHealthyToggle.statuses, [])

  const shortcutResetAll = applyMetricShortcut(shortcutHealthy, 'all')
  assert.deepEqual(shortcutResetAll.statuses, [])
  assert.deepEqual(shortcutResetAll.countries, [])
  assert.deepEqual(shortcutResetAll.protocols, [])
  assert.deepEqual(shortcutResetAll.mediaPlatforms, [])
  assert.equal(shortcutResetAll.minSpeed, 0)
  assert.equal(shortcutResetAll.chain, 'all')
})

test('1.2 batch probe target selection uses all filtered rows when no row is selected and uses selected rows regardless of virtual-table mounted positions', () => {
  const filtered = [mockNodes[0], mockNodes[1]]

  // Case 1: Empty selection -> targets ALL filtered nodes (not limited to mounted virtual rows)
  const targetsAll = buildEffectiveBatchTargets(filtered, new Set(), mockNodes)
  assert.equal(targetsAll.length, 2)
  assert.equal(targetsAll[0].name, 'HK-01')
  assert.equal(targetsAll[1].name, 'US-01')

  // Case 2: Explicit selection of node outside first virtual view
  const selectedSet = new Set(['key-jp'])
  const targetsSelected = buildEffectiveBatchTargets(filtered, selectedSet, mockNodes)
  assert.equal(targetsSelected.length, 1)
  assert.equal(targetsSelected[0].name, 'JP-01')
})

test('1.3 probe batch chunking stops scheduling subsequent chunks when aborted while keeping accumulated chunk results', async () => {
  const targetNodes = [
    { name: 'Node-1' },
    { name: 'Node-2' },
    { name: 'Node-3' },
    { name: 'Node-4' },
    { name: 'Node-5' },
    { name: 'Node-6' },
  ]

  const executedChunks = []
  const abortController = new AbortController()

  const probeChunkFn = async (chunk) => {
    executedChunks.push(chunk)
    if (executedChunks.length === 1) {
      abortController.abort()
    }
    return chunk.map((node) => ({
      name: node.name,
      status: 'ok',
      latency_ms: 100,
    }))
  }

  const { results, stopped } = await runChunkedBatchProbe(targetNodes, {
    chunkSize: 2,
    signal: abortController.signal,
    probeChunkFn,
  })

  // Executed chunk 1 only; stopped before chunk 2; kept chunk 1 results
  assert.equal(executedChunks.length, 1)
  assert.equal(results.length, 2)
  assert.deepEqual(results.map((r) => r.name), ['Node-1', 'Node-2'])
  assert.equal(stopped, true)
})

test('1.4 node-level dialer replacement deletes matching node-level bindings before creating the new validated binding', async () => {
  const existingBindings = [
    { id: 101, target_type: 'node', target_name: 'US-01', dialer_ref: 'Old-Dialer-1' },
    { id: 102, target_type: 'node', target_name: 'US-01', dialer_ref: 'Old-Dialer-2' },
    { id: 103, target_type: 'node', target_name: 'Other-Node', dialer_ref: 'Old-Dialer-3' },
    { id: 104, target_type: 'subscription', target_id: 1, dialer_ref: 'Sub-Dialer' },
  ]

  const plan = planDialerReplacement(existingBindings, 'US-01', {
    dialer_type: 'node',
    dialer_ref: 'New-Dialer',
  })
  assert.deepEqual(plan.bindingsToDelete, [101, 102])
  assert.equal(plan.payloadToCreate.target_type, 'node')
  assert.equal(plan.payloadToCreate.target_name, 'US-01')
  assert.equal(plan.payloadToCreate.dialer_ref, 'New-Dialer')

  // Real execution helper test
  const deletedIds = []
  let createdRecord = null

  const result = await replaceNodeDialerProxy({
    nodeName: 'US-01',
    dialerType: 'node',
    dialerRef: 'New-Dialer',
    existingBindings,
    deleteBindingFn: async (id) => {
      deletedIds.push(id)
      return { ok: true }
    },
    createBindingFn: async (data) => {
      createdRecord = { id: 201, ...data }
      return createdRecord
    },
    note: 'dialer replaced in workbench',
  })

  assert.deepEqual(deletedIds, [101, 102])
  assert.equal(createdRecord.target_name, 'US-01')
  assert.equal(createdRecord.dialer_ref, 'New-Dialer')
  assert.equal(result.id, 201)
})

test('3.1 probe summary pagination page merge and boolean media filtering', () => {
  // Page 1 envelope
  const page1 = {
    results: {
      'node:001': {
        node_key: 'node:001',
        status: 'ok',
        latency_ms: 40,
        speed_mbps: 50.0,
        country: 'JP',
        ip: '198.51.100.1',
        media: { youtube: true, netflix: true, disney: false },
      },
      'node:002': {
        node_key: 'node:002',
        status: 'ok',
        latency_ms: 85,
        speed_mbps: 15.0,
        country: 'US',
        ip: '198.51.100.2',
        media: { youtube: true, netflix: false, disney: false },
      },
    },
    next_cursor: 'node:002',
    has_more: true,
  }

  // Normalize page 1
  const mapP1 = normalizeNodeLedgerMap(page1)
  assert.equal(Object.keys(mapP1).length, 2)
  assert.equal(mapP1['node:001']?.status, 'ok')
  assert.equal(mapP1['node:001']?.latency_ms, 40)
  assert.equal(mapP1['node:001']?.media?.youtube, true)

  // Page 2 envelope
  const page2 = {
    results: {
      'node:003': {
        node_key: 'node:003',
        status: 'fail',
        latency_ms: null,
        speed_mbps: null,
        country: 'SG',
        ip: null,
        media: { youtube: false, netflix: false, disney: false },
      },
    },
    next_cursor: null,
    has_more: false,
  }

  // Merge page 2 into mapP1
  const mergedMap = mergeNodeLedgerProbePages(mapP1, page2)
  assert.equal(Object.keys(mergedMap).length, 3)
  assert.equal(mergedMap['node:001']?.status, 'ok')
  assert.equal(mergedMap['node:002']?.status, 'ok')
  assert.equal(mergedMap['node:003']?.status, 'fail')

  // Verify getProbeForNode lookup by node_key and name fallback
  const testNode1 = { name: 'Japan-01', node_key: 'node:001' }
  const probeFound = getProbeForNode(mergedMap, testNode1)
  assert.equal(probeFound?.country, 'JP')

  // Verify filterAndSortNodes with boolean media summaries
  const sampleNodes = [
    { name: 'Japan-01', node_key: 'node:001', type: 'vmess' },
    { name: 'US-02', node_key: 'node:002', type: 'ss' },
    { name: 'SG-03', node_key: 'node:003', type: 'trojan' },
  ]

  // Verify filterAndSortNodes defensive non-array resilience
  assert.deepEqual(filterAndSortNodes(null, mergedMap, {}), [])
  assert.deepEqual(filterAndSortNodes(undefined, mergedMap, {}), [])
  assert.deepEqual(filterAndSortNodes('not-an-array', mergedMap, {}), [])

  // Filter for youtube AND netflix full unlock
  const filteredDoubleUnlock = filterAndSortNodes(sampleNodes, mergedMap, {
    keyword: '',
    subscription: '',
    protocol: '',
    status: 'all',
    country: '',
    chain: 'all',
    minSpeed: 0,
    mediaPlatforms: ['youtube', 'netflix'],
    sortBy: 'default',
  })
  assert.equal(filteredDoubleUnlock.length, 1)
  assert.equal(filteredDoubleUnlock[0].node_key, 'node:001')

  // Filter for youtube only
  const filteredYoutubeOnly = filterAndSortNodes(sampleNodes, mergedMap, {
    keyword: '',
    subscription: '',
    protocol: '',
    status: 'all',
    country: '',
    chain: 'all',
    minSpeed: 0,
    mediaPlatforms: ['youtube'],
    sortBy: 'default',
  })
  assert.equal(filteredYoutubeOnly.length, 2)
  assert.deepEqual(filteredYoutubeOnly.map((n) => n.node_key), ['node:001', 'node:002'])

  // 3.2 Drawer detail session state machine & race suppression
  const session = createDrawerDetailSession()
  assert.equal(session.status, 'idle')

  // Node with no key transitions to unavailable
  const noKeyRes = startDrawerDetailFetch(session, { name: 'unkeyed' })
  assert.equal(noKeyRes.shouldFetch, false)
  assert.equal(session.status, 'unavailable')

  // Selecting node 1 transitions to loading with new abort controller
  const fetch1 = startDrawerDetailFetch(session, { node_key: 'node:001' })
  assert.equal(fetch1.shouldFetch, true)
  assert.equal(session.status, 'loading')
  assert.equal(session.nodeKey, 'node:001')
  const ctrl1 = fetch1.controller

  // Switching to node 2 aborts previous controller and starts new loading
  const fetch2 = startDrawerDetailFetch(session, { node_key: 'node:002' })
  assert.equal(fetch2.shouldFetch, true)
  assert.equal(ctrl1.signal.aborted, true)
  assert.equal(session.status, 'loading')
  assert.equal(session.nodeKey, 'node:002')

  // Outdated response for node 1 arrives late -> discarded, does not mutate session
  resolveDrawerDetailFetch(session, 'node:001', { data: { latency_ms: 40 } })
  assert.equal(session.status, 'loading')
  assert.equal(session.probe, null)

  // Current response for node 2 arrives -> ready
  resolveDrawerDetailFetch(session, 'node:002', { data: { latency_ms: 85 } })
  assert.equal(session.status, 'ready')
  assert.equal(session.probe?.latency_ms, 85)

  // Error on node 2 retry -> unavailable
  resolveDrawerDetailFetch(session, 'node:002', null, new Error('network down'))
  assert.equal(session.status, 'unavailable')
})

test('Task 2.2: runSlidingWorkerPool bounds concurrency and ensures slow nodes do not block freed slots', async () => {
  const items = Array.from({ length: 8 }, (_, i) => ({ name: `Node-${i}`, id: i }))
  let currentActive = 0
  let maxObservedActive = 0
  const completionOrder = []

  // Node 0 is very slow (80ms), while Nodes 1..7 are fast (10ms)
  const workerFn = async (item) => {
    currentActive++
    if (currentActive > maxObservedActive) maxObservedActive = currentActive
    const delay = item.id === 0 ? 80 : 10
    await new Promise((resolve) => setTimeout(resolve, delay))
    currentActive--
    completionOrder.push(item.id)
    return { name: item.name, status: 'ok', latency_ms: delay }
  }

  const progressTicks = []
  const { results, stopped, ok, fail, done, total } = await runSlidingWorkerPool(items, {
    concurrency: 3,
    workerFn,
    onItemDone: (res, item, prog) => {
      progressTicks.push({ id: item.id, ...prog })
    },
  })

  // Concurrency bounds: never exceeds snapshot concurrency 3
  assert.equal(maxObservedActive <= 3, true, `Active concurrency (${maxObservedActive}) must not exceed limit 3`)
  assert.equal(stopped, false)
  assert.equal(total, 8)
  assert.equal(done, 8)
  assert.equal(ok, 8)
  assert.equal(fail, 0)
  assert.equal(results.length, 8)

  // Slow node 0 does not block slots: Node 1 and Node 2 finish, and subsequent nodes (3, 4) finish BEFORE Node 0
  assert.ok(completionOrder.indexOf(1) < completionOrder.indexOf(0), 'Node 1 finishes before slow Node 0')
  assert.ok(completionOrder.indexOf(2) < completionOrder.indexOf(0), 'Node 2 finishes before slow Node 0')
  assert.ok(completionOrder.indexOf(3) < completionOrder.indexOf(0), 'Slot reclaimed: Node 3 finishes before slow Node 0')

  // Smooth progressive updates
  assert.equal(progressTicks.length, 8)
  assert.equal(progressTicks[progressTicks.length - 1].done, 8)
})

test('Task 2.3: runSlidingWorkerPool cancellation semantics, in-flight abort, and completed result preservation', async () => {
  const items = Array.from({ length: 10 }, (_, i) => ({ name: `Node-${i}`, id: i }))
  const abortCtrl = new AbortController()
  const started = []
  const abortedSignals = []

  const workerFn = async (item, signal) => {
    started.push(item.id)
    signal.addEventListener('abort', () => abortedSignals.push(item.id))
    // Node 0 and 1 finish fast (10ms), Node 2 and 3 are slow (150ms)
    const delay = item.id < 2 ? 10 : 150
    await new Promise((resolve, reject) => {
      const timer = setTimeout(resolve, delay)
      signal.addEventListener('abort', () => {
        clearTimeout(timer)
        reject(new Error('AbortError'))
      })
    })
    return { name: item.name, status: 'ok' }
  }

  // Abort after 40ms: nodes 0 and 1 should have finished, in-flight nodes aborted, nodes 4..9 never started
  setTimeout(() => abortCtrl.abort(), 40)

  const { results, stopped, ok, done } = await runSlidingWorkerPool(items, {
    concurrency: 3,
    signal: abortCtrl.signal,
    workerFn,
  })

  assert.equal(stopped, true)
  // Nodes 0 and 1 completed
  assert.equal(results.length, 2)
  assert.deepEqual(results.map((r) => r.name), ['Node-0', 'Node-1'])
  assert.equal(ok, 2)
  assert.equal(done, 2)

  // In-flight items were aborted
  assert.ok(abortedSignals.length > 0, 'In-flight signals were aborted')

  // Nodes beyond in-flight slots were not started after cancellation
  assert.ok(started.length <= 5, `Expected <= 5 nodes started before 40ms abort, got ${started.length}`)
  assert.ok(!started.includes(5) && !started.includes(6) && !started.includes(9), 'Nodes 5..9 were not started after cancellation')

  // Pre-aborted signal scenario: 0 requests dispatched
  const preAbortedCtrl = new AbortController()
  preAbortedCtrl.abort()
  const preStarted = []
  const preResult = await runSlidingWorkerPool(items, {
    concurrency: 4,
    signal: preAbortedCtrl.signal,
    workerFn: async (item) => {
      preStarted.push(item.id)
      return { name: item.name, status: 'ok' }
    },
  })
  assert.equal(preResult.stopped, true)
  assert.equal(preResult.results.length, 0)
  assert.equal(preStarted.length, 0)
})

test('Task 2.4: calculateRemainingSeconds and formatCountdown server-time calibration', () => {
  const serverNow = 1000
  const clientFetchTimestamp = 50000
  const nextExpectedAt = 1600 // 600s in the future

  // 1. Immediately after fetch: 600 seconds remaining
  const rem0 = calculateRemainingSeconds(nextExpectedAt, serverNow, clientFetchTimestamp, 50000)
  assert.equal(rem0, 600)
  assert.equal(formatCountdown(rem0), '10:00')

  // 2. 65 seconds elapsed on client (irrespective of absolute clock offset)
  const rem65 = calculateRemainingSeconds(nextExpectedAt, serverNow, clientFetchTimestamp, 50000 + 65000)
  assert.equal(rem65, 535)
  assert.equal(formatCountdown(rem65), '08:55')

  // 3. Past expiration: clamps to 0
  const remPast = calculateRemainingSeconds(nextExpectedAt, serverNow, clientFetchTimestamp, 50000 + 700000)
  assert.equal(remPast, 0)
  assert.equal(formatCountdown(remPast), '00:00')

  // 4. Null next_expected_at returns null and formatCountdown returns '--:--'
  assert.equal(calculateRemainingSeconds(null, serverNow, clientFetchTimestamp, 50000), null)
  assert.equal(formatCountdown(null), '--:--')
  assert.equal(formatCountdown(-5), '--:--')
})

test('Task 2.5: AI observation tier matrix & presentation (Claude region signal & ChatGPT tiers)', () => {
  // 1. Platform definitions include Claude
  const claudePlat = MEDIA_PLATFORMS.find((p) => p.key === 'claude')
  assert.ok(claudePlat, 'MEDIA_PLATFORMS must include claude')
  assert.equal(claudePlat.name, 'Claude')

  // 2. Claude regional signal is NEVER full unlocked
  const claudeVerified = {
    status: 'verified',
    verdict: 'unknown',
    unlocked: false,
    region: 'US',
    confidence: 'verified',
    observation_kind: 'region_signal',
    tier: 'none',
    evidence: { http_status: 200, signals: ['loc_US'], elapsed_ms: 110 },
  }
  assert.equal(isMediaFullUnlocked(claudeVerified), false)

  const claudePres = getMediaSemanticPresentation(claudeVerified, claudePlat)
  assert.equal(claudePres.isFullUnlocked, false)
  assert.notEqual(claudePres.badgeVariant, 'success')
  assert.equal(claudePres.badgeVariant, 'info')
  assert.match(claudePres.accessibleTitle, /地区信号/)
  assert.equal(claudePres.shortBadgeText, 'Claude:US')

  // Claude challenge / timeout / rate_limited / inconclusive
  const claudeTimeout = getMediaSemanticPresentation({ status: 'timeout', observation_kind: 'region_signal' }, claudePlat)
  assert.equal(claudeTimeout.isFullUnlocked, false)
  assert.equal(claudeTimeout.badgeVariant, 'neutral')

  const claudeChallenge = getMediaSemanticPresentation({ status: 'challenged', verdict: 'challenge', observation_kind: 'region_signal' }, claudePlat)
  assert.equal(claudeChallenge.isFullUnlocked, false)
  assert.equal(claudeChallenge.badgeVariant, 'warning')

  // 3. ChatGPT Tiered Presentation
  const gptPlat = MEDIA_PLATFORMS.find((p) => p.key === 'chatgpt')
  assert.ok(gptPlat, 'MEDIA_PLATFORMS must include chatgpt')

  // 3a. App Tier verified (both Web and App pass) -> Highest tier (GPT⁺), Full Unlock
  const gptApp = {
    status: 'verified',
    verdict: 'available',
    unlocked: true,
    confidence: 'verified',
    observation_kind: 'capability',
    tier: 'app',
    region: 'US',
    subobservations: {
      web: { status: 'verified', verdict: 'available', unlocked: true },
      app: { status: 'verified', verdict: 'available', unlocked: true },
    },
  }
  assert.equal(isMediaFullUnlocked(gptApp), true)
  const gptAppPres = getMediaSemanticPresentation(gptApp, gptPlat)
  assert.equal(gptAppPres.isFullUnlocked, true)
  assert.equal(gptAppPres.badgeVariant, 'success')
  assert.match(gptAppPres.label, /GPT⁺/)
  assert.match(gptAppPres.shortBadgeText, /GPT⁺/)

  // 3b. Web Tier only (Web passes, App inconclusive/partial) -> NOT full unlocked, warning badge, GPT label
  const gptWeb = {
    status: 'partial',
    verdict: 'unknown',
    unlocked: false,
    confidence: 'verified',
    observation_kind: 'capability',
    tier: 'web',
    region: 'US',
    subobservations: {
      web: { status: 'verified', verdict: 'available', unlocked: true },
      app: { status: 'challenged', verdict: 'challenge', unlocked: false },
    },
  }
  assert.equal(isMediaFullUnlocked(gptWeb), false)
  const gptWebPres = getMediaSemanticPresentation(gptWeb, gptPlat)
  assert.equal(gptWebPres.isFullUnlocked, false)
  assert.notEqual(gptWebPres.badgeVariant, 'success')
  assert.equal(gptWebPres.badgeVariant, 'warning')
  assert.match(gptWebPres.label, /GPT/)
  assert.doesNotMatch(gptWebPres.label, /GPT⁺/)
  assert.match(gptWebPres.shortBadgeText, /GPT:Web|GPT:US/)

  // 3c. Rate limited / 429 / challenged ChatGPT
  const gpt429 = getMediaSemanticPresentation({ status: 'rate_limited', verdict: 'rate_limited', tier: 'none' }, gptPlat)
  assert.equal(gpt429.isFullUnlocked, false)
  assert.equal(gpt429.badgeVariant, 'warning')

  // 3d. Legacy compatibility (legacy payload without tier still works)
  const legacyGpt = { status: 'verified', verdict: 'available', unlocked: true }
  assert.equal(isMediaFullUnlocked(legacyGpt), true)
  const legacyGptPres = getMediaSemanticPresentation(legacyGpt, gptPlat)
  assert.equal(legacyGptPres.isFullUnlocked, true)
  assert.equal(legacyGptPres.badgeVariant, 'success')
})
