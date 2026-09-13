/**
 * デモの状態。
 *
 * 公開デモはバックエンドに繋がない。ここがサーバーの代わりになる、
 * ブラウザのメモリ上だけの「真実」で、再読み込みすれば初期状態に戻る。
 *
 * proto の型はここに持ち込まない。素の TypeScript で持ち、変換は
 * messages.ts が受け持つ。こうしておくと、状態の遷移だけを試験できる。
 */

export type DemoVersion = {
  readable: boolean
  name: string
  dataVersion: number
  levelName: string
}

export type DemoWorld = {
  name: string
  sizeBytes: bigint
  lastPlayed: Date
  version: DemoVersion
  hasSessionLock: boolean
}

export type DemoQuarantine = {
  name: string
  originalLevel: string
  quarantinedAt: Date
  sizeBytes: bigint
  /** 復元による退避なら真、削除による退避なら偽。 */
  fromRestore: boolean
}

export type DemoBackup = {
  id: string
  sizeBytes: bigint
  createdAt: Date
  declaredVersion: string
  archiveLevel: string
  version: DemoVersion
  entryRoots: string[]
}

export type DemoState = {
  running: boolean
  healthy: boolean
  startedAt: Date | null
  configuredVersion: string
  /** .env の MC_LEVEL にあたるもの。 */
  activeLevel: string
  onlinePlayers: number
  maxPlayers: number
  worlds: DemoWorld[]
  quarantines: DemoQuarantine[]
  backups: DemoBackup[]
  retention: { keepCount: number; keepDays: number }
  backupDir: string
}

/** 現行版。実機の値に合わせてある。 */
const CURRENT = version('26.2', 4903)

/** 1 つ前の版。復元のときに「古い」判定を見せるために置く。 */
const PREVIOUS = version('26.1', 4844)

function version(name: string, dataVersion: number, levelName = ''): DemoVersion {
  return { readable: true, name, dataVersion, levelName }
}

function minutesAgo(minutes: number): Date {
  return new Date(Date.now() - minutes * 60_000)
}

/**
 * 初期状態。
 *
 * 稼働中のワールド、遊んでいないワールド、退避したもの、世代の違う
 * バックアップを最初から置いておく。空の画面を見せても、この管理画面が
 * 何をするものなのかが伝わらない。
 */
export function initialState(): DemoState {
  return {
    running: true,
    healthy: true,
    startedAt: minutesAgo(148),
    configuredVersion: '26.2',
    activeLevel: 'world',
    onlinePlayers: 2,
    maxPlayers: 5,
    worlds: seedWorlds(),
    quarantines: seedQuarantines(),
    backups: seedBackups(),
    retention: { keepCount: 10, keepDays: 30 },
    backupDir: '/home/pi/minecraft-server/backups',
  }
}

/** 稼働中・待機中・版が古いものを 1 つずつ。一覧の見え方の違いが分かるように。 */
function seedWorlds(): DemoWorld[] {
  return [
    {
      name: 'world',
      sizeBytes: 412_400_000n,
      lastPlayed: minutesAgo(6),
      version: { ...CURRENT, levelName: 'world' },
      hasSessionLock: true,
    },
    {
      name: 'creative',
      sizeBytes: 88_300_000n,
      lastPlayed: minutesAgo(60 * 26),
      version: { ...CURRENT, levelName: 'creative' },
      hasSessionLock: false,
    },
    {
      name: 'hardcore-2025',
      sizeBytes: 263_900_000n,
      lastPlayed: minutesAgo(60 * 24 * 43),
      version: { ...PREVIOUS, levelName: 'hardcore-2025' },
      hasSessionLock: false,
    },
  ]
}

function seedQuarantines(): DemoQuarantine[] {
  return [
    {
      name: 'world.deleted-20260901-2210',
      originalLevel: 'world',
      quarantinedAt: minutesAgo(60 * 24 * 11),
      sizeBytes: 380_100_000n,
      fromRestore: false,
    },
  ]
}

function seedBackups(): DemoBackup[] {
  return [
    backup('backup-26.2-world-20260912-0300.zip', 214_800_000n, minutesAgo(60 * 7), 'world', CURRENT),
    backup(
      'backup-26.2-world-20260911-0300-before_update.zip',
      213_500_000n,
      minutesAgo(60 * 31),
      'world',
      CURRENT,
    ),
    // 版が古いもの。復元の画面で承諾を求める流れを見せるために置く。
    backup(
      'backup-26.1-hardcore-2025-20260731-0300.zip',
      156_200_000n,
      minutesAgo(60 * 24 * 43),
      'hardcore-2025',
      PREVIOUS,
    ),
  ]
}

function backup(
  id: string,
  sizeBytes: bigint,
  createdAt: Date,
  level: string,
  v: DemoVersion,
): DemoBackup {
  return {
    id,
    sizeBytes,
    createdAt,
    declaredVersion: v.name,
    archiveLevel: level,
    version: { ...v, levelName: level },
    entryRoots: [`data/${level}`],
  }
}

let current: DemoState = initialState()
const listeners = new Set<() => void>()

export function getState(): DemoState {
  return current
}

/**
 * 状態を進める。
 *
 * 既存のオブジェクトは書き換えず、常に新しい状態に差し替える。
 * 画面は問い合わせを無効化して読み直すので、購読は要らない。
 * 試験が変化を待てるように、通知だけは出しておく。
 */
export function updateState(fn: (state: DemoState) => DemoState): DemoState {
  current = fn(current)
  for (const listener of listeners) {
    listener()
  }
  return current
}

export function subscribeState(listener: () => void): () => void {
  listeners.add(listener)
  return () => listeners.delete(listener)
}

/** 初期状態に戻す。試験と、画面の「最初からやり直す」で使う。 */
export function resetState(): void {
  updateState(() => initialState())
}

/** 稼働中のワールド。MC_LEVEL に対応するものが無ければ undefined。 */
export function activeWorld(state: DemoState): DemoWorld | undefined {
  return state.worlds.find((w) => w.name === state.activeLevel)
}

export function findBackup(state: DemoState, id: string): DemoBackup | undefined {
  return state.backups.find((b) => b.id === id)
}

/** 新しい順に並べ直す。取得と取り込みの後に使う。 */
export function sortedBackups(backups: DemoBackup[]): DemoBackup[] {
  return [...backups].sort((a, b) => b.createdAt.getTime() - a.createdAt.getTime())
}
