package download

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/kkito0726/minecraft-server/backend/internal/application/port"
	"github.com/kkito0726/minecraft-server/backend/internal/domain/backup"
)

const archiveBody = "PK\x03\x04 これはアーカイブの中身のつもり"

type fakeArchive struct{ *bytes.Reader }

func (fakeArchive) Close() error { return nil }

type fakeOpener struct {
	body string
	err  error
	// opened は開かれた ID。券と結び付いた ID が渡ることを確かめる。
	opened string
}

func (f *fakeOpener) OpenForDownload(_ context.Context, backupID string) (port.ArchiveFile, error) {
	f.opened = backupID
	if f.err != nil {
		return port.ArchiveFile{}, f.err
	}
	return port.ArchiveFile{
		Body: fakeArchive{bytes.NewReader([]byte(f.body))},
		Info: port.StoredBackup{SizeBytes: int64(len(f.body)), CreatedAt: time.Unix(1_700_000_000, 0)},
	}, nil
}

func newHandler(t *testing.T, opener *fakeOpener) (*Handler, *Tickets) {
	t.Helper()

	tickets := NewTickets("/download/backup")
	return NewHandler(tickets, opener, nil), tickets
}

func get(t *testing.T, h *Handler, target string, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()

	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, target, nil)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestServesArchiveForValidTicket(t *testing.T) {
	t.Parallel()

	opener := &fakeOpener{body: archiveBody}
	h, tickets := newHandler(t, opener)
	url, _, err := tickets.Issue("backup-26.2-world-20260916-0300.zip")
	if err != nil {
		t.Fatal(err)
	}

	rec := get(t, h, url, nil)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	if got := rec.Body.String(); got != archiveBody {
		t.Errorf("中身が違う: %q", got)
	}
	if opener.opened != "backup-26.2-world-20260916-0300.zip" {
		t.Errorf("開いた対象が %q", opener.opened)
	}
	// 保存のダイアログが出て、ファイル名が付くこと。
	if got := rec.Header().Get("Content-Disposition"); got != `attachment; filename="backup-26.2-world-20260916-0300.zip"` {
		t.Errorf("Content-Disposition が %q", got)
	}
	if got := rec.Header().Get("Content-Type"); got != "application/zip" {
		t.Errorf("Content-Type が %q", got)
	}
	// ワールドそのものなので、共有の経路に残さない。
	if got := rec.Header().Get("Cache-Control"); got != "no-store" {
		t.Errorf("Cache-Control が %q", got)
	}
}

// 途中で切れた転送を続きから取り直せること。
func TestSupportsRangeRequests(t *testing.T) {
	t.Parallel()

	h, tickets := newHandler(t, &fakeOpener{body: archiveBody})
	url, _, err := tickets.Issue("backup.zip")
	if err != nil {
		t.Fatal(err)
	}

	rec := get(t, h, url, map[string]string{"Range": "bytes=4-"})

	if rec.Code != http.StatusPartialContent {
		t.Fatalf("status = %d。206 のはず", rec.Code)
	}
	if got, want := rec.Body.String(), archiveBody[4:]; got != want {
		t.Errorf("続きが %q。%q のはず", got, want)
	}
}

func TestRejectsMissingOrExpiredTicket(t *testing.T) {
	t.Parallel()

	h, _ := newHandler(t, &fakeOpener{body: archiveBody})

	for _, target := range []string{"/download/backup", "/download/backup?t=", "/download/backup?t=にせもの"} {
		rec := get(t, h, target, nil)
		if rec.Code != http.StatusNotFound {
			t.Errorf("%s の status が %d。404 のはず", target, rec.Code)
		}
		// 存在するバックアップの名前を当てにきた相手に手がかりを渡さない。
		if body := rec.Body.String(); len(body) > 200 {
			t.Errorf("返す本文が長すぎる: %q", body)
		}
	}
}

func TestRejectsNonGet(t *testing.T) {
	t.Parallel()

	h, tickets := newHandler(t, &fakeOpener{body: archiveBody})
	url, _, err := tickets.Issue("backup.zip")
	if err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, url, nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d", rec.Code)
	}
}

// 券は通ったが実体が無い。券を出した後に削除された場合など。
func TestMissingArchiveIsNotFound(t *testing.T) {
	t.Parallel()

	opener := &fakeOpener{err: fmt.Errorf("%w: backup.zip", backup.ErrNotFound)}
	h, tickets := newHandler(t, opener)
	url, _, err := tickets.Issue("backup.zip")
	if err != nil {
		t.Fatal(err)
	}

	rec := get(t, h, url, nil)

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d", rec.Code)
	}
}

// 想定外の失敗は、内部の事情を画面へ出さない。
func TestUnexpectedFailureIsHidden(t *testing.T) {
	t.Parallel()

	opener := &fakeOpener{err: errors.New("/home/pi/minecraft-server/backups が読めない")}
	h, tickets := newHandler(t, opener)
	url, _, err := tickets.Issue("backup.zip")
	if err != nil {
		t.Fatal(err)
	}

	rec := get(t, h, url, nil)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d", rec.Code)
	}
	if body, _ := io.ReadAll(rec.Body); bytes.Contains(body, []byte("/home/pi")) {
		t.Errorf("内部のパスが漏れている: %s", body)
	}
}
