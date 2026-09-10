import { banner, envValue, expect, signIn, test, waitForOperation } from './fixtures/test'

test.use({ worlds: ['world', 'creative'] })

/**
 * ワールドの切り替え（REQ-011）。
 *
 * LEVEL は起動時に一度しか読まれないため、切り替えは必ずサーバーの
 * 再作成を伴う。切り替え前のワールドは 1 バイトも動かない。
 */
test.describe('ワールドの切り替え', () => {
  test('切り替えると稼働中の印が移り、MC_LEVEL が書き換わる', async ({ page, server }) => {
    await signIn(page)
    await page.getByRole('link', { name: 'ワールド' }).click()

    const worldRow = page.getByRole('row', { name: /^world / })
    const creativeRow = page.getByRole('row', { name: /^creative / })
    await expect(worldRow.getByText('稼働中')).toBeVisible()
    await expect(creativeRow.getByText('稼働中')).toHaveCount(0)

    await creativeRow.getByRole('button', { name: '切替' }).click()

    // 手順が画面に出る。押した直後から出るので、待たされていることが分かる。
    await expect(banner(page)).toBeVisible()
    await expect(banner(page)).toContainText('ワールドの切り替え')
    await waitForOperation(page)

    await expect(creativeRow.getByText('稼働中')).toBeVisible()
    await expect(worldRow.getByText('稼働中')).toHaveCount(0)
    expect(envValue(server.dir, 'MC_LEVEL')).toBe('creative')
  })

  // 切り替えても元のワールドは残る。いつでも戻せる。
  test('切り替えても元のワールドは一覧から消えない', async ({ page }) => {
    await signIn(page)
    await page.getByRole('link', { name: 'ワールド' }).click()

    await page.getByRole('row', { name: /^creative / }).getByRole('button', { name: '切替' }).click()
    await waitForOperation(page)

    await expect(page.getByRole('row', { name: /^world / })).toBeVisible()
  })
})

/**
 * 排他（REQ-101）。操作は同時に 1 つだけ。
 *
 * サーバー側は二本目の Start を ErrLocked で拒むが、押せてしまうと
 * 「押したのに何も起きない」ように見える。画面にも映す。
 */
test.describe('操作中の画面', () => {
  test.use({ readyDelaySeconds: 4 })

  test('操作の実行中は他の変更ができない', async ({ page }) => {
    await signIn(page)
    await page.getByRole('link', { name: 'ワールド' }).click()

    await page.getByRole('row', { name: /^creative / }).getByRole('button', { name: '切替' }).click()
    await expect(banner(page)).toBeVisible()

    // 同じ画面の変更ボタン
    await expect(page.getByRole('button', { name: '新規作成' })).toBeDisabled()
    await expect(
      page.getByRole('row', { name: /^world / }).getByRole('button', { name: '複製' }),
    ).toBeDisabled()

    // 別の画面の変更ボタンも同じ操作に従う
    await page.getByRole('link', { name: 'サーバー' }).click()
    await expect(page.getByRole('button', { name: '停止' })).toBeDisabled()
    await expect(page.getByRole('button', { name: '再起動' })).toBeDisabled()

    await waitForOperation(page)
    await expect(page.getByRole('button', { name: '再起動' })).toBeEnabled()
  })
})

/**
 * ワールドの新規作成（REQ-012）。
 *
 * ディレクトリは作らず Paper に生成させるため、確かめられるのは
 * 「MC_LEVEL と MC_SEED が書かれて起動したこと」まで。
 * 実際の生成は Paper の仕事で、ここでは fake-docker が代役をしている。
 */
test('新しいワールドを作るとシードごと設定に書かれる', async ({ page, server }) => {
  await signIn(page)
  await page.getByRole('link', { name: 'ワールド' }).click()

  await page.getByRole('button', { name: '新規作成' }).click()
  await page.getByLabel('ワールド名').fill('newworld')
  await page.getByLabel(/シード/).fill('12345')
  await page.getByRole('button', { name: '作成する' }).click()
  await waitForOperation(page)

  expect(envValue(server.dir, 'MC_LEVEL')).toBe('newworld')
  expect(envValue(server.dir, 'MC_SEED')).toBe('12345')
})

/** 名前の規則はフロントとバックで同一（NFR-304）。送る前に止まる。 */
test('使えない名前は送る前に弾かれる', async ({ page }) => {
  await signIn(page)
  await page.getByRole('link', { name: 'ワールド' }).click()

  await page.getByRole('button', { name: '新規作成' }).click()
  await page.getByLabel('ワールド名').fill('../etc')

  await expect(page.getByRole('button', { name: '作成する' })).toBeDisabled()
  await expect(banner(page)).toHaveCount(0)
})
