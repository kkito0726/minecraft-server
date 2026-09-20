#!/usr/bin/env bash
# Release から、この機械に合う mcadmind を落としてくる。
#
#   deploy/download.sh                最新の安定版
#   deploy/download.sh --tag v0.1.0   版を指定する
#
# 置いた後は deploy/install.sh がそのまま拾う。Pi 側に Go も Node も要らない。
set -euo pipefail

readonly DEFAULT_REPO=kkito0726/minecraft-server

here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repo_root="$(cd "$here/.." && pwd)"

# shellcheck source-path=SCRIPTDIR
# shellcheck source=platform.sh
. "$here/platform.sh"

repo="${MCADMIND_REPO:-$DEFAULT_REPO}"
tag=""
dest="$repo_root"

usage() {
  cat <<'USAGE'
使い方: download.sh [オプション]

  --tag <タグ>      取得する版（既定: 最新の安定版）
  --dest <パス>     置き先のディレクトリ（既定: このリポジトリ）
  --repo <owner/名> 取得元のリポジトリ
  -h, --help        この案内
USAGE
}

while [ $# -gt 0 ]; do
  case "$1" in
    --tag)  tag="$2"; shift 2 ;;
    --dest) dest="$2"; shift 2 ;;
    --repo) repo="$2"; shift 2 ;;
    -h | --help) usage; exit 0 ;;
    *) echo "不明なオプション: $1" >&2; usage >&2; exit 2 ;;
  esac
done

die() { echo "エラー: $*" >&2; exit 1; }

command -v curl >/dev/null 2>&1 || die "curl がありません"
command -v sha256sum >/dev/null 2>&1 || die "sha256sum がありません"

[ -d "$dest" ] || die "置き先がありません: $dest"

# 対応していない OS / CPU なら、mc_* が理由を stderr に出して落ちる。
goos="$(mc_goos)" || exit 1
goarch="$(mc_goarch)" || exit 1
asset="mcadmind-$goos-$goarch"

if [ -n "$tag" ]; then
  base="https://github.com/$repo/releases/download/$tag"
else
  # pre-release には latest は動かないので、ここは常に安定版を指す。
  base="https://github.com/$repo/releases/latest/download"
fi

# 検証前のものを置き先に出さない。途中で落ちた時に、壊れたバイナリが
# install.sh に拾われる状態を残さないため。
tmp="$(mktemp -d)"
# shellcheck disable=SC2064 # tmp は今の値で固定したい
trap "rm -rf '$tmp'" EXIT

echo "取得中: $base/$asset"
curl -fsSL -o "$tmp/$asset" "$base/$asset" \
  || die "$asset を取得できません。$repo にその版の Release がありますか"
curl -fsSL -o "$tmp/SHA256SUMS" "$base/SHA256SUMS" \
  || die "SHA256SUMS を取得できません"

# SHA256SUMS には全ての資産が載っている。手元にあるのは 1 つなので、
# 残りは無いものとして飛ばす。
( cd "$tmp" && sha256sum --ignore-missing -c SHA256SUMS ) \
  || die "チェックサムが一致しません。取得し直してください"

chmod 0755 "$tmp/$asset"
mv "$tmp/$asset" "$dest/$asset"

echo "置きました: $dest/$asset"
echo "次: sudo $here/install.sh --project-dir $dest"
