import assert from 'node:assert/strict'
import test from 'node:test'
import fs from 'node:fs'
import path from 'node:path'

test('LedgerDrawer component exists and handles node inspection', () => {
  const drawerPath = path.resolve(import.meta.dirname, '../src/components/ledger/LedgerDrawer.vue')
  assert.ok(fs.existsSync(drawerPath), 'LedgerDrawer.vue should exist')
  const content = fs.readFileSync(drawerPath, 'utf8')
  assert.match(content, /BaseDrawer/)
  assert.match(content, /OUTBOUND IP/)
  assert.match(content, /probe-single/)
})
