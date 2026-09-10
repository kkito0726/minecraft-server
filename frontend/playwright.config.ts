import { defineConfig, devices } from '@playwright/test'

// E2E は実際の mcadmind バイナリに対して実行する。
// docker は e2e/fixtures/fake-docker.sh に差し替え、実コンテナを起動しない
// （ADMIN_DOCKER_BIN で差し替えられるようにしてあるのはこのため）。
//
// baseURL はここには書かない。試験ごとに専用の mcadmind を別ポートで立て、
// e2e/fixtures/test.ts の fixture が baseURL を差し替える。共有すると、
// 前の試験が書き換えた .env とワールドの上で次の試験が動いてしまう。
export default defineConfig({
  testDir: './e2e',
  fullyParallel: false, // 排他ロックの検証があるため直列に実行する
  forbidOnly: !!process.env.CI,
  retries: process.env.CI ? 2 : 0,
  workers: 1,
  reporter: process.env.CI ? [['github'], ['html', { open: 'never' }]] : 'list',
  use: {
    trace: 'on-first-retry',
    video: 'retain-on-failure',
    screenshot: 'only-on-failure',
  },
  projects: [{ name: 'chromium', use: { ...devices['Desktop Chrome'] } }],
})
