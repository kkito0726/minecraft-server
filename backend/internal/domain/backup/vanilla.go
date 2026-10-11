package backup

import (
	"path"
	"slices"
	"strings"
)

/*
シングルプレイへ持ち出すための置き場所の変換規則。

Paper とバニラは、ワールドの中の .dat の置き場所が一部違う。Paper は
ワールドごとに別の設定を持てるようにするため、ゲームルールや天候を
ディメンション側に置く。バニラはワールド直下しか見ない。

この違いは Paper がサーバーとして読むときは自動で吸収される
（VanillaWorldMigration が起動時にバニラのワールドを移行する）。
逆向きの機構はバニラに無いため、持ち出す側で揃える必要がある。

揃えないと、クライアントは

	Unable to read or access the world gen settings file!
	Failed to load level data or datapacks, can't proceed with server load
	java.lang.IllegalStateException: Overworld settings missing

で読み込みを中断する。ワールド一覧には出るので、原因が分かりにくい。
*/

// vanillaRelocatedNames は Paper がディメンション側に置き、バニラが
// ワールド直下に置く .dat。
//
// これは網羅列挙である。ここに無いものは動かさない。
var vanillaRelocatedNames = []string{
	"game_rules.dat",
	"scheduled_events.dat",
	"wandering_trader.dat",
	"weather.dat",
	"world_gen_settings.dat",
}

/*
vanillaRelocateFrom は移す元のディレクトリ。オーバーワールドに限る。

オーバーワールド側の world_gen_settings.dat は 3 つのディメンション
すべての設定（minecraft:overworld / the_nether / the_end）を持っている。
ネザーやエンドのものを移すと、全体の設定を 1 ディメンション分で
上書きすることになる。
*/
const vanillaRelocateFrom = "dimensions/minecraft/overworld/data/minecraft/"

// vanillaRelocateTo は移す先。バニラが見る場所。
const vanillaRelocateTo = "data/minecraft/"

// VanillaRelocatedNames はワールド直下へ移す .dat の名前を返す。
func VanillaRelocatedNames() []string { return slices.Clone(vanillaRelocatedNames) }

/*
VanillaEntryName は保管しているアーカイブのエントリ名を、シングルプレイ
用のアーカイブでの名前に変換する。

戻り値が空なら、そのエントリは持ち出さない。持ち出さないのは次の 3 つ。

  - level のワールド以外（plugins / config / bukkit.yml / spigot.yml と
    別のワールド）。シングルプレイには置き場所が無く、saves/ の直下に
    ゴミとして並ぶ
  - ワールドのディレクトリそのものを指すエントリ
  - 展開先の外を指すエントリ。展開側でも拒否するが、作る側でも持ち込まない

ネザーとエンドにも Paper は同じ .dat を置くが、そちらは動かさずに
そのまま持ち出す。バニラは読まないので害がない。消す判断をここだけに
閉じてあるので、害があると分かったときに変えるのはこの関数だけで済む。
*/
func VanillaEntryName(entry, level string) string {
	if level == "" {
		return ""
	}

	rest, ok := vanillaRest(entry, level)
	if !ok {
		return ""
	}

	// オーバーワールド直下の .dat だけを、ワールド直下へ移す。
	// 深い位置にある同名のファイルは動かさない。
	if base, under := strings.CutPrefix(rest, vanillaRelocateFrom); under &&
		!strings.Contains(base, "/") && slices.Contains(vanillaRelocatedNames, base) {
		return level + "/" + vanillaRelocateTo + base
	}
	return level + "/" + rest
}

// vanillaRest は data/<level>/ を外した残りを返す。
// 外せない、あるいは展開先の外を指すものは偽を返す。
func vanillaRest(entry, level string) (string, bool) {
	name := strings.ReplaceAll(entry, `\`, "/")

	// ドライブレターつきの絶対パス（C:\... など）
	if len(name) >= 2 && name[1] == ':' {
		return "", false
	}
	if strings.HasPrefix(name, "/") {
		return "", false
	}

	// path.Clean が ".." を解決する。解決後にまだ ".." で始まるなら脱出している。
	cleaned := path.Clean(name)
	if cleaned == ".." || strings.HasPrefix(cleaned, "../") {
		return "", false
	}

	rest, ok := strings.CutPrefix(cleaned, dataPrefix+level+"/")
	if !ok || rest == "" {
		return "", false
	}
	return rest, true
}

// singleplayerSuffix はシングルプレイ用のファイル名に入れる印。
const singleplayerSuffix = "-singleplayer"

/*
SingleplayerFileName はシングルプレイ用に渡すときのファイル名を返す。

同じワールドの zip が 2 つ手元に並ぶので、名前で見分けられるようにする。
どちらを saves/ に置くのか分からないと、サーバー用を置いて
「起動しない」に戻ってしまう。

使える文字は英数字と . - _ だけ（backup.NewID が保証する）。印に
使うのもその範囲なので、Content-Disposition に入れても壊れない。
*/
func SingleplayerFileName(filename string) string {
	base, ok := strings.CutSuffix(filename, nameExt)
	if !ok || base == "" {
		return filename
	}
	return base + singleplayerSuffix + nameExt
}
