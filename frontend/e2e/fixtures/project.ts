import { copyFileSync, mkdirSync, readFileSync, writeFileSync } from 'node:fs'
import { dirname, join, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'
import { gunzipSync, gzipSync } from 'node:zlib'

const HERE = dirname(fileURLToPath(import.meta.url))

/** リポジトリのルート。mcadmind と実物の level.dat を取りに行く。 */
export const REPO_ROOT = resolve(HERE, '../../..')

export const MCADMIND = join(REPO_ROOT, 'mcadmind')
export const FAKE_DOCKER = join(HERE, 'fake-docker.sh')

/** 実物の level.dat。26.2 / DataVersion 4903 が入っている。 */
const LEVEL_DAT = join(
  REPO_ROOT,
  'backend/internal/infrastructure/leveldat/testdata/level.dat',
)

/** 試験用のトークン。32 文字未満だと mcadmind が起動を拒む。 */
export const TOKEN = 'e2e'.padEnd(64, '0')

export type ProjectOptions = {
  dir: string
  addr: string
  /** 起動が完了したことにするまでの秒数。進行中の画面を見る試験だけ延ばす。 */
  readyDelaySeconds?: number
  /** 最初から置いておくワールド。先頭が MC_LEVEL になる。 */
  worlds?: string[]
  /** level.dat にハードコアの印を立てるワールド。 */
  hardcoreWorlds?: string[]
  /**
   * level.dat の版（Data.Version.Name）を書き換えるワールド。
   * 実物の "26.2" と同じ 4 文字に限る（NBT の長さを変えずに済ませるため）。
   */
  worldVersions?: Record<string, string>
}

/**
 * 実物の level.dat を、必要なところだけ書き換えて返す。
 *
 * NBT を組み立て直さず、実物のバイト列のまま通す。構造は本物のままで、
 * 値だけを変えたい。
 * - ハードコア: 26.x では Data.difficulty_settings.hardcore（TAG_Byte）。
 *   名前の直後の 1 バイトが値
 * - 版: Data.Version.Name（TAG_String）。"Name" と長さ 2 バイトの直後
 */
function levelDatFor(opts: { hardcore: boolean; version?: string | undefined }): Buffer {
  const raw = gunzipSync(readFileSync(LEVEL_DAT))

  if (opts.hardcore) {
    const at = raw.indexOf('hardcore')
    if (at < 0) {
      throw new Error('level.dat に hardcore が見つからない')
    }
    raw[at + 'hardcore'.length] = 1
  }

  if (opts.version) {
    const marker = Buffer.concat([Buffer.from('Name'), Buffer.from([0x00, 0x04])])
    const at = raw.indexOf(marker)
    if (at < 0 || opts.version.length !== 4) {
      throw new Error('Version.Name を書き換えられない（4 文字の版に限る）')
    }
    raw.write(opts.version, at + marker.length, 'ascii')
  }

  return gzipSync(raw)
}

/**
 * mcadmind に渡すプロジェクトディレクトリを組み立てる。
 *
 * validateProjectDir が compose.yaml と .env を要求する。ワールドは
 * 実物の level.dat を置く（バージョン判定が本物の NBT を通る）。
 */
export function createProject(opts: ProjectOptions) {
  const { dir, addr } = opts
  const worlds = opts.worlds ?? ['world']

  mkdirSync(join(dir, 'backups'), { recursive: true })
  mkdirSync(join(dir, 'data/plugins'), { recursive: true })
  mkdirSync(join(dir, 'data/config'), { recursive: true })
  writeFileSync(join(dir, 'data/bukkit.yml'), 'settings:\n  allow-end: true\n')
  writeFileSync(join(dir, 'data/spigot.yml'), 'world-settings:\n  default:\n    view-distance: 6\n')

  const hardcore = new Set(opts.hardcoreWorlds ?? [])
  const versions = opts.worldVersions ?? {}
  for (const name of worlds) {
    mkdirSync(join(dir, 'data', name), { recursive: true })
    const target = join(dir, 'data', name, 'level.dat')
    if (hardcore.has(name) || versions[name]) {
      writeFileSync(target, levelDatFor({ hardcore: hardcore.has(name), version: versions[name] }))
    } else {
      copyFileSync(LEVEL_DAT, target)
    }
    writeFileSync(join(dir, 'data', name, 'session.lock'), '')
  }

  // 最初からコンテナが動いていることにする。実運用の常態がこれであり、
  // 止まった状態から始めると「停止中なので何もしない」経路ばかりを
  // 通ってしまい、停止と起動を挟む手順が確かめられない。
  mkdirSync(join(dir, '.fake-docker'), { recursive: true })
  writeFileSync(join(dir, '.fake-docker/running'), '')
  writeFileSync(join(dir, '.fake-docker/ready_at'), '0')

  writeFileSync(join(dir, 'compose.yaml'), COMPOSE)
  writeFileSync(
    join(dir, '.env'),
    [
      '# E2E が生成したもの。日本語のコメントを残せるかも同時に確かめる。',
      'MC_VERSION=26.2',
      `MC_LEVEL=${worlds[0]}`,
      // ハードコアのワールドを置くときだけ書く。設定画面の試験は、これらの
      // キーが無い状態から行が追記されることも確かめているので、既定では置かない。
      ...(hardcore.size > 0
        ? [
            `MC_HARDCORE=${hardcore.has(worlds[0] ?? '') ? 'TRUE' : 'FALSE'}`,
            `MC_DIFFICULTY=${hardcore.has(worlds[0] ?? '') ? 'hard' : 'normal'}`,
          ]
        : []),
      'MC_SEED=',
      'MC_MOTD="§aE2E のサーバー"',
      `ADMIN_TOKEN=${TOKEN}`,
      `ADMIN_ADDR=${addr}`,
      // LookPath に相対パスを渡すと解決できないことがあるので絶対パスにする
      `ADMIN_DOCKER_BIN=${FAKE_DOCKER}`,
      'ADMIN_BACKUP_DIR=backups',
      // 版の一覧は繋がらない先にしておく。試験の結果を外の Paper の API の
      // 状態（や CI のネットワーク）に左右させない。「一覧が取れない」経路に固定する。
      'ADMIN_VERSION_CATALOG_URL=http://127.0.0.1:9/unreachable',
      '',
    ].join('\n'),
  )
}

/** fake-docker は読まないが、validateProjectDir が存在を要求する。 */
const COMPOSE = `name: minecraft-server
services:
  mc:
    image: itzg/minecraft-server:latest
    environment:
      LEVEL: "\${MC_LEVEL:-world}"
`
