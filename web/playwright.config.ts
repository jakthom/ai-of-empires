import { defineConfig, devices } from '@playwright/test';
import { fileURLToPath } from 'node:url';

const root = fileURLToPath(new URL('..', import.meta.url));
const baseURL = 'http://127.0.0.1:18080';

export default defineConfig({
  testDir: './tests/e2e',
  fullyParallel: true,
  forbidOnly: !!process.env.CI,
  retries: process.env.CI ? 1 : 0,
  workers: 2,
  timeout: 30_000,
  expect: { timeout: 8_000 },
  outputDir: './test-results',
  reporter: [['list'], ['html', { open: 'never', outputFolder: 'playwright-report' }]],
  use: {
    baseURL,
    viewport: { width: 1440, height: 960 },
    deviceScaleFactor: 1,
    reducedMotion: 'reduce',
    trace: 'retain-on-failure',
    screenshot: 'only-on-failure',
    video: 'retain-on-failure',
    actionTimeout: 8_000,
  },
  projects: [
    { name: 'chrome', use: { ...devices['Desktop Chrome'], channel: 'chrome', viewport: { width: 1440, height: 960 } } },
    { name: 'chromium', use: { ...devices['Desktop Chrome'], channel: 'chromium', viewport: { width: 1440, height: 960 } } },
  ],
  webServer: {
    command: 'make build && exec ./bin/ai-of-empires -addr 127.0.0.1:18080 -db :memory:',
    cwd: root,
    url: `${baseURL}/api/v1/health`,
    reuseExistingServer: false,
    timeout: 120_000,
    gracefulShutdown: { signal: 'SIGTERM', timeout: 5_000 },
  },
});
