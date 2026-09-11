package upload_test

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/kkito0726/minecraft-server/backend/internal/application/operations"
	"github.com/kkito0726/minecraft-server/backend/internal/application/usecase/backupctl"
	"github.com/kkito0726/minecraft-server/backend/internal/infrastructure/persistence/backupfs"
	"github.com/kkito0726/minecraft-server/backend/internal/infrastructure/persistence/lockfile"
	"github.com/kkito0726/minecraft-server/backend/internal/presentation/upload"
)

func zipBytes(t *testing.T, files map[string]string) []byte {
	t.Helper()

	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	for name, body := range files {
		f, err := w.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := io.WriteString(f, body); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// newHandler は実物の保管先に対して動くハンドラを用意する。
func newHandler(t *testing.T) (http.Handler, string) {
	t.Helper()

	project := t.TempDir()
	dir := filepath.Join(project, "backups")

	store, err := backupfs.New(backupfs.Config{ProjectDir: project, BackupDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	ops, err := operations.NewManager(operations.Config{
		Lock: lockfile.NewLock(filepath.Join(project, ".lock")),
	})
	if err != nil {
		t.Fatal(err)
	}
	uc, err := backupctl.New(backupctl.Config{
		Runtime:    stubRuntime{},
		Console:    stubConsole{},
		Store:      store,
		Worlds:     stubWorlds{available: 1 << 30},
		Config:     stubConfig{},
		Levels:     stubLevels{},
		Operations: ops,
	})
	if err != nil {
		t.Fatal(err)
	}
	return upload.NewHandler(uc, nil), dir
}

func post(t *testing.T, h http.Handler, body []byte) *httptest.ResponseRecorder {
	t.Helper()

	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/upload/backup", bytes.NewReader(body))
	req.ContentLength = int64(len(body))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestUploadAcceptsArchiveUnderData(t *testing.T) {
	t.Parallel()

	h, _ := newHandler(t)
	rec := post(t, h, zipBytes(t, map[string]string{"data/world/level.dat": "nbt"}))

	if rec.Code != http.StatusOK {
		t.Fatalf("状態コードが %d: %s", rec.Code, rec.Body.String())
	}

	var got struct {
		ID        string `json:"id"`
		Level     string `json:"level"`
		Rewrapped bool   `json:"rewrapped"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Level != "world" {
		t.Errorf("ワールド名が %q", got.Level)
	}
	if got.Rewrapped {
		t.Error("data/ 配下なのに包み直している")
	}
}

// 配布ワールドの形をそのまま受け取れること。これが取り込みの主目的。
func TestUploadRewrapsWorldOutsideData(t *testing.T) {
	t.Parallel()

	h, _ := newHandler(t)
	rec := post(t, h, zipBytes(t, map[string]string{
		"MyWorld/level.dat":        "nbt",
		"MyWorld/region/r.0.0.mca": "chunk",
	}))

	if rec.Code != http.StatusOK {
		t.Fatalf("状態コードが %d: %s", rec.Code, rec.Body.String())
	}

	var got struct {
		Level     string `json:"level"`
		Rewrapped bool   `json:"rewrapped"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Level != "MyWorld" || !got.Rewrapped {
		t.Errorf("包み直されていない: %+v", got)
	}
}

/*
何が悪いのかを伝える。

伝えないと、利用者は同じ zip を何度も送ることになる。ここで返す
文言はどれも利用者自身のファイルについての話で、内部の情報は含まない。
*/
func TestUploadExplainsWhatIsWrong(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		files  map[string]string
		status int
		reason string
	}{
		{
			name:   "ワールドが入っていない",
			files:  map[string]string{"readme.txt": "これはワールドではない"},
			status: http.StatusBadRequest,
			reason: "ワールド",
		},
		{
			name:   "level.dat が直下にある",
			files:  map[string]string{"level.dat": "nbt"},
			status: http.StatusBadRequest,
			reason: "フォルダ",
		},
		{
			name:   "ワールドが複数",
			files:  map[string]string{"A/level.dat": "nbt", "B/level.dat": "nbt"},
			status: http.StatusBadRequest,
			reason: "複数",
		},
		{
			name:   "展開先の外を指している",
			files:  map[string]string{"../escape/level.dat": "nbt"},
			status: http.StatusBadRequest,
			reason: "外",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h, dir := newHandler(t)
			rec := post(t, h, zipBytes(t, tt.files))

			if rec.Code != tt.status {
				t.Fatalf("状態コードが %d（期待 %d）: %s", rec.Code, tt.status, rec.Body.String())
			}
			if !bytes.Contains(rec.Body.Bytes(), []byte(tt.reason)) {
				t.Errorf("理由に %q が含まれない: %s", tt.reason, rec.Body.String())
			}
			assertEmpty(t, dir)
		})
	}
}

// zip ですらないものを送られても、保管先に何も残さない。
func TestUploadRejectsNonZip(t *testing.T) {
	t.Parallel()

	h, dir := newHandler(t)
	rec := post(t, h, []byte("これは zip ではない"))

	if rec.Code == http.StatusOK {
		t.Fatal("受理された")
	}
	assertEmpty(t, dir)
}

func TestUploadRejectsGet(t *testing.T) {
	t.Parallel()

	h, _ := newHandler(t)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/upload/backup", nil))

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("GET が通った: %d", rec.Code)
	}
}

// 返すのは JSON。画面がそのまま読めるようにしておく。
func TestUploadRepliesJSON(t *testing.T) {
	t.Parallel()

	h, _ := newHandler(t)
	rec := post(t, h, zipBytes(t, map[string]string{"data/world/level.dat": "nbt"}))

	if got := rec.Header().Get("Content-Type"); got != "application/json; charset=utf-8" {
		t.Errorf("Content-Type が %q", got)
	}
}
