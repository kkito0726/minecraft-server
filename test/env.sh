#!/usr/bin/env bash
# 結合テスト環境の上げ下げ。
#
#   test/env.sh up      Minecraft コンテナと mcadmind を立ち上げる
#   test/env.sh down    両方を止める（ワールドとバックアップは残る）
#   test/env.sh clean   止めたうえでワールドとバックアップごと消す
#   test/env.sh status  いま何が動いているか
#   test/env.sh logs    Minecraft のログを追う
#
# 本番（リポジトリ直下）とは別プロジェクト・別ポート・別データで動く。
# data/world には一切触らない。
set -euo pipefail

here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repo_root="$(cd "$here/.." && pwd)"

readonly ENV_FILE="$here/.env"
readonly PID_FILE="$here/.mcadmind.pid"
readonly LOG_FILE="$here/mcadmind.log"
readonly BIN="$repo_root/mcadmind"

# compose はどこから呼ばれても同じプロジェクトを指すよう、すべて明示する。
compose() {
  docker compose --project-directory "$here" -p minecraft-server-test \
    -f "$here/compose.yaml" "$@"
}

die() { echo "エラー: $*" >&2; exit 1; }

# --- 準備 ---------------------------------------------------------------

# ensure_env は test/.env を用意する。秘密はその場で生成する。
#
# 雛形をそのままコピーすると ADMIN_TOKEN が空で、mcadmind が起動直後に
# 落ちる。理由は journal にしか出ないので、ここで埋めてしまう。
ensure_env() {
  [ -f "$ENV_FILE" ] && return 0

  echo "test/.env がないので雛形から作ります"
  sed \
    -e "s|^RCON_PASSWORD=.*|RCON_PASSWORD=$(openssl rand -hex 16)|" \
    -e "s|^ADMIN_TOKEN=.*|ADMIN_TOKEN=$(openssl rand -hex 32)|" \
    "$here/.env.example" > "$ENV_FILE"
  chmod 0600 "$ENV_FILE"
}

# ensure_binary はフロントエンドを埋め込んだ mcadmind を用意する。
#
# 埋め込みが無いと画面が白いまま起動する。作り直すのは、テスト環境に
# 古い画面が残っていて原因を取り違えるのを避けるため。
ensure_binary() {
  if [ ! -x "$BIN" ]; then
    echo "mcadmind をビルドします（初回は数十秒かかります）"
    make -C "$repo_root" build
  fi
}

value_of() { sed -n "s/^$1=//p" "$ENV_FILE" | head -1 | tr -d '"'; }

# --- 起動と停止 ---------------------------------------------------------

wait_healthy() {
  echo -n "Minecraft の起動を待っています"
  for _ in $(seq 1 120); do
    if compose ps --format json 2>/dev/null | grep -q '"Health":"healthy"'; then
      echo " → healthy"
      return 0
    fi
    echo -n "."
    sleep 5
  done
  echo
  die "起動を確認できませんでした。test/env.sh logs で確認してください"
}

start_console() {
  if [ -f "$PID_FILE" ] && kill -0 "$(cat "$PID_FILE")" 2>/dev/null; then
    echo "mcadmind は既に動いています (pid $(cat "$PID_FILE"))"
    return 0
  fi

  ensure_binary
  # -project-dir に test を渡す。これで mcadmind が読むのも書くのも
  # test/ の中だけになる。
  nohup "$BIN" -project-dir "$here" > "$LOG_FILE" 2>&1 &
  echo $! > "$PID_FILE"

  local addr
  addr="$(value_of ADMIN_ADDR)"
  addr="${addr:-127.0.0.1:8788}"
  for _ in $(seq 1 50); do
    if curl -fsS -o /dev/null "http://${addr/0.0.0.0/127.0.0.1}/" 2>/dev/null; then
      echo "mcadmind: http://${addr/0.0.0.0/127.0.0.1}/"
      echo "トークン: $(value_of ADMIN_TOKEN)"
      return 0
    fi
    sleep 0.2
  done
  die "mcadmind が起動しませんでした。$LOG_FILE を確認してください"
}

stop_console() {
  [ -f "$PID_FILE" ] || return 0
  local pid
  pid="$(cat "$PID_FILE")"
  if kill -0 "$pid" 2>/dev/null; then
    kill "$pid" 2>/dev/null || true
  fi
  rm -f "$PID_FILE"
}

# --- 入口 ---------------------------------------------------------------

case "${1:-up}" in
  up)
    ensure_env
    compose up -d
    wait_healthy
    start_console
    echo
    echo "Minecraft: localhost:$(value_of MC_PORT)"
    echo "止めるとき: test/env.sh down"
    ;;

  down)
    stop_console
    compose down
    echo "止めました。ワールドとバックアップは test/ に残っています"
    ;;

  clean)
    stop_console
    compose down -v
    # 消す前に何を消すか出す。取り違えて本番を消すことがないよう
    # パスをそのまま見せる。
    echo "次を削除します:"
    echo "  $here/data"
    echo "  $here/backups"
    rm -rf "$here/data" "$here/backups" "$LOG_FILE"
    echo "消しました"
    ;;

  status)
    compose ps
    if [ -f "$PID_FILE" ] && kill -0 "$(cat "$PID_FILE")" 2>/dev/null; then
      echo "mcadmind: 稼働中 (pid $(cat "$PID_FILE"))"
    else
      echo "mcadmind: 停止"
    fi
    ;;

  logs)
    compose logs -f mc
    ;;

  *)
    die "使い方: test/env.sh {up|down|clean|status|logs}"
    ;;
esac
