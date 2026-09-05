import assert from 'node:assert/strict'
import test from 'node:test'
import fs from 'node:fs'
import path from 'node:path'
import { scanContent } from '../scripts/visual_audit.mjs'

test('TDD 1.2: Design Token Governance — theme.css is sole token source and style.css contains zero legacy tokens or root definitions', () => {
  const themePath = path.resolve(import.meta.dirname, '../src/assets/theme.css')
  assert.ok(fs.existsSync(themePath), 'theme.css must exist as the sole token source')
  const themeContent = fs.readFileSync(themePath, 'utf8')

  // Modal scale tokens (448px, 640px, 960px)
  assert.match(themeContent, /--modal-sm:\s*448px/, 'theme.css must define --modal-sm: 448px')
  assert.match(themeContent, /--modal-md:\s*640px/, 'theme.css must define --modal-md: 640px')
  assert.match(themeContent, /--modal-lg:\s*960px/, 'theme.css must define --modal-lg: 960px')

  // Drawer width tokens (320px left, 480px right)
  assert.match(themeContent, /--drawer-nav:\s*320px|--drawer-left:\s*320px|--drawer-nav-width:\s*320px/, 'theme.css must define 320px navigation drawer width')
  assert.match(themeContent, /--drawer-detail:\s*480px|--drawer-right:\s*480px|--drawer-detail-width:\s*480px/, 'theme.css must define 480px detail drawer width')

  // Semantic spacing, radii, layers, and motion tokens
  assert.match(themeContent, /--radius-md:\s*8px/, 'theme.css must define 8px radius for modals/cards')
  assert.match(themeContent, /--radius-xl:\s*16px/, 'theme.css must define 16px radius for mobile bottom sheet')
  assert.match(themeContent, /--z-modal:\s*50|--z-dialog:\s*50/, 'theme.css must define modal z-index layer')
  assert.match(themeContent, /--z-drawer:\s*40|--z-drawer:\s*50/, 'theme.css must define drawer z-index layer')
  assert.match(themeContent, /--motion-fast|--motion-normal|--ease-standard/, 'theme.css must define semantic motion tokens')

  // style.css verification: must NOT contain :root, [data-theme] token roots, legacy aliases, or old .modal rules
  const stylePath = path.resolve(import.meta.dirname, '../src/style.css')
  assert.ok(fs.existsSync(stylePath), 'style.css must exist')
  const styleContent = fs.readFileSync(stylePath, 'utf8')

  // No :root or [data-theme] root token declarations
  assert.doesNotMatch(styleContent, /^:root\s*\{/m, 'style.css must NOT declare :root token definitions')
  assert.doesNotMatch(styleContent, /^\[data-theme="dark"\]\s*\{(?:\s*--bg-0|\s*--surface|\s*--brand)/m, 'style.css must NOT declare [data-theme] token definitions')

  // No legacy token aliases
  assert.doesNotMatch(styleContent, /var\(--brand\)/, 'style.css must NOT reference var(--brand)')
  assert.doesNotMatch(styleContent, /var\(--ink\)/, 'style.css must NOT reference var(--ink)')
  assert.doesNotMatch(styleContent, /var\(--ink-soft\)/, 'style.css must NOT reference var(--ink-soft)')
  assert.doesNotMatch(styleContent, /var\(--surface\)/, 'style.css must NOT reference var(--surface)')
  assert.doesNotMatch(styleContent, /var\(--surface-2\)/, 'style.css must NOT reference var(--surface-2)')
  assert.doesNotMatch(styleContent, /var\(--surface-3\)/, 'style.css must NOT reference var(--surface-3)')
  assert.doesNotMatch(styleContent, /var\(--bg-0\)/, 'style.css must NOT reference var(--bg-0)')
  assert.doesNotMatch(styleContent, /var\(--bg-1\)/, 'style.css must NOT reference var(--bg-1)')
  assert.doesNotMatch(styleContent, /var\(--border\)/, 'style.css must NOT reference var(--border)')
  assert.doesNotMatch(styleContent, /var\(--danger\)/, 'style.css must NOT reference var(--danger)')
  assert.doesNotMatch(styleContent, /var\(--ok\)/, 'style.css must NOT reference var(--ok)')

  // No legacy .modal or .modal-backdrop rules
  assert.doesNotMatch(styleContent, /^\.modal-backdrop\b/m, 'style.css must NOT define legacy .modal-backdrop')
  assert.doesNotMatch(styleContent, /^\.modal\b/m, 'style.css must NOT define legacy .modal')
})

test('TDD 2.2: AppModal Primitive — shared A11y lifecycle, strict sm|md|lg sizes, and 32px viewport gutter', () => {
  const modalPath = path.resolve(import.meta.dirname, '../src/components/ui/AppModal.vue')
  assert.ok(fs.existsSync(modalPath), 'AppModal.vue must exist under components/ui/')
  const modalContent = fs.readFileSync(modalPath, 'utf8')

  // Contract: title, modelValue, size (sm|md|lg) only, no width prop
  assert.match(modalContent, /title:\s*string/, 'AppModal must require title prop for a11y')
  assert.match(modalContent, /modelValue:\s*boolean/, 'AppModal must require modelValue boolean prop')
  assert.match(modalContent, /size\?:.*(?:'sm'|'md'|'lg')/, 'AppModal size prop must accept sm | md | lg')
  assert.doesNotMatch(modalContent, /width\?:/, 'AppModal must NOT expose a width prop')

  // A11y: role="dialog", aria-modal="true", :aria-labelledby, useModalA11y
  assert.match(modalContent, /role="dialog"/, 'AppModal must have role="dialog"')
  assert.match(modalContent, /aria-modal="true"/, 'AppModal must have aria-modal="true"')
  assert.match(modalContent, /:aria-labelledby=/, 'AppModal must bind aria-labelledby')
  assert.match(modalContent, /useModalA11y/, 'AppModal must reuse useModalA11y composable')
  assert.match(modalContent, /<teleport\s+to="body">/i, 'AppModal must teleport to body')

  // Layout: 8px radius (rounded-lg), max-height calc(100dvh - 64px), body scroll (overflow-y-auto), fixed header/footer
  assert.match(modalContent, /rounded-lg/, 'AppModal panel must use 8px rounded-lg on desktop')
  assert.match(modalContent, /max-h-\[calc\(100dvh-64px\)\]|max-h-\[calc\(100vh-64px\)\]/, 'AppModal max usable height must be calc(100dvh - 64px)')
  assert.match(modalContent, /overflow-y-auto/, 'AppModal body must provide independent overflow-y-auto scroll')

  // Desktop modal scale dimensions (448px, 640px, 960px) and 32px viewport gutter
  assert.match(modalContent, /max-w-\[448px\]|max-w-\[var\(--modal-sm,448px\)\]/, 'AppModal sm size must map to 448px')
  assert.match(modalContent, /max-w-\[640px\]|max-w-\[var\(--modal-md,640px\)\]/, 'AppModal md size must map to 640px')
  assert.match(modalContent, /max-w-\[960px\]|max-w-\[var\(--modal-lg,960px\)\]/, 'AppModal lg size must map to 960px')
  assert.match(modalContent, /w-\[min\(100%-32px,|w-\[calc\(100%-32px\)\]|max-w-\[calc\(100vw-32px\)\]|mx-4/, 'AppModal must constrain content by 32px viewport gutter')

  // Barrel export in components/ui/index.ts
  const indexPath = path.resolve(import.meta.dirname, '../src/components/ui/index.ts')
  const indexContent = fs.readFileSync(indexPath, 'utf8')
  assert.match(indexContent, /AppModal/, 'components/ui/index.ts must export AppModal')
})

test('TDD 2.3: AppDrawer / BaseDrawer — semantic widths (320px left, 480px right) and mobile Bottom Sheet', () => {
  const drawerPath = path.resolve(import.meta.dirname, '../src/components/ui/BaseDrawer.vue')
  assert.ok(fs.existsSync(drawerPath), 'BaseDrawer.vue must exist under components/ui/')
  const drawerContent = fs.readFileSync(drawerPath, 'utf8')

  // Reuses useModalA11y without second focus implementation
  assert.match(drawerContent, /useModalA11y/, 'BaseDrawer must use useModalA11y composable')
  assert.doesNotMatch(drawerContent, /trapFocusMethod|customFocusTrap/, 'BaseDrawer must not duplicate focus logic')

  // Semantic dimensions: left navigation 320px edge drawer, right detail 480px desktop drawer
  assert.match(drawerContent, /max-w-\[320px\]|w-80|max-w-\[var\(--drawer-nav,320px\)\]/, 'BaseDrawer left placement must enforce 320px navigation width')
  assert.match(drawerContent, /sm:max-w-\[480px\]|sm:max-w-\[var\(--drawer-detail,480px\)\]/, 'BaseDrawer right placement must enforce 480px desktop width')

  // Mobile <640px: right placement is Bottom Sheet with top radius, safe area, drag handle; left is edge drawer
  assert.match(drawerContent, /rounded-t-\[16px\]|rounded-t-2xl|rounded-t-lg/, 'BaseDrawer mobile bottom sheet must have top radius')
  assert.match(drawerContent, /pb-safe/, 'BaseDrawer bottom sheet must include pb-safe')
  assert.match(drawerContent, /min-h-\[44px\]\s+min-w-\[44px\]/, 'BaseDrawer close trigger must enforce 44px min touch target')

  // Barrel export in components/ui/index.ts: both AppDrawer and BaseDrawer
  const indexPath = path.resolve(import.meta.dirname, '../src/components/ui/index.ts')
  const indexContent = fs.readFileSync(indexPath, 'utf8')
  assert.match(indexContent, /AppDrawer/, 'components/ui/index.ts must export AppDrawer')
  assert.match(indexContent, /BaseDrawer/, 'components/ui/index.ts must export BaseDrawer')
})

test('Visual Gate & Clean Primitives: AppModal and BaseDrawer conform to negative checklist', () => {
  const modalPath = path.resolve(import.meta.dirname, '../src/components/ui/AppModal.vue')
  assert.ok(fs.existsSync(modalPath), 'AppModal.vue must exist')
  const modalContent = fs.readFileSync(modalPath, 'utf8')
  const { violations: modalViolations } = scanContent(modalContent, 'src/components/ui/AppModal.vue')
  assert.equal(modalViolations.length, 0, `AppModal.vue must have 0 visual gate violations, found: ${JSON.stringify(modalViolations)}`)

  const drawerPath = path.resolve(import.meta.dirname, '../src/components/ui/BaseDrawer.vue')
  assert.ok(fs.existsSync(drawerPath), 'BaseDrawer.vue must exist')
  const drawerContent = fs.readFileSync(drawerPath, 'utf8')
  const { violations: drawerViolations } = scanContent(drawerContent, 'src/components/ui/BaseDrawer.vue')
  assert.equal(drawerViolations.length, 0, `BaseDrawer.vue must have 0 visual gate violations, found: ${JSON.stringify(drawerViolations)}`)
})
