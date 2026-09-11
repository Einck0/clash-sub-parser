import assert from 'node:assert/strict'
import test from 'node:test'
import fs from 'node:fs'
import path from 'node:path'
import { cn } from '../src/lib/utils.ts'

const frontendDir = path.resolve(import.meta.dirname, '..')

test('Task 3.1: components.json manifest and dependency resolution contract', () => {
  const compJsonPath = path.join(frontendDir, 'components.json')
  assert.ok(fs.existsSync(compJsonPath), 'components.json must exist in frontend root')
  const compJson = JSON.parse(fs.readFileSync(compJsonPath, 'utf8'))

  assert.equal(compJson.typescript, true)
  assert.equal(compJson.framework, 'vite')
  assert.equal(compJson.tailwind?.css, 'src/assets/theme.css')
  assert.equal(compJson.aliases?.utils, '@/lib/utils')
  assert.equal(compJson.aliases?.ui, '@/components/ui')
  assert.equal(compJson.aliases?.components, '@/components')

  const pkgJsonPath = path.join(frontendDir, 'package.json')
  const pkgJson = JSON.parse(fs.readFileSync(pkgJsonPath, 'utf8'))

  // Exact pinned versions, no unpinned added dependencies, no deprecated radix-vue
  assert.ok(pkgJson.dependencies['reka-ui'], 'reka-ui must be installed')
  assert.doesNotMatch(pkgJson.dependencies['reka-ui'], /^[~^]/, 'reka-ui must be exact-pinned')
  assert.ok(pkgJson.dependencies['clsx'], 'clsx must be installed')
  assert.doesNotMatch(pkgJson.dependencies['clsx'], /^[~^]/, 'clsx must be exact-pinned')
  assert.ok(pkgJson.dependencies['tailwind-merge'], 'tailwind-merge must be installed')
  assert.doesNotMatch(pkgJson.dependencies['tailwind-merge'], /^[~^]/, 'tailwind-merge must be exact-pinned')
  assert.ok(pkgJson.dependencies['class-variance-authority'], 'class-variance-authority must be installed')
  assert.doesNotMatch(pkgJson.dependencies['class-variance-authority'], /^[~^]/, 'class-variance-authority must be exact-pinned')

  assert.equal(pkgJson.dependencies['radix-vue'], undefined, 'Deprecated radix-vue package must NEVER be installed')
  assert.equal(pkgJson.devDependencies?.['radix-vue'], undefined, 'Deprecated radix-vue package must NEVER be in devDependencies')
})

test('Task 3.1: src/lib/utils.ts cn helper class composition and deduplication', () => {
  const utilsPath = path.join(frontendDir, 'src/lib/utils.ts')
  assert.ok(fs.existsSync(utilsPath), 'src/lib/utils.ts must exist')

  // cn composition tests
  assert.equal(cn('px-2 py-1', 'bg-accent'), 'px-2 py-1 bg-accent')
  // tailwind merge override check
  assert.equal(cn('p-2', 'p-4'), 'p-4')
  assert.equal(cn('text-sm text-text-muted', { 'text-text-main font-bold': true }), 'text-sm text-text-main font-bold')
})

test('Task 3.2: theme.css freezes Void dark hierarchy (#08090a, #0d0f12, #121417)', () => {
  const themePath = path.join(frontendDir, 'src/assets/theme.css')
  const themeContent = fs.readFileSync(themePath, 'utf8')

  assert.match(themeContent, /--color-void:\s*#08090a/i, 'theme.css must define Void dark token #08090a')
  assert.match(themeContent, /--color-panel:\s*#0d0f12/i, 'theme.css must define Panel dark token #0d0f12')
  assert.match(themeContent, /--color-card:\s*#121417/i, 'theme.css must define Card dark token #121417')
  assert.match(themeContent, /--color-surface-void:\s*#08090a/i, 'theme.css must define surface-void token #08090a')

  assert.match(themeContent, /\.surface-void\s*\{[^}]*#08090a/i, 'theme.css must provide .surface-void utility')
  assert.match(themeContent, /\.surface-panel\s*\{[^}]*#0d0f12/i, 'theme.css must provide .surface-panel utility')
  assert.match(themeContent, /\.surface-card\s*\{[^}]*#121417/i, 'theme.css must provide .surface-card utility')

  assert.match(themeContent, /prefers-reduced-motion/, 'theme.css must enforce prefers-reduced-motion')
})

test('Task 3.2: Inspectable shadcn-vue and Reka UI v2 component primitives exist locally', () => {
  const uiDir = path.join(frontendDir, 'src/components/ui')

  const requiredFamilies = [
    'button',
    'badge',
    'input',
    'checkbox',
    'tooltip',
    'select',
    'dropdown-menu',
    'dialog',
    'alert-dialog',
    'sheet',
    'tabs'
  ]

  for (const family of requiredFamilies) {
    const familyDir = path.join(uiDir, family)
    assert.ok(fs.existsSync(familyDir), `UI primitive family directory '${family}' must exist under components/ui/`)
    const indexFile = path.join(familyDir, 'index.ts')
    assert.ok(fs.existsSync(indexFile), `UI primitive family '${family}' must have an index.ts barrel`)
  }

  // Verify Dialog overlay does NOT have unmanaged blur
  const dialogOverlayContent = fs.readFileSync(path.join(uiDir, 'dialog/DialogOverlay.vue'), 'utf8')
  assert.doesNotMatch(dialogOverlayContent, /backdrop-blur/, 'DialogOverlay must not use unmanaged glassmorphism')

  // Verify Sheet overlay does NOT have unmanaged blur
  const sheetOverlayContent = fs.readFileSync(path.join(uiDir, 'sheet/SheetOverlay.vue'), 'utf8')
  assert.doesNotMatch(sheetOverlayContent, /backdrop-blur/, 'SheetOverlay must not use unmanaged glassmorphism')

  // Verify Sheet Content preserves 320px left and 480px right
  const sheetContent = fs.readFileSync(path.join(uiDir, 'sheet/SheetContent.vue'), 'utf8')
  assert.match(sheetContent, /max-w-\[320px\]/, 'Sheet left drawer must enforce 320px width')
  assert.match(sheetContent, /sm:max-w-\[480px\]/, 'Sheet right drawer must enforce 480px width')
})

test('Task 3.3: Barrel export components/ui/index.ts exports all legacy adapters and shadcn primitives', () => {
  const indexPath = path.join(frontendDir, 'src/components/ui/index.ts')
  const indexContent = fs.readFileSync(indexPath, 'utf8')

  // Legacy adapters
  assert.match(indexContent, /AppModal/, 'index.ts must export AppModal adapter')
  assert.match(indexContent, /BaseDrawer/, 'index.ts must export BaseDrawer adapter')
  assert.match(indexContent, /ConfirmDialog/, 'index.ts must export ConfirmDialog adapter')
  assert.match(indexContent, /ToastContainer/, 'index.ts must export ToastContainer adapter')
  assert.match(indexContent, /IconButton/, 'index.ts must export IconButton adapter')

  // New shadcn primitives
  assert.match(indexContent, /export \* from '\.\/button'/, 'index.ts must export button family')
  assert.match(indexContent, /export \* from '\.\/badge'/, 'index.ts must export badge family')
  assert.match(indexContent, /export \* from '\.\/input'/, 'index.ts must export input family')
  assert.match(indexContent, /export \* from '\.\/checkbox'/, 'index.ts must export checkbox family')
  assert.match(indexContent, /export \* from '\.\/tooltip'/, 'index.ts must export tooltip family')
  assert.match(indexContent, /export \* from '\.\/dropdown-menu'/, 'index.ts must export dropdown-menu family')
  assert.match(indexContent, /export \* from '\.\/dialog'/, 'index.ts must export dialog family')
  assert.match(indexContent, /export \* from '\.\/alert-dialog'/, 'index.ts must export alert-dialog family')
  assert.match(indexContent, /export \* from '\.\/sheet'/, 'index.ts must export sheet family')
  assert.match(indexContent, /export \* from '\.\/tabs'/, 'index.ts must export tabs family')
})
