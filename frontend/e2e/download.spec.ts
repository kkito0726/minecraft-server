import { readFileSync, statSync } from 'node:fs'

import { expect, signIn, test, waitForOperation } from './fixtures/test'
import { entryNames, readEntry } from './fixtures/zip'

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

  /*
  シングルプレイ用は形が違う。

  ワールドのフォルダが zip の直下に来て、Paper がディメンション側に
  置いている設定がバニラの見る場所へ移る。移さないと、saves/ に置いても
  ワールド一覧に出るだけで「Overworld settings missing」で読み込めない。

  ここは実物の mcadmind を通る唯一の経路なので、包み直しの結果そのものを
  zip を開いて確かめる。
  */
  test('シングルプレイ用の形で保存できる', async ({ page }) => {
    await signIn(page)
    await page.getByRole('link', { name: 'バックアップ' }).click()
    await page.getByRole('button', { name: 'バックアップを取得' }).click()
    await page.getByRole('button', { name: '取得する' }).click()
    await waitForOperation(page)

    const [download] = await Promise.all([
      page.waitForEvent('download'),
      page.getByRole('button', { name: 'シングルプレイ用' }).click(),
    ])

    // 名前でサーバー用と見分けられること。
    expect(download.suggestedFilename()).toMatch(/^backup-.*-singleplayer\.zip$/)

    const path = await download.path()
    expect(path).not.toBeNull()
    const zip = readFileSync(path)

    // ワールドのフォルダが zip の直下に来ている。
    const names = entryNames(zip)
    expect(names).toContain('world/level.dat')

    // Paper がディメンション側に置いていた 5 つがワールド直下へ移っている。
    for (const name of [
      'game_rules',
      'scheduled_events',
      'wandering_trader',
      'weather',
      'world_gen_settings',
    ]) {
      expect(names).toContain(`world/data/minecraft/${name}.dat`)
      expect(names).not.toContain(
        `world/dimensions/minecraft/overworld/data/minecraft/${name}.dat`,
      )
    }

    // 移したのはオーバーワールドのもの。ネザー側のものを持ってくると、
    // 全ディメンション分の生成設定が 1 ディメンション分に縮む。
    expect(readEntry(zip, 'world/data/minecraft/world_gen_settings.dat')).toBe(
      'overworld-world_gen_settings',
    )
    expect(
      readEntry(zip, 'world/dimensions/minecraft/the_nether/data/minecraft/world_gen_settings.dat'),
    ).toBe('nether-gen')

    // バニラもディメンション側に置くものは動かさない。
    expect(names).toContain('world/dimensions/minecraft/overworld/data/minecraft/raids.dat')

    // サーバーの設定は持ち出さない。saves/ の直下にゴミとして並ぶため。
    expect(names.some((n) => n.startsWith('plugins/') || n.startsWith('config/'))).toBe(false)
    expect(names).not.toContain('bukkit.yml')
    expect(names).not.toContain('spigot.yml')
    // session.lock は取得の時点で除いてある。
    expect(names).not.toContain('world/session.lock')
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
