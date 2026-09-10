#!/usr/bin/env bash
# docker の代わりに mcadmind から呼ばれるスタブ。
#
# E2E は「利用者から見た振る舞い」を確かめるものであって、docker や
# Paper の動作を確かめるものではない。実コンテナを起動すると 1 本あたり
# 数分かかり、CI では docker そのものが使えないこともある。
#
# ADMIN_DOCKER_BIN でここに差し替えられるようにしてあるのは、この用途のため。
#
# 状態は FAKE_DOCKER_STATE のディレクトリに置く。mcadmind は毎回別の
# プロセスとしてこのスクリプトを起動するので、プロセス内には何も持てない。
set -euo pipefail

state_dir="${FAKE_DOCKER_STATE:?FAKE_DOCKER_STATE が未設定です}"
mkdir -p "$state_dir"

running_file="$state_dir/running"
ready_at_file="$state_dir/ready_at"
log_file="$state_dir/argv.log"

# 起動が完了したことにするまでの秒数。
# 0 なら即座に ready。進行中の画面を確かめる試験だけ長くする。
ready_delay="${FAKE_DOCKER_READY_DELAY:-0}"

printf '%s\n' "$*" >> "$log_file"

now() { date +%s; }

is_ready() {
  [ -f "$running_file" ] || return 1
  [ -f "$ready_at_file" ] || return 0
  [ "$(now)" -ge "$(cat "$ready_at_file")" ]
}

# 部分コマンドは位置ではなく走査で見つける。
# baseArgs（--project-directory / -p / -f）の並びに依存しないため。
subcommand=""
for arg in "$@"; do
  case "$arg" in
    up|down|ps|logs|exec) subcommand="$arg"; break ;;
  esac
done

case "$subcommand" in
  up)
    echo " Container minecraft-server  Creating"
    echo " Container minecraft-server  Created"
    echo " Container minecraft-server  Starting"
    echo " Container minecraft-server  Started"
    : > "$running_file"
    echo $(( $(now) + ready_delay )) > "$ready_at_file"
    ;;

  down)
    echo " Container minecraft-server  Stopping"
    echo " Container minecraft-server  Stopped"
    echo " Container minecraft-server  Removing"
    echo " Container minecraft-server  Removed"
    # 本物の down はコンテナを削除する。ps には何も出なくなる。
    rm -f "$running_file" "$ready_at_file"
    ;;

  ps)
    # コンテナが無ければ何も出さない。decodeEntries が空を返し
    # ContainerMissing になる。
    if [ -f "$running_file" ]; then
      health=starting
      if is_ready; then health=healthy; fi
      printf '{"Name":"minecraft-server","State":"running","Health":"%s","CreatedAt":"%s","Image":"itzg/minecraft-server:latest"}\n' \
        "$health" "$(date '+%Y-%m-%d %H:%M:%S %z %Z')"
    fi
    ;;

  logs)
    echo "[Server thread/INFO]: Starting minecraft server version 26.2"
    echo "[Server thread/INFO]: Preparing level \"world\""
    # 起動完了の印は ready になってから出す。
    # これが無いあいだ WaitReady は待ち続ける。
    if is_ready; then
      echo '[Server thread/INFO]: Done (7.575s)! For help, type "help"'
    fi
    ;;

  exec)
    # exec -T mc rcon-cli <コマンド...>
    rcon_command=""
    seen_cli=0
    for arg in "$@"; do
      if [ "$seen_cli" = 1 ]; then rcon_command="$arg"; break; fi
      if [ "$arg" = "rcon-cli" ]; then seen_cli=1; fi
    done

    if [ ! -f "$running_file" ]; then
      echo "service \"mc\" is not running" >&2
      exit 1
    fi

    case "$rcon_command" in
      save-off) echo "Automatic saving is now disabled" ;;
      save-all) echo "Saved the game" ;;
      # 既に有効なときの実測の戻り。冪等であることが回復設計の前提。
      save-on)  echo "Automatic saving is now enabled" ;;
      list)     echo "There are 0 of a max of 5 players online:" ;;
      *)        echo "Unknown or incomplete command" ;;
    esac
    ;;

  *)
    echo "fake-docker: 未対応の呼び出し: $*" >&2
    exit 1
    ;;
esac
