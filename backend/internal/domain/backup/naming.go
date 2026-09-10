package backup

import (
	"regexp"
	"strings"
	"time"
)

const (
	namePrefix      = "backup-"
	nameExt         = ".zip"
	timestampLayout = "20060102-150405"
	// unknownVersion は MC_VERSION が空のときの代わり。
	// 空にすると区切りが連続して解釈できない名前になる。
	unknownVersion = "unknown"
	// maxNoteLength はメモの上限。ファイル名全体を扱いやすい長さに保つ。
	maxNoteLength = 32
)

// timestampPattern は名前の中の日時を見つける。
// バージョン・ワールド名・メモのいずれもハイフンを含みうるため、
// 位置ではなく日時の形そのものを手がかりにする。
var timestampPattern = regexp.MustCompile(`(^|-)(\d{8}-\d{6})(-|$)`)

// NameInfo はファイル名から読み取れた情報。
//
// これはあくまで参考値である。バックアップの真のバージョンとワールド名は
// アーカイブ内の level.dat から読む。ファイル名は人が改名できてしまう。
type NameInfo struct {
	// Version は MC_VERSION の文字列。読み取れなければ空。
	Version string
	// Level はワールド名。旧形式のファイル名には含まれないため空になりうる。
	Level string
	// CreatedAt は取得日時。読み取れなければゼロ値。
	CreatedAt time.Time
}

// BuildName はバックアップのファイル名を組み立てる。
//
// 形式は backup-<バージョン>-<ワールド名>-<YYYYmmdd-HHMMSS>[-<メモ>].zip。
// バージョンを入れるのは、ワールドのアップグレードが片道で戻せないため
// 「どの版で取ったものか」がファイル名だけで分かる必要があるから。
func BuildName(version, level, note string, at time.Time) string {
	var b strings.Builder
	b.WriteString(namePrefix)
	b.WriteString(sanitizeVersion(version))
	b.WriteString("-")
	b.WriteString(level)
	b.WriteString("-")
	b.WriteString(at.Format(timestampLayout))
	if slug := NoteSlug(note); slug != "" {
		b.WriteString("-")
		b.WriteString(slug)
	}
	b.WriteString(nameExt)
	return b.String()
}

// ParseName はファイル名から情報を読み取る。
// 解釈できない部分はゼロ値のままにして、エラーにはしない。
// 人が付けた名前のアーカイブも一覧に出し続けるため。
func ParseName(filename string) NameInfo {
	rest, ok := strings.CutPrefix(filename, namePrefix)
	if !ok {
		return NameInfo{}
	}
	rest, ok = strings.CutSuffix(rest, nameExt)
	if !ok {
		return NameInfo{}
	}

	loc := timestampPattern.FindStringSubmatchIndex(rest)
	if loc == nil {
		return NameInfo{}
	}
	// 部分マッチ 2 が日時本体。前後の区切りは含まない。
	stamp := rest[loc[4]:loc[5]]
	createdAt, err := time.ParseInLocation(timestampLayout, stamp, time.Local)
	if err != nil {
		return NameInfo{}
	}

	version, level, _ := strings.Cut(rest[:loc[4]], "-")
	return NameInfo{
		Version:   strings.TrimSuffix(version, "-"),
		Level:     strings.TrimSuffix(level, "-"),
		CreatedAt: createdAt,
	}
}

// sanitizeVersion は区切りに使うハイフンをバージョンから取り除く。
// 含まれてしまうとワールド名との境界が判別できなくなる。
func sanitizeVersion(version string) string {
	cleaned := strings.Map(func(r rune) rune {
		if isNameSafe(r) || r == '.' {
			return r
		}
		return '_'
	}, version)
	if cleaned == "" {
		return unknownVersion
	}
	return cleaned
}

// NoteSlug は自由入力のメモをファイル名に使える形にする。
// 記号と非 ASCII はハイフンにまとめ、前後のハイフンを落とす。
//
// 日本語だけのメモは丸ごと落ちて空になる。呼び出し側は、
// メモが空でないのに結果が空なら「付けられなかった」と伝えるとよい。
func NoteSlug(note string) string {
	var b strings.Builder
	prevSep := true // 先頭のハイフンを抑止する
	for _, r := range note {
		if isNameSafe(r) {
			b.WriteRune(r)
			prevSep = false
			continue
		}
		if !prevSep {
			b.WriteRune('-')
			prevSep = true
		}
	}
	slug := strings.Trim(b.String(), "-")
	if len(slug) > maxNoteLength {
		slug = strings.Trim(slug[:maxNoteLength], "-")
	}
	return slug
}

func isNameSafe(r rune) bool {
	switch {
	case r >= 'A' && r <= 'Z', r >= 'a' && r <= 'z', r >= '0' && r <= '9':
		return true
	case r == '_':
		return true
	default:
		return false
	}
}

// FormatTimestamp はファイル名に使う日時の書式を返す。
func FormatTimestamp(at time.Time) string { return at.Format(timestampLayout) }
