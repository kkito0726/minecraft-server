/**
 * デモの取り込み。
 *
 * zip の受け口だけは Connect を通らない素の POST なので、
 * トランスポートの差し替えでは代われない。ここで別に偽装する。
 *
 * 中身は読まない。読んでも保管する先がないうえ、デモで見せたいのは
 * 「送信の進み具合が出て、一覧に増える」という流れの方だから。
 */
import { getState, sortedBackups, updateState } from './state'
import type { DemoBackup } from './state'

/** 送信の進捗を刻む回数。 */
const TICKS = 12

/** 1 目盛りの間隔。実機の 30MB の送信がこのくらいに見える。 */
const TICK_MS = 90

export type DemoUploadResult = {
  id: string
  level: string
  version: string
  sizeBytes: number
  rewrapped: boolean
}

export type DemoUploadOptions = {
  onProgress?: (ratio: number) => void
  signal?: AbortSignal
}

export function uploadDemoBackup(
  file: File,
  options: DemoUploadOptions = {},
): Promise<DemoUploadResult> {
  return new Promise((resolve, reject) => {
    let tick = 0

    const timer = setInterval(() => {
      if (options.signal?.aborted) {
        clearInterval(timer)
        reject(new Error('アップロードを中止しました。'))
        return
      }

      tick += 1
      options.onProgress?.(tick / TICKS)

      if (tick >= TICKS) {
        clearInterval(timer)
        resolve(adopt(file))
      }
    }, TICK_MS)
  })
}

/** 取り込んだことにして一覧へ足す。 */
function adopt(file: File): DemoUploadResult {
  const state = getState()
  const level = levelFrom(file.name)
  const entry: DemoBackup = {
    id: `imported-${stamp()}-${sanitize(file.name)}`,
    sizeBytes: BigInt(Math.max(1, file.size)),
    createdAt: new Date(),
    declaredVersion: state.configuredVersion,
    archiveLevel: level,
    version: {
      readable: true,
      name: state.configuredVersion,
      dataVersion: 4903,
      levelName: level,
    },
    entryRoots: [`data/${level}`],
  }

  updateState((s) => ({ ...s, backups: sortedBackups([entry, ...s.backups]) }))

  return {
    id: entry.id,
    level,
    version: entry.version.name,
    sizeBytes: Number(entry.sizeBytes),
    // 配布ワールドの形（data/ を含まない）だったことにして、
    // 包み直しの案内も見せる。
    rewrapped: true,
  }
}

/** ファイル名からワールド名らしきものを拾う。取れなければ既定の名前。 */
function levelFrom(fileName: string): string {
  const base = fileName.replace(/\.zip$/i, '')
  const matched = /^[A-Za-z0-9_-]+$/.test(base) ? base : ''
  return matched || 'imported-world'
}

function sanitize(fileName: string): string {
  return fileName.replace(/[^A-Za-z0-9_.-]/g, '_').slice(0, 40)
}

function stamp(): string {
  const now = new Date()
  const pad = (n: number) => String(n).padStart(2, '0')
  return `${now.getFullYear()}${pad(now.getMonth() + 1)}${pad(now.getDate())}-${pad(now.getHours())}${pad(now.getMinutes())}`
}
