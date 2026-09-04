import assert from 'node:assert/strict'
import test from 'node:test'
import fs from 'node:fs'
import path from 'node:path'

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
})
