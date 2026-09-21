package versions

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"slices"
	"sync/atomic"
	"testing"
	"time"
)

// 2026-09 時点の実物の応答（先頭の一部）。系列も版も新しい順に並ぶ。
const fillResponse = `{"project":{"id":"paper","name":"Paper"},"versions":{` +
	`"26.3":["26.3","26.3-rc-3"],` +
	`"26.2":["26.2","26.2-rc-2"],` +
	`"26.1":["26.1.2","26.1.1"],` +
	`"1.21":["1.21.11","1.21.11-rc3","1.21.11-pre5","1.21.10","1.21"]}}`

func TestParseKeepsOrderAndDropsPrereleases(t *testing.T) {
	t.Parallel()

	got, err := parse([]byte(fillResponse))
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"26.3", "26.2", "26.1.2", "26.1.1", "1.21.11", "1.21.10", "1.21"}
	if !slices.Equal(got, want) {
		t.Errorf("一覧が %v。%v のはず", got, want)
	}
}

// 26.1 のように、Minecraft に存在しても Paper が配っていない版がある。
// 一覧に無い版を選ばせないのが、一覧を持つ理由そのもの。
func TestParseDoesNotInventVersions(t *testing.T) {
	t.Parallel()

	got, err := parse([]byte(fillResponse))
	if err != nil {
		t.Fatal(err)
	}
	if slices.Contains(got, "26.1") {
		t.Error("Paper に無い 26.1 が一覧に入っている")
	}
}

func TestParseErrors(t *testing.T) {
	t.Parallel()

	for name, body := range map[string]string{
		"JSON でない":       "<html>",
		"versions が配列":   `{"versions":[]}`,
		"安定版が 1 つも無い":    `{"versions":{"26.4":["26.4-rc-1"]}}`,
		"versions の中身が変": `{"versions":{"26.4":"26.4"}}`,
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if _, err := parse([]byte(body)); err == nil {
				t.Error("エラーを期待したが nil")
			}
		})
	}
}

func TestStableVersionsFetchesAndCaches(t *testing.T) {
	t.Parallel()

	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		if r.Header.Get("User-Agent") == "" {
			t.Error("User-Agent が無い。PaperMC は連絡先を求めている")
		}
		_, _ = w.Write([]byte(fillResponse))
	}))
	defer srv.Close()

	now := time.Unix(0, 0)
	p := NewPaper(srv.URL, srv.Client())
	p.now = func() time.Time { return now }

	for range 3 {
		if _, err := p.StableVersions(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	if got := hits.Load(); got != 1 {
		t.Errorf("問い合わせが %d 回。持ち回るはず", got)
	}

	// 期限を過ぎたら取り直す。
	now = now.Add(cacheTTL + time.Second)
	if _, err := p.StableVersions(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := hits.Load(); got != 2 {
		t.Errorf("問い合わせが %d 回。期限後は取り直すはず", got)
	}
}

// 取れなかった結果は持ち回らない。繋がるようになったらすぐ出せるように。
func TestStableVersionsDoesNotCacheFailures(t *testing.T) {
	t.Parallel()

	var fail atomic.Bool
	fail.Store(true)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if fail.Load() {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		_, _ = w.Write([]byte(fillResponse))
	}))
	defer srv.Close()

	p := NewPaper(srv.URL, srv.Client())
	if _, err := p.StableVersions(context.Background()); err == nil {
		t.Fatal("503 でエラーにならない")
	}

	fail.Store(false)
	got, err := p.StableVersions(context.Background())
	if err != nil {
		t.Fatalf("回復後も失敗する: %v", err)
	}
	if len(got) == 0 {
		t.Error("一覧が空")
	}
}

// 廃止された v2 は 410 を返す。版の一覧が取れないことを失敗として伝える。
func TestStableVersionsReportsGone(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusGone)
	}))
	defer srv.Close()

	_, err := NewPaper(srv.URL, srv.Client()).StableVersions(context.Background())
	if err == nil {
		t.Fatal("410 でエラーにならない")
	}
}

// 繋がらないときは失敗を返す（呼ぶ側が「一覧が分からない」として扱う）。
func TestStableVersionsUnreachable(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.NotFoundHandler())
	url := srv.URL
	srv.Close()

	_, err := NewPaper(url, nil).StableVersions(context.Background())
	if err == nil {
		t.Fatal("繋がらないのにエラーにならない")
	}
	if errors.Is(err, ErrEmpty) {
		t.Error("空の一覧と取り違えている")
	}
}

// 返した一覧を書き換えられても、持ち回っている一覧は壊れない。
func TestStableVersionsReturnsCopy(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(fillResponse))
	}))
	defer srv.Close()

	p := NewPaper(srv.URL, srv.Client())
	first, _ := p.StableVersions(context.Background())
	first[0] = "壊した"

	second, _ := p.StableVersions(context.Background())
	if second[0] != "26.3" {
		t.Errorf("持ち回りが書き換わっている: %v", second)
	}
}

func TestNewPaperDefaults(t *testing.T) {
	t.Parallel()

	p := NewPaper("", nil)
	if p.url != DefaultURL {
		t.Errorf("url が %q", p.url)
	}
	if p.client == nil {
		t.Error("client が nil")
	}
}
