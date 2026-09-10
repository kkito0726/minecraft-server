package rpc

import (
	"context"
	"errors"

	"connectrpc.com/connect"

	"github.com/kkito0726/minecraft-server/backend/internal/application/operations"
	"github.com/kkito0726/minecraft-server/backend/internal/application/port"
	"github.com/kkito0726/minecraft-server/backend/internal/application/usecase/worldctl"
	"github.com/kkito0726/minecraft-server/backend/internal/domain/backup"
	"github.com/kkito0726/minecraft-server/backend/internal/domain/world"
	"github.com/kkito0726/minecraft-server/backend/internal/infrastructure/filesystem/worldfs"
)

// toConnectError はドメインやユースケースのエラーを Connect のコードに写す。
//
// 内部の詳細（ファイルパス、スタックトレース）を漏らさないため、
// 想定外のエラーは Internal にまとめてメッセージも伏せる。
// 利用者が対処できるエラーだけを、そのまま画面に出す。
func toConnectError(err error) error {
	if err == nil {
		return nil
	}

	switch {
	case errors.Is(err, operations.ErrBusy), errors.Is(err, port.ErrLocked):
		return connect.NewError(connect.CodeFailedPrecondition, err)
	case errors.Is(err, world.ErrActiveWorld):
		return connect.NewError(connect.CodeFailedPrecondition, err)
	case errors.Is(err, worldfs.ErrNotFound), errors.Is(err, world.ErrNotFound),
		errors.Is(err, backup.ErrNotFound):
		return connect.NewError(connect.CodeNotFound, err)
	case errors.Is(err, worldfs.ErrAlreadyExists), errors.Is(err, world.ErrAlreadyExists):
		return connect.NewError(connect.CodeAlreadyExists, err)
	// 打ち間違いや容量不足は利用者が自分で対処できる。
	// 内部エラーに丸めず、そのまま伝える。
	case errors.Is(err, world.ErrConfirmationMismatch):
		return connect.NewError(connect.CodeInvalidArgument, err)
	case errors.Is(err, worldctl.ErrInsufficientSpace):
		return connect.NewError(connect.CodeResourceExhausted, err)
	case errors.Is(err, world.ErrInvalidName),
		errors.Is(err, backup.ErrInvalidID),
		errors.Is(err, backup.ErrInvalidRetentionPolicy):
		return connect.NewError(connect.CodeInvalidArgument, err)
	case errors.Is(err, context.Canceled):
		return connect.NewError(connect.CodeCanceled, errors.New("操作が中断されました"))
	case errors.Is(err, context.DeadlineExceeded):
		return connect.NewError(connect.CodeDeadlineExceeded, errors.New("時間内に完了しませんでした"))
	default:
		// 想定外。詳細はログにのみ残し、画面には一般的な文言を出す。
		return connect.NewError(connect.CodeInternal,
			errors.New("処理に失敗しました。サーバーのログを確認してください"))
	}
}
