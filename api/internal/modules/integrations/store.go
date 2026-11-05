package integrations

import (
	"context"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"
)

func sortStrings(s []string) { slices.Sort(s) }

// Counts summarises what a sync stored.
type Counts struct {
	Devices        int `json:"devices"`
	Interfaces     int `json:"interfaces"`
	Observations   int `json:"observations"`
	Unmapped       int `json:"unmappedObservations"`
	Skipped        int `json:"skipped"`
	AddressesSeen  int `json:"addressesSeen"`
	Routes         int `json:"routes"`
	AddressObjects int `json:"addressObjects"`
	AddressGroups  int `json:"addressGroups"`
	Networks       int `json:"networks"`
	DHCPScopes     int `json:"dhcpScopes"`
	Warnings       int `json:"warnings"`
}

const maxPayload = 16384

// store writes a Result. Every statement here names its columns, and none of them
// is an ownership or allocation field: owner_id, business_service_id, application_id,
// allocation_state, site_id and environment are out of reach. A test enforces this.
type store struct {
	tx      pgx.Tx
	conn    string // connection ID
	kind    string
	domains map[string]string // context to routing domain
	jobID   string
	now     time.Time
	devices map[string]string // external ID to device row
	vdoms   map[string]string // device row + name to VDOM row
	counts  Counts
}

func (s *store) domain(context string) *string {
	if id, ok := s.domains[context]; ok && id != "" {
		return &id
	}
	return nil
}

// externalID scopes a platform's identifier to this connection, because two
// vCenters can each have a vm-101.
func (s *store) externalID(id string) string { return s.conn + ":" + id }

func (s *store) sources(ctx context.Context) (map[string]string, error) {
	rows, err := s.tx.Query(ctx, `SELECT name, id::text FROM observation_sources`)
	if err != nil {
		return nil, err
	}
	out := map[string]string{}
	var name, id string
	_, err = pgx.ForEachRow(rows, []any{&name, &id}, func() error {
		out[name] = id
		return nil
	})
	return out, err
}

const chunk = 1000

func (s *store) vdomID(device, vdom string) *string {
	if vdom == "" {
		return nil
	}
	return nullText(s.vdoms[device+"/"+vdom])
}

// sourceFor names the evidence source of a connection kind.
func sourceFor(kind string) string {
	return map[string]string{KindFortiGate: SourceFortiGate, KindVCenter: SourceVMware, KindAWS: SourceAWS, KindAzure: SourceAzure}[kind]
}

// batched queues statements for each of n items and sends them in chunks, in order.
// A callback on a queued statement runs when its result arrives, and no statement
// waits for a round trip of its own.
func (s *store) batched(ctx context.Context, n int, queue func(b *pgx.Batch, i int)) error {
	for start := 0; start < n; start += chunk {
		b := &pgx.Batch{}
		for i := start; i < min(start+chunk, n); i++ {
			queue(b, i)
		}
		if b.Len() == 0 {
			continue
		}
		if err := s.tx.SendBatch(ctx, b).Close(); err != nil {
			return err
		}
	}
	return nil
}

func (s *store) queueMapping(b *pgx.Batch, resource, external, object, id string) {
	b.Queue(`INSERT INTO external_resource_mappings (connection_id, resource_type, external_id, object_type, object_id, last_seen)
		VALUES ($1::uuid, $2, $3, $4, $5::uuid, $6)
		ON CONFLICT (connection_id, resource_type, external_id) DO UPDATE SET object_id = EXCLUDED.object_id, last_seen = EXCLUDED.last_seen`,
		s.conn, resource, external, object, id, s.now)
}

// save writes the whole result in the caller's transaction.
func save(ctx context.Context, tx pgx.Tx, connID, kind, jobID string, domains map[string]string, r *Result, now time.Time) (Counts, error) {
	s := &store{tx: tx, conn: connID, kind: kind, domains: domains, jobID: jobID, now: now, devices: map[string]string{}, vdoms: map[string]string{}}
	if err := s.saveInventory(ctx, r); err != nil {
		return s.counts, err
	}
	sources, err := s.sources(ctx)
	if err != nil {
		return s.counts, err
	}
	if err := s.saveNetwork(ctx, r, sources); err != nil {
		return s.counts, err
	}
	s.counts.Warnings = len(r.Warnings)
	return s.counts, nil
}

func (s *store) saveInventory(ctx context.Context, r *Result) error {
	if err := s.saveDevices(ctx, r.Devices); err != nil {
		return err
	}
	if err := s.cloud(ctx, r); err != nil {
		return err
	}
	return s.observations(ctx, r.Observations)
}

func (s *store) saveNetwork(ctx context.Context, r *Result, sources map[string]string) error {
	source := sources[sourceFor(s.kind)]
	if err := s.firewall(ctx, r); err != nil {
		return err
	}
	if err := s.routes(ctx, r.Routes, source); err != nil {
		return err
	}
	if err := s.peerings(ctx, r.Peerings, source); err != nil {
		return err
	}
	return s.scopes(ctx, r.DHCPScopes, sources[SourceDHCP])
}
