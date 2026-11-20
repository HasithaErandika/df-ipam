package intune

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"sync"
	"time"

	"dfipam/api/internal/platform/db"
)

const manualGap = time.Minute

var (
	ErrBusy    = errors.New("an Intune read is already running")
	ErrTooSoon = errors.New("intune was read less than a minute ago")
)

// Config is what an Admin set in System > Microsoft Intune.
type Config struct {
	Enabled         bool `json:"enabled"`
	IntervalMinutes int  `json:"intervalMinutes"`
}

// Load reads the settings, with defaults for those never saved.
func Load(ctx context.Context, conn db.Conn) (Config, error) {
	cfg := Config{IntervalMinutes: 60}
	rows, err := conn.Query(ctx, `SELECT key, value FROM settings WHERE key IN ('intune.enabled', 'intune.interval_minutes')`)
	if err != nil {
		return cfg, err
	}
	defer rows.Close()
	for rows.Next() {
		var k string
		var v json.RawMessage
		if err := rows.Scan(&k, &v); err != nil {
			return cfg, err
		}
		if k == "intune.enabled" {
			_ = json.Unmarshal(v, &cfg.Enabled)
		} else {
			_ = json.Unmarshal(v, &cfg.IntervalMinutes)
		}
	}
	return cfg, rows.Err()
}

// Status is how the last read went, kept in memory.
type Status struct {
	Running bool       `json:"running"`
	LastRun *time.Time `json:"lastRun"`
	LastOK  *time.Time `json:"lastOk"`
	Error   string     `json:"error"`
	Devices int        `json:"devices"`
	Matched int64      `json:"matched"`
	// Entra ID is read after Intune, and fails on its own: a missing
	// permission for one doesn't stop the other.
	EntraError   string `json:"entraError"`
	EntraDevices int    `json:"entraDevices"`
	EntraMatched int64  `json:"entraMatched"`
}

// Syncer reads Intune on a schedule when it is enabled, and on request.
type Syncer struct {
	conn       db.Conn
	client     *Client
	trigger    chan struct{}
	mu         sync.Mutex
	status     Status
	lastManual time.Time
}

// NewSyncer takes a nil client when the Entra app isn't configured.
func NewSyncer(conn db.Conn, client *Client) *Syncer {
	return &Syncer{conn: conn, client: client, trigger: make(chan struct{}, 1)}
}

// Configured says whether api/.env has the Entra app Graph is called with.
func (s *Syncer) Configured() bool { return s.client != nil }

func (s *Syncer) Snapshot() Status {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.status
}

// Trigger asks for a read now, a minute apart at most.
func (s *Syncer) Trigger() error {
	if s.client == nil {
		return ErrNotConfigured
	}
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
		cfg, err := Load(ctx, s.conn)
		wait = time.Minute
		if err == nil && cfg.IntervalMinutes > 0 {
			wait = time.Duration(cfg.IntervalMinutes) * time.Minute
		}
		if err != nil || s.client == nil || (!cfg.Enabled && !manual) {
			continue
		}
		if err := s.Sync(ctx); err != nil && ctx.Err() == nil {
			slog.WarnContext(ctx, "intune or entra read failed", "error", err)
		}
	}
}

// Sync reads Intune and Entra ID once and records each device's user on its
// sightings. Each source keeps its last good result when the other fails.
func (s *Syncer) Sync(ctx context.Context) error {
	now := time.Now()
	s.mu.Lock()
	s.status.Running, s.status.LastRun = true, &now
	s.mu.Unlock()
	devs, err := s.client.Devices(ctx)
	var matched int64
	if err == nil {
		matched, err = save(ctx, s.conn, devs)
	}
	ents, entraErr := s.client.EntraDevices(ctx)
	var entraMatched int64
	if entraErr == nil {
		entraMatched, entraErr = saveEntra(ctx, s.conn, ents)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.status.Running = false
	if err != nil {
		s.status.Error = err.Error()
	} else {
		s.status.Error, s.status.Devices, s.status.Matched = "", len(devs), matched
	}
	if entraErr != nil {
		s.status.EntraError = entraErr.Error()
	} else {
		s.status.EntraError, s.status.EntraDevices, s.status.EntraMatched = "", len(ents), entraMatched
	}
	if err == nil && entraErr == nil {
		done := time.Now()
		s.status.LastOK = &done
	}
	return errors.Join(err, entraErr)
}
