import assert from 'node:assert/strict'
import test from 'node:test'
import fs from 'node:fs'
import path from 'node:path'
import { RULES, scanContent } from '../scripts/visual_audit.mjs'

const themePath = path.resolve(import.meta.dirname, '../src/assets/theme.css')
const stylePath = path.resolve(import.meta.dirname, '../src/style.css')
const modalPath = path.resolve(import.meta.dirname, '../src/components/ui/AppModal.vue')
const drawerPath = path.resolve(import.meta.dirname, '../src/components/ui/BaseDrawer.vue')
const filterPath = path.resolve(import.meta.dirname, '../src/components/ledger/LedgerSearchFilter.vue')
const ledgerPath = path.resolve(import.meta.dirname, '../src/views/NodeLedger.vue')

test('Task 4.1 / D4: Frozen AxonHub Dual-Mode Tokens in theme.css', () => {
  assert.ok(fs.existsSync(themePath), 'theme.css must exist')
  const content = fs.readFileSync(themePath, 'utf8')

  // Elevation Ladder tokens: canvas #0F172A, panel #111C31, card #16233A, raised #1E293B, inset #0B1324
  assert.match(content, /--color-canvas:\s*#0F172A/i, 'theme.css must define dark canvas #0F172A')
  assert.match(content, /--color-surface(?:-panel)?:\s*#111C31/i, 'theme.css must define panel surface #111C31')
  assert.match(content, /--color-surface-(?:card|base):\s*#16233A/i, 'theme.css must define card surface #16233A')
  assert.match(content, /--color-raised:\s*#1E293B/i, 'theme.css must define raised surface #1E293B')
  assert.match(content, /--color-inset:\s*#0B1324/i, 'theme.css must define inset surface #0B1324')

  // Light theme tokens
  assert.match(content, /--color-canvas:\s*#F8FAFC/i, 'theme.css must define light canvas #F8FAFC')
  assert.match(content, /--color-surface(?:-panel)?:\s*#FFFFFF/i, 'theme.css must define light panel #FFFFFF')

  // Text hierarchy
  assert.match(content, /--color-text-main:\s*#E2E8F0/i, 'theme.css must define primary text #E2E8F0 in dark')
  assert.match(content, /--color-text-muted:\s*#94A3B8/i, 'theme.css must define muted text #94A3B8 in dark')

  // Accent: Emerald/Teal
  assert.match(content, /--color-accent:\s*#2DD4BF/i, 'theme.css must define dark accent as teal #2DD4BF')
  assert.match(content, /--color-accent:\s*#0F766E/i, 'theme.css must define light accent as teal #0F766E')

  // Rejection of near-black void
  assert.doesNotMatch(content, /--color-canvas:\s*#000000/i, 'theme.css must NOT contain black canvas #000000')
  assert.doesNotMatch(content, /--color-canvas:\s*#08090A/i, 'theme.css must NOT contain near-black canvas #08090A')
})

test('Task 4.1 / D4: Rejection of near-black canvas in style.css modified paths', () => {
  assert.ok(fs.existsSync(stylePath), 'style.css must exist')
  const content = fs.readFileSync(stylePath, 'utf8')

  // style.css must not fall back to old near-black colors
  assert.doesNotMatch(content, /var\(--color-canvas,\s*#000000\)/i, 'style.css must NOT fall back to #000000')
  assert.doesNotMatch(content, /var\(--color-canvas,\s*#08090a\)/i, 'style.css must NOT fall back to #08090a')
})

test('Task 1.2 / D2: Governed Overlay-Only Backdrop Blur in visual audit', () => {
  const glassRule = RULES.find(r => r.id === 'no-glassmorphism')
  assert.ok(glassRule, 'no-glassmorphism rule must exist in RULES')

  // 1. Negative fixture: Domain view using backdrop blur must be REJECTED
  const viewWithBlur = `
    <template>
      <div class="backdrop-blur-sm bg-black/50">
        <span>Illegal page blur</span>
      </div>
    </template>
  `
  const viewViolations = glassRule.check(viewWithBlur, 'src/views/NodeLedger.vue')
  assert.ok(viewViolations.length > 0, 'Domain view with backdrop blur must be rejected by visual audit')

  // 2. Negative fixture: Shared overlay with excessive blur (>4px) must be REJECTED
  const excessiveBlurOverlay = `
    <template>
      <div class="fixed inset-0 backdrop-blur-md bg-black/80">
        <span>Modal content</span>
      </div>
    </template>
  `
  const excessiveViolations = glassRule.check(excessiveBlurOverlay, 'src/components/ui/AppModal.vue')
  assert.ok(excessiveViolations.length > 0, 'AppModal with excessive blur (>4px) must be rejected')

  // 3. Positive fixture: Governed overlay primitive with restrained 4px blur must PASS
  const compliantOverlay = `
    <template>
      <div class="fixed inset-0 bg-black/80 backdrop-blur-[4px]">
        <span>Compliant modal backdrop</span>
      </div>
    </template>
  `
  const compliantViolations = glassRule.check(compliantOverlay, 'src/components/ui/AppModal.vue')
  assert.equal(compliantViolations.length, 0, 'AppModal with governed 4px blur must produce 0 violations')

  // 4. Positive fixture: BaseDrawer with restrained 4px blur must PASS
  const compliantDrawer = `
    <template>
      <div class="fixed inset-0 bg-black/80 backdrop-blur-[4px]">
        <span>Compliant drawer backdrop</span>
      </div>
    </template>
  `
  const drawerCompliantViolations = glassRule.check(compliantDrawer, 'src/components/ui/BaseDrawer.vue')
  assert.equal(drawerCompliantViolations.length, 0, 'BaseDrawer with governed 4px blur must produce 0 violations')
})

test('Task 1.2 / D4: Rejection of width animation, transition-all, and component-local tokens', () => {
  const transAllRule = RULES.find(r => r.id === 'no-transition-all')
  const widthAnimRule = RULES.find(r => r.id === 'no-width-animation')

  // Ensure rules catch violations
  assert.ok(transAllRule.check('<div class="transition-all"></div>', 'src/components/ledger/LedgerSearchFilter.vue').length > 0)
  assert.ok(widthAnimRule.check('<div class="transition-[width]"></div>', 'src/components/ledger/LedgerDrawer.vue').length > 0)

  // Verify actual components do not use transition-all or width animation
  const filterContent = fs.readFileSync(filterPath, 'utf8')
  const ledgerContent = fs.readFileSync(ledgerPath, 'utf8')

  assert.doesNotMatch(filterContent, /\btransition-all\b/, 'LedgerSearchFilter must not use transition-all')
  assert.doesNotMatch(ledgerContent, /\btransition-all\b/, 'NodeLedger must not use transition-all')
})
