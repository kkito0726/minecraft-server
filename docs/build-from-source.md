# 手元でビルドして Pi へ置く

既定の置き方は、タグを打つと GitHub Actions が作る Release から
`deploy/download.sh` で落とす方法で、手順は [admin-console.md](admin-console.md) にある。
このドキュメントは、それを使わずに**手元でクロスビルドして `scp` で送る**従来の手順を残したもの。

## どちらを使うか

| | Release から落とす（既定） | 手元でビルドして送る（この手順） |
|---|---|---|
| 取得元 | タグを打つと GA が作る Release | 開発機の `make build-arm64` |
| 開発機に要るもの | 無し（Pi だけで完結する） | Go と Node |
| Pi から GitHub への到達 | 要る | 要らない |
| 入っている版 | タグで言い切れる | 手元の作業ツリー次第 |
| 向く場面 | 通常の配置と更新 | タグを打つ前の変更を実機で試す / 手元で当てたパッチを載せる / Pi が GitHub へ出られない |

主な用途は「タグを打つほどでもない変更を、実機で先に確かめたい」。
確かめ終わったらタグを打って Release 経由に戻すと、Pi に入っているものを
版で言い切れる状態に戻る。

## 手順

開発機でクロスビルドする。`CGO_ENABLED=0` の静的バイナリなので、Pi 側に
Go も Node も要らない。

```bash
make build-arm64                                    # mcadmind-arm64 ができる
scp mcadmind-arm64 pi@<Tailscale のホスト名>:~/minecraft-server/
```

Pi で置き換える。**`--binary` を必ず付ける**（理由は次節）。

```bash
sudo deploy/install.sh --project-dir ~/minecraft-server \
  --binary ~/minecraft-server/mcadmind-arm64
```

`install.sh` は動いているサービスを止めてから置き換える
（動いているバイナリを上書きすると `text file busy` になる）。

## `--binary` を省くと古いものが入る

`install.sh` は `--binary` が無いとき、次の順でバイナリを探す。

1. `mcadmind-<os>-<arch>` — `download.sh` が置く名前
2. `mcadmind-arm64` — `make build-arm64` が置く名前
3. `mcadmind` — 同じ機械で `make build` したもの

一度でも `download.sh` を使った Pi には 1 が残っている。そこへ 2 を `scp` して
`--binary` を省くと、**送ったばかりのものではなく、前に落とした古い方が入る**。
起動には成功するので失敗として現れない。フロントエンドはバイナリに埋め込まれて
いるため、症状は「直したはずの画面が変わらない」になる。

`--binary` を毎回書きたくない場合は、先に落とした方を消しておく。

```bash
rm -f ~/minecraft-server/mcadmind-linux-arm64
```

## 入っている版を確かめる

```bash
mcadmind -version
```

Release から落としたものはタグ（`v1.0.0` など）をそのまま名乗る。
手元でビルドしたものは `git describe` の結果（`v1.0.0-3-gabc1234` など）になり、
`-dirty` が付いていればコミットしていない変更を含む。
実機で試したまま戻し忘れていないかは、ここで判断できる。

## Release 経由へ戻す

```bash
cd ~/minecraft-server
./deploy/download.sh
sudo deploy/install.sh --project-dir ~/minecraft-server
```
