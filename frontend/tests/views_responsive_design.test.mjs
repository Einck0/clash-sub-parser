import assert from 'node:assert/strict'
import test from 'node:test'
import fs from 'node:fs'
import path from 'node:path'

test('Key views (Subscriptions, NodeGroups, Rules, Generate) use MetricCard and responsive design', () => {
  const viewsDir = path.resolve(import.meta.dirname, '../src/views')

  // 1. Subscriptions.vue
  const subsPath = path.join(viewsDir, 'Subscriptions.vue')
  assert.ok(fs.existsSync(subsPath), 'Subscriptions.vue should exist')
  const subsContent = fs.readFileSync(subsPath, 'utf8')
  assert.match(subsContent, /MetricCard/, 'Subscriptions.vue should use MetricCard')
  assert.match(subsContent, /grid-cols-1\s+sm:grid-cols-2\s+md:grid-cols-3/, 'Subscriptions.vue should have responsive metric grid')
  assert.match(subsContent, /min-h-\[44px\]/, 'Subscriptions.vue should enforce 44px min touch height')
  assert.match(subsContent, /pb-safe/, 'Subscriptions.vue modals should adapt to bottom sheet with pb-safe')
  assert.doesNotMatch(subsContent, /<span class="stat-icon">[📡⚡👑]<\/span>/, 'Subscriptions.vue should not contain crude emoji stat icons')

  // 2. NodeGroups.vue
  const groupsPath = path.join(viewsDir, 'NodeGroups.vue')
  assert.ok(fs.existsSync(groupsPath), 'NodeGroups.vue should exist')
  const groupsContent = fs.readFileSync(groupsPath, 'utf8')
  assert.match(groupsContent, /MetricCard/, 'NodeGroups.vue should use MetricCard')
  assert.match(groupsContent, /grid-cols-1\s+sm:grid-cols-2\s+md:grid-cols-3/, 'NodeGroups.vue should have responsive metric grid')
  assert.match(groupsContent, /min-h-\[44px\]/, 'NodeGroups.vue should enforce 44px min touch height')
  assert.match(groupsContent, /pb-safe/, 'NodeGroups.vue preview modal should adapt to bottom sheet with pb-safe')
  assert.doesNotMatch(groupsContent, /<style scoped>/, 'NodeGroups.vue should not contain legacy scoped CSS')

  // 3. Rules.vue
  const rulesPath = path.join(viewsDir, 'Rules.vue')
  assert.ok(fs.existsSync(rulesPath), 'Rules.vue should exist')
  const rulesContent = fs.readFileSync(rulesPath, 'utf8')
  assert.match(rulesContent, /MetricCard/, 'Rules.vue should use MetricCard')
  assert.match(rulesContent, /grid-cols-1\s+sm:grid-cols-2\s+md:grid-cols-3/, 'Rules.vue should have responsive metric grid')
  assert.match(rulesContent, /min-h-\[44px\]/, 'Rules.vue should enforce 44px min touch height')

  // 4. Generate.vue
  const genPath = path.join(viewsDir, 'Generate.vue')
  assert.ok(fs.existsSync(genPath), 'Generate.vue should exist')
  const genContent = fs.readFileSync(genPath, 'utf8')
  assert.match(genContent, /MetricCard/, 'Generate.vue should use MetricCard')
  assert.match(genContent, /grid-cols-2\s+sm:grid-cols-4/, 'Generate.vue should have responsive metric grid')
  assert.match(genContent, /min-h-\[44px\]/, 'Generate.vue should enforce 44px min touch height')
})

test('BaseDrawer adapts to Bottom Sheet on mobile with pb-safe and QuickExportModal uses pb-safe', () => {
  const drawerPath = path.resolve(import.meta.dirname, '../src/components/ui/BaseDrawer.vue')
  assert.ok(fs.existsSync(drawerPath), 'BaseDrawer.vue should exist')
  const drawerContent = fs.readFileSync(drawerPath, 'utf8')
  assert.match(drawerContent, /pb-safe/, 'BaseDrawer should include pb-safe')
  assert.match(drawerContent, /rounded-t-(?:2xl|lg|\[16px\])/, 'BaseDrawer should have rounded top corners on mobile')
  assert.match(drawerContent, /sm:hidden.*rounded-full/, 'BaseDrawer should have drag indicator on mobile')

  const exportPath = path.resolve(import.meta.dirname, '../src/components/QuickExportModal.vue')
  assert.ok(fs.existsSync(exportPath), 'QuickExportModal.vue should exist')
  const exportContent = fs.readFileSync(exportPath, 'utf8')
  assert.match(exportContent, /pb-safe/, 'QuickExportModal should include pb-safe')
  assert.match(exportContent, /<AppModal\b/, 'QuickExportModal should use AppModal')
})

test('PageToolbar provides modern responsive input with min-height touch target', () => {
  const toolbarPath = path.resolve(import.meta.dirname, '../src/components/PageToolbar.vue')
  assert.ok(fs.existsSync(toolbarPath), 'PageToolbar.vue should exist')
  const toolbarContent = fs.readFileSync(toolbarPath, 'utf8')
  assert.match(toolbarContent, /min-h-\[44px\]/, 'PageToolbar should enforce 44px min touch height')
  assert.match(toolbarContent, /border-white\/10/, 'PageToolbar should conform to design border token')
  assert.match(toolbarContent, /bg-slate-900\/70/, 'PageToolbar should conform to surface design token')
})
