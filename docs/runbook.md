# 運用手順書

**対象**: Minecraft サーバー本体と管理コンソール（`mcadmind`）
**関連**: [admin-console.md](admin-console.md)（何ができるか・置き方） /
[backup-restore.md](backup-restore.md)（手順の中身） /
[raspberry-pi.md](raspberry-pi.md)（Pi 固有の準備）

日々の起動・停止・確認と、困ったときの手順。
**何ができるかと初回の置き方は [admin-console.md](admin-console.md)** にある。
ここはその後の話を扱う。

---

## 0. まず把握しておくこと

| 事実 | なぜ効いてくるか |
|---|---|
| コンテナは **1 つだけ**（`mc`）。`mcadmind` はホストのプロセス | `docker.sock` をコンテナへ渡すのは実質 root 権限を渡すのと変わらない。`docker compose ps` に管理コンソールは出ない |
| 設定の正は `.env` | `data/server.properties` を手で書き換えても起動のたびに戻る |
| `MC_LEVEL` が稼働中のワールドを決める | 実体は `data/<MC_LEVEL>/`。`data/world` とは限らない |
| 操作は**同時に 1 つだけ** | 画面の変更ボタンが押せないのは、たいてい別の操作が走っている |
| RCON の 25575 は**公開しない** | 管理コンソールも `docker compose exec` 経由。ここは変えない |

---

## 1. 日々の起動と停止

### Minecraft サーバー

画面（`/`）から起動・停止・再起動できる。コマンドで行うなら:

```bash
docker compose up -d          # 起動
docker compose logs -f mc     # コンソールを追う
docker compose down           # 停止
```

**`docker compose restart` は使わない。** `.env` の変更はコンテナの
再作成でしか反映されない。再起動したいときも `up -d` を使う。

### 管理コンソール

| 環境 | 起動 | 停止 | ログ |
|---|---|---|---|
| Pi（systemd） | `sudo systemctl start mcadmind` | `sudo systemctl stop mcadmind` | `journalctl -u mcadmind -f` |
| 手元で一時的に | `./mcadmind -project-dir .` | `Ctrl-C` | 標準出力 |
| 開発中（画面を触る） | `make build && ./mcadmind -project-dir .` と `cd frontend && npm run dev` | — | — |

開発中に `npm run dev` を使うと、Vite が `/rpc` を `127.0.0.1:8787` へ
中継する。画面の変更が即座に反映される代わりに、`mcadmind` も別途
動かしておく必要がある。

**バイナリを作り直したら置き直す。** `make build` はフロントエンドを
Go のバイナリへ埋め込む。埋め込みが古いと、画面だけが古いまま動く。

```bash
make build-arm64                                  # Pi 向け
scp mcadmind-arm64 pi@<host>:~/minecraft-server/
sudo deploy/install.sh --project-dir ~/minecraft-server   # 置き直し
```

`install.sh` は動いているサービスを止めてから置き換える
（動いているバイナリを上書きすると `text file busy` になる）。

---

## 2. 状態の確認

```bash
docker compose ps                       # healthy かどうか
docker compose exec mc rcon-cli list    # 接続人数
df -h .                                 # 空き容量
du -sh data/* backups/ | sort -h        # 何が場所を食っているか
systemctl status mcadmind               # 管理コンソール（Pi）
```

画面の `/` にも同じ情報が出る。**「保存が止まっているかもしれない」
赤い帯**が出ていたら、前回の操作が途中で終わっている（3 章）。

---

## 3. 困ったとき

### 変更ボタンが全部押せない

別の操作が走っている。画面上部の帯を見る。帯が出ていないのに押せない
場合は、`data/.admin-console.lock` が残っている。

```bash
cat data/.admin-console.lock     # どの操作の跡か
sudo systemctl restart mcadmind  # 中断として検出し、ロックを外して save-on を送る
```

再起動しても消えないときだけ手で消す。**動いている操作のロックを
奪わないよう、先に何も走っていないことを確かめる。**

### 「保存が止まっているかもしれない」の赤い帯

バックアップの途中で `mcadmind` が落ちた跡。放置すると、**それ以降の
変更が一切ディスクに書かれないまま、症状が何も出ない。**

`mcadmind` は起動時とコンテナが healthy になるたびに `save-on` を
無条件で送る。帯が出た時点で送信済みなので、通常は追加の操作は要らない。
不安なら明示的に送る（冪等）。

```bash
docker compose exec -T mc rcon-cli save-on
```

**RCON には「保存が有効か」を問い合わせる手段が無い。** 送ったことも
サーバーのログには出ない。確認できるのは戻り値の文言だけ。

### 管理コンソールが起動直後に落ちる

```bash
journalctl -u mcadmind -e
```

`ADMIN_TOKEN` が未設定か 32 文字未満なのが最も多い。

```bash
openssl rand -hex 32    # 出力を .env の ADMIN_TOKEN に書く
```

`docker が見つかりません` なら、systemd 配下では PATH が最小限になって
いる。`.env` の `ADMIN_DOCKER_BIN` に `/usr/bin/docker` と絶対パスを書く。

### 画面が白い

フロントエンドが埋め込まれていない。`make build`（または
`make build-arm64`）で作り直したバイナリを置き直す。

### 想定外のエラーが出た

画面には「サーバーのログを確認してください」としか出ない。
内部のパスを画面に出さないためで、**原因は必ずログに残っている。**

```bash
journalctl -u mcadmind -e | grep 想定外
```

### ディスクが埋まりそう

```bash
du -sh backups/ data/*.broken-* data/*.deleted-* 2>/dev/null
```

退避（`.broken-*` / `.deleted-*`）は**自動で消えない**。復元がうまく
いかなかったときに戻す先を残すための意図した動作で、ワールド 1 つぶんが
丸ごと残る。動作を確認したら画面の「退避したワールド」から消す。

バックアップは保持ポリシー（`/backups`）で減らせる。手動で適用するときは
**必ず対象を確認してから**消す。

### ワールドが変わっていないように見える

`MC_LEVEL` が別のワールドを指している可能性がある。`/worlds` で
「稼働中」の印がどこに付いているかを見る。

復元でこれが起きるのは、アーカイブのワールド名と稼働中の名前が違うとき。
復元ダイアログの「どのワールド名として復元しますか」がその選択にあたる。

### `.env` を手で編集したら「外部から変更されました」

書き込む直前に、読んだときから変わっていないかを確かめている。
画面を再読み込みすれば直る。人の `vi .env` を潰さないための仕組み。

---

## 4. 設定を変える

```bash
vi .env
docker compose up -d     # 差分があればコンテナが再作成されて反映される
```

`MC_LEVEL` を変えるときは画面（`/worlds`）から切り替えたほうがよい。
保存 → 停止 → 書き換え → 起動 → 確認を順に行い、`MC_SEED` の消し忘れも
直してくれる。

`ADMIN_*` を変えたときは `mcadmind` の再起動が要る（起動時に読む）。

```bash
sudo systemctl restart mcadmind
```

---

## 5. バージョンを上げる

**ワールドのアップグレードは片道。** 新しいバージョンで開いたワールドは、
古いサーバーでは開けない。

1. 画面（`/backups`）から**バックアップを取る**
2. 取れたことを一覧で確認する（版とワールド名が出る）
3. `.env` の `MC_VERSION` を書き換える
4. `docker compose up -d`
5. クライアント側も同じバージョンにする

上げたあとに古いバックアップを復元すると、**同じアップグレードが
もう一度走る**。復元ダイアログはこれを警告として出し、承諾を求める。

---

## 6. バックアップの運用

自動実行は持っていない（v1 の対象外）。取るのは人の操作。

- **`MC_VERSION` を上げる前**は必ず取る
- 保持ポリシーの既定は 10 世代。設定がどうであれ最新の 1 世代は必ず残る
- **`backups/` はこのディスク上にしかない。** オフサイト転送は持っていないので、
  大事な世代は別の場所へ手で控える

```bash
scp backups/backup-26.2-world-*.zip user@<別のホスト>:~/minecraft-backups/
```

復元の手順と、コンソールが使えないときの手作業は
[backup-restore.md](backup-restore.md) にある。

---

## 7. 結合テスト環境（`test/`）

**本番の `data/` に触らずに**、取得 → 復元 → 切替の一往復を通すための環境。
Minecraft・`mcadmind`・画面の**3 つともコンテナ**で立つ。

```bash
make test-env-up            # 全部立ち上げる（初回はビルドで数分）
make test-env-status
make test-env-logs          # Minecraft のログ
make test-env-console-logs  # mcadmind のログ
make test-env-down          # 止める（ワールドは残る）
make test-env-clean         # ワールドとバックアップごと消す
```

初回は `test/server/.env` を雛形から作り、`RCON_PASSWORD` と `ADMIN_TOKEN` を
その場で生成する。立ち上がると URL とトークンが表示される。

| | 本番 | テスト |
|---|---|---|
| Minecraft | `localhost:25565` | `localhost:25566` |
| 管理コンソール | `:8787`（systemd のホストプロセス） | `:8788`（コンテナ） |
| 画面の開発用（Vite） | — | `:5174`（コンテナ） |
| データ | `data/` `backups/` | `test/server/data/` `test/server/backups/` |
| 設定 | `.env` | `test/server/.env` |
| ワールド生成 | 既定 | `flat`（起動が速い） |

本番と同時に立てられる。ポートもプロジェクト名もぶつからない。

### compose のプロジェクトが 2 つに分かれている理由

```
minecraft-server-test   Minecraft 本体      test/server/compose.yaml
mcadmin-console-test    mcadmind と画面     test/compose.yaml
```

**`mcadmind` を管理対象と同じプロジェクトに入れると、自分自身を落とす。**
復元と COLD 取得は `docker compose down` を実行するが、`down` はサービス単位
ではなく**プロジェクト単位**で効くため、同居していると `mcadmind` のコンテナも
一緒に消える。操作は途中で終わり、ロックが残り、ワールドは退避されたまま戻らない。

本番でも `mcadmind` は管理対象のプロジェクトの外（systemd 配下）にいる。同じ形。

### 画面が 2 つある

| URL | 中身 |
|---|---|
| `:8788` | `mcadmind` が配信する**埋め込みの画面**。本番と同じもの |
| `:5174` | Vite の開発サーバー。**本番にこのコンテナは無い** |

動作を確かめるなら `:8788` を見る。画面を触りながら直すなら `:5174`。

### テスト環境だけの割り切り

`mcadmind` のコンテナには `/var/run/docker.sock` を渡している。**本番では
やらない。** 実質 root 権限を渡すのと変わらないためで、だから本番はホストの
プロセスにしてある（[architecture.md](spec/admin-console/architecture.md)）。
使い捨ての環境だから許容している。

`test/server` はコンテナの中にも**同じ絶対パス**で見せている。compose の
bind mount を解決するのはホストの docker デーモンなので、コンテナ側だけ
違うパスにすると `data/` の位置がずれる。

> **プロジェクト名を固定してはいけない。** `mcadmind` は
> `ADMIN_COMPOSE_PROJECT`、無ければ `compose.yaml` の `name:` から
> プロジェクト名を決める。ここを定数にしていた頃は、`test/` へ向けた
> `mcadmind` が**本番のコンテナを `down` させた**。通常は
> `ADMIN_COMPOSE_PROJECT` は空のままでよい。

### どちらを触っているか分からなくなったら

```bash
docker compose ls            # 動いているプロジェクトの一覧
make test-env-status         # テスト側だけ
docker compose ps            # リポジトリ直下 = 本番
```

`test-env-clean` は消す前に対象のパスを表示する。
**`data/` から始まっていたら本番なので実行しない。**

## 8. やってはいけないこと

- **`compose.yaml` の `ports` に `25575` を足さない。** RCON は平文で、
  総当たり攻撃への保護も無い。晒した時点でサーバーの全権を奪われる
- **`data/server.properties` をバックアップに含めない。** `rcon.password` と
  `management-server-secret` を平文で持つ
- **退避（`.broken-*`）を確認前に消さない。** 復元がうまくいかなかったときの
  戻し先がなくなる
- **復元の途中で `mcadmind` を落とさない。** 「退避済み・展開途中」という
  いちばん悪い状態になる。中断の口を用意していないのはこのため
