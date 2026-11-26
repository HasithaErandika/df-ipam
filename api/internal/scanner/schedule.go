package scanner

import (
	"context"
	"log/slog"
	"slices"
	"strings"
	"time"

	"dfipam/api/internal/platform/db"
)

// schedules are the kinds of segment scanned on their own interval, each with
// the switch that turns its scheduled scans off. Servers come first, so when
// both are due the servers are scanned first.
var schedules = []struct {
	kind, intervalKey, switchKey string
	fallback                     time.Duration
}{
	{"server", "scan.server_interval_minutes", "scan.auto_servers", DefaultServerScanInterval},
	{"dhcp", "scan.interval_minutes", "scan.auto_devices", DefaultScanInterval},
}

func (s *Scanner) scanLoop(ctx context.Context) {
	s.loadFullMarks(ctx)
	// After a restart each kind waits out its interval since it was last swept,
	// so restarting the backend doesn't scan the network again straight away.
	last := s.lastSweeps(ctx)
	for {
		req, ok := s.nextRequest(ctx, last)
		if !ok {
			return
		}
		started := time.Now()
		if err := s.scanAll(ctx, req); err != nil && ctx.Err() == nil {
			slog.ErrorContext(ctx, "network scan failed", "error", err)
		}
		for _, k := range req.kinds {
			last[k] = started
		}
	}
}

// nextRequest waits for the next scan: a kind of segment falling due, or a
// manual scan. It reports false when ctx ends.
func (s *Scanner) nextRequest(ctx context.Context, last map[string]time.Time) (request, bool) {
	for {
		due, wait := s.dueKinds(ctx, last)
		if len(due) > 0 {
			return request{by: "schedule", kinds: due}, true
		}
		var at *time.Time
		if wait > 0 {
			t := time.Now().Add(wait)
			at = &t
		} else {
			wait = time.Minute // nothing is scheduled: look at the switches again in a minute
		}
		s.mu.Lock()
		s.status.NextScanAt = at
		s.mu.Unlock()
		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			timer.Stop()
			return request{}, false
		case <-timer.C:
		case req := <-s.trigger:
			timer.Stop()
			if req.segmentID == "" { // a manual scan of everything counts for both schedules
				req.kinds = []string{"server", "dhcp"}
			}
			return req, true
		}
	}
}

// dueKinds lists the kinds whose interval has passed, and otherwise how long
// until the next one is due (zero when every scheduled scan is switched off).
func (s *Scanner) dueKinds(ctx context.Context, last map[string]time.Time) ([]string, time.Duration) {
	var due []string
	var wait time.Duration
	for _, x := range schedules {
		if !enabled(ctx, s.conn, x.switchKey) {
			continue
		}
		left := time.Until(last[x.kind].Add(interval(ctx, s.conn, x.intervalKey, x.fallback)))
		switch {
		case left <= 0:
			due = append(due, x.kind)
		case wait == 0 || left < wait:
			wait = left
		}
	}
	return due, wait
}

// lastSweeps reads when each kind of segment last finished a sweep. Sightings
// can't tell: the FortiGate reader writes ARP rows too. Until the database
// answers it keeps trying: an empty answer would start a full sweep straight
// after a restart, which is what waiting out the interval is meant to avoid.
func (s *Scanner) lastSweeps(ctx context.Context) map[string]time.Time {
	for {
		out, err := s.readLastSweeps(ctx)
		if err == nil {
			return out
		}
		slog.WarnContext(ctx, "can't read when segments were last swept; trying again", "error", err)
		select {
		case <-ctx.Done():
			return map[string]time.Time{}
		case <-time.After(10 * time.Second):
		}
	}
}

func (s *Scanner) readLastSweeps(ctx context.Context) (map[string]time.Time, error) {
	rows, err := s.conn.Query(ctx, `SELECT kind, max(last_swept_at) FROM segments
		WHERE last_swept_at IS NOT NULL GROUP BY kind`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]time.Time{}
	for rows.Next() {
		var kind string
		var at time.Time
		if err := rows.Scan(&kind, &at); err != nil {
			return nil, err
		}
		out[kind] = at
	}
	return out, rows.Err()
}

// enabled reads an on/off setting. Missing or unreadable means on.
func enabled(ctx context.Context, conn db.Conn, key string) bool {
	on := true
	_ = conn.QueryRow(ctx, `SELECT (value #>> '{}')::boolean FROM settings WHERE key = $1`, key).Scan(&on)
	return on
}

func interval(ctx context.Context, conn db.Conn, key string, fallback time.Duration) time.Duration {
	var n int
	if err := conn.QueryRow(ctx, `SELECT (value #>> '{}')::integer FROM settings WHERE key=$1`, key).Scan(&n); err != nil || n <= 0 {
		return fallback
	}
	switch {
	case strings.HasSuffix(key, "_minutes"):
		return time.Duration(n) * time.Minute
	case strings.HasSuffix(key, "_hours"):
		return time.Duration(n) * time.Hour
	}
	return time.Duration(n) * time.Second
}

// number reads a whole-number setting, or the fallback when it is missing.
func number(ctx context.Context, conn db.Conn, key string, fallback int) int {
	n := fallback
	if err := conn.QueryRow(ctx, `SELECT (value #>> '{}')::integer FROM settings WHERE key = $1`, key).Scan(&n); err != nil || n <= 0 {
		return fallback
	}
	return n
}

// only keeps the segments of the given kinds.
func only(all []segment, kinds []string) []segment {
	out := all[:0]
	for _, x := range all {
		if slices.Contains(kinds, x.kind) {
			out = append(out, x)
		}
	}
	return out
}
