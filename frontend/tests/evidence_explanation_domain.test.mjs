import assert from 'node:assert/strict'
import test from 'node:test'
import {
  filterAndSortNodes,
  getMediaSemanticPresentation,
  isMediaFullUnlocked,
  sanitizeEvidenceSummary,
} from '../src/views/nodeLedgerDomain.ts'

test('Task 3.1: isMediaFullUnlocked pure predicate contract', async (t) => {
  await t.test('handles empty, null, undefined, primitives', () => {
    assert.equal(isMediaFullUnlocked(null), false)
    assert.equal(isMediaFullUnlocked(undefined), false)
    assert.equal(isMediaFullUnlocked({}), false)
    assert.equal(isMediaFullUnlocked([]), false)
    assert.equal(isMediaFullUnlocked(''), false)
    assert.equal(isMediaFullUnlocked(false), false)
    assert.equal(isMediaFullUnlocked(true), true)
  })

  await t.test('legacy payload compatibility', () => {
    // Historical full and ok pass
    assert.equal(isMediaFullUnlocked({ status: 'full', region: 'HK' }), true)
    assert.equal(isMediaFullUnlocked({ status: 'FULL' }), true)
    assert.equal(isMediaFullUnlocked({ status: 'ok', region: 'US' }), true)
    assert.equal(isMediaFullUnlocked({ status: 'OK' }), true)
    assert.equal(isMediaFullUnlocked({ unlocked: true }), true)

    // Historical originals, fail, blocked, unknown fail full unlock
    assert.equal(isMediaFullUnlocked({ status: 'originals', region: 'HK' }), false)
    assert.equal(isMediaFullUnlocked({ status: 'fail' }), false)
    assert.equal(isMediaFullUnlocked({ status: 'blocked' }), false)
    assert.equal(isMediaFullUnlocked({ status: 'unknown' }), false)
  })

  await t.test('evidence-grade normalized states', () => {
    // Verified full and available pass
    const verifiedFull = {
      status: 'verified',
      verdict: 'full',
      unlocked: true,
      region: 'US',
      confidence: 'verified',
      evidence_version: 'catalogue-2026-09-05',
      evidence: { http_status: 200, signals: ['non_original_available', 'original_available'] },
    }
    assert.equal(isMediaFullUnlocked(verifiedFull), true)

    const verifiedAvailable = {
      status: 'verified',
      verdict: 'available',
      unlocked: true,
      region: 'US',
      confidence: 'verified',
      evidence_version: 'catalogue-2026-09-05',
      evidence: { http_status: 200, signals: ['service_available'] },
    }
    assert.equal(isMediaFullUnlocked(verifiedAvailable), true)

    // Partial / originals_only MUST fail
    const partialOriginals = {
      status: 'partial',
      verdict: 'originals_only',
      unlocked: false,
      region: 'US',
      confidence: 'verified',
      evidence_version: 'catalogue-2026-09-05',
      evidence: { http_status: 200, signals: ['original_available'] },
    }
    assert.equal(isMediaFullUnlocked(partialOriginals), false)

    // All restricted / error / challenge / rate_limited / timeout / transport / inconclusive fail
    assert.equal(isMediaFullUnlocked({ status: 'restricted', verdict: 'unsupported_region', unlocked: false, confidence: 'verified' }), false)
    assert.equal(isMediaFullUnlocked({ status: 'ip_blocked', verdict: 'blocked', unlocked: false, confidence: 'verified' }), false)
    assert.equal(isMediaFullUnlocked({ status: 'challenged', verdict: 'challenge', unlocked: false, confidence: 'verified' }), false)
    assert.equal(isMediaFullUnlocked({ status: 'rate_limited', verdict: 'rate_limited', unlocked: false, confidence: 'verified' }), false)
    assert.equal(isMediaFullUnlocked({ status: 'timeout', verdict: 'unknown', unlocked: false, confidence: 'unavailable' }), false)
    assert.equal(isMediaFullUnlocked({ status: 'transport_error', verdict: 'unknown', unlocked: false, confidence: 'unavailable' }), false)
    assert.equal(isMediaFullUnlocked({ status: 'inconclusive', verdict: 'unknown', unlocked: false, confidence: 'verified' }), false)
    assert.equal(isMediaFullUnlocked({ status: 'disabled', verdict: 'unknown', unlocked: false }), false)

    // Conflicted or unavailable confidence fails even if verdict is full
    assert.equal(isMediaFullUnlocked({ status: 'verified', verdict: 'full', unlocked: true, confidence: 'conflicted' }), false)
    assert.equal(isMediaFullUnlocked({ status: 'verified', verdict: 'full', unlocked: true, confidence: 'unavailable' }), false)
  })
})

test('Task 3.1: getMediaSemanticPresentation UI presentation helper', async (t) => {
  await t.test('verified full presentation', () => {
    const p = getMediaSemanticPresentation(
      {
        status: 'verified',
        verdict: 'full',
        region: 'US',
        confidence: 'verified',
        evidence_version: 'catalogue-2026-09-05',
        evidence: { http_status: 200, signals: ['non_original_available', 'original_available'], elapsed_ms: 150 },
      },
      { key: 'netflix', name: 'Netflix', short: 'NF' }
    )
    assert.equal(p.isFullUnlocked, true)
    assert.equal(p.isPartial, false)
    assert.equal(p.badgeVariant, 'success')
    assert.equal(p.region, 'US')
    assert.equal(p.shortBadgeText, 'NF:US')
    assert.match(p.accessibleTitle, /Netflix.*全解/)
  })

  await t.test('partial originals_only presentation displays 仅自制剧 without success badge', () => {
    const p = getMediaSemanticPresentation(
      {
        status: 'partial',
        verdict: 'originals_only',
        region: 'US',
        confidence: 'verified',
        evidence_version: 'catalogue-2026-09-05',
        evidence: { http_status: 200, signals: ['original_available'], elapsed_ms: 120 },
      },
      { key: 'netflix', name: 'Netflix', short: 'NF' }
    )
    assert.equal(p.isFullUnlocked, false)
    assert.equal(p.isPartial, true)
    assert.notEqual(p.badgeVariant, 'success')
    assert.equal(p.badgeVariant, 'warning')
    assert.match(p.label, /自制/)
    assert.equal(p.shortBadgeText, 'NF:自制')
    assert.match(p.accessibleTitle, /仅自制剧/)
  })

  await t.test('inconclusive presentation displays 未定 without success badge', () => {
    const p = getMediaSemanticPresentation(
      {
        status: 'inconclusive',
        verdict: 'unknown',
        confidence: 'verified',
        evidence_version: 'catalogue-2026-09-05',
        evidence: { http_status: 200, signals: ['unrecognized_contract'], elapsed_ms: 80 },
      },
      { key: 'netflix', name: 'Netflix', short: 'NF' }
    )
    assert.equal(p.isFullUnlocked, false)
    assert.equal(p.isInconclusive, true)
    assert.notEqual(p.badgeVariant, 'success')
    assert.equal(p.badgeVariant, 'neutral')
    assert.match(p.label, /未定/)
    assert.equal(p.shortBadgeText, 'NF:未定')
  })

  await t.test('restricted, challenged, rate_limited, transport_error never have success badge', () => {
    const restricted = getMediaSemanticPresentation({ status: 'restricted', verdict: 'unsupported_region' }, { key: 'netflix', name: 'Netflix', short: 'NF' })
    assert.notEqual(restricted.badgeVariant, 'success')
    assert.equal(restricted.isFullUnlocked, false)

    const challenged = getMediaSemanticPresentation({ status: 'challenged', verdict: 'challenge' }, { key: 'netflix', name: 'Netflix', short: 'NF' })
    assert.notEqual(challenged.badgeVariant, 'success')
    assert.equal(challenged.isFullUnlocked, false)

    const rateLimited = getMediaSemanticPresentation({ status: 'rate_limited', verdict: 'rate_limited' }, { key: 'netflix', name: 'Netflix', short: 'NF' })
    assert.notEqual(rateLimited.badgeVariant, 'success')
    assert.equal(rateLimited.isFullUnlocked, false)

    const transportErr = getMediaSemanticPresentation({ status: 'transport_error', verdict: 'unknown' }, { key: 'netflix', name: 'Netflix', short: 'NF' })
    assert.notEqual(transportErr.badgeVariant, 'success')
    assert.equal(transportErr.isFullUnlocked, false)
  })

  await t.test('historical full and originals presentation', () => {
    const histFull = getMediaSemanticPresentation({ status: 'full', region: 'HK' }, { key: 'netflix', name: 'Netflix', short: 'NF' })
    assert.equal(histFull.isFullUnlocked, true)
    assert.equal(histFull.badgeVariant, 'success')

    const histOriginals = getMediaSemanticPresentation({ status: 'originals', region: 'HK' }, { key: 'netflix', name: 'Netflix', short: 'NF' })
    assert.equal(histOriginals.isFullUnlocked, false)
    assert.equal(histOriginals.isPartial, true)
    assert.equal(histOriginals.badgeVariant, 'warning')
  })
})

test('Task 3.1: sanitizeEvidenceSummary sanitizes signals and strips sensitive information', () => {
  const cleanSummary = sanitizeEvidenceSummary({
    http_status: 200,
    signals: ['service_available', 'region_detected'],
    elapsed_ms: 145,
    redirect_class: 'none',
  })
  assert.match(cleanSummary, /HTTP 200/)
  assert.match(cleanSummary, /service_available/)
  assert.match(cleanSummary, /145ms/)

  // Secrets must be strictly scrubbed
  const leakyEvidence = {
    http_status: 200,
    signals: [
      'safe_signal',
      'authorization: Bearer eyJhbGciOi...',
      'cookie: session=secret123',
      'proxy_credential_user_pass',
      'api_token_secret',
    ],
    elapsed_ms: 90,
  }
  const scrubbed = sanitizeEvidenceSummary(leakyEvidence)
  assert.match(scrubbed, /safe_signal/)
  assert.doesNotMatch(scrubbed, /Bearer/)
  assert.doesNotMatch(scrubbed, /cookie/i)
  assert.doesNotMatch(scrubbed, /credential/i)
  assert.doesNotMatch(scrubbed, /token/i)
  assert.doesNotMatch(scrubbed, /secret/i)
})

test('Task 3.1: filterAndSortNodes integrates verified-only media filtering', () => {
  const nodes = [
    { name: 'Node-Verified-Full', type: 'vmess' },
    { name: 'Node-Partial-Originals', type: 'vmess' },
    { name: 'Node-Inconclusive', type: 'vmess' },
    { name: 'Node-Legacy-Full', type: 'vmess' },
    { name: 'Node-Legacy-Originals', type: 'vmess' },
    { name: 'Node-Restricted', type: 'vmess' },
  ]

  const probes = {
    'Node-Verified-Full': {
      status: 'ok',
      media: {
        netflix: {
          status: 'verified',
          verdict: 'full',
          unlocked: true,
          confidence: 'verified',
        },
      },
    },
    'Node-Partial-Originals': {
      status: 'ok',
      media: {
        netflix: {
          status: 'partial',
          verdict: 'originals_only',
          unlocked: false,
          confidence: 'verified',
        },
      },
    },
    'Node-Inconclusive': {
      status: 'ok',
      media: {
        netflix: {
          status: 'inconclusive',
          verdict: 'unknown',
          unlocked: false,
          confidence: 'verified',
        },
      },
    },
    'Node-Legacy-Full': {
      status: 'ok',
      media: {
        netflix: {
          status: 'full',
          unlocked: true,
        },
      },
    },
    'Node-Legacy-Originals': {
      status: 'ok',
      media: {
        netflix: {
          status: 'originals',
          unlocked: true, // Legacy record might have had unlocked: true, but status is originals!
        },
      },
    },
    'Node-Restricted': {
      status: 'ok',
      media: {
        netflix: {
          status: 'restricted',
          verdict: 'unsupported_region',
          unlocked: false,
        },
      },
    },
  }

  const baseFilters = {
    keyword: '',
    subscription: '',
    protocol: '',
    status: 'all',
    country: '',
    chain: 'all',
    minSpeed: 0,
    mediaPlatforms: ['netflix'],
    sortBy: 'default',
  }

  const result = filterAndSortNodes(nodes, probes, baseFilters)
  const resultNames = result.map((n) => n.name)

  // ONLY Verified-Full and Legacy-Full should pass!
  assert.ok(resultNames.includes('Node-Verified-Full'), 'Node-Verified-Full must pass')
  assert.ok(resultNames.includes('Node-Legacy-Full'), 'Node-Legacy-Full must pass')

  // Partial originals, inconclusive, legacy originals, restricted MUST NOT pass!
  assert.equal(resultNames.includes('Node-Partial-Originals'), false, 'Node-Partial-Originals must NOT pass')
  assert.equal(resultNames.includes('Node-Legacy-Originals'), false, 'Node-Legacy-Originals must NOT pass')
  assert.equal(resultNames.includes('Node-Inconclusive'), false, 'Node-Inconclusive must NOT pass')
  assert.equal(resultNames.includes('Node-Restricted'), false, 'Node-Restricted must NOT pass')
})
