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
  /** ワールドを作れる版の一覧（Paper）。 */
  versions: ['versions'] as const,
  backups: ['backups'] as const,
  retention: ['retention'] as const,
  /** 復元の事前確認。バックアップの id を後ろに足して使う。 */
  preflight: ['preflight'] as const,
  /** .env のゲーム設定。 */
  gameSettings: ['gameSettings'] as const,
  /** ホストの資源の使用状況。操作の前後ではなく時間で変わるので、無効化の対象には入れない。 */
  systemMetrics: ['systemMetrics'] as const,
}

/**
 * 操作の完了時に無効化するキー。
 *
 * ゲーム設定も含める。反映の操作が終わった時点で .env を読み直さないと、
 * 画面が保存前の値を持ったまま「未保存の変更がある」ように見える。
 */
export const invalidatedOnOperationFinish = [
  queryKeys.status,
  queryKeys.worlds,
  queryKeys.backups,
  queryKeys.gameSettings,
] as const
