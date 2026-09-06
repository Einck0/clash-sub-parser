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

  // 4. LedgerSchedulerBanner.vue (Task 2.4)
  const bannerPath = path.join(rootDir, 'components/ledger/LedgerSchedulerBanner.vue')
  assert.ok(fs.existsSync(bannerPath), 'LedgerSchedulerBanner.vue must exist')
  const bannerContent = fs.readFileSync(bannerPath, 'utf8')
  assert.match(bannerContent, /role="region"/, 'Banner must provide accessible region role')
  assert.match(bannerContent, /aria-label="定时质检调度状态"/, 'Banner must provide descriptive aria-label')
  assert.match(bannerContent, /getProbeStatus/, 'Banner must query getProbeStatus API')
  assert.match(bannerContent, /calculateRemainingSeconds|formatCountdown/, 'Banner must use drift-free server countdown')
  assert.match(bannerContent, /tabular-nums/, 'Banner must enforce tabular-nums on countdown numbers')
  assert.match(bannerContent, /min-h-\[44px\]/, 'Banner refresh button must provide >= 44px mobile touch target')
  assert.doesNotMatch(bannerContent, /input|select|textarea/i, 'Banner must not copy Settings configuration inputs')
  const bannerAudit = scanContent(bannerContent, 'components/ledger/LedgerSchedulerBanner.vue')
  assert.equal(bannerAudit.violations.length, 0, `LedgerSchedulerBanner.vue must have 0 visual gate violations, got ${bannerAudit.violations.length}`)

  // 5. NodeLedger worker pool and cancellation contract (Task 2.2 & 2.3)
  assert.match(ledgerContent, /LedgerSchedulerBanner/, 'NodeLedger must integrate LedgerSchedulerBanner')
  assert.match(ledgerContent, /runSlidingWorkerPool/, 'NodeLedger must use runSlidingWorkerPool')
  assert.match(ledgerContent, /getProbeSettings/, 'NodeLedger must fetch probe settings snapshot')
  assert.doesNotMatch(ledgerContent, /CHUNK_SIZE\s*=\s*10/, 'NodeLedger must eliminate hardcoded CHUNK_SIZE = 10')
  assert.doesNotMatch(ledgerContent, /probeNodesFull/, 'NodeLedger must eliminate batch probeNodesFull barrier')
  assert.match(ledgerContent, /已停止后续探测/, 'NodeLedger must use truthful "已停止后续探测" copy')
  assert.doesNotMatch(ledgerContent, /服务端作业已取消/, 'NodeLedger must not claim server-side job cancellation')
  assert.doesNotMatch(ledgerContent, /已停止后续探测任务/, 'NodeLedger must use standardized "已停止后续探测" copy')
})
