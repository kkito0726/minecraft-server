/*
Package download は保管済みアーカイブの受け渡し。

ブラウザのダウンロードはリンクを辿るだけなので、Authorization ヘッダーを
付けられない。そこで、認証済みの RPC で短命の受取券を発行し、受け取りの口は
その券だけを見る。

`ADMIN_TOKEN` を URL に載せる方法もあるが、長期の鍵がブラウザの履歴・
共有した画面・前段のプロキシの記録に残る。券なら残っても短時間で無効になる。
*/
package download

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"sync"
	"time"
)

/*
TicketTTL は受取券の有効期間。

1 回限りにはしない。ブラウザは中断した転送を Range 要求で再開し、その際に
同じ URL をもう一度叩く。1 回限りだと 200MB の転送が切れた瞬間に再開できず、
最初からやり直すことになる。期限を短く保つことで漏れたときの窓を狭める。
*/
const TicketTTL = 10 * time.Minute

// maxTickets は同時に持つ券の上限。押しただけで取りに来ない券が積もるのを防ぐ。
const maxTickets = 64

// Tickets は発行済みの受取券。プロセスのメモリにだけ置く。
//
// 再起動で全部無効になるが、押し直せば済む。ディスクに残すと、
// 停止中のプロセスの券が生き続けることになる。
type Tickets struct {
	// basePath は受け取りの口のパス。発行する URL の組み立てに使う。
	basePath string

	mu     sync.Mutex
	issued map[string]ticket
	// now は現在時刻。試験で差し替える。
	now func() time.Time
}

type ticket struct {
	backupID  string
	expiresAt time.Time
}

// NewTickets は受取券の管理を作る。
func NewTickets(basePath string) *Tickets {
	return &Tickets{
		basePath: basePath,
		issued:   map[string]ticket{},
		now:      time.Now,
	}
}

// Issue は受取券を発行し、受け取りの URL と期限を返す。
func (t *Tickets) Issue(backupID string) (url string, expiresAt time.Time, err error) {
	token, err := newToken()
	if err != nil {
		return "", time.Time{}, err
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	t.purgeExpired()
	t.enforceCap()
	expiresAt = t.now().Add(TicketTTL)
	t.issued[token] = ticket{backupID: backupID, expiresAt: expiresAt}

	return fmt.Sprintf("%s?%s=%s", t.basePath, TokenParam, token), expiresAt, nil
}

// Redeem は券を照合し、対象のバックアップを返す。
//
// 期限内なら何度でも通す。Range による再開で同じ券が再び来るため。
func (t *Tickets) Redeem(token string) (backupID string, ok bool) {
	if token == "" {
		return "", false
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	// 期限切れの破棄だけを行う。上限の強制まで一緒に走らせると、
	// 飽和状態では引き換えのたびに期限内の券が 1 枚消える。
	t.purgeExpired()
	found, ok := t.issued[token]
	if !ok {
		return "", false
	}
	return found.backupID, true
}

// purgeExpired は期限切れを捨てる。呼び出し側がロックを持っていること。
func (t *Tickets) purgeExpired() {
	now := t.now()
	for token, issued := range t.issued {
		if !issued.expiresAt.After(now) {
			delete(t.issued, token)
		}
	}
}

/*
enforceCap は上限を超えるぶんを古い順に捨てる。

**発行からだけ呼ぶこと。** 券が増えるのは発行のときだけなので、
上限の強制もそこに閉じる。引き換えから呼ぶと、飽和状態では毎回
必ず 1 枚が追い出され、期限内の券が引き換えの瞬間に消える。

呼び出し側がロックを持っていること。
*/
func (t *Tickets) enforceCap() {
	for len(t.issued) >= maxTickets {
		oldest := ""
		for token, issued := range t.issued {
			if oldest == "" || issued.expiresAt.Before(t.issued[oldest].expiresAt) {
				oldest = token
			}
		}
		delete(t.issued, oldest)
	}
}

// newToken は推測できない券の値を作る。
func newToken() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("受取券を作れません: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}
