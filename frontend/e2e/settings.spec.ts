import { banner, envValue, expect, signIn, test, waitForOperation } from './fixtures/test'

/**
 * ゲーム設定（難易度・MOTD・人数・距離）。
 *
 * .env が正で、反映はコンテナの作り直しで行う。確かめたいのは、画面の値が
 * 実物の mcadmind を通って .env に届くことと、保存だけのときはサーバーに
 * 触らないこと。E2E の .env にはこれらのキーが最初は無いので、
 * 行の追記も同時に確かめている。
 */
test.describe('ゲーム設定', () => {
  test('保存だけなら .env に書き、サーバーは作り直さない', async ({ page, server }) => {
    await signIn(page)
    await page.getByRole('link', { name: '設定' }).click()

    // 「ハードコア」がモードの選択肢に並ぶので、難易度の組に絞る。
    await page
      .getByRole('group', { name: '難易度' })
      .getByRole('radio', { name: /ハード/ })
      .check()
    await page.getByLabel('描画距離').fill('9')
    await page.getByLabel('シミュレーション距離').fill('6')
    await page.getByRole('button', { name: '保存だけする' }).click()

    await expect(page.getByText(/保存しました。まだ反映していません/)).toBeVisible()
    // 保存だけは操作にしない。進捗の帯が出ていれば作り直しが走っている。
    await expect(banner(page)).toHaveCount(0)

    expect(envValue(server.dir, 'MC_DIFFICULTY')).toBe('hard')
    expect(envValue(server.dir, 'MC_VIEW_DISTANCE')).toBe('9')
    expect(envValue(server.dir, 'MC_SIMULATION_DISTANCE')).toBe('6')
    // 既にある行は壊さない。日本語と色コードを含む MOTD の引用もそのまま残る。
    expect(envValue(server.dir, 'MC_MOTD')).toBe('"§aE2E のサーバー"')
  })

  test('今すぐ反映すると作り直しの操作が走り、画面に戻ると保存した値が出る', async ({ page, server }) => {
    await signIn(page)
    await page.getByRole('link', { name: '設定' }).click()

    await page.getByLabel('最大人数').fill('8')
    await page.getByRole('button', { name: '保存して今すぐ反映' }).click()

    await expect(banner(page)).toContainText('ゲーム設定の反映')
    await waitForOperation(page)

    expect(envValue(server.dir, 'MC_MAX_PLAYERS')).toBe('8')

    // 読み直した値で入力欄が作り直されている。
    await page.reload()
    await expect(page.getByLabel('最大人数')).toHaveValue('8')
  })

  // 規則はバックエンドが正だが、往復させる理由はない。送る前に止める。
  test('規則外の値は送る前に弾かれ、.env は変わらない', async ({ page, server }) => {
    await signIn(page)
    await page.getByRole('link', { name: '設定' }).click()

    await page.getByLabel('シミュレーション距離').fill('20')

    await expect(page.getByRole('alert')).toContainText('描画距離（7）以下')
    await expect(page.getByRole('button', { name: '保存だけする' })).toBeDisabled()
    expect(envValue(server.dir, 'MC_SIMULATION_DISTANCE')).toBe('')
  })
})
