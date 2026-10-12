# バックアップと復元

ワールドデータの退避と巻き戻しの手順。個人利用（数人以下、Tailscale か LAN で接続）を想定している。

> **画面からもできる。** 同じ手順を [管理コンソール](admin-console.md) がそのまま実行する。
> `save-on` の戻し忘れや上書き展開のような事故が構造的に起きないので、普段はそちらを使う。
> このドキュメントは**何が起きているか**と、コンソールが使えないときの手順を扱う。

## 何をバックアップすればいいか

`data/` は 246MB あるが、そのほとんどはサーバー jar とライブラリで、再ダウンロードされるため保存する意味がない。

| パス | サイズ目安 | バックアップ |
|---|---|---|
| `data/world/` | 18MB | **必須。** ワールド本体 |
| `data/plugins/` | 1MB 未満 | プラグインを使うなら必須。jar と設定が入る |
| `data/config/` | 数十 KB | **必須。** Paper の設定 |
| `data/bukkit.yml`、`data/spigot.yml` | 数十 KB | **必須。** `.env` から再生成されない。[raspberry-pi.md](raspberry-pi.md) が手編集を指示している `moved-too-quickly-multiplier` もここにある |
| `data/ops.json`、`data/whitelist.json` | 数 KB | **不要。** `.env` の `MC_OPS` / `MC_WHITELIST` から再生成される |
| `data/libraries/`、`data/versions/`、`data/cache/`、`data/*.jar` | 220MB+ | **不要。** 起動時に再取得される |
| `data/logs/` | — | **不要** |
| `data/server.properties` | — | **入れてはいけない。** 毎回 `.env` から再生成されるうえ、`rcon.password` と `management-server-secret` を平文で持つ |

**ワールドが複数あるときは `data/world` とは限らない。** 稼働中のワールドは `.env` の
`MC_LEVEL` が指すディレクトリで、既定が `world` というだけ。`MC_LEVEL=creative` なら
実体は `data/creative/` にある。手で取るときは `source .env` して `data/${MC_LEVEL}` を使う。

プレイヤーのインベントリ・体力・座標は `data/world/players/` にあるため、`data/world` を取れば一緒に保存される。

ネザーとエンドも `data/world/dimensions/minecraft/` の中にある。`world_nether` / `world_the_end` のような
別ディレクトリは Paper 26.2 では作られないので、`data/world` ひとつで全ディメンションが揃う。

`.env` と `compose.yaml` は Git で管理する（`.env` は Git 管理外なので別途控えを取る）。
これらとワールドが揃えば、サーバーは完全に再現できる。

## 取得手順

### 停止してから取る（確実・推奨）

個人利用ならこれで十分。書き込みが完全に止まるので壊れようがない。

```bash
docker compose down
zip -qr "backup-${MC_VERSION}-${MC_LEVEL}-$(date +%Y%m%d-%H%M%S).zip" \
  "data/${MC_LEVEL}" data/plugins data/config data/bukkit.yml data/spigot.yml
docker compose up -d
```

`MC_VERSION` は `source .env` で読み込める（`.env` の値のうち空白を含むものは
`MC_MOTD="..."` のようにクォートしてある。クォートを外すと `source` が失敗する）。

### 稼働させたまま取る

サーバーは常時 region ファイルを書いている。書き込み途中を掴むとそのチャンクが壊れた
バックアップになるため、**必ず先に書き込みを止める**。

```bash
docker compose exec -T mc rcon-cli save-off
docker compose exec -T mc rcon-cli save-all
zip -qr "backup-${MC_VERSION}-${MC_LEVEL}-$(date +%Y%m%d-%H%M%S).zip" \
  "data/${MC_LEVEL}" data/plugins data/config data/bukkit.yml data/spigot.yml
docker compose exec -T mc rcon-cli save-on
```

`save-on` を忘れると以降の変更がディスクに書かれない。スクリプトにするなら `trap` で必ず戻すこと。

```bash
#!/bin/bash
set -euo pipefail
cd "$(dirname "$0")"
source .env

restore_saving() { docker compose exec -T mc rcon-cli save-on >/dev/null; }
trap restore_saving EXIT

docker compose exec -T mc rcon-cli save-off
docker compose exec -T mc rcon-cli save-all
zip -qr "backup-${MC_VERSION}-${MC_LEVEL}-$(date +%Y%m%d-%H%M%S).zip" \
  "data/${MC_LEVEL}" data/plugins data/config data/bukkit.yml data/spigot.yml
```

## 復元手順

### 「上書き」ではなく「消してから展開」

ここが最も事故りやすい。既存の `data/world` にそのまま展開すると、
**バックアップに含まれていない新しい region ファイルが残ったまま混ざる。**
古い地形と新しい地形が同居した壊れたワールドになる。

```bash
docker compose down

# rm ではなく mv で退避しておくと、復元に失敗しても戻せる
mv data/world "data/world.broken-$(date +%Y%m%d-%H%M%S)"

# -o は data/plugins と data/config の上書き確認をスキップするため。
# data/world は退避済みなので、ここで丸ごと再生成される
unzip -qo backup-26.2-20260901-003000.zip

docker compose up -d
docker compose logs -f mc                  # Done (...) が出れば成功
```

問題なく動くことを確認してから、退避した `data/world.broken-*` を削除する。

`session.lock` は起動時に作り直されるので、zip に含まれていても問題ない。

**アーカイブのワールド名と、いま動いているワールド名が違うときは要注意。**
zip は取得したときの名前（たとえば `data/world/...`）を持っている。その後
`MC_LEVEL=creative` に切り替えていると、素直に展開しても `data/world` が戻るだけで
**稼働中のワールドは何も変わらない**。何も起きていないように見える。

どちらかを選ぶ。

```bash
# A. アーカイブの名前で戻し、そちらへ切り替える
unzip -qo backup-26.2-world-20260901-003000.zip
sed -i '' 's/^MC_LEVEL=.*/MC_LEVEL=world/' .env

# B. いま動いているワールドとして戻す（展開してから名前を付け替える）
unzip -qo backup-26.2-world-20260901-003000.zip
mv data/world data/creative
```

管理コンソールの復元ダイアログが「どのワールド名として復元しますか」を聞くのは、
この選択のこと。

`-o` を付けないと、既存の `data/plugins` / `data/config` について上書き確認のプロンプトが出て止まる。

### 退避したディレクトリの後始末

`.broken-*`（復元による退避）と `.deleted-*`（削除による退避）は**自動では消えない**。
これは意図した動作で、消してしまうと復元がうまくいかなかったときに戻す先が無くなる。

```bash
ls -d data/*.broken-* data/*.deleted-* 2>/dev/null   # 何が残っているか
du -sh data/*.broken-* 2>/dev/null                   # どれだけ食っているか
rm -rf "data/world.broken-20260910-214428"           # 確認できたものだけ消す
```

管理コンソールでは `/worlds` の「退避したワールド」に、大きさと日時つきで並ぶ。

**残しっぱなしにするとディスクを食う。** ワールド 1 つぶんが丸ごと残るので、
復元を何度か試すとすぐに数百 MB になる。動作を確認したら消す。

## バージョンとの関係

**ワールドのアップグレードは片道。** 一度新しいバージョンで起動したワールドは、
古いバージョンのサーバーでは開けない。

- `MC_VERSION` を上げた後に古いバックアップを復元すると、**再び同じアップグレードが走る**
- 新しいバージョンで保存されたワールドを古い `MC_VERSION` に戻すことはできない

このため、バックアップのファイル名にバージョンを入れておく。
`MC_VERSION` を上げる前には、必ずその時点のバックアップを取ること。

### ファイル名の規約

```
backup-<MC_VERSION>-<ワールド名>-<YYYYmmdd-HHMMSS>[-<メモ>].zip
backup-26.2-world-20260910-160000-before-update.zip
```

管理コンソールが付けるのもこの形。ワールド名を入れるのは、`MC_LEVEL` を
切り替えていると「どのワールドのバックアップか」がファイル名からしか
分からなくなるため。メモに使えるのは英数字と下線だけ（日本語は落ちる）。

画面から**取り込んだ** zip はメモの位置が `imported` になる
（`backup-26.2-world-20260916-203000-imported.zip`）。持ち込んだものだと
後から分かるようにしてある。版が読めなかったアーカイブは `unknown` が入る。

**ただしファイル名は改名できるので、これは目印にすぎない。** 復元するときに
本当に信用してよいのは、アーカイブの中の `level.dat` から読んだバージョンの方。
管理コンソールが一覧に出しているのも、ファイル名ではなくそちらの値。

## 保管場所

**管理コンソールが取ったバックアップも `backups/` に置くだけで、外へは自動では送らない。**
自動転送とスケジュール実行は v1 の対象外にしてある。画面の一覧にも毎回その但し書きを出している。

**写しを取る口は 2 つ。** 一覧の「ダウンロード」を押せば、そのアーカイブを手元の PC へ
zip のまま落とせる（1 世代ずつ、押したときに取れる）。決まった場所へ定期的に送りたい
場合は、下のように手作業か cron で回す。

同じ Mac の中に置いているだけでは、ディスク障害で本体もろとも消える。
最低ひとつは別の場所にコピーが行くようにしておく。

- iCloud Drive や外付けディスクにコピー先を置く
- Tailscale 経由で別マシンへ `scp` する

```bash
scp backup-*.zip user@<tailscale-hostname>:~/minecraft-backups/
```

## ワールドを作り直す

バックアップではなく、まっさらな状態から始めたい場合。

```bash
docker compose down
rm -rf data/world
docker compose up -d
```

`level-seed` を指定したい場合は `.env` の `MC_SEED` に書いてから `docker compose up -d` する。
**生成が終わったら空に戻すこと。** 残っていると、次にコンテナを作り直したときに
見えない形で効いてしまう。

別の名前で作れば、いまのワールドを消さずに増やせる。

```bash
sed -i '' 's/^MC_LEVEL=.*/MC_LEVEL=creative/' .env
docker compose up -d          # data/creative/ が生成される。data/world はそのまま
```

管理コンソールの `/worlds` はこれを画面から行い、`MC_SEED` の消し忘れも自動で直す。

## 動作確認済みの前提

この手順は以下の構成で、取得から復元まで一往復させて確認している。

- Paper 26.2 / `itzg/minecraft-server`
- `data/` はバインドマウント（`./data:/data`）なので、ホストから直接 zip / unzip できる
- `data/world` 直下は `data/ datapacks/ dimensions/ level.dat level.dat_old players/ session.lock`

確認内容:

1. 稼働中に `save-off` → `save-all` → zip → `save-on` を実行し、107 ファイル / 5.1MB の zip を取得
2. バックアップ後に `data/world` へファイルを1つ追加
3. `mv` で退避 → `unzip -qo` → 起動
4. **バックアップ時点のファイルが戻り、バックアップ後に追加したファイルが消えている**ことを確認
5. コンテナが `healthy` になることを確認

## 手作業と画面の対応

[管理コンソール](admin-console.md) が実行しているのは、ここに書いた手順そのもの。
片方しか使えない状況に備えて対応を残しておく。

| 画面の操作 | 相当する手作業 |
|---|---|
| 取得（稼働したまま） | `save-off` → `save-all` → `zip` → `save-on` |
| 取得（停止して） | `down` → `zip` → `up -d` |
| 復元 | `down` → `mv` で退避 → `unzip -qo` → `up -d` |
| 復元先の選択 | 展開後に `MC_LEVEL` を書き換えるか、ディレクトリを `mv` するか |
| 保持ポリシーの適用 | 古い zip を手で消す |
| 持ち出し（ダウンロード） | `scp user@<tailscale-hostname>:<プロジェクト>/backups/backup-*.zip .` |
| zip の取り込み | `cp <zip> backups/`（形によっては包み直しが要る。下記） |
| 退避の完全削除 | `rm -rf data/*.broken-*` |
| ワールドの切り替え | `.env` の `MC_LEVEL` を書き換えて `up -d` |

画面の側にだけあるのは 5 つ。どれも手作業では実質できない。

- **中断の検出** — `save-off` のまま落ちたことに気づいて `save-on` を送り直す
- **バージョンの検証** — アーカイブ内の `level.dat` の `DataVersion` を現在のワールドと比べる
- **取り込み時の包み直し** — `<名前>/level.dat` の形の zip を `data/` 配下へ入れ直す
- **復元時の版の照合** — 復元したワールドに `MC_VERSION` を合わせる（下記）
- **シングルプレイ用の書き出し** — バニラのクライアントが読める形に包み直す（下記）

3 つめは手でコピーしたときに効いてくる。復元は `data/` 配下のエントリしか受け付けない
ので、配布ワールドのような `<名前>/...` 形式の zip を `backups/` へ置いただけでは
「`data/` 配下ではありません」で拒否される。手で入れるなら `data/<名前>/` の形に
固め直しておくか、画面の取り込みを使う。

## 復元時の版の照合

復元すると `.env` の `MC_VERSION` が、展開し終えたワールドの `level.dat` の
`Data.Version.Name` に合わせて書き換わる。ワールド名が変わらない復元でも書き換わる。

合わせないと次のどちらかになる。

- 古いワールドを今の版で開いて、版が勝手に上がる（**元に戻せない**）
- 新しいワールドを古い版で開こうとして、サーバーが起動しない

Paper がその版を配っていない場合は、**サーバーを止める前に**断る。止めてから気づくと
落ちたまま残り、復旧が手作業になる。版の一覧が取れないとき（Pi がオフライン）は通す。
起動できなければ元のワールドへ切り替え直せば版も戻る。

版が変わるときは操作のログに警告が残る。**遊ぶ人はクライアントの版も合わせる必要がある。**

## シングルプレイへの持ち出しと取り込み

### 持ち出す（サーバー → シングルプレイ）

一覧の「シングルプレイ用」から落とす。`saves/` にそのまま展開できる形で渡される。

**保管しているアーカイブ自体がバニラの配置になっている。** 「ダウンロード」で落とした
zip も、`<ワールド>/` を取り出せばそのまま `saves/` に置ける。「シングルプレイ用」は
そこからサーバーの設定（`plugins/` など）を除いたもので、`saves/` の直下が散らない。

構成の違いの詳細は [world-layout.md](world-layout.md) にまとめてある。要点だけ再掲する。

Paper とバニラでは、ワールドの中の `.dat` の置き場所が一部違う。

| ファイル | Paper | バニラ |
| --- | --- | --- |
| `world_gen_settings.dat` | `dimensions/minecraft/overworld/data/minecraft/` | `data/minecraft/` |
| `game_rules.dat` | 同上 | 同上 |
| `scheduled_events.dat` | 同上 | 同上 |
| `wandering_trader.dat` | 同上 | 同上 |
| `weather.dat` | 同上 | 同上 |

Paper はワールドごとに別の設定を持てるようにするためディメンション側へ置く。バニラは
ワールド直下しか見ない。`world_gen_settings.dat` が見つからないとシードも地形生成器も
決められず、クライアントは

```
Unable to read or access the world gen settings file!
Failed to load level data or datapacks, can't proceed with server load
java.lang.IllegalStateException: Overworld settings missing
```

で読み込みを中断する。**ワールド一覧には出る**ので原因が分かりにくい。

管理コンソールが作るアーカイブはこの変換を済ませてある。手でワールドだけ
取り出すならこうする。

```bash
cd ~/Library/Application\ Support/minecraft/saves
mkdir main
# zip のルートがワールド名。皮は 1 枚
tar -xf ~/Downloads/backup-26.3-main-20261011-181445.zip \
    -C main --strip-components=1 main
```

2026-10 より前に取ったアーカイブは `data/<ワールド>/` の形なので、皮を 2 枚
剥がしたうえで 5 ファイルを上げる必要がある。

```bash
tar -xf <古いアーカイブ>.zip -C main --strip-components=2 data/main
cd main
for f in game_rules scheduled_events wandering_trader weather world_gen_settings; do
  mv "dimensions/minecraft/overworld/data/minecraft/$f.dat" "data/minecraft/$f.dat"
done
```

移す元は**オーバーワールドに限る**。オーバーワールド側の `world_gen_settings.dat` が
3 つのディメンションすべての設定（`minecraft:overworld` / `the_nether` / `the_end`）を
持っているため。ネザーやエンドのものを上げると、全体の設定が 1 ディメンション分に縮む。

ネザーとエンドはこれで揃う。移した `world_gen_settings.dat` に 3 ディメンション分の
生成器定義（`minecraft:nether` / `minecraft:end` を含む）とシードが入っており、
`dimensions/minecraft/the_nether/` と `the_end/` の `region/*.mca`・`chunk_tickets.dat`・
`raids.dat`・`world_border.dat`・`ender_dragon_fight.dat` もそのまま持ち出される。
ディメンション側に残る Paper 由来の `.dat` はバニラが開かないので、消さずに置いてある。

`level.dat` の `DataPacks.Enabled` に `paper` が残るため、クライアントのログに
`Missing data pack paper` が出る。致命的ではない（これを通過したあとに
`world_gen_settings` で落ちていた）。

### 取り込む（シングルプレイ → サーバー）

**置き場所の変換は要らない。** Paper が起動時に自前で移行する。

```
[WorldFolderMigration] World storage migration is required during startup.
[WorldFolderMigration] If you do not have a backup: interrupt the server now.
[WorldFolderMigration] Continuing in 30 seconds...
[VanillaWorldMigration] Starting Vanilla import for world 'demo_world_01' (minecraft:overworld)
[VanillaWorldMigration] Completed Vanilla import for world 'demo_world_01' (minecraft:overworld)
```

移行は**その場で書き換える**。実測では無損失（region 62 ファイルが 1 バイトも変化せず、
シードも 3 ディメンションの生成設定も維持）だが、Paper 自身は「バックアップが無いなら
今すぐ落とせ」と警告する。30 秒の猶予があり、管理コンソールから復元した場合は自動で
通過する（起動の完了待ちは 300 秒なので収まる）。

**管理コンソールが保管するアーカイブはバニラの配置なので、復元のたびにこの移行が
走る。** 復元の画面と操作のログに、起動に 30 秒ほど余分にかかることが出る。移行は
復元ごとに 1 回で、以降の再起動では発生しない。

要るのは置き場所ではなく**版**の整合である。26.3 のワールドは 26.2 のサーバーでは
開けない。復元時の版の照合（前節）がこれを受け持つ。
