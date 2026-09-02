import { defineConfig, devices } from '@playwright/test'

const backendURL = 'http://127.0.0.1:8081'
const frontendURL = 'http://127.0.0.1:4175'
const databaseURL = process.env.E2E_DATABASE_URL ?? 'postgres://orbit:orbit@127.0.0.1:5434/orbit_chat?sslmode=disable'

export default defineConfig({
  testDir: './e2e',
  testMatch: 'postgres.spec.ts',
  fullyParallel: false,
  workers: 1,
  forbidOnly: !!process.env.CI,
  retries: process.env.CI ? 2 : 0,
  reporter: process.env.CI ? 'github' : 'list',
  use: {
    baseURL: frontendURL,
    trace: 'retain-on-failure',
    screenshot: 'only-on-failure',
  },
  projects: [{ name: 'chromium', use: { ...devices['Desktop Chrome'] } }],
  webServer: [
    {
      command: 'go run ./cmd/server',
      cwd: 'backend',
      env: {
        APP_ENV: 'test',
        DATABASE_URL: databaseURL,
        FRONTEND_ORIGIN: frontendURL,
        PORT: '8081',
        SEED_DEMO_DATA: 'true',
      },
      url: `${backendURL}/api/health`,
      reuseExistingServer: !process.env.CI,
      timeout: 120_000,
    },
    {
      command: 'npm run dev -- --host 127.0.0.1 --port 4175',
      env: { VITE_API_BASE_URL: backendURL },
      url: frontendURL,
      reuseExistingServer: !process.env.CI,
      timeout: 120_000,
    },
  ],
})
