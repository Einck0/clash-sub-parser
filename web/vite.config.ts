import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'
import path from 'node:path'
import fs from 'node:fs'

export default defineConfig({
  plugins: [
    vue(),
    (() => {
      let resolvedOutDir = path.resolve(__dirname, 'dist')
      return {
        name: 'copy-to-webassets',
        apply: 'build' as const,
        configResolved(config: any) {
          resolvedOutDir = path.resolve(config.root, config.build.outDir)
        },
        closeBundle() {
          if (process.env.NO_COPY_WEBASSETS === '1' || process.env.COPY_WEBASSETS !== '1') {
            return
          }
          const srcDir = resolvedOutDir
          const targetDir = process.env.WEBASSETS_DIST_DIR
            ? path.resolve(process.env.WEBASSETS_DIST_DIR)
            : path.resolve(__dirname, '../internal/webassets/dist')
          if (fs.existsSync(srcDir)) {
            fs.rmSync(targetDir, { recursive: true, force: true })
            fs.mkdirSync(targetDir, { recursive: true })
            fs.cpSync(srcDir, targetDir, { recursive: true })
          }
        },
      }
    })(),
  ],
  resolve: {
    alias: {
      '@': path.resolve(__dirname, './src'),
    },
  },
  server: {
    port: 5173,
    proxy: {
      '/api': 'http://127.0.0.1:18080',
      '/healthz': 'http://127.0.0.1:18080',
      '/readyz': 'http://127.0.0.1:18080',
    },
  },
  build: {
    outDir: 'dist',
    emptyOutDir: true,
  },
  test: {
    include: ['src/**/*.test.ts', 'e2e/**/*.test.ts'],
    exclude: ['**/*.spec.ts', 'node_modules', 'dist'],
  },
})
