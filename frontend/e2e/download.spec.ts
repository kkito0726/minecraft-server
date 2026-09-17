import { statSync } from 'node:fs'

import { expect, signIn, test, waitForOperation } from './fixtures/test'

/**
 * バックアップを手元の PC へ保存する。
 *
 * ブラウザのダウンロードはリンクを辿るだけで Authorization ヘッダーを
 * 付けられない。認証済みの RPC が短命の受取券を出し、受け取りの口は
 * その券だけを見る。ここで確かめたいのは、その経路が実物の mcadmind を
 * 通って本当にファイルとして落ちてくることと、券が無ければ渡さないこと。
 */
test.describe('バックアップの保存', () => {
  test('一覧から手元へ保存できる', async ({ page }) => {
    await signIn(page)
    await page.getByRole('link', { name: 'バックアップ' }).click()
    await page.getByRole('button', { name: 'バックアップを取得' }).click()
    await page.getByRole('button', { name: '取得する' }).click()
    await waitForOperation(page)

    const [download] = await Promise.all([
      page.waitForEvent('download'),
      page.getByRole('button', { name: 'ダウンロード' }).click(),
    ])

    // 保存されるのはアーカイブそのもの。名前は一覧のファイル名と同じ。
    expect(download.suggestedFilename()).toMatch(/^backup-.*\.zip$/)

    const path = await download.path()
    expect(path).not.toBeNull()
    expect(statSync(path).size).toBeGreaterThan(0)
  })

  // 券が無ければ渡さない。ここが素通りすると、tailnet の中の誰でも
  // ワールドを丸ごと持ち出せることになる。
  test('受取券が無ければ渡さない', async ({ page, server }) => {
    const missing = await page.request.get(`${server.url}/download/backup`)
    expect(missing.status()).toBe(404)

    const bogus = await page.request.get(`${server.url}/download/backup?t=%E3%81%AB%E3%81%9B%E3%82%82%E3%81%AE`)
    expect(bogus.status()).toBe(404)
  })
})
