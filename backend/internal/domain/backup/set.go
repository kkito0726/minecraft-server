package backup

import "slices"

// extraPaths はワールド以外にアーカイブへ含めるもの。data/ からの相対パス。
//
// これはバックアップ対象の網羅列挙である（REQ-004）。ここに無いものは入れない。
//
//   - plugins / config … 手で入れたものは再取得できない
//   - bukkit.yml / spigot.yml … .env から再生成されない。
//     docs/raspberry-pi.md が手編集を指示しているため、入れないと設定が失われる
//
// 意図的に入れていないもの:
//
//   - server.properties … rcon.password と management-server-secret を平文で持つ。
//     起動のたびに .env から再生成されるので失っても困らない
//   - libraries / versions / cache / *.jar … 再取得できる。合計 227MB
//   - logs / ops.json / whitelist.json … 再生成されるか、.env から復元できる
var extraPaths = []string{"plugins", "config", "bukkit.yml", "spigot.yml"}

// excludedNames はアーカイブから除くファイル名。
//
// session.lock は稼働中のサーバーが掴んでいる。含めても起動時に
// 作り直されるため無害だが、複製元の PID を持ち込む意味がない。
var excludedNames = []string{"session.lock"}

// ExtraPaths はワールド以外のバックアップ対象を返す。
func ExtraPaths() []string { return slices.Clone(extraPaths) }

// ExcludedNames はアーカイブから除くファイル名を返す。
func ExcludedNames() []string { return slices.Clone(excludedNames) }
