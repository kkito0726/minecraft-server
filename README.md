# Minecraft Server (Docker)

Paper サーバーを Docker だけで動かす構成です。
Java・RCON クライアント・バックアップツールはすべてコンテナ内に閉じており、**ホストには何もインストールしません**。

**想定構成: Raspberry Pi 5 (4GB) でサーバーを常時稼働させ、Tailscale 経由で接続する。**
`.env` の既定値はこの前提でチューニングしてあります。

- サーバー: Paper 26.2（`.env` で変更可）
- イメージ: [`itzg/minecraft-server`](https://github.com/itzg/docker-minecraft-server)（linux/arm64 対応）
- 公開ポート: `25565` のみ（RCON 25575 はコンテナ内からのみ）
- 接続経路: Tailscale（tailnet 内からのみ到達可能。ポート開放は不要）
- 管理: ブラウザの管理コンソール（`mcadmind`、8787）。バックアップ・復元・ワールド切り替え

## ドキュメント

この README には概要と日常操作だけを置いています。詳細な手順は [docs/](docs/) にあります。

| ドキュメント | 内容 |
|---|---|
| [docs/raspberry-pi.md](docs/raspberry-pi.md) | Pi 5 のセットアップ、Tailscale、SSD 起動、スワップ、チューニングの根拠 |
| [docs/backup-restore.md](docs/backup-restore.md) | ワールドのバックアップ取得・復元、バージョンとの関係、保管場所 |
| [docs/admin-console.md](docs/admin-console.md) | 管理コンソール（mcadmind）の置き方と使い方 |

## 必要なもの

- Docker Desktop（Compose v2 系）

以上です。Java は不要です。

管理コンソールを**自分でビルドする**場合のみ、Go と Node.js が要ります
（動かす機械には要りません。静的バイナリを置くだけです）。

## セットアップ

`.env` は生成済みです（`RCON_PASSWORD` はランダム値が入っています）。
クローンした直後などで存在しない場合のみ、テンプレートから作成してください。

```bash
[ -f .env ] || cp .env.example .env
```

その場合は RCON パスワードを生成して差し替えます。

```bash
openssl rand -hex 16
```

`.env` を開いて最低限このあたりを設定します。

| 変数 | 説明 |
|---|---|
| `MC_OPS` | **OP にする Minecraft ID**（カンマ区切り）。空だと誰も管理コマンドを使えません |
| `RCON_PASSWORD` | RCON 用パスワード。生成済み。作り直す場合は上記コマンドの出力を貼る |
| `MC_VERSION` | Minecraft のバージョン。**クライアントと必ず一致させる** |
| `MC_MEMORY` | 割り当てメモリ。Docker Desktop の VM メモリ上限を超えないこと |
| `MC_WHITELIST` | 参加を許可する Minecraft ID。設定を強く推奨 |
| `MC_MEMORY` / `MC_MEM_LIMIT` | JVM ヒープとコンテナ上限。Pi 5 4GB なら `2G` / `3g` |
| `MC_LEVEL` | 稼働させるワールドの名前。実体は `data/<この値>/`。既定は `world` |
| `ADMIN_TOKEN` | 管理コンソールのトークン。`openssl rand -hex 32` で生成。**32 文字未満だと起動しない** |

## 起動・停止

```bash
docker compose up -d          # 起動（初回はサーバー jar のダウンロードで数分）
docker compose logs -f mc     # 起動ログ / コンソールを追う
docker compose down           # 停止（コンテナは削除されるがデータは data/ に残る）
docker compose restart mc     # 再起動
```

ログに以下が出れば起動完了です。

```
Done (11.800s)! For help, type "help"
```

状態は `docker compose ps` の `STATUS` が `Up ... (healthy)` かどうかで判断できます
（イメージ同梱のヘルスチェックが実際にサーバーへ ping を投げています）。

## 接続

Minecraft クライアントの「マルチプレイ」→「サーバーを追加」でアドレスを入力します。

| 接続元 | アドレス |
|---|---|
| tailnet 内の端末（推奨） | `<Pi の Tailscale ホスト名 or 100.x.y.z>` |
| 同じ LAN 内の端末 | `<Pi のローカルIP>` |
| サーバーと同じ機体 | `localhost` |

ポートは既定の `25565` なので、アドレスだけ入力すれば繋がります。
クライアントのバージョンは `.env` の `MC_VERSION` と一致させてください。

Tailscale の IP は Pi 側で確認できます。

```bash
tailscale ip -4          # 100.x.y.z
tailscale status         # MagicDNS のホスト名も出る
```

MagicDNS を有効にしていれば `minecraft-pi` のようなホスト名でそのまま接続できます。
tailnet に参加している端末からしか到達できないため、ルーターのポート開放は不要です。
詳細は [docs/raspberry-pi.md](docs/raspberry-pi.md) を参照してください。

## サーバーコマンドの実行

ホストに RCON クライアントを入れる必要はありません。コンテナ内の `rcon-cli` を使います。

```bash
docker compose exec mc rcon-cli list
docker compose exec mc rcon-cli "say Hello"
docker compose exec mc rcon-cli "whitelist add YourMinecraftID"
docker compose exec mc rcon-cli "op YourMinecraftID"
docker compose exec mc rcon-cli save-all
```

対話シェルとして使う場合:

```bash
docker compose exec mc rcon-cli
```

サーバーコンソールへ直接入る場合は `docker attach minecraft-server`（抜けるのは `Ctrl-P` `Ctrl-Q`。`Ctrl-C` はサーバーが止まります）。

### 使い分け

| 方法 | 特徴 |
|---|---|
| `docker compose exec mc rcon-cli <cmd>` | 1コマンド実行して結果を受け取る。スクリプトやバックアップ手順に組み込みやすい |
| `docker attach minecraft-server` | サーバーの標準入出力に直接繋がる。ログが流れ続ける本物のコンソール |

普段の管理操作は `rcon-cli` の方が扱いやすいです。後述のバックアップ手順で `save-off` / `save-all` / `save-on` を使っているのも、コマンド1つで完結して結果を確認できるためです。

## RCON について

RCON（Remote Console）は、サーバーのコンソールコマンドをネットワーク越しに実行するためのプロトコルです。
もともと Valve の Source エンジン由来で、Minecraft サーバーもこれを実装しています。
TCP で接続してパスワード認証し、`list` や `say Hello` といったコマンド文字列を送ると、実行結果のテキストが返ってくる、という単純なものです。

この構成では「ホストに何もインストールしない」を実現する経路として使っています。
`docker compose exec mc rcon-cli` は**コンテナ内の** `rcon-cli` を起動し、それが同じコンテナ内のサーバーへ RCON で接続します。
ホスト側に mcrcon などのクライアントは不要です。

関連する設定は `compose.yaml` の3か所です。

| 設定 | 意味 |
|---|---|
| `ENABLE_RCON: "TRUE"` | サーバーの RCON リスナーを有効化 |
| `RCON_PASSWORD` | 接続時の認証パスワード（`.env` から注入） |
| `ports` に 25575 を**書いていない** | RCON ポートを外部公開しない |

### 25575 を公開しない理由

起動ログには `RCON running on 0.0.0.0:25575` と出ますが、これは**コンテナ内部のネットワーク**での話で、ホストからは見えません（`docker compose ps` の公開ポートは `25565` のみ）。

RCON は**平文プロトコル**です。暗号化がなく、パスワードもコマンドもそのまま流れ、総当たり攻撃への保護もありません。
インターネットに晒すとパスワードが破られた時点でサーバーの全権を奪われます。
`docker compose exec` 経由なら通信がホスト内で完結するため、平文であることが問題になりません。

**`ports` に `25575` を足さないでください。** 外部の GUI 管理ツールから繋ぎたい場合も、公開する代わりに SSH ポートフォワードや VPN 越しに接続してください。
後述の管理コンソールも、この規則を破らないために `docker compose exec` 経由で RCON を叩いています。

`RCON_PASSWORD` は `.env` にあり Git 管理外です。値を変えたら `docker compose up -d` で反映します。

## ホワイトリスト

参加できるプレイヤーを限定します。**外部公開する場合は必ず設定してください。**

```
MC_WHITELIST=Alice,Bob
```

→ `docker compose up -d`

`MC_WHITELIST` に値を入れると `server.properties` の `white-list` と `enforce-whitelist` が自動で `true` になります。
空に戻せば両方 `false` に戻ります。

稼働中に足す／外す場合:

```bash
docker compose exec mc rcon-cli "whitelist add Alice"
docker compose exec mc rcon-cli "whitelist remove Alice"
docker compose exec mc rcon-cli "whitelist list"
```

`data/whitelist.json` の内容は `.env` の値とマージされ、勝手には消えません。
`.env` の内容だけを正としたい場合は `compose.yaml` に `OVERRIDE_WHITELIST: "TRUE"` を足します。

> リストを空のままホワイトリストだけ有効にすると**誰も入れなくなります**。
> ID を1つも入れずに有効化したい場合のみ `MC_ENABLE_WHITELIST=TRUE` を使ってください。

## 設定変更

**設定の正は `.env` です。** `compose.yaml` で `OVERRIDE_SERVER_PROPERTIES=TRUE` を指定しているため、
`data/server.properties` を手で書き換えても起動のたびに `.env` の内容で上書きされます。

```bash
vi .env
docker compose up -d          # 差分があればコンテナが再作成され反映される
```

`.env` にない項目を変えたい場合は `compose.yaml` の `environment:` に追記します
（指定できるキーは [itzg のドキュメント](https://docker-minecraft-server.readthedocs.io/) を参照）。

## バージョンを上げる

1. **必ず先にバックアップを取る**（ワールドのアップグレードは片道で、元のバージョンには戻せません）
2. `.env` の `MC_VERSION` を書き換える
3. `docker compose up -d`
4. クライアント側も同じバージョンに切り替える

`MC_VERSION=LATEST` は意図的に使っていません。コンテナ再作成のタイミングで勝手にバージョンが上がり、
ワールドの片道アップグレードとクライアント不整合を同時に引き起こすためです。

## バックアップ

ワールドは `data/world` 以下にまとまっています。サーバーを止めてから固めるのが最も確実です。

```bash
source .env
docker compose down
zip -qr "backup-${MC_VERSION}-${MC_LEVEL}-$(date +%Y%m%d-%H%M%S).zip" \
  "data/${MC_LEVEL}" data/plugins data/config data/bukkit.yml data/spigot.yml
docker compose up -d
```

ファイル名にバージョンを入れるのは、**ワールドのアップグレードが片道**で、
どのバージョンで取ったものかが後から分からなくなるためです。

稼働させたまま取る方法、**復元手順（上書きすると壊れます）**、バージョンとの関係、保管場所は
[docs/backup-restore.md](docs/backup-restore.md) を参照してください。

これらを画面から行うのが次の管理コンソールです。

## 管理コンソール（mcadmind）

バックアップの取得・復元、ワールドの切り替え、サーバーの起動・停止をブラウザから行います。
手順書どおりに `save-off` を打って `save-on` を忘れる、という事故をなくすために作りました。

```bash
openssl rand -hex 32                    # 出力を .env の ADMIN_TOKEN に書く
make build                              # 開発機で動かすなら
./mcadmind -project-dir .
```

Pi 5 へ置く場合はクロスビルドして systemd に登録します。**Pi 側に Go も Node も要りません。**

```bash
make build-arm64
scp mcadmind-arm64 pi@<Tailscale のホスト名>:~/minecraft-server/
sudo deploy/install.sh --project-dir ~/minecraft-server
```

ブラウザで `http://<Tailscale のホスト名>:8787` を開き、初回だけトークンを入力します。

| 画面 | できること |
|---|---|
| `/` | 状態表示、起動・停止・再起動 |
| `/worlds` | 一覧・新規作成・複製・改名・削除・**切り替え** |
| `/backups` | 取得・一覧・削除・保持ポリシー・**復元** |

**RCON の 25575 は変わらず公開しません。** 管理コンソールも
`docker compose exec -T mc rcon-cli` を経由します。到達の制御は Tailscale、
認可は `ADMIN_TOKEN` という二段構えです（`ADMIN_TOKEN` が 32 文字未満だと起動時に止まります）。

**操作は同時に 1 つだけで、取り消しはできません。** 復元の途中で止めると
「退避済み・展開途中」という最も悪い状態になるため、中断の口を用意していません。
画面を閉じても操作は止まらず、開き直せば進捗とログが戻ります。

詳しくは [docs/admin-console.md](docs/admin-console.md) を参照してください。

## プラグイン（Paper）

`data/plugins/` に `.jar` を置いて再起動するだけです。

```bash
cp SomePlugin.jar data/plugins/
docker compose restart mc
```

## ディレクトリ構成

```
.
├── compose.yaml     サーバー定義
├── .env             設定（Git 管理外）
├── .env.example     設定のテンプレート
├── Makefile         管理コンソールのビルド・テスト・検証
├── proto/           Connect RPC のスキーマ
├── backend/         管理コンソールのバックエンド（Go）
├── frontend/        管理コンソールの画面（React + TypeScript）
├── deploy/          systemd ユニットと配置スクリプト
├── backups/         バックアップの保管先（Git 管理外）
└── data/            サーバーのデータ一式（Git 管理外）
    ├── world/         ワールド（.env の MC_LEVEL が指す名前。複数置ける）
    ├── plugins/       プラグイン
    ├── logs/          ログ
    ├── bukkit.yml     Paper の設定（.env から再生成されない）
    ├── spigot.yml     Paper の設定（.env から再生成されない）
    ├── ops.json       OP 一覧（.env の MC_OPS から生成）
    ├── whitelist.json ホワイトリスト（.env の MC_WHITELIST とマージ）
    └── server.properties  起動のたびに .env から再生成される
```

## トラブルシューティング

**コンテナがすぐ終了する**
`docker compose logs mc` を確認します。`MC_VERSION` / `RCON_PASSWORD` が未設定だと Compose 側でエラーになります。

**`Done (...)` が出る前にメモリ関連で落ちる**
`MC_MEMORY` が Docker Desktop に割り当てた VM メモリを超えています。Docker Desktop の Settings → Resources を確認するか `MC_MEMORY` を下げてください。

**コンテナが突然落ちて再起動する**
`docker inspect -f '{{.State.OOMKilled}}' minecraft-server` が `true` なら、`MC_MEM_LIMIT` を超えています。`MC_MEMORY` を下げるか `MC_MEM_LIMIT` を上げてください。Pi 5 4GB では `2G` / `3g` が上限の目安です。

**カクつく・移動が巻き戻る**
`docker compose logs mc | grep -c "moved too quickly"` で回数を確認します。これはサーバーがクライアントの座標を拒否して引き戻した回数で、巻き戻りの直接の原因です。対処は [docs/raspberry-pi.md](docs/raspberry-pi.md) を参照してください。

**クライアントから繋がらない**
`docker compose ps` が `healthy` かを確認し、外部からの疎通はホストに何も入れずにこれで確認できます。

```bash
docker run --rm --network host itzg/mc-monitor status --host host.docker.internal --port 25565
```

**"Outdated client" / "Outdated server" と出る**
クライアントのバージョンと `.env` の `MC_VERSION` が違います。

**ログに `>....[K` のような文字が混じる**
`compose.yaml` の `tty: true` の副作用です。`docker attach` でコンソールに入るために有効化しています。
不要なら `tty` と `stdin_open` を消すとログがきれいになります。

**ホワイトリストに入れたのにサーバーに入れない**
`docker compose exec mc rcon-cli "whitelist list"` で反映を確認します。ID は大文字小文字まで一致している必要があります。

**設定を変えたのに反映されない**
`docker compose restart` ではなく `docker compose up -d` を使ってください。`.env` の変更はコンテナの再作成で反映されます。

**管理コンソールが開けない / 起動直後に落ちる**
`journalctl -u mcadmind -e` を確認します。`ADMIN_TOKEN` が未設定か 32 文字未満なのが
最も多い原因です。画面が白い場合はフロントエンドが埋め込まれていないので、
`make build-arm64` で作り直したバイナリを置き直してください。
詳しくは [docs/admin-console.md](docs/admin-console.md) の「困ったとき」へ。
