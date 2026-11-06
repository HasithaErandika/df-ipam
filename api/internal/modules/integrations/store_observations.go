package integrations

import (
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"
)

// liveAddress is the latest sighting of an address in a sync.
type liveAddress struct {
	domain, source string
	at             time.Time
}

// observations appends evidence, then refreshes the observation fields of records
// that already exist. Allocation state and ownership are never touched, and no
// record is created: an unknown address becomes a finding for a person to review.
func (s *store) observations(ctx context.Context, items []Observation) error {
	sources, err := s.sources(ctx)
	if err != nil {
		return err
	}
	alive := map[string]liveAddress{}
	err = s.batched(ctx, len(items), func(b *pgx.Batch, i int) { s.queueObservation(b, items[i], sources, alive) })
	if err != nil {
		return fmt.Errorf("observations: %w", err)
	}
	return s.markAlive(ctx, alive)
}

func (s *store) queueObservation(b *pgx.Batch, o Observation, sources map[string]string, alive map[string]liveAddress) {
	source, known := sources[o.Source]
	var addr *string
	if a, ok := validAddr(o.Address); ok {
		addr = &a
	}
	if !known || (addr == nil && o.MAC == "" && o.Hostname == "") {
		s.counts.Skipped++
		return
	}
	at := o.ObservedAt
	if at.IsZero() || at.After(s.now) {
		at = s.now
	}
	domain := s.domain(o.Context)
	if domain == nil {
		s.counts.Unmapped++
	}
	b.Queue(`INSERT INTO discovery_observations (sync_job_id, source_id, routing_domain_id, address, mac, hostname, payload, observed_at)
		VALUES ($1::uuid, $2::uuid, $3::uuid, $4::text::inet, $5::macaddr, $6, $7::jsonb, $8)`,
		s.jobID, source, domain, addr, validMAC(o.MAC), nullText(truncate(o.Hostname, 255)), payloadJSON(o.Payload), at)
	s.counts.Observations++
	if !o.Alive || domain == nil || addr == nil {
		return
	}
	key := *domain + "|" + *addr
	if prev, ok := alive[key]; !ok || at.After(prev.at) {
		alive[key] = liveAddress{*domain, source, at}
	}
}

func (s *store) markAlive(ctx context.Context, alive map[string]liveAddress) error {
	keys := make([]string, 0, len(alive))
	for k := range alive {
		keys = append(keys, k)
	}
	slices.Sort(keys) // a stable order stops two syncs deadlocking on the same rows
	for start := 0; start < len(keys); start += chunk {
		var domains, addrs, sourceIDs []string
		var times []time.Time
		for _, k := range keys[start:min(start+chunk, len(keys))] {
			v := alive[k]
			domains = append(domains, v.domain)
			addrs = append(addrs, k[len(v.domain)+1:])
			sourceIDs = append(sourceIDs, v.source)
			times = append(times, v.at)
		}
		tag, err := s.tx.Exec(ctx, `UPDATE ip_addresses i SET
			  observation_state = CASE WHEN i.observation_state = 'conflict_suspected' THEN i.observation_state ELSE 'observed_active' END,
			  last_seen = greatest(coalesce(i.last_seen, e.at), e.at), first_seen = coalesce(i.first_seen, e.at), discovery_source_id = e.source
			FROM unnest($1::uuid[], $2::inet[], $3::uuid[], $4::timestamptz[]) AS e(domain, address, source, at)
			WHERE i.routing_domain_id = e.domain AND i.address = e.address`, domains, addrs, sourceIDs, times)
		if err != nil {
			return fmt.Errorf("observation state: %w", err)
		}
		s.counts.AddressesSeen += int(tag.RowsAffected())
	}
	return nil
}
