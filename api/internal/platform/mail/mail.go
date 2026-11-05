// Package mail sends plain-text email over SMTP. It needs only the standard
// library, and refuses to send credentials over an unencrypted connection.
package mail

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/mail"
	"net/smtp"
	"os"
	"strconv"
	"strings"
	"time"
)

// Mailer sends one message.
type Mailer interface {
	Send(ctx context.Context, to, subject, body string) error
}

// Config is the SMTP connection. Port 465 uses implicit TLS; any other port
// must offer STARTTLS.
type Config struct {
	Host, Username, Password, From string
	Port                           int
}

// FromEnv reads SMTP_HOST, SMTP_PORT, SMTP_USERNAME, SMTP_PASSWORD and MAIL_FROM.
// It returns ok=false when SMTP_HOST is not set, meaning email is off.
func FromEnv() (Config, bool, error) {
	c := Config{
		Host:     strings.TrimSpace(os.Getenv("SMTP_HOST")),
		Username: os.Getenv("SMTP_USERNAME"),
		Password: os.Getenv("SMTP_PASSWORD"),
		From:     strings.TrimSpace(os.Getenv("MAIL_FROM")),
		Port:     587,
	}
	if c.Host == "" {
		return c, false, nil
	}
	if v := strings.TrimSpace(os.Getenv("SMTP_PORT")); v != "" {
		p, err := strconv.Atoi(v)
		if err != nil || p < 1 || p > 65535 {
			return c, false, errors.New("SMTP_PORT must be a port number")
		}
		c.Port = p
	}
	if _, err := mail.ParseAddress(c.From); err != nil {
		return c, false, errors.New("MAIL_FROM must be an email address")
	}
	return c, true, nil
}

type SMTP struct{ cfg Config }

func NewSMTP(cfg Config) *SMTP { return &SMTP{cfg} }

// clean removes line breaks, so a value can't add mail headers.
func clean(s string) string {
	return strings.NewReplacer("\r", " ", "\n", " ").Replace(s)
}

// Message builds an RFC 5322 plain-text message.
func Message(from, to, subject, body string, now time.Time) []byte {
	var b strings.Builder
	fmt.Fprintf(&b, "From: %s\r\n", clean(from))
	fmt.Fprintf(&b, "To: %s\r\n", clean(to))
	fmt.Fprintf(&b, "Subject: %s\r\n", clean(subject))
	fmt.Fprintf(&b, "Date: %s\r\n", now.UTC().Format(time.RFC1123Z))
	b.WriteString("MIME-Version: 1.0\r\nContent-Type: text/plain; charset=utf-8\r\nContent-Transfer-Encoding: 8bit\r\n\r\n")
	for _, line := range strings.Split(strings.ReplaceAll(body, "\r\n", "\n"), "\n") {
		if strings.HasPrefix(line, ".") {
			line = "." + line // dot-stuffing is done by net/smtp, but keep lines safe either way
		}
		b.WriteString(line + "\r\n")
	}
	return []byte(b.String())
}

func (s *SMTP) Send(ctx context.Context, to, subject, body string) error {
	if _, err := mail.ParseAddress(to); err != nil {
		return fmt.Errorf("invalid recipient")
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	tlsConfig := &tls.Config{ServerName: s.cfg.Host, MinVersion: tls.VersionTLS12}
	client, err := s.connect(ctx, tlsConfig)
	if err != nil {
		return err
	}
	defer client.Close()
	if err := s.secure(client, tlsConfig); err != nil {
		return err
	}
	return s.deliver(client, to, subject, body)
}

// connect opens the connection and reads the greeting. Port 465 starts in TLS.
func (s *SMTP) connect(ctx context.Context, tlsConfig *tls.Config) (*smtp.Client, error) {
	addr := net.JoinHostPort(s.cfg.Host, strconv.Itoa(s.cfg.Port))
	dialer := &net.Dialer{}
	var conn net.Conn
	var err error
	if s.cfg.Port == 465 {
		conn, err = (&tls.Dialer{NetDialer: dialer, Config: tlsConfig}).DialContext(ctx, "tcp", addr)
	} else {
		conn, err = dialer.DialContext(ctx, "tcp", addr)
	}
	if err != nil {
		return nil, fmt.Errorf("connect to mail server: %w", err)
	}
	if deadline, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(deadline)
	}
	client, err := smtp.NewClient(conn, s.cfg.Host)
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("mail server greeting: %w", err)
	}
	return client, nil
}

func (s *SMTP) secure(client *smtp.Client, tlsConfig *tls.Config) error {
	if s.cfg.Port != 465 {
		if ok, _ := client.Extension("STARTTLS"); !ok {
			return errors.New("mail server does not offer STARTTLS")
		}
		if err := client.StartTLS(tlsConfig); err != nil {
			return fmt.Errorf("starttls: %w", err)
		}
	}
	if s.cfg.Username == "" {
		return nil
	}
	if err := client.Auth(smtp.PlainAuth("", s.cfg.Username, s.cfg.Password, s.cfg.Host)); err != nil {
		return fmt.Errorf("mail server sign-in failed")
	}
	return nil
}

func (s *SMTP) deliver(client *smtp.Client, to, subject, body string) error {
	if err := client.Mail(s.cfg.From); err != nil {
		return fmt.Errorf("mail from: %w", err)
	}
	if err := client.Rcpt(to); err != nil {
		return fmt.Errorf("recipient refused")
	}
	w, err := client.Data()
	if err != nil {
		return fmt.Errorf("data: %w", err)
	}
	if _, err := w.Write(Message(s.cfg.From, to, subject, body, time.Now())); err != nil {
		return err
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("message refused: %w", err)
	}
	return client.Quit()
}
