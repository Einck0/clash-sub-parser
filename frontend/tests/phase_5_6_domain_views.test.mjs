import test from 'node:test'
import assert from 'node:assert/strict'
import fs from 'node:fs'
import path from 'node:path'
import { scanContent } from '../scripts/visual_audit.mjs'

const viewsDir = path.resolve(import.meta.dirname, '../src/views')
const componentsDir = path.resolve(import.meta.dirname, '../src/components')

test('Phase 5.6 Rules & RuleCategoryDetail: compact table, a11y primitives, zero emoji, zero visual gate violations', () => {
  const rulesPath = path.join(viewsDir, 'Rules.vue')
  const detailPath = path.join(viewsDir, 'RuleCategoryDetail.vue')
  const presetsPath = path.join(componentsDir, 'RulePresetsModal.vue')
  const importPath = path.join(componentsDir, 'RuleImportModal.vue')

  assert.ok(fs.existsSync(rulesPath), 'Rules.vue should exist')
  assert.ok(fs.existsSync(detailPath), 'RuleCategoryDetail.vue should exist')
  assert.ok(fs.existsSync(presetsPath), 'RulePresetsModal.vue should exist')
  assert.ok(fs.existsSync(importPath), 'RuleImportModal.vue should exist')

  const rulesContent = fs.readFileSync(rulesPath, 'utf8')
  const detailContent = fs.readFileSync(detailPath, 'utf8')
  const presetsContent = fs.readFileSync(presetsPath, 'utf8')
  const importContent = fs.readFileSync(importPath, 'utf8')

  // Rules.vue
  assert.match(rulesContent, /MetricCard/, 'Rules.vue must use MetricCard')
  assert.match(rulesContent, /GripVertical/, 'Rules.vue must use Lucide GripVertical icon instead of crude emoji')
  assert.doesNotMatch(rulesContent, /<style scoped>/, 'Rules.vue must not have scoped styles')
  assert.doesNotMatch(rulesContent, /[☰⚡📡🔒]/, 'Rules.vue must not have emoji icons')
  const rulesAudit = scanContent(rulesContent, 'src/views/Rules.vue')
  assert.equal(rulesAudit.violations.length, 0, `Rules.vue must have 0 visual gate violations, got ${rulesAudit.violations.length}`)

  // RuleCategoryDetail.vue
  assert.match(detailContent, /rules-table/, 'RuleCategoryDetail.vue must use rules-table')
  assert.match(detailContent, /col-index.*col-enabled.*col-name.*col-type.*col-value.*col-proxy/s, 'RuleCategoryDetail.vue must have clear column breakdown')
  assert.match(detailContent, /GripVertical/, 'RuleCategoryDetail.vue must use Lucide GripVertical')
  assert.doesNotMatch(detailContent, /<style scoped>/, 'RuleCategoryDetail.vue must not have scoped styles')
  assert.doesNotMatch(detailContent, /[☰⚡📡🔒]/, 'RuleCategoryDetail.vue must not have emoji icons')
  const detailAudit = scanContent(detailContent, 'src/views/RuleCategoryDetail.vue')
  assert.equal(detailAudit.violations.length, 0, `RuleCategoryDetail.vue must have 0 visual gate violations, got ${detailAudit.violations.length}`)

  // Modals
  assert.doesNotMatch(presetsContent, /<style scoped>/, 'RulePresetsModal.vue must not have scoped styles')
  assert.doesNotMatch(importContent, /<style scoped>/, 'RuleImportModal.vue must not have scoped styles')
  assert.doesNotMatch(presetsContent, /preset-cat-icon/, 'RulePresetsModal.vue must not render emoji icons')
})

test('Phase 5.6 DnsSettings: dual visual/raw mode, structured grid alignment, zero visual gate violations', () => {
  const dnsPath = path.join(viewsDir, 'DnsSettings.vue')
  assert.ok(fs.existsSync(dnsPath), 'DnsSettings.vue should exist')
  const dnsContent = fs.readFileSync(dnsPath, 'utf8')

  // Dual mode
  assert.match(dnsContent, /switchTab\('visual'\)/, 'DnsSettings.vue must support visual tab')
  assert.match(dnsContent, /switchTab\('raw'\)/, 'DnsSettings.vue must support raw YAML tab')
  assert.match(dnsContent, /dns-form-grid/, 'DnsSettings.vue must have structured form grid')
  assert.match(dnsContent, /toggle-grid/, 'DnsSettings.vue must have toggle grid')
  assert.match(dnsContent, /rawYaml/, 'DnsSettings.vue must support raw YAML editing')

  // Visual gate & zero scoped style
  assert.doesNotMatch(dnsContent, /<style scoped>/, 'DnsSettings.vue must not have scoped styles')
  assert.doesNotMatch(dnsContent, /[⚡☰📡]/, 'DnsSettings.vue must not have emoji icons')
  const dnsAudit = scanContent(dnsContent, 'src/views/DnsSettings.vue')
  assert.equal(dnsAudit.violations.length, 0, `DnsSettings.vue must have 0 visual gate violations, got ${dnsAudit.violations.length}`)
})

test('Phase 5.6 Generate: 5-target export center, client scheme protocols, zero Script.js, zero visual gate violations', () => {
  const genPath = path.join(viewsDir, 'Generate.vue')
  assert.ok(fs.existsSync(genPath), 'Generate.vue should exist')
  const genContent = fs.readFileSync(genPath, 'utf8')

  // 5 target export center
  assert.match(genContent, /'clash'/, 'Generate.vue must support Clash target')
  assert.match(genContent, /'mihomo'/, 'Generate.vue must support Mihomo target')
  assert.match(genContent, /'stash'/, 'Generate.vue must support Stash target')
  assert.match(genContent, /'shadowrocket'/, 'Generate.vue must support Shadowrocket target')
  assert.match(genContent, /'sing-box'/, 'Generate.vue must support Sing-box target')

  // Client wakeup schemes
  assert.match(genContent, /clash:\/\//, 'Generate.vue must support clash:// scheme')
  assert.match(genContent, /stash:\/\//, 'Generate.vue must support stash:// scheme')
  assert.match(genContent, /sub:\/\//, 'Generate.vue must support sub:// scheme for Shadowrocket')
  assert.match(genContent, /sing-box:\/\//, 'Generate.vue must support sing-box:// scheme')

  // QuickExport parity
  assert.match(genContent, /getQuickExport/, 'Generate.vue must integrate getQuickExport')

  // Zero Script.js or /script downloads
  assert.doesNotMatch(genContent, /Script\.js/i, 'Generate.vue must have zero Script.js occurrences')
  assert.doesNotMatch(genContent, /\/script['"]/i, 'Generate.vue must have zero /script download occurrences')

  // Profile compiler features preserved
  assert.match(genContent, /data-testid="generate-yaml"/, 'Generate.vue must preserve generate-yaml button')
  assert.match(genContent, /data-testid="generated-yaml-output"/, 'Generate.vue must preserve generated-yaml-output textarea')
  assert.match(genContent, /MetricCard/, 'Generate.vue must use MetricCard')

  // Zero emoji, zero glassmorphism, zero scoped style, zero visual gate violations
  assert.doesNotMatch(genContent, /<style scoped>/, 'Generate.vue must not have scoped styles')
  assert.doesNotMatch(genContent, /[⚡☰📡]/, 'Generate.vue must not have emoji icons')
  const genAudit = scanContent(genContent, 'src/views/Generate.vue')
  assert.equal(genAudit.violations.length, 0, `Generate.vue must have 0 visual gate violations, got ${genAudit.violations.length}`)
})

test('Phase 5.6 Settings & ConfigHistory: industrial dark form, token security, snapshot rollback, zero visual gate violations', () => {
  const settingsPath = path.join(viewsDir, 'Settings.vue')
  const historyPath = path.join(viewsDir, 'ConfigHistory.vue')
  assert.ok(fs.existsSync(settingsPath), 'Settings.vue should exist')
  assert.ok(fs.existsSync(historyPath), 'ConfigHistory.vue should exist')

  const settingsContent = fs.readFileSync(settingsPath, 'utf8')
  const historyContent = fs.readFileSync(historyPath, 'utf8')

  // Settings
  assert.match(settingsContent, /:type="showToken \? 'text' : 'password'"/, 'Settings.vue must support password masking for tokens')
  assert.match(settingsContent, /generateToken/, 'Settings.vue must support token generation')
  assert.match(settingsContent, /updateSecuritySettings/, 'Settings.vue must support security settings update')
  assert.doesNotMatch(settingsContent, /<style scoped>/, 'Settings.vue must not have scoped styles')
  assert.doesNotMatch(settingsContent, /[⚡☰📡]/, 'Settings.vue must not have emoji icons')
  const settingsAudit = scanContent(settingsContent, 'src/views/Settings.vue')
  assert.equal(settingsAudit.violations.length, 0, `Settings.vue must have 0 visual gate violations, got ${settingsAudit.violations.length}`)

  // ConfigHistory
  assert.match(historyContent, /snapshot-timeline/, 'ConfigHistory.vue must render timeline')
  assert.match(historyContent, /doRestore/, 'ConfigHistory.vue must support one-click restore')
  assert.match(historyContent, /createManual/, 'ConfigHistory.vue must support manual snapshot creation')
  assert.doesNotMatch(historyContent, /<style scoped>/, 'ConfigHistory.vue must not have scoped styles')
  const historyAudit = scanContent(historyContent, 'src/views/ConfigHistory.vue')
  assert.equal(historyAudit.violations.length, 0, `ConfigHistory.vue must have 0 visual gate violations, got ${historyAudit.violations.length}`)
})
