import { defineConfig, devices } from '@playwright/test'

const e2eDatabase = process.env.E2E_DATABASE_URL
export default defineConfig({
  testDir: './tests',
  fullyParallel: false,
  workers: 1,
  timeout: 45_000,
  reporter: 'list',
  use: {
    baseURL: 'http://127.0.0.1:18080',
    trace: 'retain-on-failure',
    screenshot: 'only-on-failure',
    channel: process.env.PLAYWRIGHT_CHANNEL || undefined,
    launchOptions: process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE
      ? { executablePath: process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE }
      : undefined,
  },
  projects: [
    {
      name: 'desktop',
      use: { ...devices['Desktop Chrome'], viewport: { width: 1440, height: 1000 } },
    },
    {
      name: 'mobile',
      use: {
        ...devices['Desktop Chrome'],
        viewport: { width: 375, height: 812 },
        isMobile: true,
        hasTouch: true,
      },
    },
  ],
  webServer: e2eDatabase
    ? {
        command: 'go run ./cmd/timememo',
        url: 'http://127.0.0.1:18080/healthz',
        reuseExistingServer: false,
        env: {
          DATABASE_URL: e2eDatabase,
          ADDR: '127.0.0.1:18080',
          UPLOAD_DIR: process.env.E2E_UPLOAD_DIR || '/tmp/timememo-e2e-uploads',
          COOKIE_SECURE: 'false',
        },
        timeout: 60_000,
      }
    : undefined,
})
