package discovery

import (
	"context"
	"net/netip"
	"strings"

	"github.com/jackc/pgx/v5"
)

// scopeEntry is one frozen target, parsed once per batch rather than per address.
type scopeEntry struct {
	prefix netip.Prefix
	rd     string
}

// evidenceScope is what decides whether an observation is kept.
type evidenceScope struct {
	targets    []scopeEntry
	exclusions []Exclusion
	sources    map[string]string // source name to ID
}

// loadEvidenceScope reads the frozen scope, and exclusions as they are now: an
// exclusion added mid-job still wins.
func loadEvidenceScope(ctx context.Context, tx pgx.Tx, jobID string) (evidenceScope, error) {
	var scope evidenceScope
	rows, err := tx.Query(ctx, `SELECT routing_domain_id::text, target::text FROM discovery_job_targets WHERE job_id = $1::uuid`, jobID)
	if err != nil {
		return scope, err
	}
	targets, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (Target, error) {
		var t Target
		return t, r.Scan(&t.RoutingDomainID, &t.CIDR)
	})
	if err != nil {
		return scope, err
	}
	for _, t := range targets {
		if p, err := netip.ParsePrefix(t.CIDR); err == nil {
			scope.targets = append(scope.targets, scopeEntry{p, t.RoutingDomainID})
		}
	}
	if scope.exclusions, err = activeExclusions(ctx, tx); err != nil {
		return scope, err
	}
	scope.sources, err = observationSources(ctx, tx)
	return scope, err
}

func observationSources(ctx context.Context, tx pgx.Tx) (map[string]string, error) {
	rows, err := tx.Query(ctx, `SELECT name, id::text FROM observation_sources`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	sources := map[string]string{}
	for rows.Next() {
		var name, id string
		if err := rows.Scan(&name, &id); err != nil {
			return nil, err
		}
		sources[name] = id
	}
	return sources, rows.Err()
}

// check returns the address, routing domain and source of an observation that may
// be kept.
func (s evidenceScope) check(o Observation) (a netip.Addr, rd, source string, ok bool) {
	a, err := netip.ParseAddr(strings.TrimSpace(o.Address))
	source, known := s.sources[SourceName[o.Method]]
	if err != nil || !known {
		return a, "", "", false
	}
	a = a.Unmap()
	rd = inScope(a, s.targets, s.exclusions)
	return a, rd, source, rd != ""
}

// inScope returns the routing domain of the frozen target holding the
// address, or "" when the address is outside the scope or excluded.
func inScope(a netip.Addr, scope []scopeEntry, exclusions []Exclusion) string {
	for _, t := range scope {
		if !t.prefix.Contains(a) {
			continue
		}
		for _, e := range exclusions {
			if (e.RoutingDomainID == "" || e.RoutingDomainID == t.rd) && e.CIDR.Contains(a) {
				return ""
			}
		}
		return t.rd
	}
	return ""
}
