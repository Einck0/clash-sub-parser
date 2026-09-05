import { defineConfig } from '@playwright/test'

const e2eBaseUrl = process.env.E2E_BASE_URL

export default defineConfig({
  testDir: './e2e',
  timeout: 30_000,
  forbidOnly: !!process.env.CI,
  retries: process.env.CI ? 1 : 0,
  reporter: 'list',
  use: {
    baseURL: e2eBaseUrl || 'http://127.0.0.1:4173',
    trace: 'retain-on-failure',
    screenshot: 'only-on-failure',
  },
  webServer: e2eBaseUrl
    ? undefined
    : {
        command: 'npm run preview -- --port 4173 --host 127.0.0.1',
        port: 4173,
        reuseExistingServer: true,
      },
})
