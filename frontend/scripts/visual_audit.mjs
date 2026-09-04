import fs from 'node:fs'
import path from 'node:path'
import { fileURLToPath } from 'node:url'

/**
 * Visual Gate & Negative Checklist Audit Tool
 *
 * Enforces the CSP (Clean-slate Subscription Control-plane) visual design negative checklist
 * defined in OpenSpec D4.1/D4.2 and control-plane-frontend/spec.md:
 * - Prohibits hardcoded colors, gradients, glassmorphism, emoji icons, transition-all,
 *   width animations, excessive border radius, excessive shadows, and scoped styles.
 * - Enforces semantic design tokens and 1px hairline industrial layout.
 */

/**
 * Standard rule definitions for the visual gate.
 */
export const RULES = [
  {
    id: 'no-hardcoded-colors',
    name: '硬编码颜色值',
    description: '禁止在模板或样式中硬编码 hex/rgb/rgba 颜色，必须消费主题语义 token（如 text-main, bg-canvas, border-subtle）',
    severity: 'error',
    check(content, file) {
      const violations = []
      const lines = content.split('\n')

      // 1. Tailwind arbitrary color syntax: bg-[#...], text-[rgba(...)], etc.
      const twArbitraryColorRegex = /\b(?:bg|text|border|ring|fill|stroke|from|to|via|outline|accent|decoration)-\[(?:#[0-9a-fA-F]{3,8}|(?:rgb|rgba|hsl|hsla)\([^\]]+\))\]/g

      // 2. Inline style or template literal hardcoded colors: style="... #123456 ..." or style="... rgba(...) ..."
      const inlineStyleColorRegex = /(?:style|:style)\s*=\s*["'][^"']*(?:#[0-9a-fA-F]{3,8}\b|rgba?\s*\(|hsla?\s*\))/gi

      // 3. Raw hex colors in template classes or styles (excluding SVG anchor IDs or entity references)
      const rawHexColorInClassRegex = /class(?:Name)?\s*=\s*["'][^"']*#(?:[0-9a-fA-F]{3,8})\b[^"']*["']/g

      lines.forEach((line, idx) => {
        const lineNum = idx + 1
        // Skip comment-only lines
        const trimmed = line.trim()
        if (trimmed.startsWith('//') || trimmed.startsWith('/*') || trimmed.startsWith('*')) {
          return
        }

        let match
        twArbitraryColorRegex.lastIndex = 0
        while ((match = twArbitraryColorRegex.exec(line)) !== null) {
          violations.push({
            ruleId: 'no-hardcoded-colors',
            line: lineNum,
            column: match.index + 1,
            match: match[0],
            message: `发现硬编码颜色类 '${match[0]}'，请替换为语义 token`
          })
        }

        inlineStyleColorRegex.lastIndex = 0
        while ((match = inlineStyleColorRegex.exec(line)) !== null) {
          violations.push({
            ruleId: 'no-hardcoded-colors',
            line: lineNum,
            column: match.index + 1,
            match: match[0],
            message: `发现行内硬编码颜色样式 '${match[0]}'，请使用语义类或共享 UI`
          })
        }

        rawHexColorInClassRegex.lastIndex = 0
        while ((match = rawHexColorInClassRegex.exec(line)) !== null) {
          violations.push({
            ruleId: 'no-hardcoded-colors',
            line: lineNum,
            column: match.index + 1,
            match: match[0],
            message: `发现类名中硬编码颜色 '${match[0]}'，请使用语义 token`
          })
        }
      })

      return violations
    }
  },
  {
    id: 'no-css-gradients',
    name: '渐变背景与 CSS 渐变',
    description: '禁止在数据后台使用 bg-gradient 或 CSS 渐变，采用不透明表面及 1px hairline 分区',
    severity: 'error',
    check(content, file) {
      const violations = []
      const lines = content.split('\n')
      const twGradientRegex = /\bbg-gradient-(?:to-[trbl]{1,2}|radial|conic)\b/g
      const cssGradientRegex = /\b(?:linear|radial|conic)-gradient\s*\(/g

      lines.forEach((line, idx) => {
        const lineNum = idx + 1
        const trimmed = line.trim()
        if (trimmed.startsWith('//') || trimmed.startsWith('/*') || trimmed.startsWith('*')) return

        let match
        twGradientRegex.lastIndex = 0
        while ((match = twGradientRegex.exec(line)) !== null) {
          violations.push({
            ruleId: 'no-css-gradients',
            line: lineNum,
            column: match.index + 1,
            match: match[0],
            message: `发现背景渐变类 '${match[0]}'，控制台禁止渐变背景`
          })
        }

        cssGradientRegex.lastIndex = 0
        while ((match = cssGradientRegex.exec(line)) !== null) {
          violations.push({
            ruleId: 'no-css-gradients',
            line: lineNum,
            column: match.index + 1,
            match: match[0],
            message: `发现 CSS 渐变函数 '${match[0]}'，请使用不透明语义表面`
          })
        }
      })
      return violations
    }
  },
  {
    id: 'no-glassmorphism',
    name: '磨砂玻璃与背景模糊',
    description: '禁止使用 backdrop-blur 或 backdrop-filter 玻璃拟态，采用不透明阶梯表面',
    severity: 'error',
    check(content, file) {
      const violations = []
      const lines = content.split('\n')
      const twBlurRegex = /\bbackdrop-blur(?:-[a-z0-9]+)?\b/g
      const twFilterRegex = /\bbackdrop-filter\b/g
      const cssBackdropRegex = /\bbackdrop-filter\s*:/g

      lines.forEach((line, idx) => {
        const lineNum = idx + 1
        const trimmed = line.trim()
        if (trimmed.startsWith('//') || trimmed.startsWith('/*') || trimmed.startsWith('*')) return

        let match
        twBlurRegex.lastIndex = 0
        while ((match = twBlurRegex.exec(line)) !== null) {
          violations.push({
            ruleId: 'no-glassmorphism',
            line: lineNum,
            column: match.index + 1,
            match: match[0],
            message: `发现玻璃拟态背景模糊 '${match[0]}'，必须使用不透明表面`
          })
        }

        twFilterRegex.lastIndex = 0
        while ((match = twFilterRegex.exec(line)) !== null) {
          violations.push({
            ruleId: 'no-glassmorphism',
            line: lineNum,
            column: match.index + 1,
            match: match[0],
            message: `发现 backdrop-filter 类 '${match[0]}'`
          })
        }

        cssBackdropRegex.lastIndex = 0
        while ((match = cssBackdropRegex.exec(line)) !== null) {
          violations.push({
            ruleId: 'no-glassmorphism',
            line: lineNum,
            column: match.index + 1,
            match: match[0],
            message: `发现 backdrop-filter CSS 属性 '${match[0]}'`
          })
        }
      })
      return violations
    }
  },
  {
    id: 'no-emoji-icons',
    name: 'Emoji 交互与状态图标',
    description: '禁止在 UI 模板中使用 emoji 作为操作图标或状态图标，统一使用 lucide-vue-next 图标组件',
    severity: 'error',
    check(content, file) {
      const violations = []
      // Check emoji within <template> block to avoid comments or data strings
      const templateMatch = content.match(/<template[\s\S]*<\/template>/i)
      const targetContent = templateMatch ? templateMatch[0] : content
      const lines = targetContent.split('\n')

      // Common decorative / action emojis in templates
      const emojiRegex = /[\u{1F300}-\u{1F5FF}\u{1F600}-\u{1F64F}\u{1F680}-\u{1F6FF}\u{1F900}-\u{1F9FF}\u{2600}-\u{26FF}\u{2700}-\u{27BF}]/gu

      lines.forEach((line, idx) => {
        const lineNum = idx + 1
        const trimmed = line.trim()
        if (trimmed.startsWith('<!--') || trimmed.startsWith('//') || trimmed.startsWith('*')) return

        let match
        emojiRegex.lastIndex = 0
        while ((match = emojiRegex.exec(line)) !== null) {
          violations.push({
            ruleId: 'no-emoji-icons',
            line: lineNum,
            column: match.index + 1,
            match: match[0],
            message: `发现 UI 模板中包含 emoji 图标 '${match[0]}'，请替换为 lucide-vue-next 组件`
          })
        }
      })
      return violations
    }
  },
  {
    id: 'no-transition-all',
    name: '全属性过渡 transition-all',
    description: '禁止使用 transition: all 或 transition-all，必须针对特定属性（如 opacity, transform）显式指定过渡',
    severity: 'error',
    check(content, file) {
      const violations = []
      const lines = content.split('\n')
      const twTransitionAllRegex = /\btransition-all\b/g
      const cssTransitionAllRegex = /\btransition(?:-property)?\s*:\s*all\b/g

      lines.forEach((line, idx) => {
        const lineNum = idx + 1
        const trimmed = line.trim()
        if (trimmed.startsWith('//') || trimmed.startsWith('/*') || trimmed.startsWith('*')) return

        let match
        twTransitionAllRegex.lastIndex = 0
        while ((match = twTransitionAllRegex.exec(line)) !== null) {
          violations.push({
            ruleId: 'no-transition-all',
            line: lineNum,
            column: match.index + 1,
            match: match[0],
            message: `发现全属性过渡 '${match[0]}'，请针对具体属性指定过渡（如 transition-opacity, transition-colors）`
          })
        }

        cssTransitionAllRegex.lastIndex = 0
        while ((match = cssTransitionAllRegex.exec(line)) !== null) {
          violations.push({
            ruleId: 'no-transition-all',
            line: lineNum,
            column: match.index + 1,
            match: match[0],
            message: `发现 CSS transition: all，请针对具体属性指定过渡`
          })
        }
      })
      return violations
    }
  },
  {
    id: 'no-width-animation',
    name: '宽度动画与过渡',
    description: '禁止对宽度进行过渡或动画（避免重排抖动），抽屉与侧栏使用 transform 平移或绝对定位',
    severity: 'error',
    check(content, file) {
      const violations = []
      const lines = content.split('\n')
      const twWidthTransitionRegex = /\btransition-width\b|\btransition-\[(?:max-|min-)?width\]/g
      const cssWidthTransitionRegex = /\btransition(?:-property)?\s*:\s*[^;{}]*\b(?:width|max-width|min-width)\b/g

      lines.forEach((line, idx) => {
        const lineNum = idx + 1
        const trimmed = line.trim()
        if (trimmed.startsWith('//') || trimmed.startsWith('/*') || trimmed.startsWith('*')) return

        let match
        twWidthTransitionRegex.lastIndex = 0
        while ((match = twWidthTransitionRegex.exec(line)) !== null) {
          violations.push({
            ruleId: 'no-width-animation',
            line: lineNum,
            column: match.index + 1,
            match: match[0],
            message: `发现宽度过渡类 '${match[0]}'，禁止对宽度做动画`
          })
        }

        cssWidthTransitionRegex.lastIndex = 0
        while ((match = cssWidthTransitionRegex.exec(line)) !== null) {
          violations.push({
            ruleId: 'no-width-animation',
            line: lineNum,
            column: match.index + 1,
            match: match[0],
            message: `发现 CSS 宽度过渡 '${match[0]}'，请使用 transform`
          })
        }
      })
      return violations
    }
  },
  {
    id: 'no-excessive-radius',
    name: '过大圆角堆叠 (16px+)',
    description: '常规内容区使用 8px 栅格与 8px 圆角（rounded-lg / rounded-md），应用壳最多 12px 圆角（rounded-xl），禁止 rounded-2xl/3xl 及其方向变体',
    severity: 'error',
    check(content, file) {
      const violations = []
      const lines = content.split('\n')
      // Catches rounded-2xl, rounded-3xl, rounded-t-2xl, rounded-b-3xl, etc.
      // In domain components, rounded-xl is also restricted per D4.1
      const twExcessiveRadiusRegex = /\brounded-(?:(?:[trblse]{1,2}|s|e)-)?(?:xl|2xl|3xl)\b/g

      lines.forEach((line, idx) => {
        const lineNum = idx + 1
        const trimmed = line.trim()
        if (trimmed.startsWith('//') || trimmed.startsWith('/*') || trimmed.startsWith('*')) return

        let match
        twExcessiveRadiusRegex.lastIndex = 0
        while ((match = twExcessiveRadiusRegex.exec(line)) !== null) {
          violations.push({
            ruleId: 'no-excessive-radius',
            line: lineNum,
            column: match.index + 1,
            match: match[0],
            message: `发现过大圆角 '${match[0]}'，内容区应使用 8px 圆角（rounded-lg/rounded-md），避免大圆角堆叠`
          })
        }
      })
      return violations
    }
  },
  {
    id: 'no-excessive-shadows',
    name: '过大阴影堆叠',
    description: '禁止使用 shadow-lg/xl/2xl 等大阴影，以 1px hairline 分区代替堆叠悬浮投影',
    severity: 'error',
    check(content, file) {
      const violations = []
      const lines = content.split('\n')
      const twExcessiveShadowRegex = /\bshadow-(?:lg|xl|2xl)\b/g

      lines.forEach((line, idx) => {
        const lineNum = idx + 1
        const trimmed = line.trim()
        if (trimmed.startsWith('//') || trimmed.startsWith('/*') || trimmed.startsWith('*')) return

        let match
        twExcessiveShadowRegex.lastIndex = 0
        while ((match = twExcessiveShadowRegex.exec(line)) !== null) {
          violations.push({
            ruleId: 'no-excessive-shadows',
            line: lineNum,
            column: match.index + 1,
            match: match[0],
            message: `发现过大阴影 '${match[0]}'，请使用 1px hairline 分区或紧凑 shadow-xs`
          })
        }
      })
      return violations
    }
  },
  {
    id: 'no-scoped-styles',
    name: '新增 Scoped CSS',
    description: '禁止在 Vue SFC 中新增 <style scoped>，领域组件应统一消费 Tailwind 语义类与共享原语',
    severity: 'error',
    check(content, file) {
      const violations = []
      const scopedMatch = content.match(/<style[^>]*\bscoped\b[^>]*>/i)
      if (scopedMatch) {
        // Find line number
        const preLines = content.slice(0, scopedMatch.index).split('\n')
        violations.push({
          ruleId: 'no-scoped-styles',
          line: preLines.length,
          column: (preLines[preLines.length - 1] || '').length + 1,
          match: scopedMatch[0],
          message: `发现 <style scoped> 块，领域组件禁止使用独立 scoped 样式`
        })
      }
      return violations
    }
  }
]

/**
 * Explicit, minimal allowlist.
 * Global wildcards or whole-directory exemptions are strictly forbidden.
 */
export const DEFAULT_ALLOWLIST = [
  {
    file: 'src/assets/theme.css',
    ruleId: 'no-hardcoded-colors',
    reason: 'Root design token definitions in Tailwind theme file'
  }
]

/**
 * Validates that an allowlist contains only explicit, minimal, valid entries.
 * Rejects wildcards, whole-directory bypasses, or domain view exemptions.
 */
export function validateAllowlist(allowlist) {
  if (!Array.isArray(allowlist)) {
    throw new Error('Allowlist must be an array')
  }

  for (const entry of allowlist) {
    if (!entry.file || typeof entry.file !== 'string') {
      throw new Error(`Invalid allowlist entry: missing 'file'`)
    }
    if (!entry.ruleId || typeof entry.ruleId !== 'string') {
      throw new Error(`Invalid allowlist entry for ${entry.file}: missing 'ruleId'`)
    }
    if (!entry.reason || typeof entry.reason !== 'string') {
      throw new Error(`Invalid allowlist entry for ${entry.file}: missing required 'reason'`)
    }
    // Strict prohibition against wildcards
    if (entry.file.includes('*') || entry.ruleId === '*') {
      throw new Error(`Wildcards are strictly prohibited in allowlist: file='${entry.file}', ruleId='${entry.ruleId}'`)
    }
    // Strict prohibition against exempting domain views
    if (entry.file.startsWith('src/views/') || entry.file.startsWith('views/')) {
      throw new Error(`Exempting domain views is strictly prohibited: '${entry.file}'`)
    }
  }

  return true
}

/**
 * Checks if a specific file and rule combination is allowlisted.
 */
export function isAllowlisted(filePath, ruleId, allowlist = DEFAULT_ALLOWLIST) {
  const normPath = filePath.replace(/\\/g, '/').replace(/^.*?\/(src\/)/, '$1').replace(/^src\//, '')
  return allowlist.some(entry => {
    const entryPath = entry.file.replace(/\\/g, '/').replace(/^src\//, '')
    return (normPath === entryPath || normPath.endsWith(entryPath)) && entry.ruleId === ruleId
  })
}

/**
 * Scans a single file's content against all active visual gate rules.
 */
export function scanContent(content, filePath, options = {}) {
  const allowlist = options.allowlist || DEFAULT_ALLOWLIST
  const rules = options.rules || RULES
  const activeRuleIds = options.ruleIds ? new Set(options.ruleIds) : null

  const fileViolations = []
  let allowlistedCount = 0

  for (const rule of rules) {
    if (activeRuleIds && !activeRuleIds.has(rule.id)) {
      continue
    }

    if (isAllowlisted(filePath, rule.id, allowlist)) {
      allowlistedCount++
      continue
    }

    const detected = rule.check(content, filePath)
    for (const v of detected) {
      fileViolations.push({
        ...v,
        file: filePath,
        ruleName: rule.name,
        severity: rule.severity
      })
    }
  }

  return {
    file: filePath,
    violations: fileViolations,
    allowlistedCount
  }
}

/**
 * Recursively discovers candidate files for visual audit.
 */
export function discoverAuditFiles(srcDir, options = {}) {
  const exts = options.extensions || ['.vue', '.css']
  const discovered = []

  function walk(currentDir) {
    if (!fs.existsSync(currentDir)) return
    const entries = fs.readdirSync(currentDir, { withFileTypes: true })
    for (const entry of entries) {
      const fullPath = path.join(currentDir, entry.name)
      if (entry.isDirectory()) {
        if (entry.name === 'node_modules' || entry.name === 'dist' || entry.name === '.git') {
          continue
        }
        walk(fullPath)
      } else if (entry.isFile()) {
        const ext = path.extname(entry.name)
        if (exts.includes(ext)) {
          discovered.push(fullPath)
        }
      }
    }
  }

  walk(srcDir)
  return discovered
}

/**
 * Scans an entire frontend directory or file list and aggregates visual gate results.
 */
export function runVisualAudit(targetDir, options = {}) {
  const allowlist = options.allowlist || DEFAULT_ALLOWLIST
  validateAllowlist(allowlist)

  const files = options.files || discoverAuditFiles(targetDir, options)
  const allViolations = []
  const violationsByRule = {}
  const violationsByFile = {}
  let totalAllowlisted = 0

  for (const rule of RULES) {
    violationsByRule[rule.id] = 0
  }

  const scannedFiles = []

  for (const file of files) {
    const relPath = path.relative(targetDir, file).replace(/\\/g, '/')
    scannedFiles.push(relPath)
    const content = fs.readFileSync(file, 'utf8')
    const result = scanContent(content, relPath, { ...options, allowlist })

    totalAllowlisted += result.allowlistedCount

    if (result.violations.length > 0) {
      violationsByFile[relPath] = result.violations.length
      for (const v of result.violations) {
        allViolations.push(v)
        violationsByRule[v.ruleId] = (violationsByRule[v.ruleId] || 0) + 1
      }
    }
  }

  return {
    targetDir,
    totalFiles: files.length,
    scannedFiles,
    filesWithViolations: Object.keys(violationsByFile).length,
    totalViolations: allViolations.length,
    totalAllowlisted,
    violationsByRule,
    violationsByFile,
    violations: allViolations
  }
}

/**
 * Formats the visual audit report for CLI display.
 */
export function formatReport(auditResult, options = {}) {
  const { totalFiles, filesWithViolations, totalViolations, violationsByRule, violationsByFile, violations } = auditResult
  const lines = []

  lines.push('======================================================================')
  lines.push('       CSP 前端视觉基线与负面清单审计 (Visual Gate Baseline Audit)     ')
  lines.push('======================================================================')
  lines.push(`扫描文件总数: ${totalFiles} 个`)
  lines.push(`命中违规文件: ${filesWithViolations} 个`)
  lines.push(`发现违规项数: ${totalViolations} 处`)
  lines.push('----------------------------------------------------------------------')
  lines.push('违规规则分类汇总:')

  for (const rule of RULES) {
    const count = violationsByRule[rule.id] || 0
    const marker = count > 0 ? ' [HIT]' : ' [PASS]'
    lines.push(`  ${marker.padEnd(8)} ${rule.id.padEnd(22)} (${rule.name}): ${count} 处`)
  }

  lines.push('----------------------------------------------------------------------')
  lines.push('文件违规热点统计:')
  const sortedFiles = Object.entries(violationsByFile).sort((a, b) => b[1] - a[1])
  for (const [file, count] of sortedFiles) {
    lines.push(`  ${file.padEnd(45)}: ${count} 处违规`)
  }

  if (options.verbose && violations.length > 0) {
    lines.push('----------------------------------------------------------------------')
    lines.push('详细违规项明细:')
    for (const v of violations.slice(0, options.limit || 100)) {
      lines.push(`  - ${v.file}:${v.line}:${v.column} [${v.ruleId}] ${v.message}`)
      lines.push(`    匹配内容: ${v.match}`)
    }
    if (violations.length > (options.limit || 100)) {
      lines.push(`  ... 另有 ${violations.length - (options.limit || 100)} 处省略`)
    }
  }

  lines.push('======================================================================')
  return lines.join('\n')
}

// CLI Execution entrypoint
const isMain = process.argv[1] && fileURLToPath(import.meta.url) === path.resolve(process.argv[1])
if (isMain) {
  const args = process.argv.slice(2)
  const isStrict = args.includes('--strict')
  const isJson = args.includes('--json')
  const isVerbose = args.includes('--verbose') || args.includes('-v')

  // Resolve frontend src directory dynamically relative to this script
  const scriptDir = import.meta.dirname
  const srcDir = path.resolve(scriptDir, '../src')

  const result = runVisualAudit(srcDir)

  if (isJson) {
    console.log(JSON.stringify(result, null, 2))
  } else {
    console.log(formatReport(result, { verbose: isVerbose || !isStrict }))
  }

  if (isStrict && result.totalViolations > 0) {
    console.error(`\n[门禁拦截] 发现 ${result.totalViolations} 处视觉负面清单违规，CI 门禁已阻断！`)
    process.exit(1)
  } else {
    console.log(`\n[基线扫描完毕] 记录当前基线违规项供阶段 5.2 - 5.7 逐一消解。`)
    process.exit(0)
  }
}
