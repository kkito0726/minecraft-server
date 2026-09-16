package backupctl_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"testing"
	"time"

	"github.com/kkito0726/minecraft-server/backend/internal/application/port"
	"github.com/kkito0726/minecraft-server/backend/internal/domain/backup"
)

// fakeArchive は開いたアーカイブの偽物。Seek できる形で返す。
type fakeArchive struct{ *bytes.Reader }

func (fakeArchive) Close() error { return nil }

// Open は保管済みのアーカイブを開く。中身は持たないので短い中身を返す。
func (f *fakeStore) Open(_ context.Context, id backup.ID) (port.ArchiveFile, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	entry, ok := f.entries[id.String()]
	if !ok {
		return port.ArchiveFile{}, fmt.Errorf("%w: %s", backup.ErrNotFound, id)
	}
	return port.ArchiveFile{
		Body: fakeArchive{bytes.NewReader([]byte("アーカイブの中身"))},
		Info: entry,
	}, nil
}

func TestExistsReturnsFileName(t *testing.T) {
	t.Parallel()

	h := newHarness(t, true, nil)
	h.store.seed(t, "backup-26.2-world-20260916-0300.zip", time.Now())

	name, err := h.uc.Exists(context.Background(), "backup-26.2-world-20260916-0300.zip")
	if err != nil {
		t.Fatal(err)
	}
	if name != "backup-26.2-world-20260916-0300.zip" {
		t.Errorf("ファイル名が %q", name)
	}
}

// 券を出す前に確かめる。出してしまうと、押した直後ではなく
// ダウンロードの途中で失敗し、理由が分かりにくくなる。
func TestExistsRejectsMissing(t *testing.T) {
	t.Parallel()

	h := newHarness(t, true, nil)

	if _, err := h.uc.Exists(context.Background(), "backup-26.2-world-20260101-0300.zip"); !errors.Is(err, backup.ErrNotFound) {
		t.Fatalf("ErrNotFound を期待したが %v", err)
	}
}

func TestExistsRejectsInvalidID(t *testing.T) {
	t.Parallel()

	h := newHarness(t, true, nil)

	// 保管先の外を指す名前が通ると、任意のファイルを渡せてしまう。
	for _, id := range []string{"", "../../.env", "backup.txt"} {
		if _, err := h.uc.Exists(context.Background(), id); err == nil {
			t.Errorf("%q が通った", id)
		}
	}
}

func TestOpenForDownloadStreamsArchive(t *testing.T) {
	t.Parallel()

	h := newHarness(t, true, nil)
	h.store.seed(t, "backup-26.2-world-20260916-0300.zip", time.Now())

	file, err := h.uc.OpenForDownload(context.Background(), "backup-26.2-world-20260916-0300.zip")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = file.Body.Close() }()

	body, err := io.ReadAll(file.Body)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "アーカイブの中身" {
		t.Errorf("中身が %q", body)
	}
	if file.Info.SizeBytes == 0 {
		t.Error("大きさが伝わっていない")
	}
}
