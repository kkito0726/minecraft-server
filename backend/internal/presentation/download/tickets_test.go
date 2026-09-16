package download

import (
	"strings"
	"testing"
	"time"
)

func newTestTickets(now *time.Time) *Tickets {
	t := NewTickets("/download/backup")
	t.now = func() time.Time { return *now }
	return t
}

func TestIssueAndRedeem(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 9, 16, 12, 0, 0, 0, time.UTC)
	tickets := newTestTickets(&now)

	url, expiresAt, err := tickets.Issue("backup-26.2-world-20260916-0300.zip")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(url, "/download/backup?"+TokenParam+"=") {
		t.Fatalf("受け取りの URL が %q", url)
	}
	if want := now.Add(TicketTTL); !expiresAt.Equal(want) {
		t.Errorf("期限が %v。%v のはず", expiresAt, want)
	}

	token := url[strings.Index(url, "=")+1:]
	id, ok := tickets.Redeem(token)
	if !ok || id != "backup-26.2-world-20260916-0300.zip" {
		t.Errorf("引き換えられない: %q %v", id, ok)
	}
}

/*
1 回限りにはしない。

ブラウザは中断した転送を Range 要求で再開し、その際に同じ URL をもう一度
叩く。1 回限りだと、200MB の転送が切れた瞬間に再開できなくなる。
*/
func TestRedeemIsRepeatableWithinTTL(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 9, 16, 12, 0, 0, 0, time.UTC)
	tickets := newTestTickets(&now)
	token := issueToken(t, tickets, "backup.zip")

	for i := range 3 {
		if _, ok := tickets.Redeem(token); !ok {
			t.Fatalf("%d 回目で引き換えられなくなった", i+1)
		}
	}
}

func TestRedeemRejectsExpired(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 9, 16, 12, 0, 0, 0, time.UTC)
	tickets := newTestTickets(&now)
	token := issueToken(t, tickets, "backup.zip")

	now = now.Add(TicketTTL + time.Second)
	if _, ok := tickets.Redeem(token); ok {
		t.Error("期限切れの券が通った")
	}
}

func TestRedeemRejectsUnknownToken(t *testing.T) {
	t.Parallel()

	now := time.Now()
	tickets := newTestTickets(&now)

	if _, ok := tickets.Redeem(""); ok {
		t.Error("空の券が通った")
	}
	if _, ok := tickets.Redeem("知らない券"); ok {
		t.Error("知らない券が通った")
	}
}

// 券は推測できてはいけない。発行のたびに違う値になること。
func TestTokensAreUnique(t *testing.T) {
	t.Parallel()

	now := time.Now()
	tickets := newTestTickets(&now)

	seen := map[string]bool{}
	for range 16 {
		token := issueToken(t, tickets, "backup.zip")
		if seen[token] {
			t.Fatal("同じ券が 2 度出た")
		}
		seen[token] = true
	}
}

// 押しただけで取りに来ない券が積もらないこと。
func TestOldTicketsAreEvicted(t *testing.T) {
	t.Parallel()

	now := time.Now()
	tickets := newTestTickets(&now)

	for range maxTickets + 10 {
		issueToken(t, tickets, "backup.zip")
	}

	tickets.mu.Lock()
	held := len(tickets.issued)
	tickets.mu.Unlock()
	if held > maxTickets {
		t.Errorf("券が %d 枚たまっている", held)
	}
}

func issueToken(t *testing.T, tickets *Tickets, backupID string) string {
	t.Helper()

	url, _, err := tickets.Issue(backupID)
	if err != nil {
		t.Fatal(err)
	}
	return url[strings.Index(url, "=")+1:]
}
