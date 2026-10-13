package backup_test

import (
	"slices"
	"strings"
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

// --- 保管するアーカイブでのエントリ名 ---

/*
保管する形はバニラの配置にしてある。

直せない側（クライアント）には手が届かず、吸収できる側（サーバー）には
手が届く。だから複雑さは吸収できる側に寄せる。保管が 1 形式なら、
利用者が形を選ぶ場面が無くなる。

サーバーの設定はルートに並べて残す。これを落とすと、手編集した
spigot.yml が復元で戻らなくなる（REQ-004）。
*/
func TestStoredEntryNamePutsWorldAtRoot(t *testing.T) {
	t.Parallel()

	cases := map[string]string{
		// ワールドは data/ の皮を外してルートへ
		"data/main/level.dat":                                       "main/level.dat",
		"data/main/players/data/abc.dat":                            "main/players/data/abc.dat",
		"data/main/dimensions/minecraft/overworld/region/r.0.0.mca": "main/dimensions/minecraft/overworld/region/r.0.0.mca",
		// Paper がディメンション側に置く 5 つはワールド直下へ
		"data/main/dimensions/minecraft/overworld/data/minecraft/world_gen_settings.dat": "main/data/minecraft/world_gen_settings.dat",
		"data/main/dimensions/minecraft/overworld/data/minecraft/game_rules.dat":         "main/data/minecraft/game_rules.dat",
		// サーバーの設定はルートに並べる
		"data/plugins/Essentials.jar":  "plugins/Essentials.jar",
		"data/config/paper-global.yml": "config/paper-global.yml",
		"data/bukkit.yml":              "bukkit.yml",
		"data/spigot.yml":              "spigot.yml",
	}

	for entry, want := range cases {
		if got := backup.StoredEntryName(entry, "main"); got != want {
			t.Errorf("StoredEntryName(%q) = %q。%q のはず", entry, got, want)
		}
	}
}

// 対象の網羅列挙の外にあるものは入れない（REQ-004）。
// server.properties は rcon.password を平文で持つ。
func TestStoredEntryNameDropsNonTargets(t *testing.T) {
	t.Parallel()

	dropped := []string{
		"data/server.properties",
		"data/ops.json",
		"data/whitelist.json",
		"data/logs/latest.log",
		"data/libraries/lib.jar",
		"data/versions/v.json",
		"data/cache/c.bin",
		"data/paper-26.3-174.jar",
		"data/other/level.dat",
		"data/.env",
	}

	for _, entry := range dropped {
		if got := backup.StoredEntryName(entry, "main"); got != "" {
			t.Errorf("StoredEntryName(%q) = %q。入れないはず", entry, got)
		}
	}
}

// 展開先の外を指すものは入れない。作る側でも塞ぐ。
func TestStoredEntryNameDropsEscapingEntries(t *testing.T) {
	t.Parallel()

	for _, entry := range []string{"../etc/passwd", "/etc/passwd", "data/../../etc/passwd"} {
		if got := backup.StoredEntryName(entry, "main"); got != "" {
			t.Errorf("StoredEntryName(%q) = %q。入れないはず", entry, got)
		}
	}
}

// ワールド名が空のときは何も入れない。基準が無いのに通すと、
// data/ 配下の全部をワールドとして吐き出すことになる。
func TestStoredEntryNameRequiresLevel(t *testing.T) {
	t.Parallel()

	if got := backup.StoredEntryName("data/main/level.dat", ""); got != "" {
		t.Errorf("StoredEntryName(level=\"\") = %q。入れないはず", got)
	}
}

/*
ディレクトリかファイルかは見分けない。

ExtraPaths には plugins のようなディレクトリと bukkit.yml のような
単体のファイルが混ざっている。どちらも「data/ を外した名前」で
同じに扱える。末尾の / を付けるかは走査している側が決める。

名前の末尾（.yml など）で見分けると、ExtraPaths に .yml 以外の
ファイルを足した瞬間に黙って落ちる。
*/
func TestStoredEntryNameTreatsExtraDirsAndFilesAlike(t *testing.T) {
	t.Parallel()

	cases := map[string]string{
		// ディレクトリそのもの。空なら plugins/ として残せる
		"data/plugins": "plugins",
		"data/config":  "config",
		// 単体のファイル
		"data/bukkit.yml": "bukkit.yml",
		"data/spigot.yml": "spigot.yml",
	}

	for entry, want := range cases {
		if got := backup.StoredEntryName(entry, "main"); got != want {
			t.Errorf("StoredEntryName(%q) = %q。%q のはず", entry, got, want)
		}
	}
}

// data/ そのものと、ワールドのディレクトリそのものは入れない。
// ワールドの中身は必ず data/<ワールド>/ の下にある。
func TestStoredEntryNameDropsDataAndWorldRoot(t *testing.T) {
	t.Parallel()

	for _, entry := range []string{"data", "data/", "data/main"} {
		if got := backup.StoredEntryName(entry, "main"); got != "" {
			t.Errorf("StoredEntryName(%q) = %q。入れないはず", entry, got)
		}
	}
}

/*
対象の列挙が増えても、名前の付け方は変わらないこと。

ExtraPaths() を走って確かめる。個別の名前を書き並べると、列挙に
.yml 以外のファイルを足したときに試験が追従せず、黙って落ちる経路が
できる（実際に一度その作りにしてしまった）。
*/
func TestStoredEntryNameCoversEveryExtraPath(t *testing.T) {
	t.Parallel()

	for _, extra := range backup.ExtraPaths() {
		// 対象そのもの
		if got := backup.StoredEntryName("data/"+extra, "main"); got != extra {
			t.Errorf("StoredEntryName(%q) = %q。%q のはず", "data/"+extra, got, extra)
		}
		// 対象の配下（ディレクトリの場合）
		under := extra + "/nested/file.bin"
		if got := backup.StoredEntryName("data/"+under, "main"); got != under {
			t.Errorf("StoredEntryName(%q) = %q。%q のはず", "data/"+under, got, under)
		}
	}
}

// --- シングルプレイへ渡すときの絞り込み ---

/*
保管している形が既にバニラの配置なら、置き場所の変換は要らない。

残っているのは「サーバー専用のものを落とす」ことだけ。saves/ に
展開したときに plugins/ や bukkit.yml が散らないようにする。
*/
func TestWorldOnlyEntryNameKeepsOnlyTheWorld(t *testing.T) {
	t.Parallel()

	kept := map[string]string{
		"main/level.dat": "main/level.dat",
		"main/data/minecraft/world_gen_settings.dat":            "main/data/minecraft/world_gen_settings.dat",
		"main/dimensions/minecraft/the_nether/region/r.0.0.mca": "main/dimensions/minecraft/the_nether/region/r.0.0.mca",
	}
	for entry, want := range kept {
		if got := backup.WorldOnlyEntryName(entry, "main"); got != want {
			t.Errorf("WorldOnlyEntryName(%q) = %q。%q のはず", entry, got, want)
		}
	}

	dropped := []string{
		"plugins/Essentials.jar",
		"config/paper-global.yml",
		"bukkit.yml",
		"spigot.yml",
		"other/level.dat",
		"mainly/level.dat",
		"main",
	}
	for _, entry := range dropped {
		if got := backup.WorldOnlyEntryName(entry, "main"); got != "" {
			t.Errorf("WorldOnlyEntryName(%q) = %q。落とすはず", entry, got)
		}
	}
}

// 展開先の外を指すものは渡さない。
func TestWorldOnlyEntryNameDropsEscapingEntries(t *testing.T) {
	t.Parallel()

	for _, entry := range []string{"../main/level.dat", "/main/level.dat", "main/../../etc/passwd"} {
		if got := backup.WorldOnlyEntryName(entry, "main"); got != "" {
			t.Errorf("WorldOnlyEntryName(%q) = %q。落とすはず", entry, got)
		}
	}
}

// ワールド名が無ければ何も渡さない。
func TestWorldOnlyEntryNameRequiresLevel(t *testing.T) {
	t.Parallel()

	if got := backup.WorldOnlyEntryName("main/level.dat", ""); got != "" {
		t.Errorf("WorldOnlyEntryName(level=\"\") = %q。落とすはず", got)
	}
}

// --- 復元後の起動について伝えること ---

/*
バニラの配置で保管したアーカイブを復元すると、次の起動で Paper が
置き場所を自分の形へ移す。30 秒の足止めがあるので、何が起きるのかを
先に伝える。

黙って 30 秒待たせると「復元が固まった」と見える。利用者に形を
意識させない代わりに、遅くなる理由だけは伝える。
*/
func TestWorldMigrationNoticeExplainsTheDelay(t *testing.T) {
	t.Parallel()

	notice := backup.WorldMigrationNotice()
	if notice == "" {
		t.Fatal("通知が空")
	}

	// 実測した数字が入っていること。「30 秒ほど」だけでは、
	// 固まったのか待てばよいのかを判断できない。
	for _, want := range []string{"30 秒", "1 回"} {
		if !strings.Contains(notice, want) {
			t.Errorf("通知に %q が無い: %s", want, notice)
		}
	}
	// データが壊れないことも伝える。Paper 自身は
	// 「バックアップが無いなら今すぐ落とせ」と警告するため。
	if !strings.Contains(notice, "変わりません") {
		t.Errorf("通知にデータが変わらないことが無い: %s", notice)
	}
}
