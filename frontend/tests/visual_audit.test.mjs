import assert from 'node:assert/strict'
import test from 'node:test'
import path from 'node:path'
import fs from 'node:fs'
import {
  RULES,
  DEFAULT_ALLOWLIST,
  validateAllowlist,
  isAllowlisted,
  scanContent,
  runVisualAudit,
  formatReport
} from '../scripts/visual_audit.mjs'

test('Visual Gate Rule 1: no-hardcoded-colors catches arbitrary hex/rgb classes and styles', () => {
  const rule = RULES.find(r => r.id === 'no-hardcoded-colors')
  assert.ok(rule, 'no-hardcoded-colors rule must exist')

  // Negative fixture: arbitrary Tailwind hex/rgba brackets and inline styles
  const negativeSample = `
    <template>
      <div class="flex bg-[#090D16] text-[#F8FAFC] border-[#334155] ring-[rgba(255,255,255,0.1)]">
        <span style="color: #123456; background-color: rgba(0, 0, 0, 0.5)">Label</span>
      </div>
    </template>
  `
  const violations = rule.check(negativeSample, 'src/views/NegativeTest.vue')
  assert.ok(violations.length >= 4, `Expected at least 4 color violations, got ${violations.length}`)
  assert.ok(violations.some(v => v.match.includes('bg-[#090D16]')))
  assert.ok(violations.some(v => v.match.includes('text-[#F8FAFC]')))
  assert.ok(violations.some(v => v.match.includes('border-[#334155]')))

  // Positive fixture: compliant semantic tokens
  const positiveSample = `
    <template>
      <div class="flex bg-canvas text-main border-subtle">
        <span class="text-muted bg-surface-base">Label</span>
      </div>
    </template>
  `
  const cleanViolations = rule.check(positiveSample, 'src/views/PositiveTest.vue')
  assert.equal(cleanViolations.length, 0, 'Compliant semantic classes should produce 0 color violations')
})

test('Visual Gate Rule 2: no-css-gradients catches Tailwind gradients and CSS gradient functions', () => {
  const rule = RULES.find(r => r.id === 'no-css-gradients')
  assert.ok(rule, 'no-css-gradients rule must exist')

  // Negative fixture: bg-gradient classes and CSS linear/radial gradients
  const negativeSample = `
    <template>
      <div class="bg-gradient-to-r from-blue-500 to-cyan-500">
        <div class="bg-gradient-to-b from-slate-900 to-black"></div>
      </div>
    </template>
    <style>
      .card {
        background: linear-gradient(180deg, #0f172a 0%, #1e293b 100%);
      }
      .glow {
        background: radial-gradient(circle at center, rgba(59, 130, 246, 0.2), transparent);
      }
    </style>
  `
  const violations = rule.check(negativeSample, 'src/views/NegativeGradient.vue')
  assert.ok(violations.length >= 4, `Expected at least 4 gradient violations, got ${violations.length}`)
  assert.ok(violations.some(v => v.match.includes('bg-gradient-to-r')))
  assert.ok(violations.some(v => v.match.includes('linear-gradient(')))
  assert.ok(violations.some(v => v.match.includes('radial-gradient(')))

  // Positive fixture: solid opaque surfaces
  const positiveSample = `
    <template>
      <div class="bg-surface-base border border-subtle">
        <div class="bg-surface-hover"></div>
      </div>
    </template>
  `
  const cleanViolations = rule.check(positiveSample, 'src/views/PositiveGradient.vue')
  assert.equal(cleanViolations.length, 0, 'Opaque surfaces should produce 0 gradient violations')
})

test('Visual Gate Rule 3: no-glassmorphism catches backdrop-blur and backdrop-filter', () => {
  const rule = RULES.find(r => r.id === 'no-glassmorphism')
  assert.ok(rule, 'no-glassmorphism rule must exist')

  // Negative fixture: glassmorphism blur and filter
  const negativeSample = `
    <template>
      <div class="backdrop-blur-md backdrop-blur-xs backdrop-filter">
        <span>Modal content</span>
      </div>
    </template>
    <style>
      .glass {
        backdrop-filter: blur(12px);
      }
    </style>
  `
  const violations = rule.check(negativeSample, 'src/views/NegativeGlass.vue')
  assert.ok(violations.length >= 3, `Expected at least 3 glassmorphism violations, got ${violations.length}`)
  assert.ok(violations.some(v => v.match.includes('backdrop-blur-md')))
  assert.ok(violations.some(v => v.match.includes('backdrop-filter')))

  // Positive fixture: opaque stepped surface
  const positiveSample = `
    <template>
      <div class="bg-surface-base border border-subtle">
        <span>Opaque modal</span>
      </div>
    </template>
  `
  const cleanViolations = rule.check(positiveSample, 'src/views/PositiveGlass.vue')
  assert.equal(cleanViolations.length, 0, 'Opaque surfaces should produce 0 glassmorphism violations')
})

test('Visual Gate Rule 4: no-emoji-icons catches decorative and action emoji in templates', () => {
  const rule = RULES.find(r => r.id === 'no-emoji-icons')
  assert.ok(rule, 'no-emoji-icons rule must exist')

  // Negative fixture: emoji as icons
  const negativeSample = `
    <template>
      <div class="toolbar">
        <button>⚡ 立即探测</button>
        <span class="stat-icon">📡</span>
        <span>👑 顶级节点</span>
        <button>⚙️ 规则设置</button>
      </div>
    </template>
  `
  const violations = rule.check(negativeSample, 'src/views/NegativeEmoji.vue')
  assert.ok(violations.length >= 4, `Expected at least 4 emoji violations, got ${violations.length}`)
  assert.ok(violations.some(v => v.match === '⚡'))
  assert.ok(violations.some(v => v.match === '📡'))
  assert.ok(violations.some(v => v.match === '👑'))
  assert.ok(violations.some(v => v.match === '⚙️' || v.match.startsWith('⚙')))

  // Positive fixture: Lucide icon components
  const positiveSample = `
    <template>
      <div class="toolbar">
        <button><LucideZap class="h-4 w-4" /> 立即探测</button>
        <LucideRadio class="h-4 w-4 text-brand-primary" />
        <LucideSettings class="h-4 w-4" />
      </div>
    </template>
  `
  const cleanViolations = rule.check(positiveSample, 'src/views/PositiveEmoji.vue')
  assert.equal(cleanViolations.length, 0, 'Lucide icons should produce 0 emoji violations')
})

test('Visual Gate Rule 5: no-transition-all catches transition: all and transition-all', () => {
  const rule = RULES.find(r => r.id === 'no-transition-all')
  assert.ok(rule, 'no-transition-all rule must exist')

  // Negative fixture: transition-all
  const negativeSample = `
    <template>
      <button class="transition-all duration-200 hover:scale-105">Click</button>
    </template>
    <style>
      .btn {
        transition: all 0.2s cubic-bezier(0.16, 1, 0.3, 1);
      }
      .panel {
        transition-property: all;
      }
    </style>
  `
  const violations = rule.check(negativeSample, 'src/views/NegativeTransition.vue')
  assert.ok(violations.length >= 3, `Expected at least 3 transition-all violations, got ${violations.length}`)
  assert.ok(violations.some(v => v.match === 'transition-all'))
  assert.ok(violations.some(v => v.match.includes('transition: all')))
  assert.ok(violations.some(v => v.match.includes('transition-property: all')))

  // Positive fixture: targeted explicit transitions
  const positiveSample = `
    <template>
      <button class="transition-colors duration-150 transition-opacity">Click</button>
    </template>
    <style>
      .btn {
        transition: opacity 0.16s cubic-bezier(0.16, 1, 0.3, 1);
      }
    </style>
  `
  const cleanViolations = rule.check(positiveSample, 'src/views/PositiveTransition.vue')
  assert.equal(cleanViolations.length, 0, 'Targeted transitions should produce 0 transition-all violations')
})

test('Visual Gate Rule 6: no-width-animation catches width transitions and animations', () => {
  const rule = RULES.find(r => r.id === 'no-width-animation')
  assert.ok(rule, 'no-width-animation rule must exist')

  // Negative fixture: width transitions
  const negativeSample = `
    <template>
      <div class="transition-width duration-300"></div>
      <aside class="transition-[width] duration-200"></aside>
    </template>
    <style>
      .sidebar {
        transition: width 0.25s ease-in-out;
      }
      .drawer {
        transition-property: max-width;
      }
    </style>
  `
  const violations = rule.check(negativeSample, 'src/views/NegativeWidth.vue')
  assert.ok(violations.length >= 4, `Expected at least 4 width transition violations, got ${violations.length}`)
  assert.ok(violations.some(v => v.match === 'transition-width'))
  assert.ok(violations.some(v => v.match === 'transition-[width]'))
  assert.ok(violations.some(v => v.match.includes('width')))

  // Positive fixture: transform-based slide
  const positiveSample = `
    <template>
      <div class="transition-transform duration-200 translate-x-0"></div>
    </template>
    <style>
      .drawer {
        transform: translateX(0);
        transition: transform 0.16s ease;
      }
    </style>
  `
  const cleanViolations = rule.check(positiveSample, 'src/views/PositiveWidth.vue')
  assert.equal(cleanViolations.length, 0, 'Transform transitions should produce 0 width transition violations')
})

test('Visual Gate Rule 7: no-excessive-radius catches rounded-xl/2xl/3xl and directional variants', () => {
  const rule = RULES.find(r => r.id === 'no-excessive-radius')
  assert.ok(rule, 'no-excessive-radius rule must exist')

  // Negative fixture: large rounded corners
  const negativeSample = `
    <template>
      <div class="rounded-xl rounded-2xl rounded-3xl">
        <div class="rounded-t-2xl rounded-b-xl rounded-l-2xl"></div>
      </div>
    </template>
  `
  const violations = rule.check(negativeSample, 'src/views/NegativeRadius.vue')
  assert.ok(violations.length >= 6, `Expected at least 6 radius violations, got ${violations.length}`)
  assert.ok(violations.some(v => v.match === 'rounded-xl'))
  assert.ok(violations.some(v => v.match === 'rounded-2xl'))
  assert.ok(violations.some(v => v.match === 'rounded-3xl'))
  assert.ok(violations.some(v => v.match === 'rounded-t-2xl'))

  // Positive fixture: standard 8px grid (rounded-md / rounded-lg)
  const positiveSample = `
    <template>
      <div class="rounded-lg border border-subtle">
        <div class="rounded-md"></div>
      </div>
    </template>
  `
  const cleanViolations = rule.check(positiveSample, 'src/views/PositiveRadius.vue')
  assert.equal(cleanViolations.length, 0, 'Standard 8px grid rounded corners should produce 0 violations')
})

test('Visual Gate Rule 8: no-excessive-shadows catches shadow-lg/xl/2xl', () => {
  const rule = RULES.find(r => r.id === 'no-excessive-shadows')
  assert.ok(rule, 'no-excessive-shadows rule must exist')

  // Negative fixture: stacked heavy drop shadows
  const negativeSample = `
    <template>
      <div class="shadow-lg hover:shadow-xl active:shadow-2xl">
        <span class="shadow-xl">Card</span>
      </div>
    </template>
  `
  const violations = rule.check(negativeSample, 'src/views/NegativeShadow.vue')
  assert.ok(violations.length >= 4, `Expected at least 4 shadow violations, got ${violations.length}`)
  assert.ok(violations.some(v => v.match === 'shadow-lg'))
  assert.ok(violations.some(v => v.match === 'shadow-xl'))
  assert.ok(violations.some(v => v.match === 'shadow-2xl'))

  // Positive fixture: 1px hairline border or subtle shadow-xs
  const positiveSample = `
    <template>
      <div class="border border-subtle shadow-xs">
        <span>Card</span>
      </div>
    </template>
  `
  const cleanViolations = rule.check(positiveSample, 'src/views/PositiveShadow.vue')
  assert.equal(cleanViolations.length, 0, 'Hairline borders and shadow-xs should produce 0 shadow violations')
})

test('Visual Gate Rule 9: no-scoped-styles catches <style scoped> in components', () => {
  const rule = RULES.find(r => r.id === 'no-scoped-styles')
  assert.ok(rule, 'no-scoped-styles rule must exist')

  // Negative fixture: scoped CSS block
  const negativeSample = `
    <template>
      <div class="my-card">Legacy</div>
    </template>
    <style scoped>
      .my-card {
        padding: 12px;
      }
    </style>
  `
  const violations = rule.check(negativeSample, 'src/components/NegativeScoped.vue')
  assert.equal(violations.length, 1, 'Should catch 1 scoped style violation')
  assert.ok(violations[0].match.includes('<style scoped>'))

  // Positive fixture: purely token & Tailwind utility based component
  const positiveSample = `
    <template>
      <div class="p-3 bg-surface-base border border-subtle">Modern</div>
    </template>
    <script setup lang="ts">
      // Clean script setup without scoped CSS
    </script>
  `
  const cleanViolations = rule.check(positiveSample, 'src/components/PositiveScoped.vue')
  assert.equal(cleanViolations.length, 0, 'Components without scoped style should produce 0 violations')
})

test('Allowlist Integrity: enforces explicit, minimal rules and rejects wildcards and view bypasses', () => {
  // 1. Valid allowlist must succeed
  assert.doesNotThrow(() => {
    validateAllowlist(DEFAULT_ALLOWLIST)
  })

  // 2. Rejects wildcard in file or rule
  assert.throws(() => {
    validateAllowlist([
      { file: 'src/components/**', ruleId: 'no-hardcoded-colors', reason: 'Too broad' }
    ])
  }, /Wildcards are strictly prohibited/)

  assert.throws(() => {
    validateAllowlist([
      { file: 'src/components/ui/BaseDrawer.vue', ruleId: '*', reason: 'All rules bypassed' }
    ])
  }, /Wildcards are strictly prohibited/)

  // 3. Rejects exempting domain views
  assert.throws(() => {
    validateAllowlist([
      { file: 'src/views/NodeLedger.vue', ruleId: 'no-hardcoded-colors', reason: 'Bypass domain view' }
    ])
  }, /Exempting domain views is strictly prohibited/)

  // 4. Correctly matches approved theme entrypoint
  assert.equal(isAllowlisted('src/assets/theme.css', 'no-hardcoded-colors'), true)
  assert.equal(isAllowlisted('src/assets/theme.css', 'no-glassmorphism'), false)
  assert.equal(isAllowlisted('src/views/NodeLedger.vue', 'no-hardcoded-colors'), false)
})

test('Visual Gate Live Baseline Audit: scans all views and components and records baseline debt', () => {
  const srcDir = path.resolve(import.meta.dirname, '../src')
  assert.ok(fs.existsSync(srcDir), 'src directory must exist')

  const auditResult = runVisualAudit(srcDir)

  // 1. Must scan across all domain views and components without skipping directories
  assert.ok(auditResult.totalFiles >= 35, `Expected at least 35 files scanned, got ${auditResult.totalFiles}`)

  // Verify key domain directories are fully included in the scan
  const scannedPaths = auditResult.scannedFiles || Object.keys(auditResult.violationsByFile)
  assert.ok(scannedPaths.some(p => p.startsWith('views/')), 'Must scan src/views')
  assert.ok(scannedPaths.some(p => p.startsWith('components/')), 'Must scan src/components')
  assert.ok(scannedPaths.some(p => p.startsWith('components/ledger/')), 'Must scan src/components/ledger')
  assert.ok(scannedPaths.some(p => p.startsWith('components/ui/')), 'Must scan src/components/ui without whole-directory bypass')

  // 2. Audit enforces zero visual debt across all scanned files (0 violations gate)
  assert.equal(auditResult.totalViolations, 0, 'Must achieve 0 violations across all components and views')
  assert.equal(auditResult.violationsByRule['no-hardcoded-colors'] || 0, 0)
  assert.equal(auditResult.violationsByRule['no-css-gradients'] || 0, 0)
  assert.equal(auditResult.violationsByRule['no-glassmorphism'] || 0, 0)
  assert.equal(auditResult.violationsByRule['no-emoji-icons'] || 0, 0)
  assert.equal(auditResult.violationsByRule['no-transition-all'] || 0, 0)
  assert.equal(auditResult.violationsByRule['no-excessive-radius'] || 0, 0)
  assert.equal(auditResult.violationsByRule['no-scoped-styles'] || 0, 0)

  // 3. Formatted report produces readable summary with classification
  const report = formatReport(auditResult, { verbose: false })
  assert.match(report, /CSP 前端视觉基线与负面清单审计/)
  assert.match(report, /扫描文件总数:/)
  assert.match(report, /发现违规项数:/)
  assert.match(report, /违规规则分类汇总:/)

  // Output inventory summary to test console for tracking and handoff
  console.log('\n' + report)
})
