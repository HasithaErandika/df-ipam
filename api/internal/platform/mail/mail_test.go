package mail

import (
	"encoding/base64"
	"strings"
	"testing"
	"time"
)

func TestMessageBlocksHeaderInjection(t *testing.T) {
	msg := string(Message("ipam@example.com", Letter{To: "a@example.com", Subject: "Hi\r\nBcc: evil@example.com", Text: "line one\nline two"}, time.Unix(0, 0)))
	head, body, ok := strings.Cut(msg, "\r\n\r\n")
	if !ok {
		t.Fatal("no header/body separator")
	}
	if strings.Contains(head, "\r\nBcc:") {
		t.Fatalf("injected header: %q", head)
	}
	if !strings.Contains(head, "Subject: Hi  Bcc: evil@example.com") {
		t.Fatalf("subject not kept on one line: %q", head)
	}
	got, err := base64.StdEncoding.DecodeString(strings.ReplaceAll(strings.TrimSpace(body), "\r\n", ""))
	if err != nil || string(got) != "line one\nline two" {
		t.Fatalf("body: %q (%v)", got, err)
	}
}

// With HTML the message carries both versions, and a non-ASCII subject is encoded.
func TestMessageWithHTML(t *testing.T) {
	msg := string(Message("ipam@example.com", Letter{To: "a@example.com", Subject: "Server – down", Text: "plain", HTML: "<p>rich</p>"}, time.Unix(0, 0)))
	for _, want := range []string{"multipart/alternative", "Content-Type: text/plain; charset=utf-8", "Content-Type: text/html; charset=utf-8", "Subject: =?utf-8?q?"} {
		if !strings.Contains(msg, want) {
			t.Errorf("missing %q in\n%s", want, msg)
		}
	}
}

func TestFromEnv(t *testing.T) {
	t.Setenv("SMTP_HOST", "")
	if _, ok, err := FromEnv(); ok || err != nil {
		t.Fatalf("unset host should mean off: %v %v", ok, err)
	}
	t.Setenv("SMTP_HOST", "smtp.example.com")
	t.Setenv("MAIL_FROM", "not-an-address")
	if _, _, err := FromEnv(); err == nil {
		t.Fatal("bad MAIL_FROM accepted")
	}
	t.Setenv("MAIL_FROM", "DF-IPAM <ipam@example.com>")
	t.Setenv("SMTP_PORT", "70000")
	if _, _, err := FromEnv(); err == nil {
		t.Fatal("bad port accepted")
	}
	t.Setenv("SMTP_PORT", "465")
	c, ok, err := FromEnv()
	if !ok || err != nil || c.Port != 465 {
		t.Fatalf("got %+v %v %v", c, ok, err)
	}
}
