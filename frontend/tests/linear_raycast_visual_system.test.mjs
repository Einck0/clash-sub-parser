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

test('Task 1.2 / D1: Frozen Dark Elevation Token Ladder in theme.css', () => {
  assert.ok(fs.existsSync(themePath), 'theme.css must exist')
  const content = fs.readFileSync(themePath, 'utf8')

  // Elevation Ladder tokens: canvas #08090a, panel #0d0f12, card #121417, raised #181b20, inset #090b0d
  assert.match(content, /--color-canvas:\s*#08090a/i, 'theme.css must define canvas #08090a')
  assert.match(content, /--color-surface(?:-panel)?:\s*#0d0f12/i, 'theme.css must define panel surface #0d0f12')
  assert.match(content, /--color-surface-(?:card|base):\s*#121417/i, 'theme.css must define card surface #121417')
  assert.match(content, /--color-raised:\s*#181b20/i, 'theme.css must define raised surface #181b20')
  assert.match(content, /--color-inset:\s*#090b0d/i, 'theme.css must define inset surface #090b0d')

  // Hairline border and inset highlight
  assert.match(content, /--color-border-subtle:\s*rgba\(255,\s*255,\s*255,\s*0\.07\)/i, 'theme.css must define hairline border rgba(255, 255, 255, 0.07)')
  assert.match(content, /rgba\(255,\s*255,\s*255,\s*0\.055\)/i, 'theme.css must define top edge inset highlight rgba(255, 255, 255, 0.055)')

  // Text hierarchy
  assert.match(content, /--color-text-main:\s*#f1f3f5/i, 'theme.css must define primary text #f1f3f5')
  assert.match(content, /--color-text-muted:\s*#8b919a/i, 'theme.css must define muted text #8b919a')
  assert.match(content, /--color-text-sub:\s*#626870/i, 'theme.css must define sub/disabled text #626870')

  // Accent: cool indigo only (not generic saturated blue #3B82F6)
  assert.match(content, /--color-accent:\s*#6366f1/i, 'theme.css must define accent as cool indigo #6366f1')

  // Rejection of legacy Slate visual values in theme.css
  assert.doesNotMatch(content, /#090D16/i, 'theme.css must NOT contain legacy Slate-blue canvas #090D16')
  assert.doesNotMatch(content, /#0F172A/i, 'theme.css must NOT contain legacy Slate-blue surface #0F172A')
  assert.doesNotMatch(content, /#1E293B/i, 'theme.css must NOT contain legacy Slate-blue raised #1E293B')
})

test('Task 1.2 / D1: Rejection of legacy Slate visual values in style.css modified paths', () => {
  assert.ok(fs.existsSync(stylePath), 'style.css must exist')
  const content = fs.readFileSync(stylePath, 'utf8')

  // style.css must not fall back to old slate colors
  assert.doesNotMatch(content, /#090D16/i, 'style.css must NOT contain legacy #090D16')
  assert.doesNotMatch(content, /#0F172A/i, 'style.css must NOT contain legacy #0F172A')
  assert.doesNotMatch(content, /#1E293B/i, 'style.css must NOT contain legacy #1E293B')
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
