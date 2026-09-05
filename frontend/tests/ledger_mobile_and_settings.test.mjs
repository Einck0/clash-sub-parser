import test from 'node:test'
import assert from 'node:assert/strict'
import fs from 'node:fs'
import path from 'node:path'
import { scanContent } from '../scripts/visual_audit.mjs'

const srcDir = path.resolve(import.meta.dirname, '../src')

test('NodeLedger mobile list component exists, has >=44px touch targets, and avoids horizontal overflow', () => {
  const mobileListPath = path.join(srcDir, 'components/ledger/LedgerMobileList.vue')
  assert.ok(fs.existsSync(mobileListPath), 'LedgerMobileList.vue should exist')

  const content = fs.readFileSync(mobileListPath, 'utf8')

  // Checkbox hit target: at least 44x44px
  assert.match(content, /min-w-\[44px\]\s+min-h-\[44px\]|min-h-\[44px\]\s+min-w-\[44px\]/, 'Checkbox container must have min 44px hit target')

  // Detail action hit target: at least 44px
  assert.match(content, /min-h-\[44px\]\s+min-w-\[44px\]/, 'Detail button must have min 44px hit target')

  // Probe single action hit target: at least 44px
  assert.match(content, /min-h-\[44px\]\s+min-w-\[44px\]/, 'Probe button must have min 44px hit target')

  // Truncation and overflow protection
  assert.match(content, /overflow-hidden/, 'Mobile card must prevent overflow')
  assert.match(content, /truncate/, 'Node name must be truncated')

  // Status and latency
  assert.match(content, /getProbe.*latency_ms/, 'Must show latency for ok status')
  assert.match(content, /StatusBadge/, 'Must use StatusBadge for protocol')

  // Visual gate: zero violations, zero emoji, zero scoped styles
  assert.doesNotMatch(content, /<style scoped>/, 'Must not have scoped styles')
  assert.doesNotMatch(content, /[\u{1F300}-\u{1F5FF}\u{1F600}-\u{1F64F}\u{1F680}-\u{1F6FF}]/u, 'Must have zero emoji')

  const audit = scanContent(content, 'components/ledger/LedgerMobileList.vue')
  assert.equal(audit.violations.length, 0, `LedgerMobileList.vue must have 0 visual gate violations, got ${audit.violations.length}`)
})

test('NodeLedger integrates mobile list (<640px) and preserves desktop table/grid (>=640px)', () => {
  const ledgerPath = path.join(srcDir, 'views/NodeLedger.vue')
  assert.ok(fs.existsSync(ledgerPath), 'NodeLedger.vue should exist')

  const content = fs.readFileSync(ledgerPath, 'utf8')

  // Mobile list wrapper below 640px
  assert.match(content, /block\s+sm:hidden/, 'Must display mobile list below 640px')
  assert.match(content, /LedgerMobileList/, 'Must render LedgerMobileList')

  // Desktop wrapper at 640px+
  assert.match(content, /hidden\s+sm:block/, 'Must display table/grid at 640px and above')
  assert.match(content, /VirtualNodeTable/, 'Must preserve VirtualNodeTable for desktop')

  // Concurrency 5 hardcoding removed
  assert.doesNotMatch(content, /concurrency:\s*5/, 'Hardcoded concurrency 5 must be removed')

  // Visual gate: zero violations
  const audit = scanContent(content, 'views/NodeLedger.vue')
  assert.equal(audit.violations.length, 0, `NodeLedger.vue must have 0 visual gate violations, got ${audit.violations.length}`)
})

test('Settings includes service timeout, cron enabled/interval, master probe switch, and accurate copy', () => {
  const settingsPath = path.join(srcDir, 'views/Settings.vue')
  assert.ok(fs.existsSync(settingsPath), 'Settings.vue should exist')

  const content = fs.readFileSync(settingsPath, 'utf8')

  // New fields in probeConfig reactive object
  assert.match(content, /probe_service_timeout_ms:\s*2000/, 'Must include probe_service_timeout_ms with default 2000')
  assert.match(content, /probe_cron_enabled:\s*true/, 'Must include probe_cron_enabled with default true')
  assert.match(content, /probe_cron_interval_minutes:\s*60/, 'Must include probe_cron_interval_minutes with default 60')
  assert.match(content, /probe_concurrency:\s*10/, 'probe_concurrency default must be 10')

  // Template inputs and copy
  assert.match(content, /probe_service_timeout_ms/, 'Must have service timeout input')
  assert.match(content, /非整批或整节点超时/, 'Must accurately explain per service timeout, not batch')
  assert.match(content, /probe_cron_enabled/, 'Must have cron enabled toggle')
  assert.match(content, /probe_cron_interval_minutes/, 'Must have cron interval input')
  assert.match(content, /开启节点出站校验（主开关）/, 'Must distinguish master probe switch')

  // Old probe_interval scheduling semantics removed from form
  assert.doesNotMatch(content, /v-model\.number="probeConfig\.probe_interval_minutes"/, 'Old probe_interval scheduling input must be removed from form')

  // Visual gate: zero violations
  const audit = scanContent(content, 'views/Settings.vue')
  assert.equal(audit.violations.length, 0, `Settings.vue must have 0 visual gate violations, got ${audit.violations.length}`)
})
