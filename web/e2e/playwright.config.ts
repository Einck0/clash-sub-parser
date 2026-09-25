// @ts-nocheck
import { defineConfig } from '@playwright/test'

const port = process.env.PORT || '5173'
const baseURL = process.env.BASE_URL || `http://127.0.0.1:${port}`

export default defineConfig({
  testDir: './',
  testMatch: /.*auth-real-server\.spec\.ts/,
  timeout: 30_000,
  expect: {
    timeout: 5000,
  },
  fullyParallel: false,
  forbidOnly: !!process.env.CI,
  retries: 0,
  workers: 1,
  reporter: 'list',
  use: {
    baseURL,
    browserName: 'chromium',
    trace: 'on-first-retry',
  },
  projects: [
    {
      name: 'Mobile-375',
      use: {
        browserName: 'chromium',
        viewport: { width: 375, height: 667 },
        isMobile: true,
        hasTouch: true,
      },
    },
    {
      name: 'Mobile-390',
      use: {
        browserName: 'chromium',
        viewport: { width: 390, height: 844 },
        isMobile: true,
        hasTouch: true,
      },
    },
    {
      name: 'Desktop-1440',
      use: {
        browserName: 'chromium',
        viewport: { width: 1440, height: 900 },
        isMobile: false,
        hasTouch: false,
      },
    },
  ],
})
