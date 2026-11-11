package fortigate

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"strings"
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
		cfg, err := Load(ctx, s.conn, s.keys)
		wait = time.Minute
		if err == nil && cfg.IntervalMinutes > 0 {
			wait = time.Duration(cfg.IntervalMinutes) * time.Minute
		}
		// A scheduled read needs the connection switched on; "Read now" works
		// while it is off, so an Admin can try it before enabling the schedule.
		if err != nil || (!cfg.Enabled && !manual) {
			if manual && err != nil {
				s.fail(err)
			}
			continue
		}
		if err := s.Sync(ctx, cfg); err != nil && ctx.Err() == nil {
			slog.WarnContext(ctx, "fortigate read failed", "error", err)
		}
	}
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

// save records entries as sightings. ARP and DHCP evidence is kept only inside a
// known segment, so WAN neighbours don't appear; VPN clients are kept regardless.
// A DHCP lease is never static; an ARP entry is static outside the floor's pool.
func save(ctx context.Context, conn db.Conn, entries []Entry) error {
	merged := map[string]Entry{}
	for _, e := range entries {
		e.MAC = strings.ToLower(e.MAC)
		if hw, err := net.ParseMAC(e.MAC); err == nil {
			e.MAC = hw.String()
		} else {
			e.MAC = ""
		}
		key := e.IP + "|" + e.MAC
		if old, ok := merged[key]; ok && old.Source == "dhcp" {
			continue // a lease says more than the ARP entry for the same pair
		}
		merged[key] = e
	}
	n := len(merged)
	ips, macs, names, vendors, sources := make([]string, 0, n), make([]string, 0, n), make([]string, 0, n), make([]string, 0, n), make([]string, 0, n)
	private := make([]bool, 0, n)
	for _, e := range merged {
		ips, macs, names, vendors, sources = append(ips, e.IP), append(macs, e.MAC), append(names, e.Hostname), append(vendors, e.Vendor), append(sources, e.Source)
		hw, _ := net.ParseMAC(e.MAC)
		private = append(private, len(hw) > 0 && hw[0]&0x02 != 0)
	}
	if n == 0 {
		return nil
	}
	_, err := conn.Exec(ctx, `INSERT INTO sightings (segment_id, ip, mac, hostname, vendor, source, is_static, is_private_mac, first_seen, last_seen)
		SELECT s.id, x.ip::inet, NULLIF(x.mac, '')::macaddr, x.name, x.vendor, x.source,
			CASE WHEN x.source IN ('dhcp', 'vpn') THEN false
			     WHEN s.kind <> 'dhcp' OR s.dhcp_start IS NULL THEN true
			     ELSE NOT (x.ip::inet BETWEEN s.dhcp_start AND s.dhcp_end) END,
			x.private, now(), now()
		FROM unnest($1::text[], $2::text[], $3::text[], $4::text[], $5::text[], $6::boolean[]) AS x(ip, mac, name, vendor, source, private)
		LEFT JOIN segments s ON x.ip::inet << s.network
		WHERE s.id IS NOT NULL OR x.source = 'vpn'
		ON CONFLICT (ip, mac) DO UPDATE SET segment_id = EXCLUDED.segment_id,
			hostname = COALESCE(NULLIF(EXCLUDED.hostname, ''), sightings.hostname),
			vendor = COALESCE(NULLIF(EXCLUDED.vendor, ''), sightings.vendor),
			source = EXCLUDED.source, is_static = EXCLUDED.is_static, is_private_mac = EXCLUDED.is_private_mac, last_seen = now()`,
		ips, macs, names, vendors, sources, private)
	return err
}

func (s *Syncer) fail(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	s.status.LastRun, s.status.Error = &now, err.Error()
}
