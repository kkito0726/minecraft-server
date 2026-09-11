import { writeFileSync } from 'node:fs'
import { join } from 'node:path'

import { banner, expect, signIn, test, waitForOperation } from './fixtures/test'

/**
 * リロードしても進捗が復帰する（REQ-202）。
 *
 * この試験は WatchOperation(from_seq) のリプレイの回帰試験である。
 * サーバーが from_seq 以降を流し直すからこそ、リロード直後の画面が
 * スピナーだけにならず、押す前からのログが全部埋まる。
 *
 * ここが壊れると「復元の途中で画面を閉じたら何が起きているか分からない」
 * という、いちばん怖い状態になる。しかも壊れても他の試験は通る。
 */
test.describe('操作中のリロード', () => {
  // 起動の確認で待たせて、リロードの余地を作る。
  test.use({ readyDelaySeconds: 8 })

  test('リロードしても進捗とログが残る', async ({ page, server }) => {
    await signIn(page)

    // 復元を選ぶのは、7 手順あって停止と起動を挟む最も長い操作だから。
    await page.getByRole('link', { name: 'バックアップ' }).click()
    await page.getByRole('button', { name: 'バックアップを取得' }).click()
    await page.getByRole('button', { name: '取得する' }).click()
    await waitForOperation(page)

    writeFileSync(join(server.dir, 'data/world/before-reload.txt'), 'x')

    await page.getByRole('button', { name: '復元' }).click()
    const form = page.getByRole('form', { name: 'バックアップからの復元' })
    await form.getByLabel(/確認のため/).fill('world')
    await form.getByRole('button', { name: '復元する' }).click()

    // 最後の手順（サーバーの起動）で待たされている状態を捕まえる
    await expect(banner(page)).toContainText('サーバーを起動しています')
    await page.getByRole('button', { name: /^ログ \(/ }).click()
    // ナビゲーションの項目を数えないよう、帯の中だけを見る。
    const linesBefore = await banner(page).getByRole('listitem').allInnerTexts()
    expect(linesBefore.length).toBeGreaterThan(3)

    await page.reload()

    // 覚えている操作 ID は無い。GetActiveOperation で見つけ直している。
    await expect(banner(page)).toBeVisible()
    await expect(banner(page)).toContainText('バックアップからの復元')

    await page.getByRole('button', { name: /^ログ \(/ }).click()
    const linesAfter = await banner(page).getByRole('listitem').allInnerTexts()

    // リプレイが効いていれば、押す前からのログが全部戻っている。
    // 追従だけならリロード後の分しか無く、ここで足りなくなる。
    expect(linesAfter.length).toBeGreaterThanOrEqual(linesBefore.length)
    for (const line of linesBefore) {
      expect(linesAfter).toContain(line)
    }

    await waitForOperation(page)
  })

  /**
   * 別のタブや別の端末から始めた操作も見つける。
   *
   * ミューテーションの応答を持っていないので、GetActiveOperation を
   * 定期的に叩くことでしか気づけない。
   */
  test('別の画面で始めた操作に気づく', async ({ page, context }) => {
    await signIn(page)

    const other = await context.newPage()
    await other.goto('/')
    await other.getByRole('link', { name: 'サーバー' }).click()
    await other.getByRole('button', { name: '再起動' }).click()

    // こちらは何も押していないが、発見の問い合わせで見つける。
    await expect(banner(page)).toBeVisible({ timeout: 15_000 })
    await expect(banner(page)).toContainText('サーバーの再起動')

    await other.close()
    await waitForOperation(page)
  })
})
