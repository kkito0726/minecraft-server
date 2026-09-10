// Package operation は時間のかかる操作の集約。
//
// 復元は stop_grace_period の 60 秒に Paper の起動を足して分単位でかかる。
// 利用者は必ずブラウザを再読み込みするため、進捗は画面の状態ではなく
// サーバー側のリソースとして持つ。
//
// この集約が守る不変条件は 2 つ。
//   - 状態遷移は一方向。終端（成功・失敗）に達したら二度と動かない
//   - イベントの seq は 1 始まりで単調増加する
//
// 前者が破れると UI の排他表示が解けなくなり、後者が破れると
// クライアントの再接続位置がずれてログの取りこぼしや二重表示が起きる。
package operation

// Kind は操作の種類。data/ を変更するものはすべてここに含まれ、
// 同時にひとつしか実行できない。
type Kind int

const (
	// KindUnspecified は未指定。
	KindUnspecified Kind = iota
	// KindServerStart はサーバーの起動。
	KindServerStart
	// KindServerStop はサーバーの停止。
	KindServerStop
	// KindServerRestart はサーバーの再起動。
	KindServerRestart
	// KindBackupCreate はバックアップの取得。
	KindBackupCreate
	// KindBackupRestore はバックアップからの復元。
	KindBackupRestore
	// KindWorldSwitch はワールドの切り替え。
	KindWorldSwitch
	// KindWorldCreate はワールドの新規作成。
	KindWorldCreate
	// KindWorldClone はワールドの複製。
	KindWorldClone
	// KindWorldRename はワールドの改名。
	KindWorldRename
	// KindWorldDelete はワールドの削除。
	KindWorldDelete
)

// String は種類の名前を返す。進捗バナーにそのまま出る。
func (k Kind) String() string {
	switch k {
	case KindServerStart:
		return "サーバーの起動"
	case KindServerStop:
		return "サーバーの停止"
	case KindServerRestart:
		return "サーバーの再起動"
	case KindBackupCreate:
		return "バックアップの取得"
	case KindBackupRestore:
		return "バックアップからの復元"
	case KindWorldSwitch:
		return "ワールドの切り替え"
	case KindWorldCreate:
		return "ワールドの作成"
	case KindWorldClone:
		return "ワールドの複製"
	case KindWorldRename:
		return "ワールドの改名"
	case KindWorldDelete:
		return "ワールドの削除"
	default:
		return "不明な操作"
	}
}

// State は操作の状態。
type State int

const (
	// StatePending は開始前。
	StatePending State = iota
	// StateRunning は実行中。
	StateRunning
	// StateSucceeded は成功して終了した。終端。
	StateSucceeded
	// StateFailed は失敗して終了した。終端。
	StateFailed
)

// IsTerminal は終端状態かを返す。終端に達した操作は二度と動かない。
func (s State) IsTerminal() bool { return s == StateSucceeded || s == StateFailed }

// String は状態の名前を返す。
func (s State) String() string {
	switch s {
	case StatePending:
		return "開始前"
	case StateRunning:
		return "実行中"
	case StateSucceeded:
		return "成功"
	case StateFailed:
		return "失敗"
	default:
		return "不明"
	}
}

// Level は進捗ログの深刻度。
type Level int

const (
	// LevelInfo は通常のログ。
	LevelInfo Level = iota
	// LevelWarn は注意を促すログ。
	LevelWarn
	// LevelError は失敗に関わるログ。
	LevelError
)
