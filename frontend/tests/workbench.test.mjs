import assert from 'node:assert/strict'
import test from 'node:test'
import fs from 'node:fs'
import path from 'node:path'
import { scanContent } from '../scripts/visual_audit.mjs'

test('Workbench Header and Sidebar components exist and have standard props/events', () => {
  const headerPath = path.resolve(import.meta.dirname, '../src/components/workbench/WorkbenchHeader.vue')
  assert.ok(fs.existsSync(headerPath), 'WorkbenchHeader.vue should exist')
  const headerContent = fs.readFileSync(headerPath, 'utf8')
  assert.match(headerContent, /CSP \/\/ WORKBENCH/)
  assert.match(headerContent, /open-export/)
  assert.match(headerContent, /toggle-sidebar/)

  const sidebarPath = path.resolve(import.meta.dirname, '../src/components/workbench/WorkbenchSidebar.vue')
  assert.ok(fs.existsSync(sidebarPath), 'WorkbenchSidebar.vue should exist')
  const sidebarContent = fs.readFileSync(sidebarPath, 'utf8')
  assert.match(sidebarContent, /\/nodes/)
  assert.match(sidebarContent, /\/subscriptions/)
  assert.match(sidebarContent, /\/groups/)
  assert.match(sidebarContent, /\/node-groups/)
  assert.match(sidebarContent, /\/rules/)
  assert.match(sidebarContent, /\/proxy-chains/)
  assert.match(sidebarContent, /\/dns/)
  assert.match(sidebarContent, /\/generate/)
  assert.match(sidebarContent, /\/settings/)
  assert.match(sidebarContent, /\/history/)
})

test('Workbench router config covers all 9 canonical routes and aliases', () => {
  const routerPath = path.resolve(import.meta.dirname, '../src/router/index.ts')
  assert.ok(fs.existsSync(routerPath), 'router/index.ts should exist')
  const routerContent = fs.readFileSync(routerPath, 'utf8')

  // 9 Canonical Routes
  assert.match(routerContent, /path:\s*'\/nodes'/)
  assert.match(routerContent, /path:\s*'\/subscriptions'/)
  assert.match(routerContent, /path:\s*'\/node-groups'/)
  assert.match(routerContent, /path:\s*'\/rules'/)
  assert.match(routerContent, /path:\s*'\/proxy-chains'/)
  assert.match(routerContent, /path:\s*'\/dns'/)
  assert.match(routerContent, /path:\s*'\/generate'/)
  assert.match(routerContent, /path:\s*'\/settings'/)
  assert.match(routerContent, /path:\s*'\/history'/)

  // Aliases and root redirect
  assert.match(routerContent, /alias:\s*'\/groups'/)
  assert.match(routerContent, /alias:\s*'\/chains'/)
  assert.match(routerContent, /redirect:\s*'\/nodes'/)
})

test('AppShell layout integrates WorkbenchHeader, WorkbenchSidebar, and BaseDrawer responsive drawer', () => {
  const appPath = path.resolve(import.meta.dirname, '../src/App.vue')
  assert.ok(fs.existsSync(appPath), 'App.vue should exist')
  const appContent = fs.readFileSync(appPath, 'utf8')

  // Components imported & integrated
  assert.match(appContent, /WorkbenchHeader/)
  assert.match(appContent, /WorkbenchSidebar/)
  assert.match(appContent, /BaseDrawer/)
  assert.match(appContent, /QuickExportModal/)

  // Responsive drawer bindings
  assert.match(appContent, /mobileSidebarOpen/)
  assert.match(appContent, /@toggle-sidebar/)
  assert.match(appContent, /placement="left"/)
})

test('Five-target QuickExportModal contract', () => {
  const exportPath = path.resolve(import.meta.dirname, '../src/components/QuickExportModal.vue')
  assert.ok(fs.existsSync(exportPath), 'QuickExportModal.vue should exist')
  const exportContent = fs.readFileSync(exportPath, 'utf8')

  // 5 Targets supported
  assert.match(exportContent, /'clash'/)
  assert.match(exportContent, /'mihomo'/)
  assert.match(exportContent, /'stash'/)
  assert.match(exportContent, /'shadowrocket'/)
  assert.match(exportContent, /'sing-box'/)

  // Dual modes: merged vs subscription
  assert.match(exportContent, /'merged'/)
  assert.match(exportContent, /'subscription'/)

  // Client wakeup schemes
  assert.match(exportContent, /clash:\/\//)
  assert.match(exportContent, /stash:\/\//)
  assert.match(exportContent, /sub:\/\//)
  assert.match(exportContent, /sing-box:\/\//)

  // QR Code & Copy
  assert.match(exportContent, /QrCode/)
  assert.match(exportContent, /navigator\.clipboard\.writeText/)
  assert.match(exportContent, /已复制！/)

  // Zero occurrences of legacy script
  assert.doesNotMatch(exportContent, /Script\.js/i)
  assert.doesNotMatch(exportContent, /generateScript/i)
  assert.doesNotMatch(exportContent, /直接打开 Script/i)
})

test('BaseDrawer supports left and right placement for responsive AppShell', () => {
  const drawerPath = path.resolve(import.meta.dirname, '../src/components/ui/BaseDrawer.vue')
  assert.ok(fs.existsSync(drawerPath), 'BaseDrawer.vue should exist')
  const drawerContent = fs.readFileSync(drawerPath, 'utf8')

  assert.match(drawerContent, /placement\?: 'left' \| 'right'/)
  assert.match(drawerContent, /drawer-slide-left/)
  assert.match(drawerContent, /drawer-slide-right/)
})

test('Phase 5.2 Design Tokens: theme.css defines dark default & light tokens, tabular-nums, focus-ring, and reduced-motion', () => {
  const themePath = path.resolve(import.meta.dirname, '../src/assets/theme.css')
  assert.ok(fs.existsSync(themePath), 'theme.css should exist')
  const content = fs.readFileSync(themePath, 'utf8')

  // Dark & light semantic tokens
  assert.match(content, /--color-canvas: #(?:08090a|090D16)/i)
  assert.match(content, /--color-surface-base: #(?:121417|0F172A)/i)
  assert.match(content, /--color-accent: #(?:6366f1|3B82F6)/i)
  assert.match(content, /--color-status-success: #10B981/)
  assert.match(content, /--color-status-warning: #F59E0B/)
  assert.match(content, /--color-status-danger: #EF4444/)
  assert.match(content, /--color-status-info: #06B6D4/)
  assert.match(content, /\[data-theme="light"\]/)
  assert.match(content, /--color-canvas: #F8FAFC/)
  assert.match(content, /--color-surface-base: #FFFFFF/)

  // Radius 8px/12px tokens
  assert.match(content, /--radius-md: 8px/)
  assert.match(content, /--radius-lg: 12px/)

  // Tabular numbers & focus-ring
  assert.match(content, /\.tabular-nums/)
  assert.match(content, /font-variant-numeric:\s*tabular-nums/)
  assert.match(content, /\.focus-ring/)

  // 36px table density
  assert.match(content, /36px/)

  // prefers-reduced-motion global rule
  assert.match(content, /@media\s*\(prefers-reduced-motion:\s*reduce\)/)
})

test('Phase 5.2 AppShell: App.vue has Skip Link, semantic tokens, zero scoped styles, and zero visual gate violations', () => {
  const appPath = path.resolve(import.meta.dirname, '../src/App.vue')
  assert.ok(fs.existsSync(appPath), 'App.vue should exist')
  const content = fs.readFileSync(appPath, 'utf8')

  // Fullscreen Skip Link
  assert.match(content, /href="#main-content"/)
  assert.match(content, /跳转至主工作区/)
  assert.match(content, /id="main-content"/)

  // Semantic canvas token (no hardcoded hex)
  assert.match(content, /bg-canvas/)
  assert.match(content, /text-text-main/)
  assert.doesNotMatch(content, /bg-\[#090D16\]/)
  assert.doesNotMatch(content, /text-\[#F8FAFC\]/)

  // Zero scoped styles
  assert.doesNotMatch(content, /<style scoped>/)

  // Visual Gate audit verification: 0 violations
  const { violations } = scanContent(content, 'src/App.vue')
  assert.equal(violations.length, 0, `App.vue must have 0 visual gate violations, found: ${JSON.stringify(violations)}`)
})

test('Phase 5.2 WorkbenchHeader: 48px compact height, stats, theme toggle, and zero visual gate violations', () => {
  const headerPath = path.resolve(import.meta.dirname, '../src/components/workbench/WorkbenchHeader.vue')
  assert.ok(fs.existsSync(headerPath), 'WorkbenchHeader.vue should exist')
  const content = fs.readFileSync(headerPath, 'utf8')

  // 48px compact topbar height
  assert.match(content, /\bh-12\b/)
  assert.doesNotMatch(content, /\bh-14\b/)

  // Solid surface background & hairline border (no glassmorphism)
  assert.match(content, /bg-surface-base/)
  assert.match(content, /border-border-subtle/)
  assert.doesNotMatch(content, /backdrop-blur/)

  // Stats with tabular figures
  assert.match(content, /GATEWAY:/)
  assert.match(content, /ONLINE/)
  assert.match(content, /NODES:/)
  assert.match(content, /PROBED:/)
  assert.match(content, /tabular-nums/)

  // Theme toggle
  assert.match(content, /useTheme/)

  // Visual Gate audit verification: 0 violations
  const { violations } = scanContent(content, 'src/components/workbench/WorkbenchHeader.vue')
  assert.equal(violations.length, 0, `WorkbenchHeader.vue must have 0 visual gate violations, found: ${JSON.stringify(violations)}`)
})

test('Phase 5.2 WorkbenchSidebar: 1px hairline border, clear categories, semantic active state, and zero visual gate violations', () => {
  const sidebarPath = path.resolve(import.meta.dirname, '../src/components/workbench/WorkbenchSidebar.vue')
  assert.ok(fs.existsSync(sidebarPath), 'WorkbenchSidebar.vue should exist')
  const content = fs.readFileSync(sidebarPath, 'utf8')

  // 1px hairline border
  assert.match(content, /border-r border-border-subtle/)
  assert.match(content, /bg-surface-base/)

  // Clear categorization
  assert.match(content, /Control Center/i)
  assert.match(content, /Distribution/i)
  assert.match(content, /System/i)

  // Semantic active accent state
  assert.match(content, /bg-accent-subtle/)
  assert.match(content, /text-accent/)

  // No excessive transitions or roundings
  assert.doesNotMatch(content, /transition-all/)
  assert.doesNotMatch(content, /bg-\[#090D16\]/)

  // Visual Gate audit verification: 0 violations
  const { violations } = scanContent(content, 'src/components/workbench/WorkbenchSidebar.vue')
  assert.equal(violations.length, 0, `WorkbenchSidebar.vue must have 0 visual gate violations, found: ${JSON.stringify(violations)}`)
})

test('Phase 5.2 Card Soup Eradication: style.css has zero gradients, zero backdrop-filter, and zero visual gate violations', () => {
  const stylePath = path.resolve(import.meta.dirname, '../src/style.css')
  assert.ok(fs.existsSync(stylePath), 'style.css should exist')
  const content = fs.readFileSync(stylePath, 'utf8')

  // Zero radial or linear gradients
  assert.doesNotMatch(content, /radial-gradient/)
  assert.doesNotMatch(content, /linear-gradient/)

  // Zero backdrop-filter
  assert.doesNotMatch(content, /backdrop-filter/)

  // Visual Gate audit verification: 0 violations
  const { violations } = scanContent(content, 'src/style.css')
  assert.equal(violations.length, 0, `style.css must have 0 visual gate violations, found: ${JSON.stringify(violations)}`)
})

