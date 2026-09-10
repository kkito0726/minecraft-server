/**
 * 問い合わせのキー。
 *
 * 1 箇所にまとめるのは、操作の完了時に無効化する側と、画面で
 * 取得する側が同じ文字列を使う必要があるため。打ち間違えると
 * 無効化が黙って何もしなくなり、画面が古いまま残る。
 */
export const queryKeys = {
  status: ['status'] as const,
  worlds: ['worlds'] as const,
  backups: ['backups'] as const,
  retention: ['retention'] as const,
  /** 復元の事前確認。バックアップの id を後ろに足して使う。 */
  preflight: ['preflight'] as const,
}

/** 操作の完了時に無効化するキー。 */
export const invalidatedOnOperationFinish = [
  queryKeys.status,
  queryKeys.worlds,
  queryKeys.backups,
] as const
