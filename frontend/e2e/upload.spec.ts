import { readFileSync } from 'node:fs'
import { join } from 'node:path'

import { banner, expect, signIn, test, waitForOperation } from './fixtures/test'
import { REPO_ROOT } from './fixtures/project'
import { makeZip } from './fixtures/zip'

// 実物の level.dat。バージョン判定が本物の NBT を通ることを確かめる。
const LEVEL_DAT = readFileSync(
  join(REPO_ROOT, 'backend/internal/infrastructure/leveldat/testdata/level.dat'),
)

/**
 * 外から持ち込んだ zip の取り込み（REQ-117）。
 *
 * 配布されているワールドは <名前>/level.dat の形をしている。展開側の
 * 制限（data/ 配下のみ）を緩めずに受け入れるため、取り込みの時点で
 * 包み直す。ここが壊れると「持ち込めない」か「data/ の外に書ける」の
 * どちらかになり、後者は致命的になる。
 */
test.describe('zip の取り込み', () => {
  test('配布ワールドの形を取り込んで一覧に出す', async ({ page }) => {
    await signIn(page)
    await page.getByRole('link', { name: 'バックアップ' }).click()
    await page.getByRole('button', { name: 'zip を取り込む' }).click()

    const zip = makeZip({ 'MyWorld/level.dat': 'nbt', 'MyWorld/region/r.0.0.mca': 'chunk' })
    await page.getByLabel('取り込む zip').setInputFiles({
      name: 'MyWorld.zip',
      mimeType: 'application/zip',
      buffer: zip,
    })
    await page.getByRole('button', { name: '取り込む', exact: true }).click()

    await expect(page.getByText(/取り込みました/)).toBeVisible()
    await expect(page.getByText(/包み直しました/)).toBeVisible()

    // 一覧に出て、ワールド名が読めている。
    const row = page.getByRole('row').filter({ hasText: 'MyWorld' })
    await expect(row).toHaveCount(1)

    // 取り込んだだけでは操作にならない。ワールドは動いていない。
    await expect(banner(page)).toHaveCount(0)
  })

  test('data/ 配下の形はそのまま取り込む', async ({ page }) => {
    await signIn(page)
    await page.getByRole('link', { name: 'バックアップ' }).click()
    await page.getByRole('button', { name: 'zip を取り込む' }).click()

    await page.getByLabel('取り込む zip').setInputFiles({
      name: 'backup.zip',
      mimeType: 'application/zip',
      buffer: makeZip({ 'data/imported/level.dat': 'nbt' }),
    })
    await page.getByRole('button', { name: '取り込む', exact: true }).click()

    await expect(page.getByText(/取り込みました/)).toBeVisible()
    await expect(page.getByText(/包み直しました/)).toHaveCount(0)
  })

  /**
   * 何が悪いのかを画面で伝える。
   * 伝えないと、利用者は同じ zip を何度も送ることになる。
   */
  test('取り込めない zip は理由を出す', async ({ page }) => {
    await signIn(page)
    await page.getByRole('link', { name: 'バックアップ' }).click()
    await page.getByRole('button', { name: 'zip を取り込む' }).click()

    await page.getByLabel('取り込む zip').setInputFiles({
      name: 'notaworld.zip',
      mimeType: 'application/zip',
      buffer: makeZip({ 'readme.txt': 'ワールドではない' }),
    })
    await page.getByRole('button', { name: '取り込む', exact: true }).click()

    await expect(page.getByRole('alert')).toContainText('ワールド')
  })

  // 取り込んだワールドを、そのまま復元まで通せること。
  // ここまで通って初めて「持ち込んだワールドで遊べる」ことになる。
  test('取り込んだワールドを復元できる', async ({ page }) => {
    await signIn(page)
    await page.getByRole('link', { name: 'バックアップ' }).click()
    await page.getByRole('button', { name: 'zip を取り込む' }).click()

    await page.getByLabel('取り込む zip').setInputFiles({
      name: 'MyWorld.zip',
      mimeType: 'application/zip',
      buffer: makeZip({ 'MyWorld/level.dat': LEVEL_DAT }),
    })
    await page.getByRole('button', { name: '取り込む', exact: true }).click()
    await expect(page.getByText(/取り込みました/)).toBeVisible()

    await page.getByRole('button', { name: '復元' }).click()
    const form = page.getByRole('form', { name: 'バックアップからの復元' })

    // 稼働中は world なので名前が食い違う。承諾と名前の両方が要る。
    await form.getByLabel(/確認のため/).fill('MyWorld')
    await form.getByRole('checkbox').check()
    await form.getByRole('button', { name: '復元する' }).click()
    await waitForOperation(page)

    // 取り込んだワールドが data/ に現れ、MC_LEVEL もそちらを向く。
    await page.getByRole('link', { name: 'ワールド' }).click()
    await expect(page.getByRole('row', { name: /^MyWorld / })).toBeVisible()
  })
})
