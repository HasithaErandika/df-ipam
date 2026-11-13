package scanner

import (
	"context"
	"log/slog"
	"time"
)

type fullMark struct {
	at    time.Time
	ports bool // whether that identification checked ports
}

// manualFullEvery is the least time between two full identifications of one
// host, even on Scan now, so repeated manual scans can't keep port-checking
// the whole network.
const manualFullEvery = 15 * time.Minute

// DefaultScanInterval is the scan interval when the setting is missing.
const DefaultScanInterval = 10 * time.Minute

// defaultCheckInterval is the server check interval when the setting is missing.
const defaultCheckInterval = 5 * time.Minute

// dueFull reports whether a host should be identified in full now: after
// fullEvery, after manualFullEvery on a manual scan, or when its segment has
// just started checking ports.
func (s *Scanner) dueFull(seg segment, ip string, manual bool) bool {
	s.fullMu.Lock()
	defer s.fullMu.Unlock()
	last, seen := s.lastFull[seg.id+"|"+ip]
	if !seen || (seg.checkPorts && !last.ports) {
		return true
	}
	gap := fullEvery
	if manual {
		gap = manualFullEvery
	}
	return time.Since(last.at) >= gap
}

// markFull records a finished full identification.
func (s *Scanner) markFull(seg segment, ip string) {
	s.fullMu.Lock()
	defer s.fullMu.Unlock()
	s.lastFull[seg.id+"|"+ip] = fullMark{at: time.Now(), ports: seg.checkPorts}
}

// loadFullMarks reads when each host was last identified in full, so a restart
// doesn't identify every host on the network again at once.
func (s *Scanner) loadFullMarks(ctx context.Context) {
	rows, err := s.conn.Query(ctx, `SELECT segment_id::text, host(ip), identified_at FROM sightings
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
		var at time.Time
		if rows.Scan(&seg, &ip, &at) == nil {
			s.lastFull[seg+"|"+ip] = fullMark{at: at, ports: true}
		}
	}
}
