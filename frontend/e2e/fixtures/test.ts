import { test as base } from '@playwright/test'
import type { Page } from '@playwright/test'
import { existsSync, readFileSync } from 'node:fs'
import { join } from 'node:path'

import { TOKEN } from './project'
import { startServer } from './server'
import type { TestServer } from './server'

type Options = {
  /** 起動が完了したことにするまでの秒数。進行中の画面を見る試験で延ばす。 */
  readyDelaySeconds: number
  /** 最初から置いておくワールド。先頭が MC_LEVEL になる。 */
  worlds: string[]
}

type Fixtures = {
  server: TestServer
}

/**
 * 試験ごとに専用の mcadmind を立て、その URL を baseURL にする。
 *
 * playwright.config.ts に固定の baseURL を書くと、再試行のときに
 * 前の試験が書き換えた .env の上で動いてしまう。
 */
export const test = base.extend<Options & Fixtures>({
  readyDelaySeconds: [0, { option: true }],
  worlds: [['world'], { option: true }],

  server: async ({ readyDelaySeconds, worlds }, use) => {
    const server = await startServer({ readyDelaySeconds, worlds })
    try {
      await use({ url: server.url, dir: server.dir })
    } finally {
      server.stop()
    }
  },

  baseURL: async ({ server }, use) => {
    await use(server.url)
  },
})

export { expect } from '@playwright/test'
export { TOKEN }

/** トークンを入れて中に入る。ほとんどの試験の前置き。 */
export async function signIn(page: Page): Promise<void> {
  await page.goto('/')
  await page.getByLabel('トークン').fill(TOKEN)
  await page.getByRole('button', { name: '入る' }).click()
  await page.getByRole('link', { name: 'サーバー' }).waitFor()
}

/** 進捗バナー。操作が始まると出て、終端に達しても閉じるまで残る。 */
export function banner(page: Page) {
  return page.getByRole('status', { name: '操作の進捗' })
}

/**
 * 操作が終端に達するまで待ち、バナーを閉じる。
 *
 * 終端の目印には「閉じる」の有無を使う。このボタンは終端でしか
 * 描画されないので、文言の一致より確実に判定できる。
 */
export async function waitForOperation(page: Page, timeout = 60_000): Promise<void> {
  const close = banner(page).getByRole('button', { name: '閉じる' })
  await close.waitFor({ timeout })
  await close.click()
  await banner(page).waitFor({ state: 'detached' })
}

/** プロジェクトの .env から 1 つのキーを読む。書き換えの確認に使う。 */
export function envValue(dir: string, key: string): string {
  const line = readFileSync(join(dir, '.env'), 'utf8')
    .split('\n')
    .find((l) => l.startsWith(`${key}=`))
  return line?.slice(key.length + 1).trim() ?? ''
}

/** プロジェクト内のパスが存在するか。退避や展開の確認に使う。 */
export function exists(dir: string, ...parts: string[]): boolean {
  return existsSync(join(dir, ...parts))
}
