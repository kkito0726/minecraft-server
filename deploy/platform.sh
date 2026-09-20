#!/usr/bin/env bash
# uname から GOOS / GOARCH への対応表。
#
# download.sh（どの資産を落とすか）と install.sh（どのバイナリを拾うか）の
# 両方が読む。片方だけ直して食い違うと、「落ちてはくるが exec format error
# で動かないバイナリ」という、一番理由の見えない壊れ方をする。
#
# 単体では何もしない。source して使う。

# この機械の GOOS を標準出力に出す。
# 配っていない OS なら、理由を stderr に出して 1 を返す。
mc_goos() {
  local os
  os="$(uname -s)"
  case "$os" in
    Linux) echo linux ;;
    # mcadmind は docker を操る常駐サービスで、置き先は Linux を想定している。
    # 開発機で動かすだけなら make build でその場のものが作れる。
    Darwin)
      echo "macOS 向けのバイナリは配っていません（make build で手元に作れます）。" >&2
      return 1
      ;;
    *)
      echo "対応していない OS です: $os" >&2
      return 1
      ;;
  esac
}

# この機械の GOARCH を標準出力に出す。
mc_goarch() {
  local arch
  arch="$(uname -m)"
  case "$arch" in
    # 同じ 64bit ARM でも、Linux は aarch64、macOS は arm64 と名乗る。
    aarch64 | arm64) echo arm64 ;;
    x86_64 | amd64) echo amd64 ;;
    # 32bit の Raspberry Pi OS。arm64 のバイナリは動かない。
    # 黙って渡して exec format error にするより、ここで理由を言う。
    armv6l | armv7l)
      echo "32bit の OS です（$arch）。64bit の Raspberry Pi OS が要ります。" >&2
      return 1
      ;;
    *)
      echo "対応していない CPU です: $arch" >&2
      return 1
      ;;
  esac
}
