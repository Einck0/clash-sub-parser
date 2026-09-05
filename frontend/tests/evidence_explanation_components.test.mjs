import assert from 'node:assert/strict'
import test from 'node:test'
import fs from 'node:fs'
import path from 'node:path'
import { scanContent } from '../scripts/visual_audit.mjs'

test('Task 3.2: NodePreviewList evidence explanation and badge semantics', () => {
  const filePath = path.resolve(import.meta.dirname, '../src/components/NodePreviewList.vue')
  assert.ok(fs.existsSync(filePath), 'NodePreviewList.vue must exist')
  const content = fs.readFileSync(filePath, 'utf8')

  // Visual gate audit
  const audit = scanContent(content, 'components/NodePreviewList.vue')
  assert.equal(audit.violations.length, 0, `NodePreviewList.vue must have 0 visual gate violations, got ${audit.violations.length}`)

  // Must import or consume centralized domain presentation helpers
  assert.match(content, /getMediaSemanticPresentation|isMediaFullUnlocked/, 'NodePreviewList must use centralized domain helpers')

  // Must not have crude raw inline condition claiming originals/unlocked is success
  assert.doesNotMatch(
    content,
    /status === 'originals' \|\| [^)]*unlocked/,
    'NodePreviewList must not treat originals or loose unlocked as generic success'
  )

  // Accessible titles must be present
  assert.match(content, /:title=/, 'Media badges must have accessible dynamic titles')

  // Secret safety check: no template interpolation of tokens, cookies, auth headers, or raw bodies
  assert.doesNotMatch(content, /token|cookie|auth_header|credential|password/i, 'No secret fields allowed in template')
})

test('Task 3.2: LedgerDrawer authoritative evidence proof rows', () => {
  const filePath = path.resolve(import.meta.dirname, '../src/components/ledger/LedgerDrawer.vue')
  assert.ok(fs.existsSync(filePath), 'LedgerDrawer.vue must exist')
  const content = fs.readFileSync(filePath, 'utf8')

  // Visual gate audit
  const audit = scanContent(content, 'components/ledger/LedgerDrawer.vue')
  assert.equal(audit.violations.length, 0, `LedgerDrawer.vue must have 0 visual gate violations, got ${audit.violations.length}`)

  // Must consume centralized domain presentation helper
  assert.match(content, /getMediaSemanticPresentation|isMediaFullUnlocked/, 'LedgerDrawer must consume centralized domain helpers')

  // Diagnostics row requirements: verdict, region, confidence, signal summary, evidence version, timestamp
  assert.match(content, /verdict|VERDICT/i, 'LedgerDrawer must display verdict in diagnostics')
  assert.match(content, /confidence|CONFIDENCE/i, 'LedgerDrawer must display confidence in diagnostics')
  assert.match(content, /evidence_version|evidenceVersion|VERSION/i, 'LedgerDrawer must display evidence version in diagnostics')
  assert.match(content, /signals|signalSummary|SIGNALS/i, 'LedgerDrawer must display signal summary in diagnostics')
  assert.match(content, /checked_at|checkedAt|TIMESTAMP/i, 'LedgerDrawer must display timestamp in diagnostics')

  // Tabular numerics for dense diagnostics data
  assert.match(content, /tabular-nums/, 'LedgerDrawer diagnostics must enforce tabular-nums')

  // Secret safety check: strictly no rendering or leaking of secrets
  assert.doesNotMatch(content, /cookie|token|auth_header|proxy_credential|authorization/i, 'No secret fields rendered in LedgerDrawer')
})

test('Task 3.2: Compact mobile ledger renderer (LedgerMobileList) shows compact semantic badges', () => {
  const filePath = path.resolve(import.meta.dirname, '../src/components/ledger/LedgerMobileList.vue')
  assert.ok(fs.existsSync(filePath), 'LedgerMobileList.vue must exist')
  const content = fs.readFileSync(filePath, 'utf8')

  // Visual gate audit
  const audit = scanContent(content, 'components/ledger/LedgerMobileList.vue')
  assert.equal(audit.violations.length, 0, `LedgerMobileList.vue must have 0 visual gate violations, got ${audit.violations.length}`)

  // Must retain 44px min touch target on all interactive controls
  assert.match(content, /min-h-\[44px\]/, 'Mobile list must enforce min-h-[44px] on interactive controls')
  assert.match(content, /min-w-\[44px\]/, 'Mobile list must enforce min-w-[44px] on interactive controls')

  // Must display media badges or support evidence inspection
  assert.match(content, /inspect/, 'Mobile list must support inspect to open LedgerDrawer')
})
