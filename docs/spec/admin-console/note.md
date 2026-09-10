# admin-console コンテキストノート

**作成日**: 2026-09-09
**要件名**: admin-console（Minecraft サーバー管理コンソール / `mcadmind`）

このノートは要件定義・設計・実装の各フェーズが共通で参照する前提情報をまとめたもの。
`CLAUDE.md` が存在しないプロジェクトのため、その代替も兼ねる。

---

## 1. プロジェクトの現状

`git ls-files` で追跡されているのは 7 ファイルのみで、**アプリケーションコードは 1 行も存在しない**。

```
.env.example  .gitignore  README.md  compose.yaml
docs/README.md  docs/backup-restore.md  docs/raspberry-pi.md
```

未追跡だが実在するもの: `.env`（gitignore）、`data/`（gitignore, 256MB）。

`package.json` / `go.mod` / `Makefile` / `.github/` / シェルスクリプトはいずれも存在しない。
バックアップ・復元は `docs/backup-restore.md` に書かれた**手作業のシェル手順**としてのみ存在する。

ドキュメント・コメント・コミットメッセージはすべて日本語。**新規に追加するものも日本語に揃える。**

---

## 2. 技術スタック

| 層 | 採用技術 | 備考 |
|---|---|---|
| フロントエンド | Vite + React + TypeScript + Tailwind CSS | `frontend/` |
| RPC | Connect RPC（`connectrpc.com/connect`）、buf でコード生成 | `proto/mcadmin/v1/` |
| バックエンド | Go（静的バイナリ、`CGO_ENABLED=0`） | `backend/` |
| 配信 | Go の `embed.FS` が Vite のビルド成果物を配信 | 本番に Node プロセスなし |
| 実行 | Raspberry Pi 5 (4GB) 上で **systemd 常駐**（linux/arm64） | `deploy/mcadmind.service` |
| 到達制御 | Tailscale | ルーターのポート開放なし |
| 認可 | `.env` の `ADMIN_TOKEN`（Connect interceptor、定数時間比較） | |
| テスト | Go: table-driven + `-race`。フロント: Vitest。E2E: Playwright | カバレッジ 80%+ |

ホストのツールチェーン（確認済み）: Node v22.19.0 / Go 1.26.4 darwin/arm64 / Docker Compose v5.2.0。

---

## 3. 管理対象サーバーの構成（`compose.yaml`）

- イメージ `itzg/minecraft-server:latest`、`TYPE=PAPER`、`VERSION` は `.env` の `MC_VERSION` で固定（現在 Paper 26.2）
- `container_name: minecraft-server`、`restart: unless-stopped`、`stop_grace_period: 60s`
- `mem_limit: ${MC_MEM_LIMIT:-3g}`、公開ポートは **`25565` のみ**
- ボリュームは単一のバインドマウント `./data:/data`
- `ENABLE_RCON: "TRUE"`、`RCON_PASSWORD` は `.env` から注入
- `OVERRIDE_SERVER_PROPERTIES: "TRUE"`
- `MC_VERSION` と `RCON_PASSWORD` は `:?` で fail-fast
- compose にヘルスチェックの定義はない（イメージ同梱のものが動き、`docker compose ps` に `(healthy)` と出る）

### この構成から導かれる重要な帰結

1. **設定の正は `.env`**。`OVERRIDE_SERVER_PROPERTIES=TRUE` により `data/server.properties` は起動のたびに `.env` から再生成される。手で書き換えても消える。
2. **`.env` の変更は `docker compose up -d`（コンテナ再作成）でしか反映されない。** `docker compose restart` では反映されない（README が明記）。
3. **バインドマウントなのでホストから直接 zip / unzip できる。** `docs/raspberry-pi.md` は「名前付きボリュームへの移行はバックアップ手順を壊すので採用しない」と明示的に判断している。
4. **RCON 25575 は公開されていない。** README が「`ports` に `25575` を足さないでください」とハードルールとして書いている（RCON は平文・総当たり保護なし）。したがって RCON は `docker compose exec -T mc rcon-cli <cmd>` 経由でのみ叩ける。

---

## 4. `data/` の構成

```
data/                       256MB
├── world/                   30MB  ← 唯一のワールド。これだけが代替不能
│   ├── dimensions/minecraft/{overworld,the_nether,the_end}/
│   ├── level.dat, level.dat_old
│   ├── players/  data/  datapacks/  session.lock
├── libraries/  versions/  cache/  paper-26.2-121.jar   227MB  ← 再取得可能
├── plugins/    config/    logs/
├── server.properties        ← 毎回再生成。rcon.password と management-server-secret を平文で保持
├── spigot.yml  bukkit.yml   ← 再生成されない。手編集が docs で指示されている
└── ops.json  whitelist.json  banned-*.json   ← .env から再生成／マージ
```

Paper 26.2 では `world_nether` / `world_the_end` のような別ディレクトリは作られず、
全ディメンションが `data/world/dimensions/minecraft/` 配下に入る。

`level.dat` の実測値（gzip 圧縮 NBT、直接デコードして確認）:

| キー | 値 |
|---|---|
| `Data.Version.Name` | `"26.2"` |
| `Data.Version.Id` | `4903` |
| `Data.DataVersion` | `4903` |
| `Data.LevelName` | `"world"` |
| `Data.LastPlayed` | long（ミリ秒エポック） |
| `Data.Bukkit.Version` | `"Paper/26.2-121-a2a42c5 (MC: 26.2)/26.2.build.121-stable"` |

**バージョン判定は表示文字列 `Version.Name` ではなく int の `DataVersion` をキーにする。**

---

## 5. 複数ワールドの仕組み

itzg のドキュメント（`https://docker-minecraft-server.readthedocs.io/en/latest/variables/`）より引用:

> **LEVEL** — "Maps to the level-name server property. You can either switch between world saves or run multiple containers with different saves by using the LEVEL option"（既定値 `world`）

`OVERRIDE_SERVER_PROPERTIES=TRUE` なので `.env` の値が正になる。よってワールド切替は
「`.env` の `MC_LEVEL` を書き換えて `docker compose up -d`」で成立し、**ワールドのバイト列は 1 バイトも動かない**。

⚠️ **ただし itzg のドキュメントは「ワールドが `/data/<LEVEL>/` に作られる」とは明記していない。**
実装フェーズ 2 で実機確認する。失敗した場合のフォールバックは
「ライブラリを `data/worlds/<名前>/` に置き、アクティブなものを `data/world` へ `mv` する」方式
（リポジトリ既存の `mv` 退避規約と同形）で、変更範囲は `internal/world/switch.go` と
`worldfs.Repository` の実装のみ。proto・操作モデル・フロントエンドは無変更で通る。

---

## 6. 検証済みのバックアップ・復元手順（`docs/backup-restore.md`）

ユーザーが実際に一往復させて確認済み（107 ファイル / 5.1MB の zip を取得し、
バックアップ後に追加したファイルが復元で消えることを確認、コンテナが `healthy` になることを確認）。

**HOT 取得**:
```bash
docker compose exec -T mc rcon-cli save-off
docker compose exec -T mc rcon-cli save-all
zip -qr "backup-${MC_VERSION}-$(date +%Y%m%d-%H%M%S).zip" data/world data/plugins data/config
docker compose exec -T mc rcon-cli save-on
```

**復元**（「上書き」ではなく「退避してから展開」）:
```bash
docker compose down
mv data/world "data/world.broken-$(date +%Y%m%d-%H%M%S)"   # rm ではなく mv。失敗しても戻せる
unzip -qo backup-26.2-20260901-003000.zip
docker compose up -d
```

### 実機で測ったこと（2026-09-10、Paper 26.2 稼働中）

| 確認したこと | 結果 |
|---|---|
| 保存が有効な状態で `save-on` を再送 | `Saving is already turned on` を返して終了コード 0。**サーバー側のログには何も出ない**。冪等であることを実測で確認 |
| HOT 取得で実際に届いたコマンド | `Automatic saving is now disabled` → `Saved the game` → `Automatic saving is now enabled` の 3 行がサーバーログに並ぶ |
| 27MB のワールドの zip | 11.4MB（`flate.BestSpeed`）。取得は 1 秒未満 |
| 取得したアーカイブのルート集合 | `data/world` `data/plugins` `data/config` `data/bukkit.yml` `data/spigot.yml` の 5 つと完全一致。`server.properties` は含まれない |
| 展開して稼働中の `data/world` と比較 | 差分は `session.lock` のみ（71 対 72 ファイル） |
| 復元後、バックアップ後に増やしたファイル | **消える**。退避してから展開しているため。上書き展開なら残ってしまう |
| その増やしたファイルの退避先 | `data/<名前>.broken-<日時>/` に残る。`mv` であって `rm` ではないので戻せる |
| `RESTORE_TARGET_CURRENT_LEVEL` の書き換え | アーカイブの `data/restoretest/...` が `data/restoretest2/...` として展開され、`MC_LEVEL` は変わらない。他のワールドのディレクトリは触られない |
| ワールドを複製して `MC_LEVEL` を切り替えたあとの `level.dat` | `LevelName` が新しい名前に書き換わる。Paper が起動時に `server.properties` の `level-name` を反映するため |
| 取得の開始直後に `kill -9` | ロックファイルが**サイズ 0 で残る**（`O_CREATE｜O_EXCL` の直後、メタデータ書き込み前）。再起動時に中断として検出され、ロックは削除され、`save-on` が再送された |

`save-on` がログに出ないという性質は重要である。「起動時に無条件で再送する」という
回復設計は、**送ったことがサーバーログから確認できない**ことを前提にしている。
確認できるのは `rcon-cli` の終了コードと戻り値の文言だけ。

### 手順から抽出した不変条件

1. **`save-on` を忘れると以降の変更がディスクに書かれない。しかも症状が何も出ない。** docs 自身が `trap` の使用を指示している。
2. **既存の `data/world` に上書き展開してはいけない。** バックアップに含まれない新しい region ファイルが残り、古い地形と新しい地形が同居した壊れたワールドになる。
3. **ワールドのアップグレードは片道。** 新しいバージョンで開いたワールドは古いサーバーで開けない。だからファイル名にバージョンを入れる。
4. `session.lock` は起動時に作り直されるので zip に含まれていても無害。

---

## 7. 実機の制約（`docs/raspberry-pi.md`）

> このドキュメントは冒頭に「実機で未検証」と明記されている。現在は開発者の Mac 上で動作しており、Pi 5 は移行先。

- **ハードウェアがチューニングに勝る**（優先順）: ① SD カードではなく USB SSD / NVMe から起動（SD のランダム I/O ではチャンク読み込みが数百 ms 停止し、`moved too quickly` によるラバーバンドを引き起こす。「どの環境変数でも埋め合わせられない」）② スワップ無効化（JVM ヒープのページアウトは数秒のフリーズ。OOM kill + 自動再起動の方がまし）③ 能動冷却 + 公式 27W PD 電源
- `MC_MEMORY=2G` / `MC_MEM_LIMIT=3g`: 実 RSS はヒープの 1.3〜1.4 倍（≒2.7GB）。OS に約 600MB 残すとこれが上限
- `MC_AIKAR_FLAGS=FALSE`: Aikar のフラグは約 10GB ヒープ向け。2G では GC 頻度が上がり、GC 停止がそのままラバーバンドになる
- `ENABLE_AUTOPAUSE` は**検討のうえ不採用**: knockd が `eth0` 前提で Tailscale の `tailscale0` と噛み合わず、ヘルスチェックとも競合する
- Tailscale: `0.0.0.0:25565` にバインドしているので `tailscale0` からそのまま到達できる。**tailnet 参加 ≠ プレイ許可**なので `MC_WHITELIST` が別レイヤーの制御として要る

**管理コンソールへの含意**: MC が既に約 2.7GB を使う 4GB 機なので、zip は必ずストリーミング処理する。
`io.ReadAll` は禁止。圧縮は `flate.BestSpeed`（`.mca` は内部で既に zlib 圧縮済みなので、
最大圧縮は Pi の CPU を焼くだけで縮まない）。

---

## 8. 既存構成で見つかった不具合（本要件で併せて修正する）

| # | 内容 | 影響 |
|---|---|---|
| 1 | `compose.yaml:78-79` と `README.md:181` が `MC_ENABLE_WHITELIST` を参照しているが、**`.env` にも `.env.example` にも存在しない** | 未定義変数。README の手順どおりに操作できない |
| 2 | `data/spigot.yml` は `docs/raspberry-pi.md` が手編集を指示しているのに**バックアップ集合に入っておらず、`.env` からも再生成されない** | 復元すると手編集した `moved-too-quickly-multiplier` が失われる |
| 3 | `README.md:212` のバックアップレシピはファイル名に `MC_VERSION` を含まないが、`docs/backup-restore.md` は「片道アップグレードのため版を入れること」と主張 | 2 つの docs が矛盾している |

---

## 9. 開発ルール（ユーザーのグローバル設定より）

- **イミュータブル必須**: 既存オブジェクトを変更せず、常に新しいオブジェクトを構築する
- **小さいファイルを多く**: 200〜400 行が標準、800 行が上限。関数は 50 行未満、ネストは 4 段まで
- **エラー処理を網羅**: `console.log` を残さない、ハードコードした値を残さない
- **入力検証**: フロントエンドは zod
- **TDD**: テストを先に書く（RED → GREEN → REFACTOR）。カバレッジ 80%+（ユニット / 統合 / E2E すべて）
- **コミット**: Conventional Commits（`type: 説明`）。type は英語、本文は日本語

---

## 10. 関連文書

| 文書 | 内容 |
|---|---|
| [requirements.md](requirements.md) | EARS 記法の機能要件・非機能要件・エッジケース |
| [interview-record.md](interview-record.md) | ヒアリング記録と信頼性レベルの変化 |
| [user-stories.md](user-stories.md) | ユーザーストーリー |
| [acceptance-criteria.md](acceptance-criteria.md) | 受け入れ基準とテストケース |
| `README.md` | サーバーの日常操作 |
| `docs/backup-restore.md` | 検証済みのバックアップ・復元手順 |
| `docs/raspberry-pi.md` | Pi 5 のセットアップとチューニングの根拠 |
| `compose.yaml` | サーバー定義 |
