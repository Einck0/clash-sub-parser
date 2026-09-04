import assert from 'node:assert/strict'
import test from 'node:test'
import fs from 'node:fs'
import path from 'node:path'
import { scanContent } from '../scripts/visual_audit.mjs'

test('Ledger components (MetricsBar, SearchFilter) exist and define contract interfaces', () => {
  const metricsPath = path.resolve(import.meta.dirname, '../src/components/ledger/LedgerMetricsBar.vue')
  assert.ok(fs.existsSync(metricsPath), 'LedgerMetricsBar.vue should exist')
  const metricsContent = fs.readFileSync(metricsPath, 'utf8')
  assert.match(metricsContent, /TOTAL NODES/)
  assert.match(metricsContent, /HEALTHY \(ONLINE\)/)

  const filterPath = path.resolve(import.meta.dirname, '../src/components/ledger/LedgerSearchFilter.vue')
  assert.ok(fs.existsSync(filterPath), 'LedgerSearchFilter.vue should exist')
  const filterContent = fs.readFileSync(filterPath, 'utf8')
  assert.match(filterContent, /全部协议/)
  assert.match(filterContent, /change-view/)
  assert.match(filterContent, /解锁过滤/)
  assert.match(filterContent, /快捷地区/)
  assert.match(filterContent, /包含测速/)
})

test('Phase 5.4 NodeLedger and domain components satisfy visual gate and 36px density', () => {
  const rootDir = path.resolve(import.meta.dirname, '../src')

  // 1. LedgerMetricsBar.vue
  const metricsPath = path.join(rootDir, 'components/ledger/LedgerMetricsBar.vue')
  const metricsContent = fs.readFileSync(metricsPath, 'utf8')
  assert.match(metricsContent, /MetricCard/, 'LedgerMetricsBar must use shared MetricCard primitive')
  assert.match(metricsContent, /tabular-nums/, 'LedgerMetricsBar must use tabular-nums on figures')
  const metricsAudit = scanContent(metricsContent, 'components/ledger/LedgerMetricsBar.vue')
  assert.equal(metricsAudit.violations.length, 0, `LedgerMetricsBar.vue must have 0 visual gate violations, got ${metricsAudit.violations.length}`)

  // 2. LedgerSearchFilter.vue
  const filterPath = path.join(rootDir, 'components/ledger/LedgerSearchFilter.vue')
  const filterContent = fs.readFileSync(filterPath, 'utf8')
  assert.match(filterContent, /lucide-vue-next/, 'LedgerSearchFilter must use Lucide icons')
  assert.doesNotMatch(filterContent, /[\u{1F300}-\u{1F5FF}\u{1F600}-\u{1F64F}\u{1F680}-\u{1F6FF}]/u, 'LedgerSearchFilter must have zero emoji icons')
  const filterAudit = scanContent(filterContent, 'components/ledger/LedgerSearchFilter.vue')
  assert.equal(filterAudit.violations.length, 0, `LedgerSearchFilter.vue must have 0 visual gate violations, got ${filterAudit.violations.length}`)

  // 3. NodeLedger.vue
  const ledgerPath = path.join(rootDir, 'views/NodeLedger.vue')
  const ledgerContent = fs.readFileSync(ledgerPath, 'utf8')
  assert.match(ledgerContent, /VirtualNodeTable/, 'NodeLedger must use VirtualNodeTable for 2080+ nodes')
  assert.match(ledgerContent, /:estimate-size="36"/, 'NodeLedger VirtualNodeTable must use 36px estimated row size')
  assert.match(ledgerContent, /table-row-dense|h-\[36px\]/, 'NodeLedger must specify 36px compact row density')
  assert.match(ledgerContent, /tabular-nums/, 'NodeLedger must enforce tabular-nums across numerical columns')
  assert.doesNotMatch(ledgerContent, /backdrop-blur/, 'NodeLedger must eliminate all glassmorphism')
  const ledgerAudit = scanContent(ledgerContent, 'views/NodeLedger.vue')
  assert.equal(ledgerAudit.violations.length, 0, `NodeLedger.vue must have 0 visual gate violations, got ${ledgerAudit.violations.length}`)
})
