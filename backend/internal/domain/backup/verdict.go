// Package backup はバックアップのドメインモデル。
package backup

import "github.com/kkito0726/minecraft-server/backend/internal/domain/shared"

// Verdict はアーカイブと現在のワールドのバージョン比較の結果。
type Verdict int

const (
	// VerdictUnknown はどちらかの level.dat を読めなかったことを表す。
	// 安全側に倒して承諾を求める。
	VerdictUnknown Verdict = iota
	// VerdictMatch は DataVersion が一致していることを表す。
	VerdictMatch
	// VerdictOlderWillUpgrade はアーカイブの方が古いことを表す。
	// 復元すると片道アップグレードが再度走る。
	VerdictOlderWillUpgrade
	// VerdictNewerIncompatible はアーカイブの方が新しいことを表す。
	// 現在の MC_VERSION では開けない可能性が高い。
	VerdictNewerIncompatible
)

// CompareVersions はアーカイブと現在のワールドのバージョンを比べる。
//
// 比較するのは DataVersion（整数）どうしであって、MC_VERSION の文字列ではない。
// 文字列と整数の対応表はネットワークなしに作れないため、
// 判定の主軸を「アーカイブ vs 現在ディスク上のワールド」に限定している。
func CompareVersions(archive, current shared.WorldVersion) Verdict {
	archiveVersion, archiveOK := archive.DataVersion()
	currentVersion, currentOK := current.DataVersion()

	if !archiveOK || !currentOK {
		return VerdictUnknown
	}

	switch {
	case archiveVersion.Equals(currentVersion):
		return VerdictMatch
	case archiveVersion.LessThan(currentVersion):
		return VerdictOlderWillUpgrade
	default:
		return VerdictNewerIncompatible
	}
}

// RequiresConfirmation は復元に利用者の明示的な承諾が要るかを返す。
// 一致していない場合は必ず承諾を求める。
func (v Verdict) RequiresConfirmation() bool { return v != VerdictMatch }

// Warning は画面に出す警告文を返す。一致している場合は空文字。
func (v Verdict) Warning() string {
	switch v {
	case VerdictOlderWillUpgrade:
		return "このバックアップは現在のワールドより古いバージョンです。" +
			"復元すると、ワールドのアップグレードが再度実行されます。" +
			"アップグレードは片道で、元のバージョンには戻せません。"
	case VerdictNewerIncompatible:
		return "このバックアップは現在のワールドより新しいバージョンです。" +
			"現在の MC_VERSION では開けない可能性が高いため、" +
			"先に MC_VERSION を上げることを検討してください。"
	case VerdictUnknown:
		return "バックアップまたは現在のワールドの level.dat を読めず、" +
			"バージョンの整合を確認できません。"
	case VerdictMatch:
		return ""
	default:
		return ""
	}
}

// String は判定の名前を返す。ログとテストの表示用。
func (v Verdict) String() string {
	switch v {
	case VerdictMatch:
		return "一致"
	case VerdictOlderWillUpgrade:
		return "アーカイブの方が古い"
	case VerdictNewerIncompatible:
		return "アーカイブの方が新しい"
	case VerdictUnknown:
		return "判定不能"
	default:
		return "未定義"
	}
}
