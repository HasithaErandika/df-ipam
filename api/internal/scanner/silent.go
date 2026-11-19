package scanner

import (
	"context"
	"time"
)

// silentEvery is how often a scheduled scan also asks every address that
// ignores ping (NetBIOS, then a few TCP ports), so a new device that blocks
// ping is still found within the hour. In between, only addresses known to be
// in use are asked again.
const silentEvery = time.Hour

// knownDays is how long an address counts as known after it was last seen.
const knownDays = "30 days"

// knownAddresses lists a segment's addresses seen in the last 30 days or
// registered: those are worth asking again when they don't answer ping.
func (s *Scanner) knownAddresses(ctx context.Context, segmentID string) (map[string]bool, error) {
	rows, err := s.conn.Query(ctx, `SELECT host(ip) FROM sightings WHERE segment_id = $1::uuid AND last_seen > now() - $2::interval
		UNION SELECT host(ip) FROM ip_addresses WHERE segment_id = $1::uuid`, segmentID, knownDays)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]bool{}
	for rows.Next() {
		var ip string
		if err := rows.Scan(&ip); err != nil {
			return nil, err
		}
		out[ip] = true
	}
	return out, rows.Err()
}

// silentDue reports whether this sweep of a segment asks every silent address,
// and if so records it. A manual scan always does.
func (s *Scanner) silentDue(segmentID string, manual bool) bool {
	s.fullMu.Lock()
	defer s.fullMu.Unlock()
	if !manual && time.Since(s.lastSilent[segmentID]) < silentEvery {
		return false
	}
	s.lastSilent[segmentID] = time.Now()
	return true
}
