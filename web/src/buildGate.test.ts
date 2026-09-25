import { describe, it, expect, beforeEach, afterEach } from 'vitest'
import fs from 'node:fs'
import os from 'node:os'
import path from 'node:path'
import { syncDistToWebassets } from './buildGate'

describe('vite.config.ts copy-to-webassets closeBundle safety gate', () => {
  let tmpRoot: string
  let srcDir: string
  let targetDir: string
  let sentinelPath: string

  beforeEach(() => {
    tmpRoot = fs.mkdtempSync(path.join(os.tmpdir(), 'csp-vite-gate-'))
    srcDir = path.join(tmpRoot, 'web-dist')
    targetDir = path.join(tmpRoot, 'webassets-dist')
    fs.mkdirSync(srcDir, { recursive: true })
    fs.mkdirSync(targetDir, { recursive: true })
    fs.writeFileSync(path.join(srcDir, 'index.html'), '<!doctype html><script src="/assets/new.js"></script>', 'utf8')
    sentinelPath = path.join(targetDir, 'sentinel-original-asset.txt')
    fs.writeFileSync(sentinelPath, 'protected-user-asset-bytes', 'utf8')
  })

  afterEach(() => {
    fs.rmSync(tmpRoot, { recursive: true, force: true })
  })

  it('enforces explicit opt-in COPY_WEBASSETS=1, NO_COPY_WEBASSETS=1, and apply: build guard in vite.config.ts', () => {
    const viteConfigText = fs.readFileSync(path.resolve(__dirname, '../vite.config.ts'), 'utf8')
    expect(viteConfigText).toContain("apply: 'build'")
    expect(viteConfigText).toContain("process.env.NO_COPY_WEBASSETS === '1' || process.env.COPY_WEBASSETS !== '1'")
    expect(viteConfigText).toContain('process.env.WEBASSETS_DIST_DIR')
  })

  it('does not write or overwrite target webassets when COPY_WEBASSETS is not set (default build)', () => {
    const copied = syncDistToWebassets({
      srcDir,
      defaultTargetDir: targetDir,
      env: {},
    })

    expect(copied).toBe(false)
    expect(fs.existsSync(sentinelPath)).toBe(true)
    expect(fs.readFileSync(sentinelPath, 'utf8')).toBe('protected-user-asset-bytes')
    expect(fs.readdirSync(targetDir)).toEqual(['sentinel-original-asset.txt'])
  })

  it('does not write or overwrite target webassets when NO_COPY_WEBASSETS=1 even if COPY_WEBASSETS=1', () => {
    const copied = syncDistToWebassets({
      srcDir,
      defaultTargetDir: targetDir,
      env: {
        NO_COPY_WEBASSETS: '1',
        COPY_WEBASSETS: '1',
      },
    })

    expect(copied).toBe(false)
    expect(fs.existsSync(sentinelPath)).toBe(true)
    expect(fs.readFileSync(sentinelPath, 'utf8')).toBe('protected-user-asset-bytes')
    expect(fs.readdirSync(targetDir)).toEqual(['sentinel-original-asset.txt'])
  })

  it('cleanly replaces target webassets only when COPY_WEBASSETS=1 and NO_COPY_WEBASSETS!=1', () => {
    const copied = syncDistToWebassets({
      srcDir,
      defaultTargetDir: targetDir,
      env: {
        COPY_WEBASSETS: '1',
      },
    })

    expect(copied).toBe(true)
    expect(fs.existsSync(sentinelPath)).toBe(false)
    expect(fs.existsSync(path.join(targetDir, 'index.html'))).toBe(true)
  })
})
