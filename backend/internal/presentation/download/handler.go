package download

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	"github.com/kkito0726/minecraft-server/backend/internal/application/port"
	"github.com/kkito0726/minecraft-server/backend/internal/domain/backup"
)

// TokenParam は受取券を載せる問い合わせ文字列の名前。
const TokenParam = "t"

// Opener は保管済みのアーカイブを開く口。
type Opener interface {
	OpenForDownload(ctx context.Context, backupID string) (port.ArchiveFile, error)
}

// Handler は受取券と引き換えにアーカイブを流す。
//
// **このルートは認証ミドルウェアで包まない。** 券そのものが認可で、
// 認証済みの RPC でしか発行されない。包むと、ヘッダーを付けられない
// ブラウザのダウンロードが通らなくなる。
type Handler struct {
	tickets *Tickets
	opener  Opener
	logger  *slog.Logger
}

// NewHandler はダウンロードのハンドラを作る。
func NewHandler(tickets *Tickets, opener Opener, logger *slog.Logger) *Handler {
	if logger == nil {
		logger = slog.Default()
	}
	return &Handler{tickets: tickets, opener: opener, logger: logger}
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		http.Error(w, "GET で取得してください", http.StatusMethodNotAllowed)
		return
	}

	backupID, ok := h.tickets.Redeem(r.URL.Query().Get(TokenParam))
	if !ok {
		// 券の有無以上のことは言わない。存在するバックアップの名前を
		// 当てにきた相手に、手がかりを渡す理由がない。
		http.Error(w, "リンクの有効期限が切れています。もう一度ダウンロードを押してください",
			http.StatusNotFound)
		return
	}

	file, err := h.opener.OpenForDownload(r.Context(), backupID)
	if err != nil {
		h.fail(w, backupID, err)
		return
	}
	defer func() { _ = file.Body.Close() }()

	w.Header().Set("Content-Type", "application/zip")
	// ファイル名は英数字と . - _ だけ（backup.NewID が保証する）。
	w.Header().Set("Content-Disposition", `attachment; filename="`+backupID+`"`)
	// 中身はワールドそのもの。共有の経路に残さない。
	w.Header().Set("Cache-Control", "no-store")

	// ServeContent が Range 要求と条件付き GET を扱う。
	// 途中で切れた転送を、続きから取り直せるようにするため。
	http.ServeContent(w, r, backupID, file.Info.CreatedAt, file.Body)
}

func (h *Handler) fail(w http.ResponseWriter, backupID string, err error) {
	if errors.Is(err, backup.ErrNotFound) || errors.Is(err, backup.ErrInvalidID) {
		http.Error(w, "バックアップが見つかりません", http.StatusNotFound)
		return
	}
	// 原因は記録にだけ残す。パスや内部の事情を画面へ出さない。
	h.logger.Error("バックアップを渡せませんでした", "backup_id", backupID, "error", err)
	http.Error(w, "ダウンロードに失敗しました。サーバーのログを確認してください",
		http.StatusInternalServerError)
}
