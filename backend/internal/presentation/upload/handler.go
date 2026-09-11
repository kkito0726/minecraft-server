// Package upload は zip のアップロードを受け取る HTTP ハンドラ。
//
// Connect の外側に置いている。ブラウザは平文の HTTP/2 へ昇格しないため
// client-streaming の RPC が使えず、単項 RPC では本体を丸ごとメモリに
// 載せることになる。MC が 2.7GB を使う 4GB の Pi でそれはできない。
// 素の POST なら本文をそのままディスクへ流せる。
package upload

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"os"

	"github.com/kkito0726/minecraft-server/backend/internal/application/port"
	"github.com/kkito0726/minecraft-server/backend/internal/application/usecase/backupctl"
	"github.com/kkito0726/minecraft-server/backend/internal/domain/backup"
	"github.com/kkito0726/minecraft-server/backend/internal/infrastructure/archive"
	"github.com/kkito0726/minecraft-server/backend/internal/infrastructure/persistence/backupfs"
)

// Handler は zip を受け取って保管先に取り込む。
type Handler struct {
	uc     *backupctl.UseCase
	logger *slog.Logger
}

// NewHandler はハンドラを作る。
func NewHandler(uc *backupctl.UseCase, logger *slog.Logger) *Handler {
	if logger == nil {
		logger = slog.Default()
	}
	return &Handler{uc: uc, logger: logger}
}

// response は成功したときに返す内容。
type response struct {
	ID        string `json:"id"`
	Level     string `json:"level"`
	Version   string `json:"version"`
	SizeBytes int64  `json:"sizeBytes"`
	// Rewrapped は data/ 配下へ包み直したか。画面の案内を変えるために返す。
	Rewrapped bool `json:"rewrapped"`
}

type errorResponse struct {
	Message string `json:"message"`
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		writeError(w, http.StatusMethodNotAllowed, "POST で送ってください")
		return
	}
	defer func() { _ = r.Body.Close() }()

	// Content-Length は目安。無い場合もあるので 0 を「不明」として扱い、
	// 受け取る側の上限で守る。
	result, err := h.uc.Import(r.Context(), r.Body, r.ContentLength)
	if err != nil {
		h.fail(w, r, err)
		return
	}

	writeJSON(w, http.StatusOK, response{
		ID:        result.ID.String(),
		Level:     result.Level,
		Version:   versionText(result),
		SizeBytes: result.SizeBytes,
		Rewrapped: result.Rewrapped,
	})
}

func versionText(result backupctl.ImportResult) string {
	if !result.Version.Readable() {
		return ""
	}
	return result.Version.Name()
}

/*
fail はエラーを状態コードに写す。

利用者が自分で直せるもの（形が違う、大きすぎる、名前が衝突した）は
そのまま伝える。伝えないと、何をどう直せばよいか分からないまま
同じ zip を何度も送ることになる。想定外のものだけ伏せて記録に残す。
*/
func (h *Handler) fail(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, backup.ErrNotImportable), errors.Is(err, archive.ErrUnsafeEntry):
		writeError(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, backupfs.ErrTooLarge):
		writeError(w, http.StatusRequestEntityTooLarge, err.Error())
	case errors.Is(err, port.ErrInsufficientSpace):
		writeError(w, http.StatusInsufficientStorage, err.Error())
	case errors.Is(err, os.ErrExist):
		writeError(w, http.StatusConflict, "同じ名前のバックアップが既にあります")
	case errors.Is(err, r.Context().Err()) && r.Context().Err() != nil:
		// 送信の途中で切れた。返す相手がもういないので記録だけ残す。
		h.logger.InfoContext(r.Context(), "アップロードが中断されました")
	default:
		h.logger.ErrorContext(r.Context(), "アップロードに失敗しました", "error", err)
		writeError(w, http.StatusInternalServerError,
			"取り込みに失敗しました。サーバーのログを確認してください")
	}
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, errorResponse{Message: message})
}
