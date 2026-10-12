package archive

import (
	"path"
	"strings"
)

/*
Layout はアーカイブのエントリの形。

アーカイブには 2 つの形がある。

	LayoutUnderData   data/<ワールド>/...   2026-10 より前に取ったもの
	LayoutWorldRoot   <ワールド>/...        現在の保管形式

**復元は両方を受け付けなければならない。** 片方だけにすると、既に
保管してあるアーカイブが戻せなくなる。

形によって変えるのは「置き場所の検証」と「展開の基点」だけで、
エントリ名は書き換えない。書き換えずに基点を変えるので、zip-slip の
不変条件が形ごとに増えない。
*/
type Layout int

const (
	// LayoutUnderData は data/<ワールド>/... の形。
	//
	// 既定にしてあるのは、ゼロ値で誤って緩い側へ倒れないようにするため。
	LayoutUnderData Layout = iota
	// LayoutWorldRoot は <ワールド>/... の形。
	//
	// シングルプレイの saves/ にそのまま置ける。展開の基点は data/ になる。
	LayoutWorldRoot
)

// String は形の名前を返す。ログと試験の表示用。
func (l Layout) String() string {
	switch l {
	case LayoutUnderData:
		return "data/ 配下"
	case LayoutWorldRoot:
		return "ワールド名から"
	default:
		return "不明"
	}
}

// requiredPrefix はその形でエントリに要求する接頭辞。
//
// LayoutWorldRoot は接頭辞を要求しない。展開の基点が data/ なので、
// 置き場所は基点そのものが保証する。
func (l Layout) requiredPrefix() string {
	if l == LayoutWorldRoot {
		return ""
	}
	return allowedRoot
}

/*
detectLayout は level.dat の位置から形を判定する。

data/ 配下にあるものを優先する。コンソール自身が作った古い形であり、
両方に level.dat があるアーカイブで新しい形と誤認すると、data/ を
二重に掘って展開することになる（backup.PlanImport と同じ判断）。

level.dat が見つからないときは、全エントリが data/ で始まるかで決める。
ワールドの無いアーカイブは復元の手前で弾かれるので、ここでは形だけ
決めれば足りる。
*/
func detectLayout(names []string) Layout {
	atRoot := false
	allUnderData := true

	for _, name := range names {
		if !strings.HasPrefix(name, allowedRoot) {
			allUnderData = false
		}
		if path.Base(name) != levelDatName {
			continue
		}
		switch depthOfLevelDat(name) {
		case underData:
			// data/<ワールド>/level.dat が 1 つでもあれば確定。
			return LayoutUnderData
		case atWorldRoot:
			atRoot = true
		}
	}

	if atRoot {
		return LayoutWorldRoot
	}
	if allUnderData && len(names) > 0 {
		return LayoutUnderData
	}
	return LayoutWorldRoot
}

// levelDatName はワールドの版が入るファイル。これがある場所で形を見分ける。
const levelDatName = "level.dat"

// level.dat の置かれている深さ。
type levelDatDepth int

const (
	// elsewhere は形の判定に使えない位置。
	elsewhere levelDatDepth = iota
	// underData は data/<ワールド>/level.dat。
	underData
	// atWorldRoot は <ワールド>/level.dat。
	atWorldRoot
)

func depthOfLevelDat(name string) levelDatDepth {
	dir := path.Dir(name)
	if rest, ok := strings.CutPrefix(dir, allowedRoot); ok {
		if rest != "" && !strings.Contains(rest, "/") {
			return underData
		}
		return elsewhere
	}
	if dir != "." && !strings.Contains(dir, "/") {
		return atWorldRoot
	}
	return elsewhere
}

// rootOf はエントリが属するルートを返す。復元の画面に出す。
//
//	LayoutUnderData   data/world/level.dat -> data/world、data/spigot.yml -> data/spigot.yml
//	LayoutWorldRoot   world/level.dat      -> world、    spigot.yml      -> spigot.yml
func rootOf(name string, layout Layout) string {
	rest, ok := strings.CutPrefix(name, layout.requiredPrefix())
	if !ok {
		return ""
	}
	first, _, _ := strings.Cut(rest, "/")
	if first == "" {
		return ""
	}
	return layout.requiredPrefix() + first
}

// levelDirOf はエントリがワールドの level.dat ならワールド名を返す。
func levelDirOf(name string, layout Layout) (string, bool) {
	if path.Base(name) != levelDatName {
		return "", false
	}
	rest, ok := strings.CutPrefix(name, layout.requiredPrefix())
	if !ok {
		return "", false
	}
	dir, file, hasSlash := strings.Cut(rest, "/")
	if !hasSlash || file != levelDatName || dir == "" {
		return "", false
	}
	return dir, true
}
