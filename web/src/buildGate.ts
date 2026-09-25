import fs from 'node:fs'
import path from 'node:path'

export interface SyncWebassetsOptions {
  srcDir: string
  defaultTargetDir: string
  env?: NodeJS.ProcessEnv
}

/**
 * Safely synchronizes built frontend assets into internal/webassets/dist ONLY when
 * explicitly opted in via COPY_WEBASSETS=1 and not disabled via NO_COPY_WEBASSETS=1.
 * Prevents default/accidental main-repository writes while supporting clean isolated builds.
 */
export function syncDistToWebassets(options: SyncWebassetsOptions): boolean {
  const env = options.env ?? process.env
  if (env.NO_COPY_WEBASSETS === '1' || env.COPY_WEBASSETS !== '1') {
    return false
  }
  const targetDir = env.WEBASSETS_DIST_DIR
    ? path.resolve(env.WEBASSETS_DIST_DIR)
    : path.resolve(options.defaultTargetDir)
  if (!fs.existsSync(options.srcDir)) {
    return false
  }
  fs.rmSync(targetDir, { recursive: true, force: true })
  fs.mkdirSync(targetDir, { recursive: true })
  fs.cpSync(options.srcDir, targetDir, { recursive: true })
  return true
}
