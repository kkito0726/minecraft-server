# バックアップと復元

ワールドデータの退避と巻き戻しの手順。個人利用（数人以下、Tailscale か LAN で接続）を想定している。

## 何をバックアップすればいいか

`data/` は 246MB あるが、そのほとんどはサーバー jar とライブラリで、再ダウンロードされるため保存する意味がない。

| パス | サイズ目安 | バックアップ |
|---|---|---|
| `data/world/` | 18MB | **必須。** ワールド本体 |
| `data/plugins/` | 1MB 未満 | プラグインを使うなら必須。jar と設定が入る |
| `data/config/`、`bukkit.yml`、`spigot.yml` | 数十 KB | Paper のチューニング設定。変更していれば |
| `data/ops.json`、`data/whitelist.json` | 数 KB | **不要。** `.env` の `MC_OPS` / `MC_WHITELIST` から再生成される |
| `data/libraries/`、`data/versions/`、`data/cache/`、`data/*.jar` | 220MB+ | **不要。** 起動時に再取得される |
| `data/logs/` | — | **不要** |
| `data/server.properties` | — | **不要。** 毎回 `.env` から再生成される |

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
zip -qr "backup-${MC_VERSION}-$(date +%Y%m%d-%H%M%S).zip" data/world data/plugins data/config
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
zip -qr "backup-${MC_VERSION}-$(date +%Y%m%d-%H%M%S).zip" data/world data/plugins data/config
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
zip -qr "backup-${MC_VERSION}-$(date +%Y%m%d-%H%M%S).zip" data/world data/plugins data/config
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

`-o` を付けないと、既存の `data/plugins` / `data/config` について上書き確認のプロンプトが出て止まる。

## バージョンとの関係

**ワールドのアップグレードは片道。** 一度新しいバージョンで起動したワールドは、
古いバージョンのサーバーでは開けない。

- `MC_VERSION` を上げた後に古いバックアップを復元すると、**再び同じアップグレードが走る**
- 新しいバージョンで保存されたワールドを古い `MC_VERSION` に戻すことはできない

このため、バックアップのファイル名にバージョンを入れておく（上記手順では `backup-26.2-...` の形）。
`MC_VERSION` を上げる前には、必ずその時点のバックアップを取ること。

## 保管場所

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

`level-seed` を指定したい場合は `compose.yaml` の `environment` に `SEED` を追加する。

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
