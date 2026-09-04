import assert from 'node:assert/strict'
import test from 'node:test'
import fs from 'node:fs'
import path from 'node:path'
import { scanContent } from '../scripts/visual_audit.mjs'

test('LedgerDrawer component exists and handles node inspection', () => {
  const drawerPath = path.resolve(import.meta.dirname, '../src/components/ledger/LedgerDrawer.vue')
  assert.ok(fs.existsSync(drawerPath), 'LedgerDrawer.vue should exist')
  const content = fs.readFileSync(drawerPath, 'utf8')
  assert.match(content, /BaseDrawer/)
  assert.match(content, /OUTBOUND IP/)
  assert.match(content, /probe-single/)
  assert.match(content, /save-chain/)
  assert.match(content, /clear-chain/)
  assert.match(content, /STREAMING & AI UNLOCKS/)
})

test('Phase 5.4 LedgerDrawer conforms to visual gate and accessibility contracts', () => {
  const drawerPath = path.resolve(import.meta.dirname, '../src/components/ledger/LedgerDrawer.vue')
  const content = fs.readFileSync(drawerPath, 'utf8')

  // Visual gate audit
  const audit = scanContent(content, 'components/ledger/LedgerDrawer.vue')
  assert.equal(audit.violations.length, 0, `LedgerDrawer.vue must have 0 visual gate violations, got ${audit.violations.length}`)

  // A11y and industrial standards
  assert.match(content, /min-h-\[44px\]/, 'Interactive tab buttons and selects must have 44px min touch target')
  assert.match(content, /tabular-nums/, 'LedgerDrawer must enforce tabular-nums on IP, latency, speed, and ports')
  assert.doesNotMatch(content, /backdrop-blur/, 'LedgerDrawer must not use backdrop-blur glassmorphism')
  assert.doesNotMatch(content, /rounded-xl/, 'LedgerDrawer must not use excessive rounded-xl radius')
})
