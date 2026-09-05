import assert from 'node:assert/strict'
import test from 'node:test'
import fs from 'node:fs'
import path from 'node:path'

test('Workbench Shell & global styles enforce zero document horizontal overflow and >=768 sidebar', () => {
  const appPath = path.resolve(import.meta.dirname, '../src/App.vue')
  assert.ok(fs.existsSync(appPath), 'App.vue must exist')
  const appContent = fs.readFileSync(appPath, 'utf8')

  assert.match(appContent, /max-w-full\s+overflow-x-hidden.*min-w-0/, 'App shell must clamp horizontal overflow')
  assert.match(appContent, /hidden\s+md:flex/, 'Sidebar must be hidden on mobile (<768px) and flex on md+ (>=768px)')
  assert.match(appContent, /placement="left"/, 'Mobile navigation drawer must use left edge drawer placement')

  const themePath = path.resolve(import.meta.dirname, '../src/assets/theme.css')
  assert.ok(fs.existsSync(themePath), 'theme.css must exist')
  const themeContent = fs.readFileSync(themePath, 'utf8')
  assert.match(themeContent, /html,\s*body\s*\{[^}]*overflow-x:\s*hidden/, 'theme.css must prevent document-level horizontal scroll')

  const stylePath = path.resolve(import.meta.dirname, '../src/style.css')
  assert.ok(fs.existsSync(stylePath), 'style.css must exist')
  const styleContent = fs.readFileSync(stylePath, 'utf8')
  assert.match(styleContent, /html,\s*body\s*\{[^}]*overflow-x:\s*hidden/, 'style.css must prevent document-level horizontal scroll')
})

test('WorkbenchHeader and WorkbenchSidebar provide accessible >=44px mobile navigation targets', () => {
  const headerPath = path.resolve(import.meta.dirname, '../src/components/workbench/WorkbenchHeader.vue')
  assert.ok(fs.existsSync(headerPath), 'WorkbenchHeader.vue must exist')
  const headerContent = fs.readFileSync(headerPath, 'utf8')

  // Hamburger toggle target >= 44x44
  assert.match(headerContent, /min-h-\[44px\]\s+min-w-\[44px\]/, 'Header mobile hamburger button must have min 44x44px tap target')
  assert.match(headerContent, /aria-label="打开导航菜单"/, 'Header hamburger must have accessible aria-label')

  const sidebarPath = path.resolve(import.meta.dirname, '../src/components/workbench/WorkbenchSidebar.vue')
  assert.ok(fs.existsSync(sidebarPath), 'WorkbenchSidebar.vue must exist')
  const sidebarContent = fs.readFileSync(sidebarPath, 'utf8')

  assert.match(sidebarContent, /mobile\s*\?\s*'min-h-\[44px\]'/, 'Sidebar items must enforce 44px min height when rendered in mobile drawer')
})

test('BaseDrawer satisfies mobile Bottom Sheet, A11y contracts, and handle-only drag threshold', () => {
  const drawerPath = path.resolve(import.meta.dirname, '../src/components/ui/BaseDrawer.vue')
  assert.ok(fs.existsSync(drawerPath), 'BaseDrawer.vue must exist')
  const drawerContent = fs.readFileSync(drawerPath, 'utf8')

  // Left vs right placement: left stays edge drawer, right becomes bottom sheet <640px
  assert.match(drawerContent, /drawer-slide-left/, 'BaseDrawer must support left edge sliding')
  assert.match(drawerContent, /h-auto\s+max-h-\[88vh\]\s+sm:h-full/, 'BaseDrawer right placement must adapt to bottom sheet on mobile')
  assert.match(drawerContent, /rounded-t-lg\s+sm:rounded-none/, 'Bottom sheet on mobile must have rounded top corners')
  assert.match(drawerContent, /pb-safe/, 'Bottom sheet on mobile must respect bottom safe-area')

  // Close target >= 44x44
  assert.match(drawerContent, /min-h-\[44px\]\s+min-w-\[44px\]/, 'Close button must be >= 44x44px touch target')
  assert.match(drawerContent, /aria-label="关闭抽屉"/, 'Close button must have aria-label')

  // A11y lifecycle
  assert.match(drawerContent, /useModalA11y/, 'BaseDrawer must use modal accessibility composable')
  assert.match(drawerContent, /role="dialog"/, 'BaseDrawer must have dialog role')
  assert.match(drawerContent, /aria-modal="true"/, 'BaseDrawer must have aria-modal true')
  assert.match(drawerContent, /lockScroll:\s*true/, 'BaseDrawer must lock background scroll')
  assert.match(drawerContent, /closeOnEscape:\s*true/, 'BaseDrawer must close on Escape')
  assert.match(drawerContent, /trapFocus:\s*true/, 'BaseDrawer must trap focus')

  // Handle-only drag to dismiss with threshold and form exclusion
  assert.match(drawerContent, /onHandlePointerDown/, 'BaseDrawer must bind drag gesture to handle only')
  assert.match(drawerContent, /closest\('input,\s*textarea,\s*select,\s*button,\s*a'\)/, 'BaseDrawer drag gesture must exclude form controls and buttons')
  assert.match(drawerContent, /deltaY\s*>=\s*96\s*\|\|\s*velocity\s*>=\s*0\.5/, 'BaseDrawer must enforce 96px displacement or 0.5px/ms velocity threshold')
})

test('Management views (Subscriptions, NodeGroups, ProxyChains, Rules) retain mobile layout and >=44px touch targets', () => {
  const viewsDir = path.resolve(import.meta.dirname, '../src/views')

  // Subscriptions
  const subsContent = fs.readFileSync(path.join(viewsDir, 'Subscriptions.vue'), 'utf8')
  assert.match(subsContent, /max-w-full\s+overflow-x-hidden/, 'Subscriptions must clamp horizontal overflow')
  assert.match(subsContent, /min-h-\[44px\]/, 'Subscriptions buttons must meet 44px min touch target')

  // NodeGroups
  const groupsContent = fs.readFileSync(path.join(viewsDir, 'NodeGroups.vue'), 'utf8')
  assert.match(groupsContent, /max-w-full\s+overflow-x-hidden/, 'NodeGroups must clamp horizontal overflow')
  assert.match(groupsContent, /min-h-\[44px\]/, 'NodeGroups buttons must meet 44px min touch target')

  // ProxyChains
  const chainsContent = fs.readFileSync(path.join(viewsDir, 'ProxyChains.vue'), 'utf8')
  assert.match(chainsContent, /max-w-full\s+overflow-x-hidden/, 'ProxyChains must clamp horizontal overflow')
  assert.match(chainsContent, /min-h-\[44px\]/, 'ProxyChains controls must meet 44px min touch target')

  // Rules
  const rulesContent = fs.readFileSync(path.join(viewsDir, 'Rules.vue'), 'utf8')
  assert.match(rulesContent, /max-w-full\s+overflow-x-hidden/, 'Rules must clamp horizontal overflow')
  assert.match(rulesContent, /border-border-subtle/, 'Rules must use Workbench semantic border token')
  assert.match(rulesContent, /bg-surface/, 'Rules must use Workbench semantic surface tokens')
  assert.match(rulesContent, /text-text-main/, 'Rules must use Workbench semantic text token')
  assert.match(rulesContent, /text-accent/, 'Rules must use Workbench semantic accent token')
  assert.doesNotMatch(rulesContent, /bg-slate-|text-slate-|border-white\/|text-blue-|bg-blue-/, 'Rules must not use legacy raw colors')
})
