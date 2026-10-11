package backup_test

import (
	"slices"
	"testing"

	"github.com/kkito0726/minecraft-server/backend/internal/domain/backup"
)

// 移すファイルの列挙。これが欠けるとバニラのクライアントは
// IllegalStateException: Overworld settings missing で読み込みを中断する。
func TestVanillaRelocatedNames(t *testing.T) {
	t.Parallel()

	got := backup.VanillaRelocatedNames()
	want := []string{
		"game_rules.dat",
		"scheduled_events.dat",
		"wandering_trader.dat",
		"weather.dat",
		"world_gen_settings.dat",
	}

	if !slices.Equal(got, want) {
		t.Errorf("VanillaRelocatedNames() = %v。%v のはず", got, want)
	}
}

// 呼び出し側が書き換えても次の呼び出しに影響しない。
func TestVanillaRelocatedNamesIsNotShared(t *testing.T) {
	t.Parallel()

	first := backup.VanillaRelocatedNames()
	first[0] = "書き換え"

	if backup.VanillaRelocatedNames()[0] != "game_rules.dat" {
		t.Error("VanillaRelocatedNames() が内部の配列を共有している")
	}
}

/*
world_gen_settings.dat がワールド直下へ移ることが、この変換の要。

Paper は多世界対応のためこれをディメンション側に置くが、バニラは
ワールド直下しか見ない。見つからないと生成器もシードも決められず、
「Unable to read or access the world gen settings file!」のあと
「can't proceed with server load」で止まる。
*/
func TestVanillaEntryNameRelocatesWorldGenSettings(t *testing.T) {
	t.Parallel()

	const entry = "data/main/dimensions/minecraft/overworld/data/minecraft/world_gen_settings.dat"
	const want = "main/data/minecraft/world_gen_settings.dat"

	if got := backup.VanillaEntryName(entry, "main"); got != want {
		t.Errorf("VanillaEntryName(%q) = %q。%q のはず", entry, got, want)
	}
}

// 移すのは 5 つすべて。1 つでも残すとバニラ側が既定値で上書きする。
func TestVanillaEntryNameRelocatesAllNames(t *testing.T) {
	t.Parallel()

	for _, name := range backup.VanillaRelocatedNames() {
		entry := "data/main/dimensions/minecraft/overworld/data/minecraft/" + name
		want := "main/data/minecraft/" + name

		if got := backup.VanillaEntryName(entry, "main"); got != want {
			t.Errorf("VanillaEntryName(%q) = %q。%q のはず", entry, got, want)
		}
	}
}

/*
移すのはオーバーワールドのものだけ。

オーバーワールド側の world_gen_settings.dat は 3 つのディメンション
すべての設定を持っている（minecraft:overworld / the_nether / the_end）。
ネザーやエンドのものを移すと、全体の設定を 1 ディメンション分で
上書きすることになる。
*/
func TestVanillaEntryNameKeepsOtherDimensionsInPlace(t *testing.T) {
	t.Parallel()

	for _, dim := range []string{"the_nether", "the_end"} {
		entry := "data/main/dimensions/minecraft/" + dim + "/data/minecraft/world_gen_settings.dat"
		want := "main/dimensions/minecraft/" + dim + "/data/minecraft/world_gen_settings.dat"

		if got := backup.VanillaEntryName(entry, "main"); got != want {
			t.Errorf("VanillaEntryName(%q) = %q。%q のはず", entry, got, want)
		}
	}
}

// バニラもディメンション側に置くものは動かさない。
func TestVanillaEntryNameKeepsDimensionOwnedFiles(t *testing.T) {
	t.Parallel()

	for _, name := range []string{"chunk_tickets.dat", "raids.dat", "world_border.dat"} {
		entry := "data/main/dimensions/minecraft/overworld/data/minecraft/" + name
		want := "main/dimensions/minecraft/overworld/data/minecraft/" + name

		if got := backup.VanillaEntryName(entry, "main"); got != want {
			t.Errorf("VanillaEntryName(%q) = %q。%q のはず", entry, got, want)
		}
	}
}

// ワールドの中身は data/<名前>/ の皮を外して、そのまま持ち出す。
func TestVanillaEntryNameStripsDataPrefix(t *testing.T) {
	t.Parallel()

	cases := map[string]string{
		"data/main/level.dat":     "main/level.dat",
		"data/main/level.dat_old": "main/level.dat_old",
		"data/main/icon.png":      "main/icon.png",
		"data/main/dimensions/minecraft/overworld/region/r.0.0.mca": "main/dimensions/minecraft/overworld/region/r.0.0.mca",
		"data/main/players/data/abc.dat":                            "main/players/data/abc.dat",
		"data/main/data/minecraft/scoreboard.dat":                   "main/data/minecraft/scoreboard.dat",
	}

	for entry, want := range cases {
		if got := backup.VanillaEntryName(entry, "main"); got != want {
			t.Errorf("VanillaEntryName(%q) = %q。%q のはず", entry, got, want)
		}
	}
}

/*
サーバーの設定は持ち出さない。

シングルプレイには plugins も config も無く、置いても読まれない。
bukkit.yml や spigot.yml に至っては saves/ の直下にゴミとして並ぶ。
*/
func TestVanillaEntryNameDropsServerConfig(t *testing.T) {
	t.Parallel()

	dropped := []string{
		"data/plugins/spark/config.json",
		"data/config/paper-global.yml",
		"data/bukkit.yml",
		"data/spigot.yml",
	}

	for _, entry := range dropped {
		if got := backup.VanillaEntryName(entry, "main"); got != "" {
			t.Errorf("VanillaEntryName(%q) = %q。持ち出さないはず", entry, got)
		}
	}
}

// 別のワールドは持ち出さない。1 つの zip に 2 つのワールドが入ると、
// どちらを遊んでいるのか分からなくなる。
func TestVanillaEntryNameDropsOtherWorlds(t *testing.T) {
	t.Parallel()

	for _, entry := range []string{"data/other/level.dat", "data/mainly/level.dat"} {
		if got := backup.VanillaEntryName(entry, "main"); got != "" {
			t.Errorf("VanillaEntryName(%q) = %q。持ち出さないはず", entry, got)
		}
	}
}

// 展開先の外を指すエントリは持ち出さない。作る側でも塞ぐ。
func TestVanillaEntryNameDropsEscapingEntries(t *testing.T) {
	t.Parallel()

	escaping := []string{
		"data/main/../../etc/passwd",
		"../data/main/level.dat",
		"/data/main/level.dat",
		"data/../../etc/passwd",
	}

	for _, entry := range escaping {
		if got := backup.VanillaEntryName(entry, "main"); got != "" {
			t.Errorf("VanillaEntryName(%q) = %q。持ち出さないはず", entry, got)
		}
	}
}

// Windows で作られたアーカイブは区切りが \ になっていることがある。
// 判定はすべて / に揃えてから行う。
func TestVanillaEntryNameNormalizesBackslashes(t *testing.T) {
	t.Parallel()

	const entry = `data\main\dimensions\minecraft\overworld\data\minecraft\world_gen_settings.dat`
	const want = "main/data/minecraft/world_gen_settings.dat"

	if got := backup.VanillaEntryName(entry, "main"); got != want {
		t.Errorf("VanillaEntryName(%q) = %q。%q のはず", entry, got, want)
	}
}

// ワールド名が空のときは何も持ち出さない。判定の基準が無いのに
// 通すと、data/ 配下の全部をワールドとして吐き出すことになる。
func TestVanillaEntryNameRequiresLevel(t *testing.T) {
	t.Parallel()

	if got := backup.VanillaEntryName("data/main/level.dat", ""); got != "" {
		t.Errorf("VanillaEntryName(level=\"\") = %q。持ち出さないはず", got)
	}
}

// ワールドのディレクトリそのものを指すエントリは持ち出さない。
func TestVanillaEntryNameDropsWorldRootItself(t *testing.T) {
	t.Parallel()

	for _, entry := range []string{"data/main", "data/main/", "data", "data/"} {
		if got := backup.VanillaEntryName(entry, "main"); got != "" {
			t.Errorf("VanillaEntryName(%q) = %q。持ち出さないはず", entry, got)
		}
	}
}

/*
シングルプレイ用のファイル名は、サーバー用と見分けがつくこと。

同じワールドの zip が 2 つ手元に並ぶ。どちらを saves/ に置けば
よいのか名前で分からないと、間違えたほうを置いて「起動しない」に戻る。
*/
func TestSingleplayerFileName(t *testing.T) {
	t.Parallel()

	cases := map[string]string{
		"backup-26.3-main-20261011-181445.zip":           "backup-26.3-main-20261011-181445-singleplayer.zip",
		"backup-26.2-world-20260916-030000-imported.zip": "backup-26.2-world-20260916-030000-imported-singleplayer.zip",
	}

	for in, want := range cases {
		if got := backup.SingleplayerFileName(in); got != want {
			t.Errorf("SingleplayerFileName(%q) = %q。%q のはず", in, got, want)
		}
	}
}

// .zip で終わらない名前は触らない。ID は .zip を要求するので
// 本来は来ないが、名前の規則が壊れたときに黙って変な名前を作らない。
func TestSingleplayerFileNameLeavesUnexpectedNamesAlone(t *testing.T) {
	t.Parallel()

	for _, in := range []string{"backup.tar", "", "backup"} {
		if got := backup.SingleplayerFileName(in); got != in {
			t.Errorf("SingleplayerFileName(%q) = %q。そのままのはず", in, got)
		}
	}
}
