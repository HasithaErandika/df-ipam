package discovery

import (
	"context"
	"encoding/json"
	"net"
	"time"

	"github.com/jackc/pgx/v5"
)

// evidenceRows holds accepted observations as columns, ready for one INSERT.
type evidenceRows struct {
	sources, domains, addresses, hostnames, payloads []string
	macs                                             []*string
	observed                                         []time.Time
}

// sighting is one live address, merged across a batch for one UPDATE.
type sighting struct {
	rd, address, source string
	first, last         time.Time
}

// storeObservations keeps what is in scope and returns how many were kept and
// dropped. A batch costs two statements however many observations it holds.
func storeObservations(ctx context.Context, tx pgx.Tx, jobID string, list []Observation) (accepted, dropped int, err error) {
	scope, err := loadEvidenceScope(ctx, tx, jobID)
	if err != nil {
		return 0, 0, err
	}
	var rows evidenceRows
	var seen []sighting
	index := map[string]int{}
	for _, o := range list {
		a, rd, source, ok := scope.check(o)
		if !ok {
			dropped++
			continue
		}
		observed := rows.add(o, a.String(), rd, source)
		if o.Alive {
			seen = mergeSighting(seen, index, sighting{rd, a.String(), source, observed, observed})
		}
		accepted++
	}
	if err := rows.insert(ctx, tx, jobID); err != nil {
		return 0, 0, err
	}
	return accepted, dropped, markSeen(ctx, tx, seen)
}

// add appends an observation and returns the time it is recorded at.
func (e *evidenceRows) add(o Observation, address, rd, source string) time.Time {
	var mac *string
	if hw, err := net.ParseMAC(o.MAC); err == nil {
		s := hw.String()
		mac = &s
	}
	observed := o.ObservedAt
	if observed.IsZero() || observed.After(time.Now()) {
		observed = time.Now()
	}
	payload := o.Payload
	if len(payload) == 0 || len(payload) > 16384 || !json.Valid(payload) {
		payload = json.RawMessage(`{}`)
	}
	e.sources, e.domains, e.addresses = append(e.sources, source), append(e.domains, rd), append(e.addresses, address)
	e.macs, e.hostnames = append(e.macs, mac), append(e.hostnames, truncate(o.Hostname, 255))
	e.payloads, e.observed = append(e.payloads, string(payload)), append(e.observed, observed)
	return observed
}

func (e *evidenceRows) insert(ctx context.Context, tx pgx.Tx, jobID string) error {
	if len(e.addresses) == 0 {
		return nil
	}
	_, err := tx.Exec(ctx, `INSERT INTO discovery_observations (job_id, source_id, routing_domain_id, address, mac, hostname, payload, observed_at)
		SELECT $1::uuid, source, rd, address::inet, mac::macaddr, NULLIF(hostname, ''), payload::jsonb, observed
		FROM unnest($2::uuid[], $3::uuid[], $4::text[], $5::text[], $6::text[], $7::text[], $8::timestamptz[])
		  AS t(source, rd, address, mac, hostname, payload, observed)`,
		jobID, e.sources, e.domains, e.addresses, e.macs, e.hostnames, e.payloads, e.observed)
	return err
}

func mergeSighting(seen []sighting, index map[string]int, s sighting) []sighting {
	key := s.rd + "|" + s.address
	i, ok := index[key]
	if !ok {
		index[key] = len(seen)
		return append(seen, s)
	}
	if s.first.Before(seen[i].first) {
		seen[i].first = s.first
	}
	if s.last.After(seen[i].last) {
		seen[i].last = s.last
	}
	seen[i].source = s.source
	return seen
}

// markSeen updates observation fields only. Allocation state and ownership are
// never touched.
func markSeen(ctx context.Context, tx pgx.Tx, seen []sighting) error {
	if len(seen) == 0 {
		return nil
	}
	domains, addresses, sources := make([]string, len(seen)), make([]string, len(seen)), make([]string, len(seen))
	firsts, lasts := make([]time.Time, len(seen)), make([]time.Time, len(seen))
	for i, s := range seen {
		domains[i], addresses[i], sources[i], firsts[i], lasts[i] = s.rd, s.address, s.source, s.first, s.last
	}
	_, err := tx.Exec(ctx, `UPDATE ip_addresses i SET
		  observation_state = CASE WHEN i.observation_state = 'conflict_suspected' THEN i.observation_state ELSE 'observed_active' END,
		  last_seen = greatest(coalesce(i.last_seen, t.last_at), t.last_at),
		  first_seen = coalesce(i.first_seen, t.first_at), discovery_source_id = t.source
		FROM unnest($1::uuid[], $2::text[], $3::uuid[], $4::timestamptz[], $5::timestamptz[]) AS t(rd, address, source, first_at, last_at)
		WHERE i.routing_domain_id = t.rd AND i.address = t.address::inet`, domains, addresses, sources, firsts, lasts)
	return err
}
