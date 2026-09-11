#!/usr/bin/env bash
# 結合テスト環境の上げ下げ。
#
#   test/env.sh up      Minecraft・mcadmind・フロントエンドを立ち上げる
#   test/env.sh down    止める（ワールドとバックアップは残る）
#   test/env.sh clean   止めたうえでワールドとバックアップごと消す
#   test/env.sh status  いま何が動いているか
#   test/env.sh logs    Minecraft のログを追う
#
# compose のプロジェクトは 2 つある。
#
#   minecraft-server-test   Minecraft 本体（test/server/compose.yaml）
#   mcadmin-console-test    mcadmind とフロントエンド（test/compose.yaml）
#
# 分けてあるのは、mcadmind が復元や COLD 取得で実行する docker compose down が
# プロジェクト単位で効くため。同じプロジェクトに居ると自分自身を巻き込んで落ちる。
#
# どちらも本番（リポジトリ直下）とは別プロジェクト・別ポート・別データで動く。
set -euo pipefail

here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# mcadmind に渡すプロジェクトディレクトリ。
# コンテナの中にも**同じ絶対パス**で見せる必要がある（compose の bind mount は
# ホストの docker デーモンが解決するため）。
export TEST_SERVER_DIR="$here/server"

readonly ENV_FILE="$TEST_SERVER_DIR/.env"

die() { echo "エラー: $*" >&2; exit 1; }

# Minecraft 本体。mcadmind が操作するのもこのプロジェクト。
server_compose() {
  docker compose --project-directory "$TEST_SERVER_DIR" -p minecraft-server-test \
    -f "$TEST_SERVER_DIR/compose.yaml" "$@"
}

# 管理コンソール一式。
console_compose() {
  docker compose --project-directory "$here" -p mcadmin-console-test \
    --env-file "$ENV_FILE" -f "$here/compose.yaml" "$@"
}

value_of() { sed -n "s/^$1=//p" "$ENV_FILE" | head -1 | tr -d '"'; }

# --- 準備 ---------------------------------------------------------------

# ensure_env は test/server/.env を用意する。秘密はその場で生成する。
#
# 雛形をそのままコピーすると ADMIN_TOKEN が空で、mcadmind が起動直後に
# 落ちる。理由はコンテナのログにしか出ないので、ここで埋めてしまう。
ensure_env() {
  [ -f "$ENV_FILE" ] && return 0

  echo "test/server/.env がないので雛形から作ります"
  sed \
    -e "s|^RCON_PASSWORD=.*|RCON_PASSWORD=$(openssl rand -hex 16)|" \
    -e "s|^ADMIN_TOKEN=.*|ADMIN_TOKEN=$(openssl rand -hex 32)|" \
    "$TEST_SERVER_DIR/.env.example" > "$ENV_FILE"
  chmod 0600 "$ENV_FILE"
}

# --- 起動と停止 ---------------------------------------------------------

wait_healthy() {
  echo -n "Minecraft の起動を待っています"
  for _ in $(seq 1 120); do
    if server_compose ps --format json 2>/dev/null | grep -q '"Health":"healthy"'; then
      echo " → healthy"
      return 0
    fi
    echo -n "."
    sleep 5
  done
  echo
  die "起動を確認できませんでした。test/env.sh logs で確認してください"
}

wait_console() {
  local port="$1"
  echo -n "管理コンソールの起動を待っています"
  for _ in $(seq 1 60); do
    if curl -fsS -o /dev/null "http://127.0.0.1:$port/" 2>/dev/null; then
      echo " → 応答あり"
      return 0
    fi
    echo -n "."
    sleep 1
  done
  echo
  die "mcadmind が応答しません。console_compose logs mcadmind で確認してください"
}

# --- 入口 ---------------------------------------------------------------

case "${1:-up}" in
  up)
    ensure_env
    # Minecraft を先に上げる。mcadmind は起動時に save-on を送るので、
    # 相手が居たほうが立ち上がりのログが素直になる。
    server_compose up -d
    wait_healthy

    # 初回はフロントエンドのビルドを含むので数分かかる。
    console_compose up -d --build

    admin_port="$(value_of ADMIN_PORT)"
    wait_console "${admin_port:-8788}"

    echo
    echo "管理コンソール : http://127.0.0.1:${admin_port:-8788}/   （本番と同じ埋め込みの画面）"
    echo "画面の開発用   : http://127.0.0.1:$(value_of FRONTEND_PORT)/   （Vite。本番にこのコンテナは無い）"
    echo "Minecraft      : localhost:$(value_of MC_PORT)"
    echo "トークン       : $(value_of ADMIN_TOKEN)"
    echo
    echo "止めるとき: make test-env-down"
    ;;

  down)
    console_compose down
    server_compose down
    echo "止めました。ワールドとバックアップは test/server/ に残っています"
    ;;

  clean)
    console_compose down -v
    server_compose down -v
    # 消す前に何を消すか出す。取り違えて本番を消すことがないよう
    # パスをそのまま見せる。
    echo "次を削除します:"
    echo "  $TEST_SERVER_DIR/data"
    echo "  $TEST_SERVER_DIR/backups"
    rm -rf "$TEST_SERVER_DIR/data" "$TEST_SERVER_DIR/backups"
    echo "消しました"
    ;;

  status)
    echo "--- Minecraft (minecraft-server-test) ---"
    server_compose ps
    echo "--- 管理コンソール (mcadmin-console-test) ---"
    console_compose ps
    ;;

  logs)
    server_compose logs -f mc
    ;;

  console-logs)
    console_compose logs -f mcadmind
    ;;

  *)
    die "使い方: test/env.sh {up|down|clean|status|logs|console-logs}"
    ;;
esac
