# Raspberry Pi 5 (4GB) で動かす

Pi 5 上でサーバーを常時稼働させ、Mac から Tailscale 経由で接続する構成の準備手順と、
`compose.yaml` / `.env` の既定値をその値にした理由。

> このドキュメントの手順は実機で未検証。Pi 側のコマンドは Raspberry Pi OS (64bit, Bookworm) 想定。

## 先に効く順に

チューニングよりハードウェア側の2点の方が体感差が大きい。

### 1. SD カードではなく USB SSD / NVMe から起動する

**これが最重要。** Minecraft サーバーはプレイヤーが移動するたびに region ファイルを
読み書きする。SD カードのランダム I/O はこれに追いつかず、チャンク読み込みが数百 ms 止まる。
その間クライアントの移動がサーバーに反映されず、まとめて届いた時点で
`moved too quickly` 判定 → 座標の巻き戻りになる。環境変数でこれを補うことはできない。

- USB 3.0 接続の SSD、または PCIe HAT + NVMe
- ブート順の変更は `sudo raspi-config` → Advanced Options → Boot Order
- NVMe HAT を使う場合は `/boot/firmware/config.txt` に `dtparam=pciex1`
  （Gen 3 で動かすなら `dtparam=pciex1_gen=3`。安定しなければ外す）

### 2. スワップを無効にする

4GB のうち JVM が 2G を占めるため、スワップに落ちると JVM のヒープが
ストレージ経由になり数秒単位で固まる。`mem_limit` を入れてあるので、
メモリが足りない場合はスワップで粘るより OOM kill → 自動再起動の方が被害が小さい。

```bash
sudo dphys-swapfile swapoff
sudo dphys-swapfile uninstall
sudo systemctl disable dphys-swapfile
free -h                      # Swap が 0 になっていること
```

### 3. 冷却と電源

Pi 5 は負荷をかけるとすぐサーマルスロットリングに入る。アクティブクーラーは実質必須。
SSD を USB 給電で繋ぐ場合は公式 27W USB-C PD 電源でないと電圧不足で落ちる。

```bash
vcgencmd measure_temp        # 80℃ を超えるならスロットリング域
vcgencmd get_throttled       # 0x0 以外なら電圧不足か高温
```

## セットアップ

### Docker

```bash
curl -fsSL https://get.docker.com | sh
sudo usermod -aG docker "$USER"
sudo systemctl enable docker      # 再起動後に自動でサーバーも上がる
# 一度ログインし直す
```

イメージは `linux/arm64` のマニフェストを持っているので、Pi 5 でもそのまま `docker compose up -d` で動く。

### Tailscale

```bash
curl -fsSL https://tailscale.com/install.sh | sh
sudo tailscale up
tailscale ip -4                   # 100.x.y.z
tailscale status                  # MagicDNS のホスト名
```

Mac 側も同じ tailnet に参加させ、クライアントには `100.x.y.z` か
MagicDNS のホスト名（例 `minecraft-pi`）を入力する。ポートは既定の `25565` なのでアドレスだけでよい。

compose の `ports` は `0.0.0.0:25565` で待つため、`tailscale0` インターフェース側にも
自動的に届く。追加の設定は要らない。ルーターのポート開放も不要で、
tailnet に参加していない端末からは到達できない。

> tailnet に招待した相手は全員 25565 に到達できる。`.env` の `MC_WHITELIST` は
> それとは別に「誰がゲームに入れるか」を決めるので、両方設定しておく。

### サーバー

```bash
git clone <このリポジトリ>
cd minecraft-server
cp .env.example .env
# RCON_PASSWORD を生成して差し替える
openssl rand -hex 16
# MC_OPS と MC_WHITELIST に自分の Minecraft ID を入れる
docker compose up -d
docker compose logs -f mc         # Done (...) が出れば起動完了
```

## 既定値の根拠

`.env.example` の値は Pi 5 4GB 前提で決めている。

| 設定 | 値 | 理由 |
|---|---|---|
| `MC_MEMORY` | `2G` | `-Xms` と `-Xmx` の両方になる。実 RSS はヒープの 1.3〜1.4 倍（メタスペース、GC 構造体、Netty のダイレクトバッファ）で約 2.7G。OS に 600MB 残すとこれが上限 |
| `MC_MEM_LIMIT` | `3g` | JVM は `-Xmx` 明示時に cgroup 上限を見ないため、超えると GC ではなくコンテナごと OOM kill される。スワップで数秒固まるより落として再起動させる判断 |
| `MC_AIKAR_FLAGS` | `FALSE` | Aikar のフラグは 10GB 級のヒープ向け。`G1NewSizePercent=30` などが 2G では old 世代を圧迫して GC 頻度を上げる。GC 停止はそのまま巻き戻りにつながる |
| `MC_VIEW_DISTANCE` | `7` | クライアントに送るチャンク範囲。CPU と帯域の両方に効く |
| `MC_SIMULATION_DISTANCE` | `5` | エンティティとレッドストーンの処理範囲。`view` より重いので更に小さく |
| `MC_MAX_PLAYERS` | `5` | 2G ヒープで現実的な人数 |
| `MC_NETWORK_COMPRESSION_THRESHOLD` | `256` | 既定のまま。Tailscale の経路は LAN 直結とは限らず、Pi では CPU より帯域がボトルネックになりやすい。**同一 LAN で直結していることを確認できた場合のみ** `-1`（圧縮無効）にする価値がある |

`compose.yaml` 側にも Pi 向けの設定を入れてある。

| 設定 | 理由 |
|---|---|
| `stop_grace_period: 60s` | `docker stop` の既定猶予は 10 秒で、ワールド保存が間に合わず SIGKILL される。停電・再起動時のチャンク破損対策 |
| `logging` の `max-size` / `max-file` | Docker の json ログは既定で無制限。ストレージを食い潰さないよう 10MB × 3 に制限 |
| `restart: unless-stopped` | Pi の再起動後・OOM kill 後に自動復帰させる |

## 動作の確認と計測

```bash
# ティックが間に合っているか（プレイヤーがいる状態で見ること）
docker compose exec mc rcon-cli tps

# 処理落ちの記録。0 でないならサーバー側が遅れている
docker compose logs mc | grep -c "Can't keep up"

# 巻き戻りの回数。これが出ていれば座標が拒否されている
docker compose logs mc | grep -c "moved too quickly"

# メモリと CPU
docker stats --no-stream
docker inspect -f '{{.State.OOMKilled}}' minecraft-server

# Pi 本体
vcgencmd measure_temp
vcgencmd get_throttled
free -h
```

**プレイヤーが0人のときの `TPS 20.0` は健全性の証拠にならない。** アイドル状態の
サーバーは CPU が枯渇していても 20 を維持する。必ず接続した状態で測ること。

## カクつく・巻き戻るときの切り分け

1. `grep -c "moved too quickly"` — 出ていれば座標の拒否が起きている（巻き戻りの直接原因）
2. `grep -c "Can't keep up"` — 出ていればサーバーの CPU 側。距離設定を下げる、SSD にする
3. どちらも 0 なら経路かクライアント側。`tailscale ping <host>` で遅延とジッタを見る
4. `vcgencmd get_throttled` が `0x0` 以外なら冷却か電源の問題

判定自体を緩める対症療法として、`data/spigot.yml` の
`moved-too-quickly-multiplier`（既定 `10.0`）を `20.0` 程度に上げる手もある。
身内サーバーならチート対策を緩めるデメリットは実質ない。
このファイルは `server.properties` と違って `.env` から再生成されないため、
直接編集すれば残る（再起動後に値が残っているか一度確認すること）。
**再生成されない = 失うと戻せない**ので、`data/spigot.yml` と `data/bukkit.yml` は
バックアップ対象に入れてある（[backup-restore.md](backup-restore.md)）。

## 検討したが入れなかったもの

**`ENABLE_AUTOPAUSE`** — プレイヤーがいない間 JVM を一時停止してリソースを空ける機能。
Pi では魅力的だが、実装が knockd によるポートノックで `AUTOPAUSE_KNOCK_INTERFACE`
（既定 `eth0`）に依存する。Tailscale 経由の接続は `tailscale0` から来るため設定が要り、
イメージ同梱のヘルスチェックとも競合する。復帰時に初回接続がタイムアウトすることもある。
必要になったら [itzg のドキュメント](https://docker-minecraft-server.readthedocs.io/) を参照。

**ワールドを名前付きボリュームに移す** — Pi の Linux では bind mount に
macOS のような仮想ファイルシステムのオーバーヘッドがない。移すメリットがないうえ、
[backup-restore.md](backup-restore.md) の手順が壊れる。

## 管理コンソールを常駐させる

バックアップと復元、ワールドの切り替えをブラウザから行う `mcadmind` を
systemd で常駐させる。置き方と使い方は [admin-console.md](admin-console.md)。
ここでは Pi 固有の前提だけを書く。

### コンテナではなくホストで動かす

`docker.sock` をコンテナへマウントすれば同じことはできるが、採らなかった。

- マウントは実質 root 権限を渡すのと変わらない
- ホストとコンテナでパスが食い違い、`data/` の位置が一致しない

代わりに Go の静的バイナリを Pi へ置く。`CGO_ENABLED=0` なので依存が無く、
Pi 側に Go も Node も要らない。

```bash
make build-arm64                     # 開発機で
scp mcadmind-arm64 pi@<host>:~/minecraft-server/
```

### docker グループが要る

`docker compose` を叩くので、動かすユーザーが `docker` グループに入っている必要がある。

```bash
sudo usermod -aG docker $USER        # 反映にはログインし直す
```

ユニットにも `SupplementaryGroups=docker` を入れてあるが、手動で `docker` を
叩けるようにもしておくと切り分けが楽になる。

### メモリの余地

MC が実 RSS で約 2.7GB を使う 4GB 機なので、`mcadmind` に残る余地は多くない。
zip の作成と展開は常に 32KB のバッファで流し、ファイル全体をメモリに載せない
実装にしてある。それでも、バックアップ中に MC が重くなるのは避けられない。
**人がいない時間帯に取るのが望ましい。**

### 到達経路

`ADMIN_ADDR` は `0.0.0.0:8787`。`compose.yaml` が 25565 を `0.0.0.0` に
バインドしているのと同じ理由で、到達境界は Tailscale が持つ。
`127.0.0.1` にすると Mac のブラウザから開けず、原因も分かりにくい。

**tailnet に参加できること ≠ 管理してよいこと**なので、`ADMIN_TOKEN` を別の層として
必ず設定する（`MC_WHITELIST` が別レイヤーの制御として要るのと同じ構図）。
