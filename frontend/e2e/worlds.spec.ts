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

/**
 * ゲームモードとハードコアは生成の瞬間にしか効かないので、作成の画面で
 * 決めて .env へ書く。ハードコアは後から外せないため、名前の完全一致を
 * 打たせてから送る。
 */
test('ハードコアを選ぶと難易度が固定され、.env に書かれる', async ({ page, server }) => {
  await signIn(page)
  await page.getByRole('link', { name: 'ワールド' }).click()

  await page.getByRole('button', { name: '新規作成' }).click()
  await page.getByLabel('ワールド名').fill('hardmode')
  await page.getByRole('radio', { name: /ハードコア/ }).check()

  // ハードコアでは難易度を選べない。Minecraft がハードに固定するため。
  const difficulty = page.getByRole('group', { name: '難易度' })
  await expect(difficulty.getByRole('radio', { name: /ピースフル/ })).toBeDisabled()

  // 確認を打つまでは送れない。
  await expect(page.getByRole('button', { name: '作成する' })).toBeDisabled()
  await page.getByLabel(/確認のため/).fill('hardmode')

  await page.getByRole('button', { name: '作成する' }).click()
  await waitForOperation(page)

  expect(envValue(server.dir, 'MC_LEVEL')).toBe('hardmode')
  // ハードコアはサバイバルに hardcore が付いたもの。
  expect(envValue(server.dir, 'MC_MODE')).toBe('survival')
  expect(envValue(server.dir, 'MC_DIFFICULTY')).toBe('hard')
  expect(envValue(server.dir, 'MC_HARDCORE')).toBe('TRUE')
})

/** ハードコアを選ばなければ、モードと難易度はそのまま書かれる。 */
test('モードと難易度を選ぶと .env に書かれる', async ({ page, server }) => {
  await signIn(page)
  await page.getByRole('link', { name: 'ワールド' }).click()

  await page.getByRole('button', { name: '新規作成' }).click()
  await page.getByLabel('ワールド名').fill('sandbox')
  await page.getByRole('radio', { name: /クリエイティブ/ }).check()
  await page
    .getByRole('group', { name: '難易度' })
    .getByRole('radio', { name: /ピースフル/ })
    .check()

  await page.getByRole('button', { name: '作成する' }).click()
  await waitForOperation(page)

  expect(envValue(server.dir, 'MC_MODE')).toBe('creative')
  expect(envValue(server.dir, 'MC_DIFFICULTY')).toBe('peaceful')
  expect(envValue(server.dir, 'MC_HARDCORE')).toBe('FALSE')
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

/**
 * MC_HARDCORE はサーバー全体の値だが、ハードコアかどうかは本来ワールドが
 * level.dat に持っている。切り替えのたびに切り替え先に合わせる。
 * 合わせないと、普通に作ったワールドがハードコアで動いてしまう。
 */
test.describe('ハードコアのワールドとの行き来', () => {
  test.use({ worlds: ['hardmode', 'casual'], hardcoreWorlds: ['hardmode'] })

  test('切り替え先の level.dat に合わせてハードコアと難易度が変わる', async ({ page, server }) => {
    await signIn(page)
    await page.getByRole('link', { name: 'ワールド' }).click()

    // 切り替える前に分かる。
    const hardRow = page.getByRole('row', { name: /^hardmode / })
    const casualRow = page.getByRole('row', { name: /^casual / })
    await expect(hardRow.getByText('ハードコア')).toBeVisible()
    await expect(casualRow.getByText('ハードコア')).toHaveCount(0)

    // 普通のワールドへ: ハードコアを外し、難易度はそのワールドの値（normal）に戻る。
    await casualRow.getByRole('button', { name: '切替' }).click()
    await waitForOperation(page)
    expect(envValue(server.dir, 'MC_HARDCORE')).toBe('FALSE')
    expect(envValue(server.dir, 'MC_DIFFICULTY')).toBe('normal')

    // ハードコアのワールドへ戻る: ハードコアに戻し、難易度をハードにする。
    await hardRow.getByRole('button', { name: '切替' }).click()
    await waitForOperation(page)
    expect(envValue(server.dir, 'MC_HARDCORE')).toBe('TRUE')
    expect(envValue(server.dir, 'MC_DIFFICULTY')).toBe('hard')
  })
})

/**
 * サーバーの版は、切り替え先のワールドが最後に開かれた版に合わせる。
 * 合わせないと、古いワールドは今の版で開かれて勝手に上がり（元に戻せない）、
 * 新しいワールドは古い版では開けずにサーバーが起動しない。
 *
 * E2E では版の一覧を取れない状態に固定してある（外の API に左右させない）。
 * 一覧が取れないときも切り替えは通す、という振る舞いもあわせて確かめる。
 */
test.describe('版の違うワールドとの行き来', () => {
  test.use({ worlds: ['world', 'newer'], worldVersions: { newer: '26.3' } })

  test('切り替え先の level.dat の版に MC_VERSION が合う', async ({ page, server }) => {
    await signIn(page)
    await page.getByRole('link', { name: 'ワールド' }).click()

    const newerRow = page.getByRole('row', { name: /^newer / })
    await expect(newerRow.getByText('26.3')).toBeVisible()

    await newerRow.getByRole('button', { name: '切替' }).click()
    await waitForOperation(page)
    expect(envValue(server.dir, 'MC_VERSION')).toBe('26.3')

    await page.getByRole('row', { name: /^world / }).getByRole('button', { name: '切替' }).click()
    await waitForOperation(page)
    expect(envValue(server.dir, 'MC_VERSION')).toBe('26.2')
  })
})

/** 版の一覧が取れないとき（オフライン）も、現在の版でなら作れる。 */
test('版の一覧が取れなくても現在の版で作成できる', async ({ page, server }) => {
  await signIn(page)
  await page.getByRole('link', { name: 'ワールド' }).click()

  await page.getByRole('button', { name: '新規作成' }).click()
  const version = page.getByLabel('版')
  await expect(version).toBeDisabled()
  await expect(version).toHaveValue('26.2')
  await expect(page.getByText(/版の一覧を取得できないため/)).toBeVisible()

  await page.getByLabel('ワールド名').fill('offline')
  await page.getByRole('button', { name: '作成する' }).click()
  await waitForOperation(page)

  expect(envValue(server.dir, 'MC_LEVEL')).toBe('offline')
  expect(envValue(server.dir, 'MC_VERSION')).toBe('26.2')
})
