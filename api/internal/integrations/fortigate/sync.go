package fortigate

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	"dfipam/api/internal/modules/monitoring"
	"dfipam/api/internal/platform/db"
	"dfipam/api/internal/platform/secrets"
)

const manualGap = time.Minute

var (
	ErrBusy    = errors.New("a FortiGate read is already running")
	ErrTooSoon = errors.New("the FortiGate was read less than a minute ago")
)

// Status is how the last read went, kept in memory.
type Status struct {
	Running  bool       `json:"running"`
	LastRun  *time.Time `json:"lastRun"`
	LastOK   *time.Time `json:"lastOk"`
	Error    string     `json:"error"`
	Counts   Counts     `json:"counts"`
	Firewall string     `json:"firewall"`
}

// Syncer reads the FortiGate on a schedule when it is enabled, and on request.
type Syncer struct {
	conn       db.Conn
	keys       *secrets.Keyring
	trigger    chan struct{}
	mu         sync.Mutex
	status     Status
	lastManual time.Time
}

func NewSyncer(conn db.Conn, keys *secrets.Keyring) *Syncer {
	return &Syncer{conn: conn, keys: keys, trigger: make(chan struct{}, 1)}
}

func (s *Syncer) Keys() *secrets.Keyring { return s.keys }

// Connect builds a client from the saved connection, for readers other than
// the scheduled sync (Network Links). It works while the schedule is off, but
// not while the connection is switched off.
func (s *Syncer) Connect(ctx context.Context) (*Client, error) {
	cfg, err := Load(ctx, s.conn, s.keys)
	if err != nil {
		return nil, err
	}
	return cfg.Client()
}

func (s *Syncer) Snapshot() Status {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.status
}

// Trigger asks for a read now, a minute apart at most.
func (s *Syncer) Trigger() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.status.Running {
		return ErrBusy
	}
	if time.Since(s.lastManual) < manualGap {
		return ErrTooSoon
	}
	select {
	case s.trigger <- struct{}{}:
		s.lastManual = time.Now()
		return nil
	default:
		return ErrBusy
	}
}

// Run reads on the configured interval while enabled. It returns when ctx ends.
func (s *Syncer) Run(ctx context.Context) {
	wait := time.Minute
	for {
		manual := false
		select {
		case <-ctx.Done():
			return
		case <-time.After(wait):
		case <-s.trigger:
			manual = true
		}
		cfg, ok := s.due(ctx, manual, &wait)
		if !ok {
			continue
		}
		if err := s.Sync(ctx, cfg); err != nil && ctx.Err() == nil {
			slog.WarnContext(ctx, "fortigate read failed", "error", err)
		}
	}
}

// due loads the settings, sets the next wait from them and says whether to
// read now. A scheduled read needs the schedule on; "Read now" works while it
// is off, so an Admin can try it before enabling the schedule. Neither runs
// while the connection is off (Client refuses, and says so).
func (s *Syncer) due(ctx context.Context, manual bool, wait *time.Duration) (Config, bool) {
	cfg, err := Load(ctx, s.conn, s.keys)
	*wait = time.Minute
	if err == nil && cfg.IntervalMinutes > 0 {
		*wait = time.Duration(cfg.IntervalMinutes) * time.Minute
	}
	if err != nil || (!manual && (!cfg.Enabled || !cfg.Active)) {
		if manual && err != nil {
			s.fail(err)
		}
		return cfg, false
	}
	return cfg, true
}

// Sync reads the firewall once and records what it knows as sightings.
func (s *Syncer) Sync(ctx context.Context, cfg Config) error {
	now := time.Now()
	s.mu.Lock()
	s.status.Running, s.status.LastRun = true, &now
	s.mu.Unlock()
	counts, name, err := s.read(ctx, cfg)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.status.Running = false
	if err != nil {
		s.status.Error = err.Error()
		return err
	}
	done := time.Now()
	s.status.Error, s.status.LastOK, s.status.Counts, s.status.Firewall = "", &done, counts, name
	return nil
}

func (s *Syncer) read(ctx context.Context, cfg Config) (Counts, string, error) {
	client, err := cfg.Client()
	if err != nil {
		return Counts{}, "", err
	}
	name, err := client.Status(ctx)
	if err != nil {
		return Counts{}, "", err
	}
	entries, counts, err := client.ReadAll(ctx)
	if err != nil {
		return counts, name, err
	}
	if err := save(ctx, s.conn, entries); err != nil {
		return counts, name, err
	}
	return counts, name, monitoring.New(s.conn).Evaluate(ctx)
}

func (s *Syncer) fail(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	s.status.LastRun, s.status.Error = &now, err.Error()
}
