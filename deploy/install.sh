#!/usr/bin/env bash
# mcadmind を systemd のサービスとして配置する。
#
#   sudo deploy/install.sh --project-dir /home/pi/minecraft-server
#
# 何をするかを先に確かめたいときは --dry-run を付ける。
# 何も書き換えず、置こうとしているユニットの中身を表示する。
set -euo pipefail

readonly UNIT_NAME=mcadmind.service
readonly UNIT_DIR=/etc/systemd/system
readonly BIN_DEST=/usr/local/bin/mcadmind
readonly MIN_TOKEN_LENGTH=32

here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repo_root="$(cd "$here/.." && pwd)"

project_dir="$repo_root"
binary=""
service_user="${SUDO_USER:-$(id -un)}"
dry_run=0

usage() {
  cat <<'USAGE'
使い方: install.sh [オプション]

  --project-dir <パス>  compose.yaml と .env があるディレクトリ
                        （既定: このリポジトリ）
  --binary <パス>       配置する mcadmind（既定: mcadmind-arm64 か mcadmind）
  --user <名前>         サービスを動かすユーザー（既定: sudo を実行した人）
  --dry-run             何も書き換えず、置こうとする内容だけ表示する
  -h, --help            この案内
USAGE
}

while [ $# -gt 0 ]; do
  case "$1" in
    --project-dir) project_dir="$2"; shift 2 ;;
    --binary)      binary="$2"; shift 2 ;;
    --user)        service_user="$2"; shift 2 ;;
    --dry-run)     dry_run=1; shift ;;
    -h|--help)     usage; exit 0 ;;
    *) echo "不明なオプション: $1" >&2; usage >&2; exit 2 ;;
  esac
done

die() { echo "エラー: $*" >&2; exit 1; }

# require は「これが無いと動かない」条件。
#
# --dry-run は何も書き換えないので、止めずに知らせるだけにする。
# 配置先とは別の機械でユニットの中身を確かめる用途があるため。
require() {
  if [ "$dry_run" = 1 ]; then
    echo "注意: $*" >&2
    return 0
  fi
  die "$*"
}

# --- 置く前に確かめる ---------------------------------------------------

project_dir="$(cd "$project_dir" 2>/dev/null && pwd)" \
  || die "プロジェクトディレクトリが見つかりません"

for required in compose.yaml .env; do
  [ -f "$project_dir/$required" ] \
    || die "$required が $project_dir にありません"
done

if [ -z "$binary" ]; then
  # Pi へ配るのは arm64。同じ機械でビルドしたなら mcadmind。
  for candidate in "$repo_root/mcadmind-arm64" "$repo_root/mcadmind"; do
    [ -f "$candidate" ] && { binary="$candidate"; break; }
  done
fi
[ -n "$binary" ] || require "配置するバイナリがありません（make build-arm64 を実行してください）"
[ -z "$binary" ] || [ -f "$binary" ] || require "バイナリが見つかりません: $binary"

id "$service_user" >/dev/null 2>&1 || die "ユーザーがいません: $service_user"

# トークンが無いと mcadmind は起動直後に落ちる。
# systemd の下だと理由が journal にしか出ず気づきにくいので、ここで止める。
token="$(sed -n 's/^ADMIN_TOKEN=//p' "$project_dir/.env" | head -1 | tr -d '"' | tr -d "'")"
if [ "${#token}" -lt "$MIN_TOKEN_LENGTH" ]; then
  require ".env の ADMIN_TOKEN が短すぎます（${MIN_TOKEN_LENGTH} 文字以上）。
     openssl rand -hex 32 で生成して .env に書いてください"
fi

# docker グループに入っていないと docker.sock を触れない。
# SupplementaryGroups=docker はグループの存在が前提。
#
# --dry-run は配置先とは別の機械（開発機）で内容を確かめる用途があるので、
# ここは止めずに知らせるだけにする。
check_docker_group() {
  if ! getent group docker >/dev/null 2>&1; then
    if [ "$dry_run" = 1 ]; then
      echo "注意: この機械には docker グループがありません（配置先で必要です）。" >&2
      return 0
    fi
    die "docker グループがありません。docker は入っていますか"
  fi
  if ! id -nG "$service_user" | tr ' ' '\n' | grep -qx docker; then
    echo "注意: $service_user は docker グループに入っていません。" >&2
    echo "      SupplementaryGroups=docker で補いますが、手動の docker 操作には" >&2
    echo "      sudo usermod -aG docker $service_user が要ります。" >&2
  fi
}
check_docker_group

service_group="$(id -gn "$service_user")"

render_unit() {
  sed \
    -e "s#@USER@#${service_user}#g" \
    -e "s#@GROUP@#${service_group}#g" \
    -e "s#@PROJECT_DIR@#${project_dir}#g" \
    -e "s#@BIN@#${BIN_DEST}#g" \
    "$here/$UNIT_NAME"
}

if [ "$dry_run" = 1 ]; then
  # 標準出力にはユニットだけを出す。そのまま systemd-analyze verify に
  # 渡せるようにするため、説明はすべて標準エラーへ。
  echo "$UNIT_DIR/$UNIT_NAME として次の内容を置きます" >&2
  echo "バイナリ: ${binary:-（未ビルド）} -> $BIN_DEST" >&2
  render_unit
  exit 0
fi

# --- ここから書き換える -------------------------------------------------

[ "$(id -u)" = 0 ] || die "root で実行してください（sudo deploy/install.sh）"

# 動いているバイナリを上書きすると "text file busy" になる。
systemctl stop "$UNIT_NAME" 2>/dev/null || true

install -m 0755 "$binary" "$BIN_DEST"
render_unit > "$UNIT_DIR/$UNIT_NAME"
chmod 0644 "$UNIT_DIR/$UNIT_NAME"

systemctl daemon-reload
systemctl enable --now "$UNIT_NAME"

echo
systemctl --no-pager --lines=0 status "$UNIT_NAME" || true
echo
addr="$(sed -n 's/^ADMIN_ADDR=//p' "$project_dir/.env" | head -1 | tr -d '"')"
port="${addr##*:}"
echo "配置しました。ログ: journalctl -u $UNIT_NAME -f"
echo "画面: http://<Tailscale の IP>:${port:-8787}"
