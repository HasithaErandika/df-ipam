package server

import (
	"context"
	"strings"
	"sync"
	"testing"

	"dfipam/api/internal/modules/notifications"
	"dfipam/api/internal/notify"
	"dfipam/api/internal/platform/mail"
)

// fakeMail keeps what would have been sent.
type fakeMail struct {
	mu   sync.Mutex
	sent []mail.Letter
}

func (f *fakeMail) Send(_ context.Context, l mail.Letter) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sent = append(f.sent, l)
	return nil
}

func (f *fakeMail) take() []mail.Letter {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := f.sent
	f.sent = nil
	return out
}

// Critical goes at once, High waits for the digest, a team mailbox takes the
// team's email once, and the daily summary goes once a day.
func TestEmailDelivery(t *testing.T) {
	e := newAPI(t)
	set := func(key, value string) {
		t.Helper()
		if _, err := e.tx.Exec(e.ctx, `INSERT INTO settings (key, value) VALUES ($1, $2::jsonb)
			ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value`, key, value); err != nil {
			t.Fatal(err)
		}
	}
	set("email.enabled", "true")
	set("email.summary_hour", "23") // no summary until the last step
	box := &fakeMail{}
	out := notifications.NewOutbox(e.tx, box, "https://ipam.example", nil)
	tick := func() []mail.Letter {
		t.Helper()
		if err := out.Tick(e.ctx); err != nil {
			t.Fatal(err)
		}
		return box.take()
	}
	holders := func(sev string, d notify.Delivery) {
		t.Helper()
		if err := notify.Holders(e.ctx, e.tx, "alerts:manage", "alert", "Test "+sev+" alert", "/alerts", sev, d); err != nil {
			t.Fatal(err)
		}
	}

	holders("critical", notify.Now)
	if got := tick(); len(got) != 1 || got[0].To != "admin@example.com" || !strings.Contains(got[0].Subject, "Critical alert") {
		t.Fatalf("critical: %+v", got)
	}

	holders("high", notify.Digest)
	if got := tick(); len(got) != 0 {
		t.Fatalf("digest sent before it was due: %+v", got)
	}
	if _, err := e.tx.Exec(e.ctx, `UPDATE notifications SET created_at = now() - interval '1 hour' WHERE email_mode = 'digest'`); err != nil {
		t.Fatal(err)
	}
	if got := tick(); len(got) != 1 || !strings.Contains(got[0].Subject, "1 new alert") || !strings.Contains(got[0].HTML, "Test high alert") {
		t.Fatalf("digest: %+v", got)
	}

	set("email.team_address", `"netops@example.com"`)
	holders("critical", notify.Now)
	if got := tick(); len(got) != 1 || got[0].To != "netops@example.com" {
		t.Fatalf("team mailbox: %+v", got)
	}

	if _, err := e.tx.Exec(e.ctx, `INSERT INTO alerts (alert_type, severity, message, dedupe_key)
		VALUES ('unknown_device', 'high', 'Unapproved device on Floor 2', 'test:email')`); err != nil {
		t.Fatal(err)
	}
	set("email.summary_hour", "0")
	if got := tick(); len(got) != 1 || got[0].To != "netops@example.com" || !strings.Contains(got[0].Subject, "Daily summary") {
		t.Fatalf("summary: %+v", got)
	}
	if got := tick(); len(got) != 0 {
		t.Fatalf("summary sent twice: %+v", got)
	}
}
