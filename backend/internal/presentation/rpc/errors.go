package rpc

import (
	"context"
	"errors"

	"connectrpc.com/connect"

	"github.com/kkito0726/minecraft-server/backend/internal/application/operations"
	"github.com/kkito0726/minecraft-server/backend/internal/application/port"
	"github.com/kkito0726/minecraft-server/backend/internal/application/usecase/backupctl"
	"github.com/kkito0726/minecraft-server/backend/internal/domain/backup"
	"github.com/kkito0726/minecraft-server/backend/internal/domain/world"
	"github.com/kkito0726/minecraft-server/backend/internal/infrastructure/archive"
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
	// 承諾がまだ、という状態は画面が直せる。伏せずにそのまま返す。
	case errors.Is(err, backupctl.ErrConfirmationRequired),
		errors.Is(err, backupctl.ErrUnknownArchiveLevel),
		errors.Is(err, backupctl.ErrInvalidMode):
		return connect.NewError(connect.CodeFailedPrecondition, err)
	case errors.Is(err, port.ErrInsufficientSpace):
		return connect.NewError(connect.CodeResourceExhausted, err)
	case errors.Is(err, world.ErrInvalidName),
		errors.Is(err, backup.ErrInvalidID),
		errors.Is(err, backup.ErrInvalidRetentionPolicy):
		return connect.NewError(connect.CodeInvalidArgument, err)
	// 安全でないアーカイブは利用者が包み直せる。エントリ名は
	// 利用者自身の zip から来たものなので、伝えても内部は漏れない。
	case errors.Is(err, archive.ErrUnsafeEntry):
		return connect.NewError(connect.CodeInvalidArgument, err)
	case errors.Is(err, context.Canceled):
		return connect.NewError(connect.CodeCanceled, errors.New("操作が中断されました"))
	case errors.Is(err, context.DeadlineExceeded):
		return connect.NewError(connect.CodeDeadlineExceeded, errors.New("時間内に完了しませんでした"))
	default:
		// 想定外。詳細はログにのみ残し、画面には一般的な文言を出す。
		// 原因は internalError が運び、LoggingInterceptor が記録する。
		return connect.NewError(connect.CodeInternal, &internalError{cause: err})
	}
}

// internalError は画面に伏せた原因を、記録のために運ぶ。
//
// Connect はエラーの Error() をそのままクライアントへ送る。だから
// メッセージは一般的な文言でなければならない一方、原因を捨てると
// サーバー側でも何が起きたか分からなくなる。表に出す文字列と、
// 内部に残す原因を別々に持たせることで両立させる。
type internalError struct {
	cause error
}

// Error はクライアントへ送られる文言。内部の詳細を含めてはいけない。
func (e *internalError) Error() string {
	return "処理に失敗しました。サーバーのログを確認してください"
}

// Unwrap は伏せた原因を返す。記録にのみ使う。
func (e *internalError) Unwrap() error { return e.cause }
