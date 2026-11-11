// Package scanner runs bounded, read-only Lite network discovery and health checks.
package scanner

import (
	"context"
	"log/slog"
	"time"

	"dfipam/api/internal/platform/db"
)

type Scanner struct{ conn db.Conn }

func New(conn db.Conn) *Scanner { return &Scanner{conn: conn} }

// Run starts discovery and monitored-IP checks. It returns when ctx is cancelled.
func (s *Scanner) Run(ctx context.Context) {
	go s.loop(ctx, "network scan", "scan.interval_minutes", 5*time.Minute, s.scanAll)
	go s.loop(ctx, "server checks", "server_check.interval_seconds", time.Minute, s.checkAll)
	<-ctx.Done()
}

func (s *Scanner) loop(ctx context.Context, name, setting string, fallback time.Duration, run func(context.Context) error) {
	for {
		if err := run(ctx); err != nil && ctx.Err() == nil {
			slog.ErrorContext(ctx, name+" failed", "error", err)
		}
		wait := interval(ctx, s.conn, setting, fallback)
		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
}

func interval(ctx context.Context, conn db.Conn, key string, fallback time.Duration) time.Duration {
	var seconds int
	if err := conn.QueryRow(ctx, `SELECT (value #>> '{}')::integer FROM settings WHERE key=$1`, key).Scan(&seconds); err != nil || seconds <= 0 {
		return fallback
	}
	if key == "scan.interval_minutes" {
		return time.Duration(seconds) * time.Minute
	}
	return time.Duration(seconds) * time.Second
}
