package backup

import (
	"fmt"

	"github.com/kkito0726/minecraft-server/backend/internal/domain/shared"
)

// RestoreDecision は復元してよいかの判断。
//
// 「一致しているとき以外は必ず承諾を求める」が原則。ワールドの
// アップグレードは片道で、新しいバージョンで開いたワールドは
// 古いサーバーでは二度と開けない。
//
// 復元はサーバーの版をアーカイブのワールドに合わせる（versionsync）ため、
// 版が勝手に上がることは起きない。それでもクライアント側の版は人が
// 合わせる必要があるので、食い違いは必ず伝える。
type RestoreDecision struct {
	Verdict Verdict
	// LevelNameMismatch はアーカイブのワールド名が現在稼働中のものと違う。
	LevelNameMismatch bool
	// RequiresConfirmation は利用者の明示的な承諾が要るか。
	RequiresConfirmation bool
	// Warnings は承諾を求める理由。日本語。
	//
	// 承諾を求めるときは必ず 1 つ以上入る。「確認してください」だけでは
	// 利用者は何を確認すべきか分からない。
	Warnings []string
}

// DecideRestore は復元前の判断を組み立てる。副作用を持たない。
//
// この関数は復元の実行時にもう一度呼ばれる。画面が出した確認の結果を
// そのまま信じず、サーバー側で必ず再判定する（REQ-113）。
func DecideRestore(
	archive, current shared.WorldVersion,
	archiveLevel, currentLevel string,
) RestoreDecision {
	decision := RestoreDecision{
		Verdict:           CompareVersions(archive, current),
		LevelNameMismatch: archiveLevel != currentLevel,
	}
	decision.Warnings = warningsFor(decision.Verdict, archiveLevel, currentLevel)
	decision.RequiresConfirmation = decision.Verdict != VerdictMatch || decision.LevelNameMismatch
	return decision
}

func warningsFor(verdict Verdict, archiveLevel, currentLevel string) []string {
	var warnings []string

	switch verdict {
	case VerdictOlderWillUpgrade:
		warnings = append(warnings,
			"アーカイブの方が古いバージョンです。復元すると、サーバーの版も"+
				"アーカイブのワールドに合わせて下げます。ワールドが勝手に"+
				"新しい版へ上がることはありませんが、遊ぶ人はクライアントの版も"+
				"合わせる必要があります。")
	case VerdictNewerIncompatible:
		warnings = append(warnings,
			"アーカイブの方が新しいバージョンです。復元すると、サーバーの版も"+
				"アーカイブのワールドに合わせて上げます。遊ぶ人はクライアントの版も"+
				"合わせる必要があります。")
	case VerdictUnknown:
		warnings = append(warnings,
			"バージョンを読み取れませんでした。復元後にワールドが開けるかを確認できません。")
	case VerdictMatch:
		// 一致しているときは何も言わない。
	}

	if archiveLevel == currentLevel {
		return warnings
	}
	if archiveLevel == "" {
		return append(warnings,
			"アーカイブに含まれるワールドの名前を判定できませんでした。復元先を指定してください。")
	}
	return append(warnings, fmt.Sprintf(
		"アーカイブのワールドは %q ですが、現在稼働しているのは %q です。"+
			"そのまま復元すると稼働中のワールドは変化しません。",
		archiveLevel, currentLevel))
}
