// Package compose は docker compose を実行して Minecraft サーバーを操作する。
//
// RCON のポート 25575 は公開していないため、コンテナへの指示はすべてここを通る。
// README が「ports に 25575 を足さないでください」をハードルールとして
// 定めており、平文の RCON をネットワークに晒さない方針を守るための構成。
package compose

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/kkito0726/minecraft-server/backend/internal/application/port"
)

// Config は Runner の設定。
type Config struct {
	// DockerBin は docker のパス。systemd 配下では PATH が通らないことがある。
	DockerBin string
	// ProjectDir は compose.yaml と .env があるディレクトリ。
	ProjectDir string
	// ProjectName は compose のプロジェクト名。
	//
	// 省略すると Compose はディレクトリ名から推測するため、systemd 配下から
	// 叩いたときに手動操作とは別プロジェクトのコンテナが立ち上がり、
	// 同じサーバーが二重に動く。必ず明示する。
	ProjectName string
	// Service は操作対象のサービス名。
	Service string
}

// Runner は docker compose の実行。
type Runner struct {
	cfg Config
}

// NewRunner は Runner を作る。
func NewRunner(cfg Config) *Runner { return &Runner{cfg: cfg} }

// baseArgs は毎回付ける固定の引数。
//
// 作業ディレクトリに依存しないよう、プロジェクトディレクトリ・プロジェクト名・
// compose ファイルをすべて明示する。systemd 配下では cwd がリポジトリと
// 一致しないため、省略すると動かない。
func (r *Runner) baseArgs() []string {
	return []string{
		"compose",
		"--project-directory", r.cfg.ProjectDir,
		"-p", r.cfg.ProjectName,
		"-f", filepath.Join(r.cfg.ProjectDir, "compose.yaml"),
	}
}

// Up はコンテナを作り直して起動する。
//
// restart ではなく up -d を使う。OVERRIDE_SERVER_PROPERTIES=TRUE のため
// server.properties は起動時に .env から再生成されるが、サーバープロセスが
// それを読むのは起動時の 1 回だけで、restart では .env の変更が反映されない。
func (r *Runner) Up(ctx context.Context, sink port.LogSink) error {
	return r.runStreaming(ctx, sink, "up", "-d")
}

// Down はコンテナを停止して削除する。
// compose.yaml の stop_grace_period（60 秒）が尊重される。
func (r *Runner) Down(ctx context.Context, sink port.LogSink) error {
	return r.runStreaming(ctx, sink, "down")
}

// Exec はサービス内でコマンドを実行して出力を返す。
//
// -T を付けるのは TTY を割り当てないため。systemd 配下には TTY が無く、
// 付けないと "the input device is not a TTY" で失敗する。
func (r *Runner) Exec(ctx context.Context, argv []string) (string, error) {
	args := append(r.baseArgs(), "exec", "-T", r.cfg.Service)
	args = append(args, argv...)

	out, err := r.runCapture(ctx, args...)
	if err != nil {
		return "", err
	}
	return out, nil
}

// runStreaming はコマンドを実行し、出力を 1 行ずつ sink に流す。
func (r *Runner) runStreaming(ctx context.Context, sink port.LogSink, sub ...string) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	args := append(r.baseArgs(), sub...)
	cmd := exec.CommandContext(ctx, r.cfg.DockerBin, args...)

	pipe, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("出力を受け取れません: %w", err)
	}
	cmd.Stderr = cmd.Stdout

	var tail lineTail
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("docker compose を起動できません: %w", err)
	}

	scanLines(pipe, func(line string) {
		tail.add(line)
		if sink != nil {
			sink(line)
		}
	})

	if err := cmd.Wait(); err != nil {
		return fmt.Errorf("docker compose %s に失敗しました: %w\n%s",
			strings.Join(sub, " "), err, tail.String())
	}
	return nil
}

// runCapture はコマンドを実行して出力をまとめて返す。
func (r *Runner) runCapture(ctx context.Context, args ...string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}

	cmd := exec.CommandContext(ctx, r.cfg.DockerBin, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("docker compose の実行に失敗しました: %w\n%s",
			err, strings.TrimSpace(stderr.String()))
	}
	return stdout.String(), nil
}

func scanLines(r io.Reader, fn func(string)) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		fn(scanner.Text())
	}
}

// maxTailLines はエラーに含める出力の行数。
// 全部載せるとログが読めなくなるので、原因が分かる程度に絞る。
const maxTailLines = 20

// lineTail は直近の出力を保持する。失敗したときの原因究明に使う。
type lineTail struct {
	lines []string
}

func (t *lineTail) add(line string) {
	t.lines = append(t.lines, line)
	if len(t.lines) > maxTailLines {
		t.lines = t.lines[1:]
	}
}

func (t *lineTail) String() string { return strings.Join(t.lines, "\n") }

// pollInterval は状態を確認する間隔。
const pollInterval = 100 * time.Millisecond

// WaitStopped はコンテナが存在しなくなるまで待つ。
func (r *Runner) WaitStopped(ctx context.Context, timeout time.Duration) error {
	return r.poll(ctx, timeout, "停止", func() (bool, error) {
		st, err := r.Status(ctx)
		if err != nil {
			return false, err
		}
		return st.State.IsStopped(), nil
	})
}

// poll は条件が満たされるまで一定間隔で確認する。
func (r *Runner) poll(
	ctx context.Context,
	timeout time.Duration,
	what string,
	done func() (bool, error),
) error {
	deadline := time.Now().Add(timeout)
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	for {
		ok, err := done()
		if err != nil {
			return err
		}
		if ok {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("%sを %s 以内に確認できませんでした", what, timeout)
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

var _ port.ContainerRuntime = (*Runner)(nil)
