/**
 * デモの操作エンジン。
 *
 * 実物の mcadmind は、手順を 1 つずつ進めながら進捗とログを配信する。
 * デモでその見え方を再現するのがここ。時間をかけて手順を進め、
 * 購読している画面へ 1 イベントずつ流す。
 *
 * 実物と同じく**同時にひとつしか走らせない**。実物はロックファイルで
 * プロセスをまたいで排他するが、ここは 1 つのタブの中の話なので
 * 実行中かどうかだけを見る。
 */
import { LogLevel, OperationKind, OperationState } from '../gen/mcadmin/v1/common_pb'

/** 1 手順にかける時間。短いと読めず、長いと退屈する。 */
const STEP_INTERVAL_MS = 850

/** バイト数を伴う手順を、何回に分けて進めるか。 */
const BYTE_TICKS = 6

export type DemoLogEvent = {
  seq: bigint
  at: Date
  level: LogLevel
  message: string
  snapshot: DemoOperation
}

export type DemoOperation = {
  id: string
  kind: OperationKind
  state: OperationState
  startedAt: Date
  finishedAt: Date | null
  stepIndex: number
  stepTotal: number
  currentStep: string
  stepNames: string[]
  bytesDone: bigint
  bytesTotal: bigint
  errorCode: string
  errorMessage: string
  attributes: Record<string, string>
}

/** 1 手順の定義。 */
export type DemoStep = {
  name: string
  /** この手順で流すログ。空なら手順名をそのまま出す。 */
  lines?: string[]
  /** 真なら、バイト数の進捗を刻みながら進める。 */
  withBytes?: boolean
  /** 警告として流す。 */
  warn?: boolean
}

export type StartOptions = {
  kind: OperationKind
  steps: DemoStep[]
  /** 転送量。withBytes の手順で使う。 */
  totalBytes?: bigint
  attributes?: Record<string, string>
  /** 成功したときに状態へ反映する処理。最後の手順の後に 1 回だけ呼ぶ。 */
  commit?: () => void
  /** 指定すると、その手順（1 始まり）で失敗する。失敗の見た目を見せるために使う。 */
  failAtStep?: number
  failMessage?: string
}

type Record_ = {
  operation: DemoOperation
  events: DemoLogEvent[]
  waiters: (() => void)[]
  timers: ReturnType<typeof setTimeout>[]
}

const records = new Map<string, Record_>()
let activeId: string | null = null
let counter = 0

/** 他の操作が実行中。実物の ErrBusy にあたる。 */
export class DemoBusyError extends Error {
  constructor() {
    super('他の操作が実行中です')
    this.name = 'DemoBusyError'
  }
}

export function isBusy(): boolean {
  return activeId !== null
}

export function getOperation(id: string): DemoOperation | undefined {
  return records.get(id)?.operation
}

export function getActiveOperation(): DemoOperation | undefined {
  return activeId ? records.get(activeId)?.operation : undefined
}

/** 新しい順の履歴。 */
export function listOperations(limit: number): DemoOperation[] {
  return [...records.values()]
    .map((r) => r.operation)
    .sort((a, b) => b.startedAt.getTime() - a.startedAt.getTime())
    .slice(0, limit > 0 ? limit : 50)
}

/**
 * 操作を始める。
 *
 * 呼び出し側にはすぐ最初の状態を返し、手順は時間をかけて進める。
 * 実物も同じで、変更系の応答は「始まった」ことだけを返す。
 */
export function startOperation(options: StartOptions): DemoOperation {
  if (activeId !== null) {
    throw new DemoBusyError()
  }

  counter += 1
  const id = `demo-op-${String(counter).padStart(3, '0')}`
  const steps = options.steps
  const operation: DemoOperation = {
    id,
    kind: options.kind,
    state: OperationState.PENDING,
    startedAt: new Date(),
    finishedAt: null,
    stepIndex: 0,
    stepTotal: steps.length,
    currentStep: '',
    stepNames: steps.map((s) => s.name),
    bytesDone: 0n,
    bytesTotal: options.totalBytes ?? 0n,
    errorCode: '',
    errorMessage: '',
    attributes: options.attributes ?? {},
  }

  records.set(id, { operation, events: [], waiters: [], timers: [] })
  activeId = id
  schedule(id, options)
  return operation
}

/** 手順を順に予約する。実時間で進めることで、実物の待ち時間の感覚を出す。 */
function schedule(id: string, options: StartOptions): void {
  let elapsed = 0

  options.steps.forEach((step, index) => {
    const stepNumber = index + 1
    const ticks = step.withBytes ? BYTE_TICKS : 1

    for (let tick = 1; tick <= ticks; tick++) {
      elapsed += STEP_INTERVAL_MS / ticks
      at(id, elapsed, () => advance(id, options, step, stepNumber, tick, ticks))
    }
  })

  elapsed += STEP_INTERVAL_MS / 2
  at(id, elapsed, () => finish(id, options))
}

function at(id: string, delay: number, fn: () => void): void {
  const record = records.get(id)
  if (!record) {
    return
  }
  const timer = setTimeout(fn, delay)
  records.set(id, { ...record, timers: [...record.timers, timer] })
}

/** 1 目盛りぶん進める。 */
function advance(
  id: string,
  options: StartOptions,
  step: DemoStep,
  stepNumber: number,
  tick: number,
  ticks: number,
): void {
  const record = records.get(id)
  if (!record || record.operation.finishedAt) {
    return
  }

  if (options.failAtStep === stepNumber && tick === ticks) {
    fail(id, options, step)
    return
  }

  const total = options.totalBytes ?? 0n
  const done = step.withBytes ? (total * BigInt(tick)) / BigInt(ticks) : record.operation.bytesDone

  const operation: DemoOperation = {
    ...record.operation,
    state: OperationState.RUNNING,
    stepIndex: stepNumber,
    currentStep: step.name,
    bytesDone: done,
  }

  emit(id, operation, step.warn ? LogLevel.WARN : LogLevel.INFO, lineFor(step, tick, ticks))
}

function lineFor(step: DemoStep, tick: number, ticks: number): string {
  const lines = step.lines ?? []
  if (lines.length === 0) {
    return ticks > 1 ? `${step.name}（${Math.round((tick / ticks) * 100)}%）` : step.name
  }
  return lines[Math.min(tick - 1, lines.length - 1)] ?? step.name
}

function fail(id: string, options: StartOptions, step: DemoStep): void {
  const record = records.get(id)
  if (!record) {
    return
  }

  const operation: DemoOperation = {
    ...record.operation,
    state: OperationState.FAILED,
    finishedAt: new Date(),
    currentStep: step.name,
    errorCode: 'demo_failed',
    errorMessage: options.failMessage ?? 'デモ用の失敗です。実際のサーバーは動いていません。',
  }

  emit(id, operation, LogLevel.ERROR, operation.errorMessage)
  activeId = null
}

/** 最後まで進んだ。状態への反映はここで 1 回だけ行う。 */
function finish(id: string, options: StartOptions): void {
  const record = records.get(id)
  if (!record || record.operation.finishedAt) {
    return
  }

  options.commit?.()

  const operation: DemoOperation = {
    ...record.operation,
    state: OperationState.SUCCEEDED,
    finishedAt: new Date(),
    stepIndex: record.operation.stepTotal,
    bytesDone: record.operation.bytesTotal,
  }

  emit(id, operation, LogLevel.INFO, '完了しました')
  activeId = null
}

/** イベントを 1 つ記録し、購読している側を起こす。 */
function emit(id: string, operation: DemoOperation, level: LogLevel, message: string): void {
  const record = records.get(id)
  if (!record) {
    return
  }

  const event: DemoLogEvent = {
    seq: BigInt(record.events.length + 1),
    at: new Date(),
    level,
    message,
    snapshot: operation,
  }

  records.set(id, {
    ...record,
    operation,
    events: [...record.events, event],
    waiters: [],
  })
  for (const wake of record.waiters) {
    wake()
  }
}

export function isTerminalState(state: OperationState): boolean {
  return state === OperationState.SUCCEEDED || state === OperationState.FAILED
}

/**
 * 進捗を配信する。
 *
 * fromSeq 以降の記録済みイベントを先に流してから、新しいものを待つ。
 * 実物と同じ約束にしておくと、再接続や再読み込みの動きもそのまま試せる。
 */
export async function* watchOperation(
  id: string,
  fromSeq: bigint,
  signal?: AbortSignal,
): AsyncGenerator<DemoLogEvent> {
  let cursor = fromSeq > 0n ? fromSeq - 1n : 0n

  while (!signal?.aborted) {
    const record = records.get(id)
    if (!record) {
      return
    }

    const pending = record.events.filter((e) => e.seq > cursor)
    for (const event of pending) {
      cursor = event.seq
      yield event
    }

    const last = record.events.at(-1)
    if (last && isTerminalState(last.snapshot.state)) {
      return
    }
    await nextEvent(id, signal)
  }
}

/** 次のイベントが来るまで待つ。中断されたら起こす。 */
function nextEvent(id: string, signal?: AbortSignal): Promise<void> {
  return new Promise((resolve) => {
    const record = records.get(id)
    if (!record) {
      resolve()
      return
    }

    const wake = () => resolve()
    records.set(id, { ...record, waiters: [...record.waiters, wake] })
    signal?.addEventListener('abort', wake, { once: true })
  })
}

/** 予約済みのタイマーを全部落とす。試験の後始末で使う。 */
export function resetOperations(): void {
  for (const record of records.values()) {
    for (const timer of record.timers) {
      clearTimeout(timer)
    }
  }
  records.clear()
  activeId = null
  counter = 0
}
