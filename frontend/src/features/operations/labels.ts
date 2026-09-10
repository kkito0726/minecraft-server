import { LogLevel, OperationKind, OperationState } from '../../gen/mcadmin/v1/common_pb'

/**
 * 操作の種類の表示名。
 *
 * proto の enum をそのまま画面に出さない。生成コードの識別子は
 * 英大文字で、利用者にとって意味を持たない。
 */
const KIND_LABELS: Record<OperationKind, string> = {
  [OperationKind.UNSPECIFIED]: '操作',
  [OperationKind.SERVER_START]: 'サーバーの起動',
  [OperationKind.SERVER_STOP]: 'サーバーの停止',
  [OperationKind.SERVER_RESTART]: 'サーバーの再起動',
  [OperationKind.BACKUP_CREATE]: 'バックアップの取得',
  [OperationKind.BACKUP_RESTORE]: 'バックアップからの復元',
  [OperationKind.WORLD_SWITCH]: 'ワールドの切り替え',
  [OperationKind.WORLD_CREATE]: 'ワールドの作成',
  [OperationKind.WORLD_CLONE]: 'ワールドの複製',
  [OperationKind.WORLD_RENAME]: 'ワールドの改名',
  [OperationKind.WORLD_DELETE]: 'ワールドの削除',
}

export function kindLabel(kind: OperationKind): string {
  return KIND_LABELS[kind] ?? '操作'
}

const STATE_LABELS: Record<OperationState, string> = {
  [OperationState.UNSPECIFIED]: '不明',
  [OperationState.PENDING]: '待機中',
  [OperationState.RUNNING]: '実行中',
  [OperationState.SUCCEEDED]: '完了',
  [OperationState.FAILED]: '失敗',
}

export function stateLabel(state: OperationState): string {
  return STATE_LABELS[state] ?? '不明'
}

/** ログの重要度に対応する見た目の強さ。 */
export function levelTone(level: LogLevel): 'neutral' | 'warn' | 'danger' {
  switch (level) {
    case LogLevel.ERROR:
      return 'danger'
    case LogLevel.WARN:
      return 'warn'
    default:
      return 'neutral'
  }
}
