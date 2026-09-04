import assert from 'node:assert/strict'
import test from 'node:test'
import fs from 'node:fs'
import path from 'node:path'

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
