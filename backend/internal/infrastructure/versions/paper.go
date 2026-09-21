// Package versions はワールドを作れる版の一覧を Paper の API から取る。
//
// 一覧を持つ理由は、Paper に存在しない版を MC_VERSION に書くとサーバーが
// 起動しないため。書式が正しくても存在するとは限らない（例: 26.1.1 と
// 26.1.2 はあるが 26.1 は無い）。
//
// 取りに行くのはバックエンドから。ブラウザから直接だと CORS と防御ヘッダに
// 阻まれる。サーバーの jar の取得に Pi の外向き通信はもともと要るので、
// ここで新たな依存が増えるわけではない。
package versions

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

// DefaultURL は Paper の Fill API（v3）。v2 は 2026 年時点で廃止済み（410）。
const DefaultURL = "https://fill.papermc.io/v3/projects/paper"

const (
	// requestTimeout は 1 回の問い合わせの上限。ダイアログを開くたびに
	// 待たされるので短くする。オフラインならすぐ諦めて現在の版だけを出す。
	requestTimeout = 5 * time.Second
	// cacheTTL は一覧を持ち回る時間。新しい版は数週間に一度しか出ない。
	cacheTTL = time.Hour
	// maxBody は応答の上限。一覧は数 KB で、それを大きく超えるなら異常。
	maxBody = 1 << 20
	// userAgent は PaperMC の求めに従って連絡先を含める。
	userAgent = "mcadmind (https://github.com/kkito0726/minecraft-server)"
)

// ErrEmpty は一覧が空だったことを表す。
var ErrEmpty = errors.New("Paper の版の一覧が空でした")

// Paper は port.VersionCatalog の実装。
type Paper struct {
	url    string
	client *http.Client
	now    func() time.Time

	mu       sync.Mutex
	cached   []string
	cachedAt time.Time
}

// NewPaper は Paper を作る。url が空なら DefaultURL を使う。
func NewPaper(url string, client *http.Client) *Paper {
	if url == "" {
		url = DefaultURL
	}
	if client == nil {
		client = &http.Client{Timeout: requestTimeout}
	}
	return &Paper{url: url, client: client, now: time.Now}
}

// StableVersions は安定版を新しい順に返す。
//
// 取れた一覧は一定時間持ち回る。取れなかったときは持ち回らない。
// 次に開いたときに、繋がるようになっていればすぐ出せるようにするため。
func (p *Paper) StableVersions(ctx context.Context) ([]string, error) {
	p.mu.Lock()
	if p.cached != nil && p.now().Sub(p.cachedAt) < cacheTTL {
		out := append([]string(nil), p.cached...)
		p.mu.Unlock()
		return out, nil
	}
	p.mu.Unlock()

	fetched, err := p.fetch(ctx)
	if err != nil {
		return nil, err
	}

	p.mu.Lock()
	p.cached, p.cachedAt = fetched, p.now()
	p.mu.Unlock()
	return append([]string(nil), fetched...), nil
}

func (p *Paper) fetch(ctx context.Context) ([]string, error) {
	ctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.url, http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("版の一覧の問い合わせを作れません: %w", err)
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "application/json")

	res, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("Paper に繋がりません: %w", err)
	}
	// 閉じるのに失敗しても一覧の正しさには関わらない。読み終えた本文は
	// もう手元にある。
	defer func() { _ = res.Body.Close() }()

	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Paper が %d を返しました", res.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(res.Body, maxBody))
	if err != nil {
		return nil, fmt.Errorf("Paper の応答を読めません: %w", err)
	}
	return parse(body)
}

// project は Fill API の応答のうち、使う部分だけ。
//
// versions は「系列 → その系列の版（新しい順）」の対応で、系列も新しい順に
// 並んでいる。JSON のオブジェクトは順序を持たないので、順序を保って読む。
type project struct {
	Versions orderedVersions `json:"versions"`
}

// parse は応答から安定版を新しい順に取り出す。
func parse(body []byte) ([]string, error) {
	var p project
	if err := json.Unmarshal(body, &p); err != nil {
		return nil, fmt.Errorf("Paper の応答を解釈できません: %w", err)
	}

	var out []string
	for _, group := range p.Versions {
		for _, v := range group {
			if isStable(v) {
				out = append(out, v)
			}
		}
	}
	if len(out) == 0 {
		return nil, ErrEmpty
	}
	return out, nil
}

// isStable はプレリリースでないかを返す。
//
// Paper は 26.3-rc-3 や 1.21.11-pre5 のような試作版も並べる。ワールドを
// 作る先としては勧めないので、ハイフンを含むものを除く。
func isStable(v string) bool {
	return v != "" && !strings.Contains(v, "-")
}
