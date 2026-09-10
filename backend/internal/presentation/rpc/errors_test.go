package rpc

import (
	"context"
	"errors"
	"strings"
	"testing"

	"connectrpc.com/connect"

	"github.com/kkito0726/minecraft-server/backend/internal/application/operations"
	"github.com/kkito0726/minecraft-server/backend/internal/application/usecase/worldctl"
	"github.com/kkito0726/minecraft-server/backend/internal/domain/world"
	"github.com/kkito0726/minecraft-server/backend/internal/infrastructure/filesystem/worldfs"
)

// エラーの写像。画面の分岐に使うので、コードが安定していること。
func TestToConnectError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		err  error
		want connect.Code
	}{
		{"実行中", operations.ErrBusy, connect.CodeFailedPrecondition},
		{"稼働中のワールド", world.ErrActiveWorld, connect.CodeFailedPrecondition},
		{"見つからない", worldfs.ErrNotFound, connect.CodeNotFound},
		{"既に存在する", worldfs.ErrAlreadyExists, connect.CodeAlreadyExists},
		{"名前が不正", world.ErrInvalidName, connect.CodeInvalidArgument},
		{"確認名の不一致", world.ErrConfirmationMismatch, connect.CodeInvalidArgument},
		{"容量不足", worldctl.ErrInsufficientSpace, connect.CodeResourceExhausted},
		{"見つからない（ドメイン）", world.ErrNotFound, connect.CodeNotFound},
		{"既に存在（ドメイン）", world.ErrAlreadyExists, connect.CodeAlreadyExists},
		{"中断", context.Canceled, connect.CodeCanceled},
		{"時間切れ", context.DeadlineExceeded, connect.CodeDeadlineExceeded},
		{"想定外", errors.New("何かが壊れた"), connect.CodeInternal},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := toConnectError(tt.err)
			if got == nil {
				t.Fatal("エラーが返るはず")
			}
			if code := connect.CodeOf(got); code != tt.want {
				t.Errorf("コードが %v。%v のはず", code, tt.want)
			}
		})
	}

	if toConnectError(nil) != nil {
		t.Error("nil は nil のまま返すはず")
	}
}

// ラップされたエラーも正しく写る。
func TestToConnectErrorUnwraps(t *testing.T) {
	t.Parallel()

	wrapped := errors.Join(errors.New("文脈"), operations.ErrBusy)
	if got := connect.CodeOf(toConnectError(wrapped)); got != connect.CodeFailedPrecondition {
		t.Errorf("コードが %v", got)
	}
}

// 利用者が自分で対処できるエラーは、内部エラーに丸めない。
// 打ち間違いに対して「サーバーのログを確認してください」と返すのは不親切。
func TestActionableErrorsKeepTheirMessage(t *testing.T) {
	t.Parallel()

	tests := []error{
		world.ErrConfirmationMismatch,
		world.ErrActiveWorld,
		world.ErrInvalidName,
		worldctl.ErrInsufficientSpace,
	}

	for _, err := range tests {
		t.Run(err.Error(), func(t *testing.T) {
			t.Parallel()
			got := toConnectError(err)
			if strings.Contains(got.Error(), "サーバーのログ") {
				t.Errorf("内部エラーに丸められている: %v", got)
			}
			if !strings.Contains(got.Error(), err.Error()) {
				t.Errorf("元のメッセージが失われている: %v", got)
			}
		})
	}
}

// 想定外のエラーは内部の詳細を漏らさない。
// ファイルパスやスタックトレースが画面に出てはいけない。
func TestToConnectErrorHidesInternals(t *testing.T) {
	t.Parallel()

	internal := errors.New("/Users/ken/secret/path/config.yaml を開けません: permission denied")
	got := toConnectError(internal)

	if strings.Contains(got.Error(), "/Users/ken") {
		t.Errorf("内部のパスが漏れている: %v", got)
	}
	if strings.Contains(got.Error(), "permission denied") {
		t.Errorf("内部の詳細が漏れている: %v", got)
	}
}
