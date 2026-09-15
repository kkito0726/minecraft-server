/**
 * デモで見せる手順。
 *
 * 文言は実物の進捗ログに寄せてある。ここを適当にすると、デモを見た人が
 * 「何が起きているのか」を実物と違う形で覚えてしまう。
 */
import type { DemoStep } from './operations'

export const START_STEPS: DemoStep[] = [
  { name: 'サーバーを起動しています', lines: ['Container minecraft-server Creating'] },
  { name: '起動を確認しています', lines: ['Container minecraft-server Healthy'] },
]

export const STOP_STEPS: DemoStep[] = [
  { name: 'ワールドを保存しています', lines: ['save-all を送信しました'] },
  { name: 'サーバーを停止しています', lines: ['Container minecraft-server Stopped'] },
]

export const RESTART_STEPS: DemoStep[] = [
  { name: 'ワールドを保存しています', lines: ['save-all を送信しました'] },
  { name: 'サーバーを停止しています', lines: ['Container minecraft-server Stopped'] },
  { name: 'サーバーを起動しています', lines: ['Container minecraft-server Created'] },
  { name: '起動を確認しています', lines: ['Container minecraft-server Healthy'] },
]

/** 稼働したまま取る。保存を止めてから固め、必ず戻す。 */
export const HOT_BACKUP_STEPS: DemoStep[] = [
  { name: '保存を止めています', lines: ['save-off / save-all を送信しました'] },
  { name: 'ワールドを書き出しています', withBytes: true },
  { name: '保存を再開しています', lines: ['save-on を送信しました'] },
  { name: '保持ポリシーを適用しています' },
]

/** 停止してから取る。確実だが、その間サーバーは止まる。 */
export const COLD_BACKUP_STEPS: DemoStep[] = [
  { name: 'サーバーを停止しています', lines: ['Container minecraft-server Stopped'] },
  { name: 'ワールドを書き出しています', withBytes: true },
  { name: 'サーバーを起動しています', lines: ['Container minecraft-server Created'] },
  { name: '起動を確認しています', lines: ['Container minecraft-server Healthy'] },
]

/**
 * 復元。
 *
 * 「退避してから展開」であって上書きではない。デモでもその順序を見せる。
 */
export const RESTORE_STEPS: DemoStep[] = [
  { name: '事前確認をやり直しています' },
  { name: 'サーバーを停止しています', lines: ['Container minecraft-server Stopped'] },
  {
    name: '現在のワールドを退避しています',
    lines: ['data/world を .broken-日時 へ退避しました'],
    warn: true,
  },
  { name: 'アーカイブを展開しています', withBytes: true },
  { name: 'MC_LEVEL を切り替えています' },
  { name: 'サーバーを起動しています', lines: ['Container minecraft-server Created'] },
  { name: '起動を確認しています', lines: ['Container minecraft-server Healthy'] },
]

export const SWITCH_STEPS: DemoStep[] = [
  { name: 'ワールドを保存しています', lines: ['save-all を送信しました'] },
  { name: 'サーバーを停止しています', lines: ['Container minecraft-server Removed'] },
  { name: 'MC_LEVEL を切り替えています' },
  { name: 'サーバーを起動しています', lines: ['Container minecraft-server Created'] },
  { name: '起動を確認しています', lines: ['Container minecraft-server Healthy'] },
]

export const CREATE_WORLD_STEPS: DemoStep[] = [
  { name: 'MC_LEVEL と MC_SEED を書き込んでいます' },
  { name: 'サーバーを起動しています', lines: ['Container minecraft-server Created'] },
  { name: 'ワールドを生成しています', lines: ['Preparing spawn area'] },
  { name: '起動を確認しています', lines: ['Container minecraft-server Healthy'] },
]

export const CLONE_STEPS: DemoStep[] = [{ name: 'ワールドを複製しています', withBytes: true }]

export const RENAME_STEPS: DemoStep[] = [
  { name: 'サーバーを停止しています', lines: ['Container minecraft-server Stopped'] },
  { name: 'ディレクトリを改名しています' },
  { name: 'サーバーを起動しています', lines: ['Container minecraft-server Healthy'] },
]

export const DELETE_STEPS: DemoStep[] = [
  { name: 'ワールドを退避しています', lines: ['data/<名前>.deleted-日時 へ退避しました'], warn: true },
]
