import { defineConfig, devices } from '@playwright/test'

// E2E は実際の mcadmind バイナリに対して実行する。
// docker は e2e/fixtures/fake-docker.sh に差し替え、実コンテナを起動しない
// （ADMIN_DOCKER_BIN で差し替えられるようにしてあるのはこのため）。
export default defineConfig({
  testDir: './e2e',
  fullyParallel: false, // 排他ロックの検証があるため直列に実行する
  forbidOnly: !!process.env.CI,
  retries: process.env.CI ? 2 : 0,
  workers: 1,
  reporter: process.env.CI ? [['github'], ['html', { open: 'never' }]] : 'list',
  use: {
    baseURL: 'http://127.0.0.1:8788',
    trace: 'on-first-retry',
    video: 'retain-on-failure',
    screenshot: 'only-on-failure',
  },
  projects: [{ name: 'chromium', use: { ...devices['Desktop Chrome'] } }],
})
