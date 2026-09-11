import { readdirSync, writeFileSync } from 'node:fs'
import { join } from 'node:path'

import { banner, exists, expect, signIn, test, waitForOperation } from './fixtures/test'

test.use({ worlds: ['world', 'creative'] })

/** 退避先（data/<名前>.broken-<日時>）を探す。 */
function quarantines(dir: string): string[] {
  return readdirSync(join(dir, 'data')).filter((name) => name.includes('.broken-'))
}

async function createBackup(page: import('@playwright/test').Page) {
  await page.getByRole('link', { name: 'バックアップ' }).click()
  await page.getByRole('button', { name: 'バックアップを取得' }).click()
  await page.getByRole('button', { name: '取得する' }).click()
  await waitForOperation(page)
}

test('取得したバックアップが一覧に出る', async ({ page }) => {
  await signIn(page)
  await createBackup(page)

  const row = page.getByRole('row').filter({ hasText: '.zip' })
  await expect(row).toHaveCount(1)
  // 表示するバージョンはファイル名ではなくアーカイブ内の level.dat から読む。
  await expect(row).toContainText('26.2')
  await expect(row).toContainText('world')
})

/**
 * REQ-114。復元は「退避してから展開」であって上書きではない。
 *
 * 上書き展開だと、アーカイブに含まれない新しい region ファイルが残り、
 * 古い地形と新しい地形が同居した壊れたワールドになる。しかも起動はする。
 */
test('復元すると取得後のファイルは消え、退避先に残る', async ({ page, server }) => {
  await signIn(page)
  await createBackup(page)

  writeFileSync(join(server.dir, 'data/world/after-backup.txt'), '取得の後に増えたもの')

  await page.getByRole('button', { name: '復元' }).click()
  const form = page.getByRole('form', { name: 'バックアップからの復元' })

  // アーカイブも現在のワールドも world なので、承諾は要らない。
  await expect(form.getByRole('checkbox')).toHaveCount(0)
  await form.getByLabel(/確認のため/).fill('world')
  await form.getByRole('button', { name: '復元する' }).click()
  await waitForOperation(page)

  expect(exists(server.dir, 'data/world/after-backup.txt')).toBe(false)

  const quarantined = quarantines(server.dir)
  expect(quarantined).toHaveLength(1)
  expect(exists(server.dir, 'data', quarantined[0]!, 'after-backup.txt')).toBe(true)
})

/**
 * REQ-109 / REQ-112 / REQ-123。取り消せない操作の関門。
 *
 * アーカイブのワールド名と稼働中の名前が食い違うと、そのまま戻しても
 * 稼働中のワールドは何も変わらない。承諾と名前の両方を要求する。
 */
test.describe('食い違いがあるときの復元', () => {
  test('名前と承諾の両方がそろうまで実行できない', async ({ page }) => {
    await signIn(page)
    await createBackup(page)

    // world のバックアップを持ったまま creative へ切り替える
    await page.getByRole('link', { name: 'ワールド' }).click()
    await page.getByRole('row', { name: /^creative / }).getByRole('button', { name: '切替' }).click()
    await waitForOperation(page)

    await page.getByRole('link', { name: 'バックアップ' }).click()
    await page.getByRole('button', { name: '復元' }).click()

    const form = page.getByRole('form', { name: 'バックアップからの復元' })
    const submit = form.getByRole('button', { name: '復元する' })

    // 食い違いはサーバーが警告文にして返す。画面はそれをそのまま出す。
    await expect(form).toContainText('稼働しているのは "creative"')
    await expect(form.getByRole('checkbox')).toBeVisible()
    await expect(submit).toBeDisabled()

    await form.getByLabel(/確認のため/).fill('world')
    await expect(submit).toBeDisabled() // 名前だけでは通らない

    await form.getByRole('checkbox').check()
    await expect(submit).toBeEnabled() // 両方そろって初めて押せる
  })

  test('復元先を変えると打ち込んだ名前が消える', async ({ page }) => {
    await signIn(page)
    await createBackup(page)

    await page.getByRole('link', { name: 'ワールド' }).click()
    await page.getByRole('row', { name: /^creative / }).getByRole('button', { name: '切替' }).click()
    await waitForOperation(page)

    await page.getByRole('link', { name: 'バックアップ' }).click()
    await page.getByRole('button', { name: '復元' }).click()

    const form = page.getByRole('form', { name: 'バックアップからの復元' })
    await form.getByLabel(/確認のため/).fill('world')
    await form.getByRole('radio', { name: /稼働中/ }).check()

    // 求められる名前が creative に変わる。入力が残っていると
    // 「合っているのに押せない」状態になり、理由が画面から読めない。
    await expect(form.getByLabel(/確認のため/)).toHaveValue('')
    await expect(form).toContainText('creative')
  })
})

test('バックアップは削除できる', async ({ page }) => {
  await signIn(page)
  await createBackup(page)

  await page.getByRole('button', { name: '削除' }).click()
  await page.getByRole('button', { name: '本当に削除' }).click()

  await expect(page.getByText('まだバックアップがありません。')).toBeVisible()
  await expect(banner(page)).toHaveCount(0) // 削除は操作にしない
})
