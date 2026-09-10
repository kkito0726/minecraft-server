import { ContainerState, SavingState } from '../../gen/mcadmin/v1/server_pb'

/**
 * オンライン人数の表示。
 *
 * rcon-cli list の出力書式はサーバーの版に依存するため、解釈できない
 * 場合バックエンドは -1 を返す（EDGE-002）。そのまま出すと
 * 「-1 / 5 人」になる。
 */
export function playerCountLabel(online: number, max: number): string {
  if (online < 0) {
    return '不明'
  }
  if (max <= 0) {
    return String(online)
  }
  return `${online} / ${max}`
}

const CONTAINER_LABELS: Record<ContainerState, string> = {
  [ContainerState.UNSPECIFIED]: '不明',
  [ContainerState.RUNNING]: '実行中',
  [ContainerState.EXITED]: '停止',
  [ContainerState.RESTARTING]: '再起動中',
  [ContainerState.MISSING]: 'コンテナなし',
}

/**
 * コンテナの状態。
 *
 * 実行中でもヘルスチェックに通っていなければ、まだ遊べない。
 * README が「STATUS が Up ... (healthy) かどうかで判断する」と
 * 書いているとおり、この 2 つは別の事実として示す。
 */
export function containerStateLabel(state: ContainerState, healthy: boolean): string {
  const base = CONTAINER_LABELS[state] ?? '不明'
  if (state === ContainerState.RUNNING && healthy) {
    return '実行中（正常）'
  }
  return base
}

const SAVING_LABELS: Record<SavingState, string> = {
  [SavingState.UNSPECIFIED]: '不明',
  [SavingState.ASSUMED_ON]: '有効',
  // RCON に「保存が有効か」を問い合わせる手段がないため、これは推定でしかない。
  [SavingState.SUSPECT_OFF]: '停止したままの可能性',
}

export function savingStateLabel(state: SavingState): string {
  return SAVING_LABELS[state] ?? '不明'
}

/** 起動してからの経過時間。 */
export function uptimeLabel(startedAt: Date | undefined, now: Date): string {
  if (!startedAt) {
    return ''
  }
  // 時計のずれで未来になることがある。マイナスを出さない。
  const seconds = Math.max(0, Math.floor((now.getTime() - startedAt.getTime()) / 1000))

  if (seconds < 60) {
    return `${seconds} 秒`
  }
  const minutes = Math.floor(seconds / 60)
  if (minutes < 60) {
    return `${minutes} 分`
  }
  const hours = Math.floor(minutes / 60)
  if (hours < 24) {
    return `${hours} 時間 ${minutes % 60} 分`
  }
  return `${Math.floor(hours / 24)} 日 ${hours % 24} 時間`
}
