import assert from 'node:assert/strict'
import test from 'node:test'
import fs from 'node:fs'
import path from 'node:path'

test('Tailwind theme and UI primitives exist and conform to design tokens', () => {
  const themePath = path.resolve(import.meta.dirname, '../src/assets/theme.css')
  assert.ok(fs.existsSync(themePath), 'theme.css should exist')
  const themeContent = fs.readFileSync(themePath, 'utf8')
  assert.match(themeContent, /--color-canvas: #(?:08090a|090D16)/i)
  assert.match(themeContent, /--font-mono: 'JetBrains Mono'/)

  const drawerPath = path.resolve(import.meta.dirname, '../src/components/ui/BaseDrawer.vue')
  assert.ok(fs.existsSync(drawerPath), 'BaseDrawer.vue should exist')

  const badgePath = path.resolve(import.meta.dirname, '../src/components/ui/StatusBadge.vue')
  assert.ok(fs.existsSync(badgePath), 'StatusBadge.vue should exist')

  const metricCardPath = path.resolve(import.meta.dirname, '../src/components/ui/MetricCard.vue')
  assert.ok(fs.existsSync(metricCardPath), 'MetricCard.vue should exist')
  const metricCardContent = fs.readFileSync(metricCardPath, 'utf8')
  assert.match(metricCardContent, /label/)
  assert.match(metricCardContent, /status/)
  assert.match(metricCardContent, /font-mono/)

  const virtualTablePath = path.resolve(import.meta.dirname, '../src/components/ui/VirtualNodeTable.vue')
  assert.ok(fs.existsSync(virtualTablePath), 'VirtualNodeTable.vue should exist')
  const virtualTableContent = fs.readFileSync(virtualTablePath, 'utf8')
  assert.match(virtualTableContent, /role="table"/)
  assert.match(virtualTableContent, /role="row"/)
})

test('Phase 5.3 Button and IconButton primitives conform to design tokens and a11y', () => {
  const uiDir = path.resolve(import.meta.dirname, '../src/components/ui')

  // 1. Button.vue
  const btnPath = path.join(uiDir, 'Button.vue')
  assert.ok(fs.existsSync(btnPath), 'Button.vue should exist')
  const btnContent = fs.readFileSync(btnPath, 'utf8')

  // Variants: primary, secondary, danger, ghost
  assert.match(btnContent, /primary/)
  assert.match(btnContent, /secondary/)
  assert.match(btnContent, /danger/)
  assert.match(btnContent, /ghost/)

  // :active press scale 0.98
  assert.match(btnContent, /active:scale-\[0\.98\]/, 'Button must micro-scale on active press')

  // focus-visible ring
  assert.match(btnContent, /focus-visible:ring-2/, 'Button must provide uniform focus-visible ring')

  // Mobile 44px min touch target
  assert.match(btnContent, /min-h-\[44px\]/, 'Button must enforce 44px min height for mobile touch ergonomics')

  // Lucide Loader2 for loading state
  assert.match(btnContent, /Loader2/, 'Button must use Lucide Loader2 for loading state')
  assert.doesNotMatch(btnContent, /transition-all/, 'Button must not use transition-all')

  // 2. IconButton.vue
  const iconBtnPath = path.join(uiDir, 'IconButton.vue')
  assert.ok(fs.existsSync(iconBtnPath), 'IconButton.vue should exist')
  const iconBtnContent = fs.readFileSync(iconBtnPath, 'utf8')

  assert.match(iconBtnContent, /label: string/, 'IconButton must require label for a11y')
  assert.match(iconBtnContent, /aria-label="label"/, 'IconButton must bind aria-label')
  assert.match(iconBtnContent, /active:scale-\[0\.98\]/, 'IconButton must micro-scale on active press')
  assert.match(iconBtnContent, /min-h-\[44px\]\s+min-w-\[44px\]/, 'IconButton must enforce 44px min touch target')
  assert.doesNotMatch(iconBtnContent, /transition-all/, 'IconButton must not use transition-all')
})

test('Phase 5.3 Field, Select, and Combobox primitives provide accessible form controls', () => {
  const uiDir = path.resolve(import.meta.dirname, '../src/components/ui')

  // 1. Field.vue
  const fieldPath = path.join(uiDir, 'Field.vue')
  assert.ok(fs.existsSync(fieldPath), 'Field.vue should exist')
  const fieldContent = fs.readFileSync(fieldPath, 'utf8')

  assert.match(fieldContent, /label/, 'Field must support label prop')
  assert.match(fieldContent, /error/, 'Field must support error prop')
  assert.match(fieldContent, /hint/, 'Field must support hint prop')
  assert.match(fieldContent, /font-mono tabular-nums/, 'Field must support monospace tabular-nums')
  assert.match(fieldContent, /role="alert"/, 'Field error must have role=alert')
  assert.match(fieldContent, /min-h-\[44px\]/, 'Field must enforce 44px min touch height on mobile')

  // 2. Select.vue
  const selectPath = path.join(uiDir, 'Select.vue')
  assert.ok(fs.existsSync(selectPath), 'Select.vue should exist')
  const selectContent = fs.readFileSync(selectPath, 'utf8')

  assert.match(selectContent, /ChevronDown/, 'Select must use Lucide ChevronDown icon')
  assert.match(selectContent, /min-h-\[44px\]/, 'Select must enforce 44px min touch target')
  assert.match(selectContent, /font-mono tabular-nums/, 'Select must support mono numbers')

  // 3. Combobox.vue
  const comboboxPath = path.join(uiDir, 'Combobox.vue')
  assert.ok(fs.existsSync(comboboxPath), 'Combobox.vue should exist')
  const comboboxContent = fs.readFileSync(comboboxPath, 'utf8')

  assert.match(comboboxContent, /role="combobox"/, 'Combobox must specify role=combobox')
  assert.match(comboboxContent, /aria-expanded=/, 'Combobox must manage aria-expanded state')
  assert.match(comboboxContent, /role="listbox"/, 'Combobox popup must specify role=listbox')
  assert.match(comboboxContent, /role="option"/, 'Combobox options must specify role=option')
  assert.match(comboboxContent, /Search/, 'Combobox must use Lucide Search')
  assert.match(comboboxContent, /Check/, 'Combobox must use Lucide Check')
  assert.match(comboboxContent, /Escape/, 'Combobox must support Escape key closing')
  assert.match(comboboxContent, /ArrowDown/, 'Combobox must support ArrowDown navigation')
  assert.match(comboboxContent, /min-h-\[44px\]/, 'Combobox trigger must enforce 44px touch target')
})

test('Phase 5.3 StatusBadge and MetricCard provide compact industrial indicators without emoji or glassmorphism', () => {
  const uiDir = path.resolve(import.meta.dirname, '../src/components/ui')

  // 1. StatusBadge.vue
  const badgePath = path.join(uiDir, 'StatusBadge.vue')
  assert.ok(fs.existsSync(badgePath), 'StatusBadge.vue should exist')
  const badgeContent = fs.readFileSync(badgePath, 'utf8')

  assert.match(badgeContent, /emerald/, 'StatusBadge must support emerald success')
  assert.match(badgeContent, /amber/, 'StatusBadge must support amber warning')
  assert.match(badgeContent, /rose/, 'StatusBadge must support rose danger')
  assert.match(badgeContent, /cyan/, 'StatusBadge must support cyan info')
  assert.match(badgeContent, /slate/, 'StatusBadge must support slate neutral')
  assert.match(badgeContent, /h-1\.5 w-1\.5 rounded-full/, 'StatusBadge must have dot indicator')

  // 2. MetricCard.vue
  const metricPath = path.join(uiDir, 'MetricCard.vue')
  assert.ok(fs.existsSync(metricPath), 'MetricCard.vue should exist')
  const metricContent = fs.readFileSync(metricPath, 'utf8')

  assert.match(metricContent, /font-mono tabular-nums/, 'MetricCard must enforce monospace tabular figures')
  assert.doesNotMatch(metricContent, /backdrop-blur/, 'MetricCard must not contain glassmorphism blur')
  assert.doesNotMatch(metricContent, /rounded-xl/, 'MetricCard must not contain excessive radius')
  assert.doesNotMatch(metricContent, /transition-all/, 'MetricCard must not contain transition-all')
  assert.doesNotMatch(metricContent, /[📡⚡👑🎬📁]/, 'MetricCard must not contain emoji icons')
  assert.match(metricContent, /active/, 'MetricCard must support active filter state')
  assert.match(metricContent, /@keydown\.enter/, 'MetricCard must support enter key navigation when clickable')
})

test('Phase 5.3 BaseDrawer satisfies full A11y lifecycle, Escape closing, and mobile bottom sheet', () => {
  const drawerPath = path.resolve(import.meta.dirname, '../src/components/ui/BaseDrawer.vue')
  assert.ok(fs.existsSync(drawerPath), 'BaseDrawer.vue should exist')
  const drawerContent = fs.readFileSync(drawerPath, 'utf8')

  // A11y roles and attributes
  assert.match(drawerContent, /role="dialog"/, 'BaseDrawer must have role=dialog')
  assert.match(drawerContent, /aria-modal="true"/, 'BaseDrawer must have aria-modal=true')
  assert.match(drawerContent, /:aria-labelledby="titleId"/, 'BaseDrawer must have accessible label binding')

  // Composable integration (focus trap, initial focus, escape, scroll lock)
  assert.match(drawerContent, /useModalA11y/, 'BaseDrawer must use useModalA11y for accessible lifecycle')
  assert.match(drawerContent, /X/, 'BaseDrawer must use Lucide X component for close button')
  assert.match(drawerContent, /min-h-\[44px\]\s+min-w-\[44px\]/, 'BaseDrawer close button must have 44px min touch size')

  // Visual gate compliance - only governed 4px overlay blur allowed
  assert.doesNotMatch(drawerContent, /backdrop-blur-(?!\[4px\])/, 'BaseDrawer must not contain arbitrary glassmorphism backdrop blur')
  assert.doesNotMatch(drawerContent, /<style scoped>/, 'BaseDrawer must not contain scoped CSS')

  // Responsive mobile sheet specs
  assert.match(drawerContent, /pb-safe/, 'BaseDrawer must include pb-safe for mobile home bar')
  assert.match(drawerContent, /rounded-t-(?:2xl|lg)/, 'BaseDrawer must have rounded top corners on mobile')
  assert.match(drawerContent, /sm:hidden.*rounded-full/, 'BaseDrawer must have handle pill indicator on mobile')
})

test('Phase 5.3 ConfirmDialog and ToastContainer satisfy a11y, focus management, and non-blocking notifications', () => {
  const componentsDir = path.resolve(import.meta.dirname, '../src/components')

  // 1. ConfirmDialog.vue
  const confirmPath = path.join(componentsDir, 'ConfirmDialog.vue')
  assert.ok(fs.existsSync(confirmPath), 'ConfirmDialog.vue should exist')
  const confirmContent = fs.readFileSync(confirmPath, 'utf8')

  assert.match(confirmContent, /role="alertdialog"/, 'ConfirmDialog must have role=alertdialog')
  assert.match(confirmContent, /aria-modal="true"/, 'ConfirmDialog must have aria-modal=true')
  assert.match(confirmContent, /useModalA11y/, 'ConfirmDialog must integrate useModalA11y')
  assert.match(confirmContent, /AlertTriangle/, 'ConfirmDialog must use Lucide AlertTriangle icon')
  assert.match(confirmContent, /min-h-\[44px\]/, 'ConfirmDialog buttons must enforce 44px min touch height')
  assert.doesNotMatch(confirmContent, /backdrop-filter/, 'ConfirmDialog must not contain glassmorphism')
  assert.doesNotMatch(confirmContent, /linear-gradient/, 'ConfirmDialog must not contain linear-gradient')
  assert.doesNotMatch(confirmContent, /<style scoped>/, 'ConfirmDialog must not contain scoped CSS')

  // 2. ToastContainer.vue
  const toastPath = path.join(componentsDir, 'ToastContainer.vue')
  assert.ok(fs.existsSync(toastPath), 'ToastContainer.vue should exist')
  const toastContent = fs.readFileSync(toastPath, 'utf8')

  assert.match(toastContent, /aria-live="polite"/, 'ToastContainer must have aria-live=polite')
  assert.match(toastContent, /role="status"/, 'Toast item must have role=status')
  assert.match(toastContent, /CheckCircle2/, 'ToastContainer must use Lucide CheckCircle2')
  assert.match(toastContent, /AlertCircle/, 'ToastContainer must use Lucide AlertCircle')
  assert.match(toastContent, /AlertTriangle/, 'ToastContainer must use Lucide AlertTriangle')
  assert.match(toastContent, /X/, 'ToastContainer must use Lucide X icon')
  assert.doesNotMatch(toastContent, /'✓'|'✕'|'⚠'|'ℹ'/, 'ToastContainer must not contain crude text glyphs')
  assert.doesNotMatch(toastContent, /backdrop-filter/, 'ToastContainer must not contain glassmorphism')
  assert.doesNotMatch(toastContent, /transition-all/, 'ToastContainer must not contain transition-all')
  assert.doesNotMatch(toastContent, /<style scoped>/, 'ToastContainer must not contain scoped CSS')
})

test('Phase 5.3 VirtualNodeTable supports keyboard navigation, focus-visible, and zero glassmorphism', () => {
  const tablePath = path.resolve(import.meta.dirname, '../src/components/ui/VirtualNodeTable.vue')
  assert.ok(fs.existsSync(tablePath), 'VirtualNodeTable.vue should exist')
  const tableContent = fs.readFileSync(tablePath, 'utf8')

  assert.match(tableContent, /role="table"/, 'VirtualNodeTable must have role=table')
  assert.match(tableContent, /role="row"/, 'VirtualNodeTable must have role=row')
  assert.match(tableContent, /handleKeyDown/, 'VirtualNodeTable must handle keyboard events')
  assert.match(tableContent, /ArrowDown/, 'VirtualNodeTable must navigate with ArrowDown')
  assert.match(tableContent, /ArrowUp/, 'VirtualNodeTable must navigate with ArrowUp')
  assert.match(tableContent, /PageDown/, 'VirtualNodeTable must support PageDown')
  assert.match(tableContent, /PageUp/, 'VirtualNodeTable must support PageUp')
  assert.match(tableContent, /focus-visible:ring-2/, 'VirtualNodeTable must have focus-visible ring')
  assert.doesNotMatch(tableContent, /backdrop-blur/, 'VirtualNodeTable must not have glassmorphism')
  assert.doesNotMatch(tableContent, /rounded-xl/, 'VirtualNodeTable must not have rounded-xl')
})

test('Phase 5.3 UI Primitives barrel export index.ts exports all components and types', () => {
  const indexPath = path.resolve(import.meta.dirname, '../src/components/ui/index.ts')
  assert.ok(fs.existsSync(indexPath), 'index.ts should exist')
  const indexContent = fs.readFileSync(indexPath, 'utf8')

  assert.match(indexContent, /Button/, 'index.ts must export Button')
  assert.match(indexContent, /IconButton/, 'index.ts must export IconButton')
  assert.match(indexContent, /Field/, 'index.ts must export Field')
  assert.match(indexContent, /Select/, 'index.ts must export Select')
  assert.match(indexContent, /Combobox/, 'index.ts must export Combobox')
  assert.match(indexContent, /StatusBadge/, 'index.ts must export StatusBadge')
  assert.match(indexContent, /MetricCard/, 'index.ts must export MetricCard')
  assert.match(indexContent, /BaseDrawer/, 'index.ts must export BaseDrawer')
  assert.match(indexContent, /ConfirmDialog/, 'index.ts must export ConfirmDialog')
  assert.match(indexContent, /ToastContainer/, 'index.ts must export ToastContainer')
  assert.match(indexContent, /VirtualNodeTable/, 'index.ts must export VirtualNodeTable')
  assert.match(indexContent, /types/, 'index.ts must export types')
})
