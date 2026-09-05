import assert from 'node:assert/strict'
import test from 'node:test'
import fs from 'node:fs'
import path from 'node:path'
import { scanContent } from '../scripts/visual_audit.mjs'

const srcDir = path.resolve(import.meta.dirname, '../src')

test('TDD 3.1: QuickExportModal is migrated to AppModal size="md" and contains zero raw overlay markup', () => {
  const filePath = path.resolve(srcDir, 'components/QuickExportModal.vue')
  assert.ok(fs.existsSync(filePath), 'QuickExportModal.vue must exist')
  const content = fs.readFileSync(filePath, 'utf8')

  // Must import and use AppModal with size="md"
  assert.match(content, /<AppModal\b/, 'QuickExportModal must use <AppModal>')
  assert.match(content, /size="md"/, 'QuickExportModal must specify size="md"')
  assert.doesNotMatch(content, /<div[^>]*\bfixed\s+inset-0\b/, 'QuickExportModal must not declare its own fixed overlay')
  assert.doesNotMatch(content, /class="[^"]*\bmodal\b[^"]*"/, 'QuickExportModal must not use legacy .modal class')
  assert.doesNotMatch(content, /class="[^"]*\bmodal-backdrop\b[^"]*"/, 'QuickExportModal must not use legacy .modal-backdrop class')

  // Retain domain capabilities
  assert.match(content, /TARGET_DEFS/, 'Must retain 5 export targets')
  assert.match(content, /clash.*mihomo.*stash.*shadowrocket.*sing-box/s, 'Must retain all 5 export formats')
  assert.match(content, /exportMode/, 'Must retain merged vs subscription export modes')
  assert.match(content, /currentSchemeUrl|clientWakeupLabel/, 'Must retain scheme client wakeup')
  assert.match(content, /currentExportUrl/, 'Must retain export URL generation')
  assert.match(content, /copyUrl/, 'Must retain copy URL action')
  assert.match(content, /QrCode/, 'Must retain QR code display')
})

test('TDD 3.2: RuleImportModal is migrated to AppModal size="md" with zero legacy CSS classes or private styles', () => {
  const filePath = path.resolve(srcDir, 'components/RuleImportModal.vue')
  assert.ok(fs.existsSync(filePath), 'RuleImportModal.vue must exist')
  const content = fs.readFileSync(filePath, 'utf8')

  // Must use AppModal size="md"
  assert.match(content, /<AppModal\b/, 'RuleImportModal must use <AppModal>')
  assert.match(content, /size="md"/, 'RuleImportModal must specify size="md"')
  assert.doesNotMatch(content, /<style\b/, 'RuleImportModal must not have private <style> blocks')
  assert.doesNotMatch(content, /class="[^"]*\bmodal-backdrop\b[^"]*"/, 'RuleImportModal must not use legacy .modal-backdrop')
  assert.doesNotMatch(content, /class="[^"]*\bmodal\b[^"]*"/, 'RuleImportModal must not use legacy .modal')

  // Retain parsing & apply capabilities
  assert.match(content, /parseInput/, 'Must retain parseInput function')
  assert.match(content, /applySelected/, 'Must retain applySelected function')
  assert.match(content, /selectAll/, 'Must retain selectAll function')
  assert.match(content, /deselectAll/, 'Must retain deselectAll function')
  assert.match(content, /placeholders/, 'Must retain format placeholders')
})

test('TDD 3.2: RulePresetsModal is migrated to AppModal size="lg" with zero legacy CSS classes or private styles', () => {
  const filePath = path.resolve(srcDir, 'components/RulePresetsModal.vue')
  assert.ok(fs.existsSync(filePath), 'RulePresetsModal.vue must exist')
  const content = fs.readFileSync(filePath, 'utf8')

  // Must use AppModal size="lg"
  assert.match(content, /<AppModal\b/, 'RulePresetsModal must use <AppModal>')
  assert.match(content, /size="lg"/, 'RulePresetsModal must specify size="lg"')
  assert.doesNotMatch(content, /<style\b/, 'RulePresetsModal must not have private <style> blocks')
  assert.doesNotMatch(content, /class="[^"]*\bmodal-backdrop\b[^"]*"/, 'RulePresetsModal must not use legacy .modal-backdrop')
  assert.doesNotMatch(content, /class="[^"]*\bmodal\b[^"]*"/, 'RulePresetsModal must not use legacy .modal')

  // Retain search, proxyOverride, and apply capabilities
  assert.match(content, /rulePresetCategories/, 'Must retain rule preset categories')
  assert.match(content, /filteredCategories/, 'Must retain preset filtering')
  assert.match(content, /togglePreset/, 'Must retain togglePreset function')
  assert.match(content, /applySelected/, 'Must retain applySelected function')
})

test('TDD 3.3: NodeGroupModal is migrated to AppModal size="lg" and regex preview is an in-modal panel without raw fixed overlay', () => {
  const filePath = path.resolve(srcDir, 'components/NodeGroupModal.vue')
  assert.ok(fs.existsSync(filePath), 'NodeGroupModal.vue must exist')
  const content = fs.readFileSync(filePath, 'utf8')

  // Must use AppModal size="lg"
  assert.match(content, /<AppModal\b/, 'NodeGroupModal must use <AppModal>')
  assert.match(content, /size="lg"/, 'NodeGroupModal must specify size="lg"')
  assert.doesNotMatch(content, /class="[^"]*\bmodal-backdrop\b[^"]*"/, 'NodeGroupModal must not use legacy .modal-backdrop')
  assert.doesNotMatch(content, /class="[^"]*\bmodal\b[^"]*"/, 'NodeGroupModal must not use legacy .modal')

  // Nested regex preview must NOT use raw fixed overlay
  assert.doesNotMatch(content, /class="[^"]*\bregex-preview-layer\b[^"]*"/, 'NodeGroupModal must eliminate raw fixed .regex-preview-layer')
  assert.doesNotMatch(content, /<div[^>]*\bfixed\s+inset-0\b[^>]*role="dialog"/, 'NodeGroupModal must not create raw unmanaged fixed dialogs')

  // Retain group editing, regex and probe filtering
  assert.match(content, /createNodeGroup|updateNodeGroup/, 'Must retain group API interactions')
  assert.match(content, /availablePlatforms/, 'Must retain probe platform list')
  assert.match(content, /validateRegex/, 'Must retain regex validation')
  assert.match(content, /detectReferenceCycle/, 'Must retain reference cycle detection')
  assert.match(content, /normalizeEntries/, 'Must retain entry normalization')
})

test('TDD 3.3: SubscriptionForm manual node edit is migrated to AppModal size="md"', () => {
  const filePath = path.resolve(srcDir, 'components/SubscriptionForm.vue')
  assert.ok(fs.existsSync(filePath), 'SubscriptionForm.vue must exist')
  const content = fs.readFileSync(filePath, 'utf8')

  // Manual node edit must use AppModal size="md"
  assert.match(content, /<AppModal\b[^>]*\bsize="md"/, 'SubscriptionForm manual node edit must use <AppModal size="md">')
  assert.doesNotMatch(content, /class="[^"]*\bmanual-node-edit-backdrop\b[^"]*"/, 'SubscriptionForm must not use manual-node-edit-backdrop')
  assert.doesNotMatch(content, /<div[^>]*class="[^"]*\bmodal-backdrop\b[^"]*"/, 'SubscriptionForm must not use legacy .modal-backdrop')

  // Retain manual node editing capabilities
  assert.match(content, /parseManualNodeYaml/, 'Must retain manual node YAML parser')
  assert.match(content, /serializeManualNode/, 'Must retain manual node YAML serializer')
  assert.match(content, /saveManualNodeEdit/, 'Must retain saveManualNodeEdit function')
})

test('TDD 3.4: Zero legacy .modal and .modal-backdrop references across all components and views', () => {
  const filesToCheck = [
    'components/QuickExportModal.vue',
    'components/RuleImportModal.vue',
    'components/RulePresetsModal.vue',
    'components/NodeGroupModal.vue',
    'components/SubscriptionForm.vue',
    'components/ManualNodeEditor.vue',
    'views/Subscriptions.vue',
    'views/NodeGroups.vue',
    'views/Rules.vue',
    'views/RuleCategoryDetail.vue',
    'views/ProxyChains.vue',
    'views/Settings.vue',
    'views/NodeLedger.vue'
  ]

  for (const rel of filesToCheck) {
    const full = path.resolve(srcDir, rel)
    if (!fs.existsSync(full)) continue
    const txt = fs.readFileSync(full, 'utf8')
    assert.doesNotMatch(txt, /class="[^"]*\bmodal-backdrop\b[^"]*"/, `${rel} must not contain .modal-backdrop`)
    assert.doesNotMatch(txt, /class="[^"]*\bmodal\b[^"]*"/, `${rel} must not contain .modal class`)
    assert.doesNotMatch(txt, /<style[^>]*\bscoped\b/, `${rel} must not contain <style scoped>`)
  }
})
