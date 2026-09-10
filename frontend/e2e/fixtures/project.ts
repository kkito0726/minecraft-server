import { copyFileSync, mkdirSync, writeFileSync } from 'node:fs'
import { dirname, join, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

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

  for (const name of worlds) {
    mkdirSync(join(dir, 'data', name), { recursive: true })
    copyFileSync(LEVEL_DAT, join(dir, 'data', name, 'level.dat'))
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
      'MC_SEED=',
      'MC_MOTD="§aE2E のサーバー"',
      `ADMIN_TOKEN=${TOKEN}`,
      `ADMIN_ADDR=${addr}`,
      // LookPath に相対パスを渡すと解決できないことがあるので絶対パスにする
      `ADMIN_DOCKER_BIN=${FAKE_DOCKER}`,
      'ADMIN_BACKUP_DIR=backups',
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
