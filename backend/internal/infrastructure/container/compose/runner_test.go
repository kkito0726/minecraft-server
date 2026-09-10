package compose_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kkito0726/minecraft-server/backend/internal/domain/server"
	"github.com/kkito0726/minecraft-server/backend/internal/infrastructure/container/compose"
)

// newRunner は argv を記録するスタブ docker を使う Runner を作る。
//
// 実際の docker を使わないのは、CI で動かすためだけではない。
// 「どんな引数で呼んだか」を検証したいのであって、
// docker が何をするかは docker の責任だから。
func newRunner(t *testing.T, script string) (*compose.Runner, string) {
	t.Helper()

	dir := t.TempDir()
	argvLog := filepath.Join(dir, "argv.log")
	bin := filepath.Join(dir, "fake-docker")

	full := "#!/bin/sh\n" +
		"printf '%s\\n' \"$*\" >> " + argvLog + "\n" +
		script + "\n"
	if err := os.WriteFile(bin, []byte(full), 0o700); err != nil {
		t.Fatal(err)
	}

	projectDir := filepath.Join(dir, "project")
	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		t.Fatal(err)
	}

	r := compose.NewRunner(compose.Config{
		DockerBin:   bin,
		ProjectDir:  projectDir,
		ProjectName: "minecraft-server",
		Service:     "mc",
	})
	return r, argvLog
}

func readArgv(t *testing.T, path string) []string {
	t.Helper()

	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		t.Fatal(err)
	}
	return strings.Split(strings.TrimRight(string(b), "\n"), "\n")
}

// systemd 配下では作業ディレクトリがリポジトリと一致しない。
// プロジェクト名を省略すると、手動の docker compose とは別プロジェクトの
// コンテナが立ち上がり、同じサーバーが二重に動く。
func TestUpPassesProjectIdentity(t *testing.T) {
	t.Parallel()

	r, argvLog := newRunner(t, "exit 0")

	if err := r.Up(context.Background(), nil); err != nil {
		t.Fatalf("Up に失敗: %v", err)
	}

	argv := readArgv(t, argvLog)
	if len(argv) != 1 {
		t.Fatalf("呼び出しが %d 回", len(argv))
	}
	for _, want := range []string{
		"compose",
		"--project-directory",
		"-p minecraft-server",
		"-f",
		"compose.yaml",
		"up -d",
	} {
		if !strings.Contains(argv[0], want) {
			t.Errorf("引数に %q が無い: %s", want, argv[0])
		}
	}
}

// .env の変更はコンテナの再作成でしか反映されない。
// restart を使うと「設定を変えたのに反映されない」になる。
func TestUpDoesNotUseRestart(t *testing.T) {
	t.Parallel()

	r, argvLog := newRunner(t, "exit 0")
	if err := r.Up(context.Background(), nil); err != nil {
		t.Fatal(err)
	}

	if strings.Contains(readArgv(t, argvLog)[0], "restart") {
		t.Error("restart を使っている。.env の変更が反映されない")
	}
}

func TestDown(t *testing.T) {
	t.Parallel()

	r, argvLog := newRunner(t, "exit 0")
	if err := r.Down(context.Background(), nil); err != nil {
		t.Fatalf("Down に失敗: %v", err)
	}

	if !strings.Contains(readArgv(t, argvLog)[0], "down") {
		t.Errorf("down を呼んでいない: %s", readArgv(t, argvLog)[0])
	}
}

// コマンドの出力は 1 行ずつ進捗ログに流す。
func TestLogSinkReceivesLines(t *testing.T) {
	t.Parallel()

	r, _ := newRunner(t, `echo "1 行目"; echo "2 行目"; exit 0`)

	var lines []string
	if err := r.Up(context.Background(), func(l string) { lines = append(lines, l) }); err != nil {
		t.Fatal(err)
	}

	if len(lines) != 2 || lines[0] != "1 行目" || lines[1] != "2 行目" {
		t.Errorf("受け取った行が %v", lines)
	}
}

// 失敗したら、docker の出力をエラーに含める。
// これが無いと「compose に失敗しました」だけで原因が分からない。
func TestFailureIncludesOutput(t *testing.T) {
	t.Parallel()

	r, _ := newRunner(t, `echo "no configuration file provided" >&2; exit 1`)

	err := r.Up(context.Background(), nil)
	if err == nil {
		t.Fatal("エラーになるはず")
	}
	if !strings.Contains(err.Error(), "no configuration file provided") {
		t.Errorf("エラーに docker の出力が含まれていない: %v", err)
	}
}

// docker compose ps --format json の形は Compose のバージョンで変わる。
// 必要なフィールドだけを拾い、未知のフィールドは無視する。
func TestStatusParsing(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		output      string
		wantState   server.ContainerState
		wantHealthy bool
	}{
		{
			name: "実行中で healthy（単一オブジェクト）",
			output: `{"Name":"minecraft-server","State":"running","Health":"healthy",` +
				`"CreatedAt":"2026-09-10 00:50:00 +0900 JST","Image":"itzg/minecraft-server","Unknown":"x"}`,
			wantState:   server.ContainerRunning,
			wantHealthy: true,
		},
		{
			name: "起動中（healthy でない）",
			output: `{"Name":"minecraft-server","State":"running","Health":"starting",` +
				`"CreatedAt":"2026-09-10 00:50:00 +0900 JST"}`,
			wantState:   server.ContainerRunning,
			wantHealthy: false,
		},
		{
			name:      "終了している",
			output:    `{"Name":"minecraft-server","State":"exited"}`,
			wantState: server.ContainerExited,
		},
		{
			name:      "再起動中",
			output:    `{"Name":"minecraft-server","State":"restarting"}`,
			wantState: server.ContainerRestarting,
		},
		{
			name:      "コンテナが無い（空の出力）",
			output:    "",
			wantState: server.ContainerMissing,
		},
		{
			name:      "コンテナが無い（空の配列）",
			output:    "[]",
			wantState: server.ContainerMissing,
		},
		{
			name: "行区切り JSON",
			output: `{"Name":"other","State":"exited"}` + "\n" +
				`{"Name":"minecraft-server","State":"running","Health":"healthy"}`,
			wantState:   server.ContainerRunning,
			wantHealthy: true,
		},
		{
			name:        "配列形式",
			output:      `[{"Name":"minecraft-server","State":"running","Health":"healthy"}]`,
			wantState:   server.ContainerRunning,
			wantHealthy: true,
		},
		{
			name:      "未知の State は不明として扱う",
			output:    `{"Name":"minecraft-server","State":"paused"}`,
			wantState: server.ContainerMissing,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			script := "exit 0"
			if tt.output != "" {
				script = "cat <<'JSON'\n" + tt.output + "\nJSON"
			}
			r, _ := newRunner(t, script)

			got, err := r.Status(context.Background())
			if err != nil {
				t.Fatalf("Status に失敗: %v", err)
			}
			if got.State != tt.wantState {
				t.Errorf("State が %v。%v のはず", got.State, tt.wantState)
			}
			if got.Healthy != tt.wantHealthy {
				t.Errorf("Healthy が %v。%v のはず", got.Healthy, tt.wantHealthy)
			}
		})
	}
}

// 解釈できない出力でも、状態取得そのものを失敗させない。
// 画面全体が使えなくなるより「不明」と出た方がよい。
func TestStatusWithUnparsableOutput(t *testing.T) {
	t.Parallel()

	r, _ := newRunner(t, `echo "これは JSON ではない"`)

	got, err := r.Status(context.Background())
	if err != nil {
		t.Fatalf("エラーにせず不明として扱うはず: %v", err)
	}
	if got.State != server.ContainerMissing {
		t.Errorf("State が %v", got.State)
	}
}

// 停止の完了を待つ。stop_grace_period の 60 秒を尊重するため
// 十分な猶予が要るが、無限には待たない。
func TestWaitStopped(t *testing.T) {
	t.Parallel()

	t.Run("すぐ停止していれば即座に返る", func(t *testing.T) {
		t.Parallel()
		r, _ := newRunner(t, "exit 0") // 出力なし = コンテナ無し
		if err := r.WaitStopped(context.Background(), 2*time.Second); err != nil {
			t.Errorf("エラー: %v", err)
		}
	})

	t.Run("停止しなければタイムアウトする", func(t *testing.T) {
		t.Parallel()
		r, _ := newRunner(t, `echo '{"Name":"minecraft-server","State":"running"}'`)
		err := r.WaitStopped(context.Background(), 300*time.Millisecond)
		if err == nil {
			t.Error("タイムアウトするはず")
		}
	})
}

// 起動完了の判定は「ログの Done (」と「healthy」のいずれかで行う。
// ログはローテーションやバッファリングで取りこぼしうるため、片方に頼らない。
func TestWaitReady(t *testing.T) {
	t.Parallel()

	t.Run("ログの Done で判定する", func(t *testing.T) {
		t.Parallel()
		// ps は healthy を返さないが、logs に Done がある
		script := `case "$*" in
  *logs*) echo '[00:50:28 INFO]: Done (7.575s)! For help, type "help"' ;;
  *) echo '{"Name":"minecraft-server","State":"running","Health":"starting"}' ;;
esac`
		r, _ := newRunner(t, script)
		if err := r.WaitReady(context.Background(), 3*time.Second); err != nil {
			t.Errorf("Done を検出できていない: %v", err)
		}
	})

	t.Run("healthy で判定する", func(t *testing.T) {
		t.Parallel()
		// logs に Done が出ないが、ps は healthy
		script := `case "$*" in
  *logs*) echo '[00:50:20 INFO]: Preparing level "world"' ;;
  *) echo '{"Name":"minecraft-server","State":"running","Health":"healthy"}' ;;
esac`
		r, _ := newRunner(t, script)
		if err := r.WaitReady(context.Background(), 3*time.Second); err != nil {
			t.Errorf("healthy を検出できていない: %v", err)
		}
	})

	t.Run("どちらも来なければタイムアウトする", func(t *testing.T) {
		t.Parallel()
		script := `case "$*" in
  *logs*) echo '起動中' ;;
  *) echo '{"Name":"minecraft-server","State":"running","Health":"starting"}' ;;
esac`
		r, _ := newRunner(t, script)
		if err := r.WaitReady(context.Background(), 300*time.Millisecond); err == nil {
			t.Error("タイムアウトするはず")
		}
	})
}

// Exec はサービス内でコマンドを実行する。RCON はこれを経由する。
// -T を付けないと、TTY が無い環境（systemd 配下）で失敗する。
func TestExecUsesNoTTY(t *testing.T) {
	t.Parallel()

	r, argvLog := newRunner(t, `echo "There are 0 of a max of 5 players online:"`)

	out, err := r.Exec(context.Background(), []string{"rcon-cli", "list"})
	if err != nil {
		t.Fatalf("Exec に失敗: %v", err)
	}
	if !strings.Contains(out, "0 of a max of 5") {
		t.Errorf("出力が %q", out)
	}

	argv := readArgv(t, argvLog)[0]
	for _, want := range []string{"exec", "-T", "mc", "rcon-cli list"} {
		if !strings.Contains(argv, want) {
			t.Errorf("引数に %q が無い: %s", want, argv)
		}
	}
}

// キャンセルされた context では実行しない。
func TestRespectsContext(t *testing.T) {
	t.Parallel()

	r, argvLog := newRunner(t, "exit 0")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if err := r.Up(ctx, nil); err == nil {
		t.Error("エラーになるはず")
	}
	if argv := readArgv(t, argvLog); len(argv) != 0 {
		t.Errorf("キャンセル済みなのに実行された: %v", argv)
	}
}
