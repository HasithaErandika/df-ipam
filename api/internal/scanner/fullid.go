package scanner

import (
	"context"
	"hash/fnv"
	"log/slog"
	"time"
)

type fullMark struct {
	at      time.Time // last full identification
	portsAt time.Time // last port check; zero when never checked
}

// manualFullEvery is the least time between two full identifications of one
// host, even on Scan now, so repeated manual scans can't keep port-checking
// the whole network.
const manualFullEvery = 15 * time.Minute

// DefaultScanInterval is the user-network scan interval when the setting is missing.
const DefaultScanInterval = 15 * time.Minute

// DefaultServerScanInterval is the server-network scan interval when the setting is missing.
const DefaultServerScanInterval = 10 * time.Minute

// defaultCheckInterval is the server check interval when the setting is missing.
const defaultCheckInterval = 5 * time.Minute

// DefaultPortEvery is how often a device's ports are checked, where the
// segment asks for it, when the setting is missing.
const DefaultPortEvery = 24 * time.Hour

// spread is up to an hour, fixed per address, taken off fullEvery so the hosts
// found in one scan aren't all identified again together six hours later.
func spread(ip string) time.Duration {
	h := fnv.New32a()
	_, _ = h.Write([]byte(ip))
	return time.Duration(h.Sum32()%3600) * time.Second
}

// dueFull reports whether a host should be identified in full now: after
// fullEvery (less its spread), after manualFullEvery on a manual scan, or when
// its ports are due on a segment that checks them.
func (s *Scanner) dueFull(seg segment, ip string, manual bool) bool {
	s.fullMu.Lock()
	defer s.fullMu.Unlock()
	last, seen := s.lastFull[seg.id+"|"+ip]
	if !seen || (seg.checkPorts && time.Since(last.portsAt) >= s.portEvery()) {
		return true
	}
	gap := fullEvery - spread(ip)
	if manual {
		gap = manualFullEvery
	}
	return time.Since(last.at) >= gap
}

// portsDue reports whether a host's ports should be checked in this identification.
func (s *Scanner) portsDue(seg segment, ip string) bool {
	s.fullMu.Lock()
	defer s.fullMu.Unlock()
	return time.Since(s.lastFull[seg.id+"|"+ip].portsAt) >= s.portEvery()
}

func (s *Scanner) portEvery() time.Duration {
	if s.portGap > 0 {
		return s.portGap
	}
	return DefaultPortEvery
}

// markFull records a finished full identification, and whether it checked ports.
func (s *Scanner) markFull(seg segment, ip string, ports bool) {
	s.fullMu.Lock()
	defer s.fullMu.Unlock()
	key := seg.id + "|" + ip
	mark := fullMark{at: time.Now(), portsAt: s.lastFull[key].portsAt}
	if ports {
		mark.portsAt = mark.at
	}
	s.lastFull[key] = mark
}

// loadFullMarks reads when each host was last identified in full, so a restart
// doesn't identify every host on the network again at once.
func (s *Scanner) loadFullMarks(ctx context.Context) {
	rows, err := s.conn.Query(ctx, `SELECT segment_id::text, host(ip), identified_at, COALESCE(ports_checked_at, 'epoch') FROM sightings
		WHERE segment_id IS NOT NULL AND identified_at > now() - $1::interval`, fullEvery.String())
	if err != nil {
		slog.WarnContext(ctx, "could not read identification times", "error", err)
		return
	}
	defer rows.Close()
	s.fullMu.Lock()
	defer s.fullMu.Unlock()
	for rows.Next() {
		var seg, ip string
		var at, ports time.Time
		if rows.Scan(&seg, &ip, &at, &ports) == nil {
			s.lastFull[seg+"|"+ip] = fullMark{at: at, portsAt: ports}
		}
	}
}
