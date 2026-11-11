// Package scanner runs bounded, read-only Lite network discovery and health checks.
package scanner

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	"dfipam/api/internal/platform/db"
)

// manualGap is the least time between two scans started by hand, so repeated
// clicks can't keep the network busy.
const manualGap = time.Minute

var (
	ErrBusy    = errors.New("a scan is already running")
	ErrTooSoon = errors.New("a scan was started less than a minute ago")
)

// SegmentResult is how one segment's last sweep went.
type SegmentResult struct {
	ID         string    `json:"id"`
	Name       string    `json:"name"`
	Found      int       `json:"found"`
	Error      string    `json:"error,omitempty"`
	DurationMs int64     `json:"durationMs"`
	ScannedAt  time.Time `json:"scannedAt"`
}

// Status is the scanner's state since the backend started. It lives in memory:
// the evidence itself is in sightings.
type Status struct {
	Running    bool            `json:"running"`
	Trigger    string          `json:"trigger"`
	StartedAt  *time.Time      `json:"startedAt"`
	FinishedAt *time.Time      `json:"finishedAt"`
	NextScanAt *time.Time      `json:"nextScanAt"`
	Segments   []SegmentResult `json:"segments"`
}

type request struct{ segmentID, by string }

type Scanner struct {
	conn       db.Conn
	trigger    chan request
	mu         sync.Mutex
	status     Status
	lastManual time.Time
	fullMu     sync.Mutex
	lastFull   map[string]fullMark // segment and IP → last full identification
}

type fullMark struct {
	at    time.Time
	ports bool // whether that identification checked ports
}

// dueFull reports whether a host should be identified in full now, and if so
// records that it was. It is due on a manual scan, after fullEvery, or when its
// segment has just started checking ports.
func (s *Scanner) dueFull(seg segment, ip string, manual bool) bool {
	key := seg.id + "|" + ip
	s.fullMu.Lock()
	defer s.fullMu.Unlock()
	last, seen := s.lastFull[key]
	if seen && !manual && time.Since(last.at) < fullEvery && (last.ports || !seg.checkPorts) {
		return false
	}
	s.lastFull[key] = fullMark{at: time.Now(), ports: seg.checkPorts}
	return true
}

func New(conn db.Conn) *Scanner {
	return &Scanner{conn: conn, trigger: make(chan request, 1), status: Status{Segments: []SegmentResult{}}, lastFull: map[string]fullMark{}}
}

// Run starts discovery and monitored-IP checks. It returns when ctx is cancelled.
func (s *Scanner) Run(ctx context.Context) {
	go s.scanLoop(ctx)
	go s.checkLoop(ctx)
	<-ctx.Done()
}

// Trigger asks for a scan now, of one segment or of all when segmentID is empty.
func (s *Scanner) Trigger(segmentID, by string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.status.Running {
		return ErrBusy
	}
	if time.Since(s.lastManual) < manualGap {
		return ErrTooSoon
	}
	select {
	case s.trigger <- request{segmentID, by}:
		s.lastManual = time.Now()
		return nil
	default:
		return ErrBusy
	}
}

// Snapshot returns a copy of the current status.
func (s *Scanner) Snapshot() Status {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := s.status
	out.Segments = append([]SegmentResult(nil), s.status.Segments...)
	return out
}

func (s *Scanner) scanLoop(ctx context.Context) {
	next := request{by: "startup"}
	if !s.waitForStartup(ctx, &next) {
		return
	}
	for {
		if err := s.scanAll(ctx, next); err != nil && ctx.Err() == nil {
			slog.ErrorContext(ctx, "network scan failed", "error", err)
		}
		wait := interval(ctx, s.conn, "scan.interval_minutes", 5*time.Minute)
		at := time.Now().Add(wait)
		s.mu.Lock()
		s.status.NextScanAt = &at
		s.mu.Unlock()
		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
			next = request{by: "schedule"}
		case next = <-s.trigger:
			timer.Stop()
		}
	}
}

// waitForStartup holds the first scan after a restart until the interval since
// the last sweep has passed, so restarting the backend doesn't scan the network
// again straight away. A manual scan still starts at once. It reports false when
// ctx ends first.
func (s *Scanner) waitForStartup(ctx context.Context, next *request) bool {
	var last *time.Time
	if err := s.conn.QueryRow(ctx, `SELECT max(last_seen) FROM sightings WHERE source IN ('ping', 'arp', 'netbios')`).Scan(&last); err != nil || last == nil {
		return true
	}
	wait := time.Until(last.Add(interval(ctx, s.conn, "scan.interval_minutes", 5*time.Minute)))
	if wait <= 0 {
		return true
	}
	at := time.Now().Add(wait)
	s.mu.Lock()
	s.status.NextScanAt = &at
	s.mu.Unlock()
	timer := time.NewTimer(wait)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		*next = request{by: "schedule"}
	case *next = <-s.trigger:
	}
	return true
}

func (s *Scanner) checkLoop(ctx context.Context) {
	for {
		if err := s.checkAll(ctx); err != nil && ctx.Err() == nil {
			slog.ErrorContext(ctx, "server checks failed", "error", err)
		}
		timer := time.NewTimer(interval(ctx, s.conn, "server_check.interval_seconds", time.Minute))
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
}

func interval(ctx context.Context, conn db.Conn, key string, fallback time.Duration) time.Duration {
	var n int
	if err := conn.QueryRow(ctx, `SELECT (value #>> '{}')::integer FROM settings WHERE key=$1`, key).Scan(&n); err != nil || n <= 0 {
		return fallback
	}
	if key == "scan.interval_minutes" {
		return time.Duration(n) * time.Minute
	}
	return time.Duration(n) * time.Second
}
