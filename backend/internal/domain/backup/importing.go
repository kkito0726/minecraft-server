package backup

import (
	"errors"
	"fmt"
	"path"
	"sort"
	"strings"

	"github.com/kkito0726/minecraft-server/backend/internal/domain/world"
)

// ErrNotImportable は取り込めないアーカイブであることを表す。
var ErrNotImportable = errors.New("このアーカイブは取り込めません")

// levelDatName はワールドの版が入るファイル。これがある場所でワールドを見分ける。
const levelDatName = "level.dat"

// dataPrefix は展開先として許される唯一の接頭辞。
const dataPrefix = "data/"

// ImportPlan は持ち込まれたアーカイブをどう取り込むかの計画。
type ImportPlan struct {
	// Level はアーカイブに入っているワールドの名前。
	Level string
	// Prefix は取り込むときに各エントリの前に付ける文字列。
	//
	// 空なら包み直しは要らない（既に data/ 配下にある）。
	Prefix string
}

// NeedsRewrap は包み直しが要るかを返す。
func (p ImportPlan) NeedsRewrap() bool { return p.Prefix != "" }

/*
PlanImport はアーカイブ内の level.dat の位置から取り込み方を決める。

管理コンソールが作るアーカイブは data/<名前>/level.dat の形をしている
が、配布されているワールドや他のサーバーから持ってきたものは
<名前>/level.dat の形をしていることが多い。後者をそのまま展開すると
data/ の外に書くことになるため、取り込みの時点で包み直す。

どちらとも判断できないものは受け取らない。名前を勝手に決めたり
片方を黙って選んだりすると、利用者が戻したつもりのワールドが
別物になる。
*/
func PlanImport(levelDatEntries []string) (ImportPlan, error) {
	underData, atRoot, hasBare := classify(levelDatEntries)

	// data/ 配下にあるものを優先する。コンソール自身が作った形。
	if len(underData) > 0 {
		// 並び順で結果が変わらないようにする。
		sort.Strings(underData)
		return newPlan(underData[0], "")
	}

	switch {
	case len(atRoot) == 1:
		return newPlan(atRoot[0], dataPrefix)
	case len(atRoot) > 1:
		sort.Strings(atRoot)
		return ImportPlan{}, fmt.Errorf(
			"%w: ワールドが複数あります（%s）。1 つだけにしてください",
			ErrNotImportable, strings.Join(atRoot, ", "))
	case hasBare:
		return ImportPlan{}, fmt.Errorf(
			"%w: level.dat が zip の直下にあります。"+
				"ワールド名のフォルダに入れてから固め直してください", ErrNotImportable)
	default:
		return ImportPlan{}, fmt.Errorf(
			"%w: ワールドが見つかりません（level.dat がありません）", ErrNotImportable)
	}
}

// classify は level.dat のエントリを置き場所ごとに仕分ける。
//
// 返すのはワールド名であって、エントリ名ではない。
func classify(entries []string) (underData, atRoot []string, hasBare bool) {
	for _, entry := range entries {
		name := path.Clean(strings.ReplaceAll(entry, `\`, "/"))
		if path.Base(name) != levelDatName {
			continue
		}

		dir := path.Dir(name)
		switch {
		case dir == ".":
			// zip の直下。ワールド名が決まらない。
			hasBare = true
		case strings.HasPrefix(dir, dataPrefix):
			if level := strings.TrimPrefix(dir, dataPrefix); !strings.Contains(level, "/") {
				underData = appendUnique(underData, level)
			}
		case !strings.Contains(dir, "/"):
			atRoot = appendUnique(atRoot, dir)
		}
	}
	return underData, atRoot, hasBare
}

func appendUnique(list []string, value string) []string {
	for _, existing := range list {
		if existing == value {
			return list
		}
	}
	return append(list, value)
}

// newPlan はワールド名を検証したうえで計画を作る。
//
// 取り込んだ名前はそのままディレクトリ名になる。world.NewName を
// 通すのは、名前の規則の出典を 1 つに保つため（NFR-304）。
func newPlan(level, prefix string) (ImportPlan, error) {
	if _, err := world.NewName(level); err != nil {
		return ImportPlan{}, fmt.Errorf("%w: %w", ErrNotImportable, err)
	}
	return ImportPlan{Level: level, Prefix: prefix}, nil
}

// IsLevelDatOf は entry が level のワールドの level.dat かを返す。
//
// data/<level>/level.dat と <level>/level.dat の両方を同じものとみなす。
// 取り込みの前後で置き場所が変わるため、どちらの形でも引けるようにする。
func IsLevelDatOf(entry, level string) bool {
	name := path.Clean(strings.ReplaceAll(entry, `\`, "/"))
	if path.Base(name) != levelDatName {
		return false
	}
	dir := path.Dir(name)
	return dir == level || dir == dataPrefix+level
}
