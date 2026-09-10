import { banner, expect, signIn, test } from './fixtures/test'
import { TOKEN } from './fixtures/project'

/**
 * トークンによる認可（REQ-301）。
 *
 * Tailscale の到達制御の上に重ねる二段構えの、内側の一段。
 * ここが素通りすると、tailnet に入れる誰もがワールドを消せる。
 */
test.describe('トークンの入力', () => {
  test('トークンが無ければ画面に入れない', async ({ page }) => {
    await page.goto('/')

    await expect(page.getByLabel('トークン')).toBeVisible()
    // ゲートの外ではどのルートも描画しない。
    await expect(page.getByRole('link', { name: 'サーバー' })).toHaveCount(0)
  })

  test('直接ルートを開いてもゲートが先に出る', async ({ page }) => {
    await page.goto('/backups')

    await expect(page.getByLabel('トークン')).toBeVisible()
    await expect(page.getByRole('heading', { name: 'バックアップ' })).toHaveCount(0)
  })

  test('長さが足りないトークンは送る前に弾かれる', async ({ page }) => {
    await page.goto('/')
    await page.getByLabel('トークン').fill('short')
    await page.getByRole('button', { name: '入る' }).click()

    await expect(page.getByRole('alert')).toBeVisible()
    await expect(page.getByLabel('トークン')).toBeVisible()
  })

  test('誤ったトークンは拒まれ、画面に入れない', async ({ page }) => {
    await page.goto('/')
    await page.getByLabel('トークン').fill('x'.repeat(64))
    await page.getByRole('button', { name: '入る' }).click()

    await expect(page.getByRole('alert')).toBeVisible()
    await expect(page.getByRole('link', { name: 'サーバー' })).toHaveCount(0)
  })

  test('正しいトークンで入れる', async ({ page }) => {
    await signIn(page)

    await expect(page.getByRole('heading', { name: 'サーバーの状態' })).toBeVisible()
    await expect(banner(page)).toHaveCount(0)
  })

  /**
   * 保存したトークンで次回から素通しになる。localStorage を使うのは
   * XSS に晒される代わりに、単一利用者の使い勝手を取ったため。
   */
  test('一度入れば再読み込みで聞かれない', async ({ page }) => {
    await signIn(page)
    await page.reload()

    await expect(page.getByRole('heading', { name: 'サーバーの状態' })).toBeVisible()
    await expect(page.getByLabel('トークン')).toHaveCount(0)
  })

  test('保存されたトークンが無効になったら聞き直す', async ({ page }) => {
    await signIn(page)

    await page.evaluate(() => window.localStorage.setItem('mcadmin.token', 'y'.repeat(64)))
    await page.reload()

    await expect(page.getByLabel('トークン')).toBeVisible()
  })
})

test('トークンは画面のどこにも書き出さない', async ({ page }) => {
  await signIn(page)

  const body = await page.locator('body').innerText()
  expect(body).not.toContain(TOKEN)
})
