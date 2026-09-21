import { expect, signIn, test } from './fixtures/test'

/**
 * ホストの資源の表示。
 *
 * 確かめたいのは、画面が実物の mcadmind を通ってこの機械の実測値を
 * 受け取れること。値そのものは機械によって違うので検証しない。
 *
 * ストレージだけを見るのは、これが unix なら必ず読めるため。CPU と
 * メモリは /proc を読むので Linux でしか出ず、CI（ubuntu）と開発機
 * （macOS）で結果が変わる。ここで分岐させると、壊れたときに
 * 「環境差か不具合か」を毎回考えることになる。
 */
test.describe('リソース', () => {
  test('この機械の使用状況を表示する', async ({ page, server }) => {
    await signIn(page)
    await page.getByRole('link', { name: 'リソース' }).click()

    const storage = page.getByRole('meter', { name: 'ストレージ の使用率' })
    await expect(storage).toBeVisible()

    // 0〜100 の範囲に収まった実測値が入っていること。
    const value = Number(await storage.getAttribute('aria-valuenow'))
    expect(Number.isFinite(value)).toBe(true)
    expect(value).toBeGreaterThanOrEqual(0)
    expect(value).toBeLessThanOrEqual(100)

    // 測っているのは --project-dir に渡した場所。別の場所を測っても
    // 画面には数字が出てしまうので、ここは文字で確かめる。
    await expect(page.getByText(server.dir, { exact: false })).toBeVisible()
  })
})
