import assert from 'node:assert/strict'
import test from 'node:test'
import fs from 'node:fs'
import path from 'node:path'
import * as domain from '../src/views/nodeLedgerDomain.ts'

const fixturesPath = path.resolve(import.meta.dirname, './fixtures/node_ledger_fixtures.json')
const fixtures = JSON.parse(fs.readFileSync(fixturesPath, 'utf8'))

test('Contract 1.2-D1: strict map/envelope/array boundary rejects missing and mismatched node_key records', () => {
  const validKey = 'valid|vmess|server.example.com:443'
  const validRecord = { node_key: validKey, name: 'Display Name', status: 'ok' }

  assert.deepEqual(domain.normalizeNodeLedgerMap({ [validKey]: validRecord }), {
    [validKey]: validRecord,
  })
  assert.deepEqual(
    domain.normalizeNodeLedgerMap({ results: { [validKey]: validRecord } }),
    { [validKey]: validRecord },
    'Envelope results must use the same strict map contract'
  )
  assert.deepEqual(
    domain.normalizeNodeLedgerMap([validRecord]),
    { [validKey]: validRecord },
    'Array records must be indexed only by their explicit node_key'
  )

  assert.deepEqual(domain.normalizeNodeLedgerMap({ [validKey]: { name: 'Missing key' } }), {})
  assert.deepEqual(domain.normalizeNodeLedgerMap([{ name: 'Missing key' }]), {})
  assert.deepEqual(
    domain.normalizeNodeLedgerMap({ [validKey]: { ...validRecord, node_key: 'other' } }),
    {},
    'Mismatched map key and record node_key must be rejected'
  )
  assert.deepEqual(
    domain.normalizeNodeLedgerMap([{ ...validRecord, node_key: 'other' }]),
    { other: { ...validRecord, node_key: 'other' } },
    'An array has no external map key, so its own explicit node_key is authoritative'
  )
})

test('Contract 2.1: ledger collections accept envelope arrays and reject invalid identities', () => {
  const key = 'array|vmess|array.example.com:443'
  const valid = { node_key: key, name: 'Array Node' }

  assert.deepEqual(
    domain.normalizeNodeLedgerMap({ results: [valid] }),
    { [key]: valid },
    'Envelope arrays must use the same strict key contract'
  )
  assert.deepEqual(domain.normalizeNodeLedgerItems([valid, { name: 'Missing key' }]), [valid])
  assert.deepEqual(
    domain.normalizeNodeLedgerItems({ [key]: { ...valid, node_key: 'wrong' } }),
    [],
    'A ledger map entry with a mismatched identity must be rejected'
  )
})

test('Contract 2.1: name fallback requires a unique current ledger row', () => {
  const key = 'unique|vmess|unique.example.com:443'
  const response = { name: 'Unique', status: 'ok' }
  const uniqueRows = [{ node_key: key, name: 'Unique' }]
  const duplicateRows = [
    { node_key: key, name: 'Unique' },
    { node_key: 'other|ss|other.example.com:443', name: 'Unique' },
  ]

  assert.deepEqual(domain.normalizeImmediateProbeResponse(response, key, uniqueRows), {
    ...response,
    node_key: key,
  })
  assert.equal(domain.normalizeImmediateProbeResponse(response, key, duplicateRows), undefined)
  assert.equal(domain.normalizeImmediateProbeResponse(response, key), undefined)
})

test('Contract 2.1: batch targets never select by display name', () => {
  const nodes = [
    { node_key: 'one|vmess|one.example.com:443', name: 'Duplicate' },
    { node_key: 'two|vmess|two.example.com:443', name: 'Duplicate' },
  ]

  assert.deepEqual(domain.buildEffectiveBatchTargets(nodes, new Set(['Duplicate']), nodes), [])
})

test('Contract 2.1: batch probe responses require their exact node_key', () => {
  const key = 'batch|vmess|batch.example.com:443'
  const rows = [{ node_key: key, name: 'Batch Node' }]

  assert.equal(domain.normalizeKeyedProbeResponse({ name: 'Batch Node', status: 'ok' }, key), undefined)
  assert.deepEqual(
    domain.normalizeKeyedProbeResponse({ node_key: key, name: 'Batch Node', status: 'ok' }, key),
    { node_key: key, name: 'Batch Node', status: 'ok' }
  )
  assert.equal(domain.normalizeImmediateProbeResponse({ name: 'Batch Node', status: 'ok' }, key, rows)?.node_key, key)
})
test('Contract 1.2-D1: probe lookup never falls back to display names or records under a different map key', () => {
  const key = 'exact|vmess|server.example.com:443'
  const record = { node_key: key, name: 'Duplicate Name', status: 'ok' }
  const probes = { [key]: record, 'Duplicate Name': record }

  assert.equal(domain.getProbeForNode(probes, { node_key: key, name: 'Duplicate Name' }), record)
  assert.equal(domain.getProbeForNode(probes, { name: 'Duplicate Name' }), undefined)
  assert.equal(
    domain.getProbeForNode({ wrong: record }, { node_key: key, name: 'Duplicate Name' }),
    undefined
  )
})
test('Contract 1.2-D1 legacy fixtures retain strict map behavior', () => {
  // Valid records: map key and entry.node_key match
  const page1 = fixtures.paged_probe_summaries.page_1
  const map = domain.normalizeNodeLedgerMap(page1)
  const hkKey = 'HK-01|vmess|hk1.example.com:443'
  assert.ok(map[hkKey], `Record must exist under canonical node_key: ${hkKey}`)
  assert.equal(map[hkKey].node_key, hkKey)
  assert.equal(map['HK-01'], undefined)

  // Mismatched records must be rejected from both possible indexes
  const mismatchedMap = domain.normalizeNodeLedgerMap(fixtures.paged_probe_summaries.mismatched_key_page)
  assert.equal(mismatchedMap['legitimate_map_key|vmess|server.com:443'], undefined)
  assert.equal(mismatchedMap['tampered_or_different_node_key|vmess|other.com:443'], undefined)
})

test('Contract 1.2-D3: Explicit closed status algebra (ok, fail, timeout, skipped, unknown, untested)', () => {
  // Red light assertion: normalizeProbeStatus helper must be exported
  assert.equal(
    typeof domain.normalizeProbeStatus,
    'function',
    'normalizeProbeStatus pure helper must be exported from nodeLedgerDomain.ts'
  )

  // Explicit status closure contract
  assert.equal(domain.normalizeProbeStatus(undefined, false), 'untested', 'No record must normalize to untested')
  assert.equal(domain.normalizeProbeStatus('ok', true), 'ok')
  assert.equal(domain.normalizeProbeStatus('OK', true), 'ok', 'Status normalization must be case-insensitive')
  assert.equal(domain.normalizeProbeStatus('fail', true), 'fail')
  assert.equal(domain.normalizeProbeStatus('timeout', true), 'timeout')
  assert.equal(domain.normalizeProbeStatus('skipped', true), 'skipped')

  // Unsupported or malformed status with existing record must normalize to unknown, NEVER untested
  assert.equal(
    domain.normalizeProbeStatus('unexpected_backend_status_code', true),
    'unknown',
    'Unrecognized probe status must normalize to unknown'
  )
  assert.equal(
    domain.normalizeProbeStatus('', true),
    'unknown',
    'Empty probe status on existing record must normalize to unknown, strictly prohibited from degrading to untested'
  )
  assert.equal(
    domain.normalizeProbeStatus(null, true),
    'unknown',
    'Null probe status on existing record must normalize to unknown'
  )

  // Red light assertion: getProbePresentation adapter must be exported
  assert.equal(
    typeof domain.getProbePresentation,
    'function',
    'getProbePresentation semantic presentation adapter must be exported from nodeLedgerDomain.ts'
  )

  // Test presentation adapter against canonical nodes and probes
  const page1Map = domain.normalizeNodeLedgerMap(fixtures.paged_probe_summaries.page_1)
  const page2Map = domain.normalizeNodeLedgerMap(fixtures.paged_probe_summaries.page_2)
  const fullProbes = { ...page1Map, ...page2Map }

  const hkNode = fixtures.current_nodes.find((n) => n.name === 'HK-01')
  const timeoutNode = fixtures.current_nodes.find((n) => n.name === 'JP-Timeout')
  const skippedNode = fixtures.current_nodes.find((n) => n.name === 'SG-Skipped')
  const unknownNode = fixtures.current_nodes.find((n) => n.name === 'KR-Unknown')
  const untestedNode = fixtures.current_nodes.find((n) => n.name === 'UK-Untested')

  // OK presentation
  const presOk = domain.getProbePresentation(hkNode, fullProbes)
  assert.equal(presOk.status, 'ok')
  assert.equal(presOk.isTested, true)
  assert.ok(presOk.label.includes('45'), 'OK presentation must include latency')

  // Timeout presentation
  const presTimeout = domain.getProbePresentation(timeoutNode, fullProbes)
  assert.equal(presTimeout.status, 'timeout')
  assert.equal(presTimeout.isTested, true)
  assert.equal(presTimeout.label, '超时')

  // Skipped presentation
  const presSkipped = domain.getProbePresentation(skippedNode, fullProbes)
  assert.equal(presSkipped.status, 'skipped')
  assert.equal(presSkipped.isTested, true)
  assert.ok(presSkipped.label.includes('跳过'), 'Skipped presentation must indicate skipped')

  // Unknown presentation
  const presUnknown = domain.getProbePresentation(unknownNode, fullProbes)
  assert.equal(presUnknown.status, 'unknown')
  assert.equal(presUnknown.isTested, true)
  assert.equal(presUnknown.label, '状态未知', 'Unknown presentation label must be "状态未知", never "未测"')

  // Untested presentation
  const presUntested = domain.getProbePresentation(untestedNode, fullProbes)
  assert.equal(presUntested.status, 'untested')
  assert.equal(presUntested.isTested, false)
  assert.equal(presUntested.label, '未测')
})

test('Contract 1.2-D3: Closed status filtering consistency (unknown & skipped never match untested)', () => {
  // Untested filter
  assert.equal(domain.nodeMatchesStatus(undefined, 'untested'), true, 'Absent probe must match untested filter')
  assert.equal(domain.nodeMatchesStatus({ status: 'untested' }, 'untested'), true)

  // Red light assertions: unknown and skipped must NEVER match untested
  assert.equal(
    domain.nodeMatchesStatus({ status: 'unknown' }, 'untested'),
    false,
    'Probe with status "unknown" must NOT match "untested" filter'
  )
  assert.equal(
    domain.nodeMatchesStatus({ status: 'skipped' }, 'untested'),
    false,
    'Probe with status "skipped" must NOT match "untested" filter'
  )
  assert.equal(
    domain.nodeMatchesStatus({ status: 'unexpected_backend_code' }, 'untested'),
    false,
    'Probe with unrecognized status must NOT match "untested" filter'
  )
  assert.equal(
    domain.nodeMatchesStatus({ status: '' }, 'untested'),
    false,
    'Probe record with empty status must NOT match "untested" filter'
  )

  // Distinct matching for unknown and skipped
  assert.equal(
    domain.nodeMatchesStatus({ status: 'unknown' }, 'unknown'),
    true,
    'Probe with status "unknown" must match "unknown" filter'
  )
  assert.equal(
    domain.nodeMatchesStatus({ status: 'unexpected_backend_code' }, 'unknown'),
    true,
    'Probe with unrecognized status must normalize to unknown and match "unknown" filter'
  )
  assert.equal(
    domain.nodeMatchesStatus({ status: 'skipped' }, 'skipped'),
    true,
    'Probe with status "skipped" must match "skipped" filter'
  )
})

test('Contract 1.2-D3: Static code gate bans getProbe(name) and direct template map lookup in NodeLedger.vue', () => {
  const ledgerVuePath = path.resolve(import.meta.dirname, '../src/views/NodeLedger.vue')
  const content = fs.readFileSync(ledgerVuePath, 'utf8')

  // 1. Template direct getProbe(name) call ban
  const templateMatch = content.match(/<template>([\s\S]*?)<\/template>/)
  assert.ok(templateMatch, 'NodeLedger.vue must contain a <template> block')
  const templateContent = templateMatch[1]

  const forbiddenTemplateCalls = [
    /getProbe\s*\(\s*item\.name\s*\)/g,
    /getProbe\s*\(\s*row\.name\s*\)/g,
    /getProbe\s*\(\s*node\.name\s*\)/g,
    /probes\s*\[\s*item\.name\s*\]/g,
    /probes\s*\[\s*row\.name\s*\]/g,
    /probes\s*\[\s*node\.name\s*\]/g,
  ]

  for (const regex of forbiddenTemplateCalls) {
    const hits = templateContent.match(regex) || []
    assert.equal(
      hits.length,
      0,
      `Template contains prohibited direct display-name probe lookup: ${regex}. Found ${hits.length} occurrence(s). All renderers must use key-based adapter.`
    )
  }

  // 2. Script local getProbe(name) definition ban
  const scriptMatches = Array.from(content.matchAll(/<script[\s\S]*?>([\s\S]*?)<\/script>/g))
  const scriptContent = scriptMatches.map((m) => m[1]).join('\n')

  const hasGetProbeFn = /function\s+getProbe\s*\(\s*name/g.test(scriptContent)
  assert.equal(
    hasGetProbeFn,
    false,
    'NodeLedger.vue <script> must NOT define local "function getProbe(name)". Use key-based getProbePresentation(node, probes) adapter.'
  )
})

test('Contract 1.2-D2: Duplicate display-name nodes have strictly isolated probe states and guarded chain actions', () => {
  const duplicateNodes = fixtures.current_nodes.filter((n) => n.name === 'US-Relay')
  assert.equal(duplicateNodes.length, 2, 'Fixture must have exactly 2 nodes named "US-Relay"')

  const node1 = duplicateNodes[0] // vmess, 443
  const node2 = duplicateNodes[1] // ss, 8388

  assert.notEqual(node1.node_key, node2.node_key, 'Same-name nodes must have distinct node_keys')

  const probes = {
    [node1.node_key]: {
      node_key: node1.node_key,
      name: 'US-Relay',
      status: 'ok',
      latency_ms: 120,
    },
    [node2.node_key]: {
      node_key: node2.node_key,
      name: 'US-Relay',
      status: 'fail',
      latency_ms: 0,
    },
  }

  // Probe isolation
  const probe1 = domain.getProbeForNode(probes, node1)
  const probe2 = domain.getProbeForNode(probes, node2)

  assert.ok(probe1, 'probe1 must be found for node1')
  assert.ok(probe2, 'probe2 must be found for node2')
  assert.equal(probe1.status, 'ok')
  assert.equal(probe1.latency_ms, 120)
  assert.equal(probe2.status, 'fail')
  assert.notEqual(probe1.status, probe2.status, 'Same-name nodes must have independent probe results')

  // Red light assertion: checkNodeChainActionEligibility must be exported and guard duplicate-name nodes
  assert.equal(
    typeof domain.checkNodeChainActionEligibility,
    'function',
    'checkNodeChainActionEligibility helper must be exported from nodeLedgerDomain.ts'
  )

  const allNodes = fixtures.current_nodes
  const eligibility1 = domain.checkNodeChainActionEligibility(node1, allNodes)
  assert.equal(
    eligibility1.canEdit,
    false,
    'Node with duplicate display-name must have chain editing disabled'
  )
  assert.equal(
    eligibility1.reason,
    '名称重复，暂不能安全编辑跳板',
    'Chain action disabled reason must match exact contract copy'
  )

  const eligibility2 = domain.checkNodeChainActionEligibility(node2, allNodes)
  assert.equal(eligibility2.canEdit, false)
  assert.equal(eligibility2.reason, '名称重复，暂不能安全编辑跳板')

  // Unique-name node must have chain editing enabled
  const uniqueNode = fixtures.current_nodes.find((n) => n.name === 'HK-01')
  const eligibilityUnique = domain.checkNodeChainActionEligibility(uniqueNode, allNodes)
  assert.equal(eligibilityUnique.canEdit, true)
})

test('Contract 1.2-D1/D3: Progressive pagination merge and reload persistence preserve timeout/fail/unknown without degrading to untested', () => {
  // Page 1 load
  let probesMap = domain.normalizeNodeLedgerMap(fixtures.paged_probe_summaries.page_1)
  const jpTimeoutKey = 'JP-Timeout|trojan|jp.example.com:443'
  assert.ok(probesMap[jpTimeoutKey], 'JP-Timeout must be loaded in page 1')
  assert.equal(probesMap[jpTimeoutKey].status, 'timeout')

  // Page 2 progressive merge
  probesMap = domain.mergeNodeLedgerProbePages(probesMap, fixtures.paged_probe_summaries.page_2)

  // Verify page 1 results were NOT lost during page 2 merge
  assert.ok(probesMap[jpTimeoutKey], 'JP-Timeout must remain present after page 2 merge')
  assert.equal(probesMap[jpTimeoutKey].status, 'timeout')

  // Verify page 2 results are integrated
  const usFailKey = 'US-Relay|ss|us-west.example.com:8388'
  const krUnknownKey = 'KR-Unknown|vmess|kr.example.com:443'
  assert.ok(probesMap[usFailKey], 'US-Relay (ss) must be present in merged map')
  assert.equal(probesMap[usFailKey].status, 'fail')
  assert.ok(probesMap[krUnknownKey], 'KR-Unknown must be present in merged map')

  // Rehydration & presentation check
  assert.equal(
    typeof domain.getProbePresentation,
    'function',
    'getProbePresentation must be exported for presentation check'
  )

  const jpNode = fixtures.current_nodes.find((n) => n.name === 'JP-Timeout')
  const usNode = fixtures.current_nodes.find((n) => n.node_key === usFailKey)
  const krNode = fixtures.current_nodes.find((n) => n.name === 'KR-Unknown')

  const presJp = domain.getProbePresentation(jpNode, probesMap)
  assert.equal(presJp.status, 'timeout')
  assert.equal(presJp.label, '超时', 'Timeout record must retain "超时" label, never degrade to "未测"')

  const presUs = domain.getProbePresentation(usNode, probesMap)
  assert.equal(presUs.status, 'fail')
  assert.equal(presUs.label, '失败', 'Fail record must retain "失败" label, never degrade to "未测"')

  const presKr = domain.getProbePresentation(krNode, probesMap)
  assert.equal(presKr.status, 'unknown')
  assert.equal(presKr.label, '状态未知', 'Unknown record must retain "状态未知" label, never degrade to "未测"')
})
