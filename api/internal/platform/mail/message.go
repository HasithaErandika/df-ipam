package mail

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"mime"
	"strings"
	"time"
)

// Letter is one email: a plain-text body for every client, and HTML for those
// that show it. HTML may be empty.
type Letter struct {
	To, Subject, Text, HTML string
}

// clean removes line breaks, so a value can't add mail headers.
func clean(s string) string {
	return strings.NewReplacer("\r", " ", "\n", " ").Replace(s)
}

// Message builds an RFC 5322 message: multipart/alternative when there is
// HTML, plain text otherwise. Bodies are base64, so no line is too long and no
// leading dot needs stuffing; a non-ASCII subject is encoded.
func Message(from string, m Letter, now time.Time) []byte {
	var b strings.Builder
	fmt.Fprintf(&b, "From: %s\r\n", clean(from))
	fmt.Fprintf(&b, "To: %s\r\n", clean(m.To))
	fmt.Fprintf(&b, "Subject: %s\r\n", mime.QEncoding.Encode("utf-8", clean(m.Subject)))
	fmt.Fprintf(&b, "Date: %s\r\n", now.UTC().Format(time.RFC1123Z))
	b.WriteString("MIME-Version: 1.0\r\nAuto-Submitted: auto-generated\r\n")
	if m.HTML == "" {
		part(&b, "text/plain", m.Text)
		return []byte(b.String())
	}
	boundary := newBoundary()
	fmt.Fprintf(&b, "Content-Type: multipart/alternative; boundary=%q\r\n\r\n", boundary)
	fmt.Fprintf(&b, "--%s\r\n", boundary)
	part(&b, "text/plain", m.Text)
	fmt.Fprintf(&b, "--%s\r\n", boundary)
	part(&b, "text/html", m.HTML)
	fmt.Fprintf(&b, "--%s--\r\n", boundary)
	return []byte(b.String())
}

// part writes one body with its headers, base64 in 76-character lines.
func part(b *strings.Builder, contentType, body string) {
	fmt.Fprintf(b, "Content-Type: %s; charset=utf-8\r\nContent-Transfer-Encoding: base64\r\n\r\n", contentType)
	enc := base64.StdEncoding.EncodeToString([]byte(strings.ReplaceAll(body, "\r\n", "\n")))
	for len(enc) > 76 {
		b.WriteString(enc[:76] + "\r\n")
		enc = enc[76:]
	}
	b.WriteString(enc + "\r\n")
}

func newBoundary() string {
	var r [12]byte
	_, _ = rand.Read(r[:])
	return "dfipam-" + hex.EncodeToString(r[:])
}
