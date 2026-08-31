# Minecraft Server (Docker)

Paper サーバーを Docker だけで動かす構成です。
Java・RCON クライアント・バックアップツールはすべてコンテナ内に閉じており、**ホストには何もインストールしません**。

- サーバー: Paper 26.2（`.env` で変更可）
- イメージ: [`itzg/minecraft-server`](https://github.com/itzg/docker-minecraft-server)
- 公開ポート: `25565` のみ（RCON 25575 はコンテナ内からのみ）

## 必要なもの

- Docker Desktop（Compose v2 系）

以上です。Java は不要です。

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
| 同じ PC | `localhost` |
| 同じ LAN 内の別端末 | `<ホストPCのローカルIP>:25565` |
| インターネット越し | ルーターで 25565/TCP をポート開放するか、Tailscale 等の VPN を使う |

クライアントのバージョンは `.env` の `MC_VERSION` と一致させてください。

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

`RCON_PASSWORD` は `.env` にあり Git 管理外です。値を変えたら `docker compose up -d` で反映します。

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

ワールドは `data/world` 以下にまとまっています（ネザー・エンドも `data/world/dimensions/` の中）。
停止中にコピーするのが最も安全です。

```bash
docker compose down
tar czf "backup-$(date +%Y%m%d-%H%M%S).tar.gz" data/world
docker compose up -d
```

稼働させたままなら、先にディスクへ書き出してから固めます。

```bash
docker compose exec mc rcon-cli save-off
docker compose exec mc rcon-cli save-all
tar czf "backup-$(date +%Y%m%d-%H%M%S).tar.gz" data/world
docker compose exec mc rcon-cli save-on
```

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
└── data/            サーバーのデータ一式（Git 管理外）
    ├── world/         ワールド
    ├── plugins/       プラグイン
    ├── logs/          ログ
    ├── ops.json       OP 一覧（.env の MC_OPS から生成）
    └── server.properties  起動のたびに .env から再生成される
```

## トラブルシューティング

**コンテナがすぐ終了する**
`docker compose logs mc` を確認します。`MC_VERSION` / `RCON_PASSWORD` が未設定だと Compose 側でエラーになります。

**`Done (...)` が出る前にメモリ関連で落ちる**
`MC_MEMORY` が Docker Desktop に割り当てた VM メモリを超えています。Docker Desktop の Settings → Resources を確認するか `MC_MEMORY` を下げてください。

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

**設定を変えたのに反映されない**
`docker compose restart` ではなく `docker compose up -d` を使ってください。`.env` の変更はコンテナの再作成で反映されます。

## ワールドを作り直す

```bash
docker compose down
rm -rf data/world
docker compose up -d
```
