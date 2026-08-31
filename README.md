# Minecraft Server (Docker)

Paper サーバーを Docker だけで動かす構成です。
Java・RCON クライアント・バックアップツールはすべてコンテナ内に閉じており、**ホストには何もインストールしません**。

- サーバー: Paper 26.2（`.env` で変更可）
- イメージ: [`itzg/minecraft-server`](https://github.com/itzg/docker-minecraft-server)
- 公開ポート: `25565` のみ（RCON 25575 はコンテナ内からのみ）
- 外部公開: ngrok トンネルをオプションで同梱（`--profile tunnel`）

## ドキュメント

この README には概要と日常操作だけを置いています。詳細な手順は [docs/](docs/) にあります。

| ドキュメント | 内容 |
|---|---|
| [docs/backup-restore.md](docs/backup-restore.md) | ワールドのバックアップ取得・復元、バージョンとの関係、保管場所 |

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
| `MC_WHITELIST` | 参加を許可する Minecraft ID。外部公開するなら必須 |
| `NGROK_AUTHTOKEN` | 外部公開する場合のみ。LAN 内だけなら空でよい |

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
| インターネット越し | 後述の「外部から接続する（ngrok）」を参照 |

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

## 外部から接続する（ngrok）

ルーターのポート開放をせずに、LAN 外から接続できるようにします。
ngrok もコンテナで動かすので、ホストに ngrok コマンドをインストールする必要はありません。

> ngrok は Minecraft 向けの最適解ではありません。無料プランではアドレスが接続のたびに変わり、
> 中継を挟むぶん遅延も乗ります。固定メンバーで遊ぶなら [Tailscale](https://tailscale.com/)、
> Minecraft 特化なら [playit.gg](https://playit.gg/) の方が扱いやすい選択肢です。

### 手順

1. **先にホワイトリストを設定する。** 公開する以上、`MC_WHITELIST` は必須と考えてください。
   `MC_ONLINE_MODE=TRUE` は正規アカウントであることしか保証せず、誰が入れるかは制限しません。
2. [ngrok のダッシュボード](https://dashboard.ngrok.com/get-started/your-authtoken)でトークンを取得し、`.env` に書く。

   ```
   NGROK_AUTHTOKEN=2abc...
   ```

3. トンネル付きで起動する。

   ```bash
   docker compose --profile tunnel up -d
   ```

4. **割り当てられたアドレスをログから読む。**

   ```bash
   docker compose logs ngrok | grep -i url
   ```

   `tcp://0.tcp.ngrok.io:12345` のような行が出ます。
   クライアントにはスキームを除いた `0.tcp.ngrok.io:12345` を、**ポート番号まで含めて**入力します。
   ポートが 25565 ではないので、ホスト名だけでは繋がりません。

5. 止めるとき。

   ```bash
   docker compose --profile tunnel down
   ```

### 挙動と制約

- `ngrok` サービスには `profiles: ["tunnel"]` を付けてあります。通常の `docker compose up -d` では**起動しません**。外部公開は明示的に `--profile tunnel` を付けたときだけです。
- ngrok は compose ネットワーク経由で `mc:25565` に直結します。ホスト側の `25565` 公開とは独立しているので、LAN 内プレイと併用できます。
- 割り当てアドレスがセッションごとに変わるかどうかは契約プランによります。**毎回ログで確認してください。**固定したい場合は ngrok 側で予約アドレスを用意する必要があります。
- トンネルを開けている間はアドレスを知る誰でも接続を試せます。遊び終わったら閉じるのが安全です。

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
docker compose down
zip -qr "backup-$(date +%Y%m%d-%H%M%S).zip" data/world data/plugins data/config
docker compose up -d
```

稼働させたまま取る方法、**復元手順（上書きすると壊れます）**、バージョンとの関係、保管場所は
[docs/backup-restore.md](docs/backup-restore.md) を参照してください。

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
    ├── whitelist.json ホワイトリスト（.env の MC_WHITELIST とマージ）
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

**ngrok が起動直後に落ちて再起動を繰り返す**
`docker compose logs ngrok` を確認します。`ERR_NGROK_4018`（authentication failed）なら `.env` の `NGROK_AUTHTOKEN` が未設定か誤りです。`restart: unless-stopped` を付けているため、直るまで再試行し続けます。

**ホワイトリストに入れたのにサーバーに入れない**
`docker compose exec mc rcon-cli "whitelist list"` で反映を確認します。ID は大文字小文字まで一致している必要があります。

**設定を変えたのに反映されない**
`docker compose restart` ではなく `docker compose up -d` を使ってください。`.env` の変更はコンテナの再作成で反映されます。
