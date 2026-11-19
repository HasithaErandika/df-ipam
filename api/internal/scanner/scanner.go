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
	Stopped    bool            `json:"stopped"` // the last scan was stopped by hand
	Trigger    string          `json:"trigger"`
	StartedAt  *time.Time      `json:"startedAt"`
	FinishedAt *time.Time      `json:"finishedAt"`
	NextScanAt *time.Time      `json:"nextScanAt"`
	Segments   []SegmentResult `json:"segments"`
}

// request is one scan: of one segment, of the kinds of segment due on the
// schedule, or of everything (a manual scan with no segment).
type request struct {
	segmentID, by string
	kinds         []string
}

type Scanner struct {
	conn       db.Conn
	trigger    chan request
	mu         sync.Mutex
	status     Status
	lastManual time.Time
	cancel     context.CancelFunc // ends the running scan; nil when none runs
	fullMu     sync.Mutex
	lastFull   map[string]fullMark  // segment and IP → last full identification
	lastSilent map[string]time.Time // segment → last sweep that asked silent addresses too
	portGap    time.Duration        // how often ports are checked; set at each scan
	pace       *pace                // shared by sweeps and server checks
}

func New(conn db.Conn) *Scanner {
	return &Scanner{conn: conn, trigger: make(chan request, 1), status: Status{Segments: []SegmentResult{}}, lastFull: map[string]fullMark{},
		lastSilent: map[string]time.Time{}, pace: newPace(DefaultRate)}
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
	case s.trigger <- request{segmentID: segmentID, by: by}:
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

func (s *Scanner) checkLoop(ctx context.Context) {
	for {
		if !enabled(ctx, s.conn, "server_check.enabled") {
			slog.DebugContext(ctx, "automatic server checks are off")
		} else if err := s.checkAll(ctx); err != nil && ctx.Err() == nil {
			slog.ErrorContext(ctx, "server checks failed", "error", err)
		}
		timer := time.NewTimer(interval(ctx, s.conn, "server_check.interval_seconds", defaultCheckInterval))
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
}

// PacedRate is the current probe rate, for the status line.
func (s *Scanner) PacedRate() int { return s.pace.perSecond() }
