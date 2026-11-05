package mail

import (
	"strings"
	"testing"
	"time"
)

func TestMessageBlocksHeaderInjection(t *testing.T) {
	msg := string(Message("ipam@example.com", "a@example.com", "Hi\r\nBcc: evil@example.com", "line one\nline two", time.Unix(0, 0)))
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
	if body != "line one\r\nline two\r\n" {
		t.Fatalf("body: %q", body)
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
