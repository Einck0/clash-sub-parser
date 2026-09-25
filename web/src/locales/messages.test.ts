import { describe, it, expect } from 'vitest'
import { readFileSync, readdirSync, statSync } from 'node:fs'
import { resolve, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { zhCN, enUS } from './messages'

function getLeafKeys(obj: Record<string, any>, prefix = ''): string[] {
  let res: string[] = []
  for (const [k, v] of Object.entries(obj)) {
    const fullKey = prefix ? `${prefix}.${k}` : k
    if (v && typeof v === 'object' && !Array.isArray(v)) {
      res = res.concat(getLeafKeys(v, fullKey))
    } else {
      res.push(fullKey)
    }
  }
  return res
}

function getByPath(obj: Record<string, any>, pathStr: string): any {
  return pathStr.split('.').reduce((acc, part) => acc && acc[part], obj)
}

function walkDir(dir: string): string[] {
  let files: string[] = []
  for (const item of readdirSync(dir)) {
    const full = join(dir, item)
    if (statSync(full).isDirectory()) {
      files = files.concat(walkDir(full))
    } else if (full.endsWith('.vue') || full.endsWith('.ts') || full.endsWith('.js')) {
      files.push(full)
    }
  }
  return files
}

describe('i18n locale dictionaries (messages.ts)', () => {
  it('zh-CN and en-US dictionaries should have identical leaf keys without asymmetry', () => {
    const zhKeys = getLeafKeys(zhCN).sort()
    const enKeys = getLeafKeys(enUS).sort()

    const missingInEn = zhKeys.filter((k) => !enKeys.includes(k))
    const missingInZh = enKeys.filter((k) => !zhKeys.includes(k))

    expect(missingInEn, 'en-US dictionary is missing keys present in zh-CN').toEqual([])
    expect(missingInZh, 'zh-CN dictionary is missing keys present in en-US').toEqual([])
    expect(zhKeys.length).toBeGreaterThan(0)
    expect(zhKeys.length).toBe(enKeys.length)
  })

  it('no message value should be empty string or whitespace only', () => {
    const zhKeys = getLeafKeys(zhCN)
    for (const key of zhKeys) {
      const zhVal = getByPath(zhCN, key)
      const enVal = getByPath(enUS, key)

      expect(typeof zhVal, `zh-CN key "${key}" should be a string`).toBe('string')
      expect(typeof enVal, `en-US key "${key}" should be a string`).toBe('string')
      expect(zhVal.trim().length, `zh-CN key "${key}" should not be blank`).toBeGreaterThan(0)
      expect(enVal.trim().length, `en-US key "${key}" should not be blank`).toBeGreaterThan(0)
    }
  })

  it('preserves named interpolation placeholders between zh-CN and en-US', () => {
    const zhKeys = getLeafKeys(zhCN)
    const placeholderRegex = /\{([a-zA-Z0-9_]+)\}/g

    for (const key of zhKeys) {
      const zhVal = getByPath(zhCN, key) as string
      const enVal = getByPath(enUS, key) as string

      const zhPlaceholders = Array.from(zhVal.matchAll(placeholderRegex), (m) => m[1]).sort()
      const enPlaceholders = Array.from(enVal.matchAll(placeholderRegex), (m) => m[1]).sort()

      expect(
        enPlaceholders,
        `Placeholder mismatch between zh-CN and en-US for key "${key}"`
      ).toEqual(zhPlaceholders)
    }
  })

  it('covers all static t(...) keys and nav labelKeys used across web/src', () => {
    const srcDir = resolve(fileURLToPath(new URL('.', import.meta.url)), '..')
    const files = walkDir(srcDir)

    const keyUsages = new Set<string>()

    const tCallRegex = /\bt\(\s*['"]([a-zA-Z0-9_.-]+)['"]/g
    const navLabelRegex = /labelKey:\s*['"]([a-zA-Z0-9_.-]+)['"]/g

    for (const file of files) {
      // Exclude messages and test files from scanned usage targets
      if (file.includes('locales/messages.ts') || file.includes('.test.ts') || file.includes('.spec.ts')) {
        continue
      }
      const content = readFileSync(file, 'utf8')

      let match: RegExpExecArray | null
      while ((match = tCallRegex.exec(content)) !== null) {
        keyUsages.add(match[1])
      }
      while ((match = navLabelRegex.exec(content)) !== null) {
        keyUsages.add(match[1])
      }
    }

    const missingInZh: string[] = []
    const missingInEn: string[] = []

    for (const key of keyUsages) {
      if (getByPath(zhCN, key) === undefined) {
        missingInZh.push(key)
      }
      if (getByPath(enUS, key) === undefined) {
        missingInEn.push(key)
      }
    }

    expect(missingInZh, 'Keys used in code but missing from zh-CN').toEqual([])
    expect(missingInEn, 'Keys used in code but missing from en-US').toEqual([])
  })
})
