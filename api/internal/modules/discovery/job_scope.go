package discovery

import (
	"context"
	"dfipam/api/internal/platform/httpx"
	"fmt"
	"net/http"
	"net/netip"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

// loadSubnetScopes reads every chosen subnet in one query, in the order chosen.
func loadSubnetScopes(ctx context.Context, tx pgx.Tx, ids []string) ([]subnetScope, error) {
	for _, id := range ids {
		if !httpx.IsUUID(id) {
			return nil, httpx.Validation("subnetIds", "Every subnet ID must be valid.")
		}
	}
	rows, err := tx.Query(ctx, `SELECT s.id::text, s.routing_domain_id::text, s.cidr::text, s.name, s.sensitivity, s.discovery_enabled,
		coalesce(bs.is_sensitive, false) FROM subnets s LEFT JOIN business_services bs ON bs.id = s.business_service_id
		WHERE s.id = ANY($1::uuid[]) AND s.archived_at IS NULL`, ids)
	if err != nil {
		return nil, err
	}
	found, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (subnetScope, error) {
		var sc subnetScope
		return sc, r.Scan(&sc.ID, &sc.RoutingDomainID, &sc.CIDR, &sc.Name, &sc.Sensitivity, &sc.Enabled, &sc.ServiceSensitive)
	})
	if err != nil {
		return nil, err
	}
	byID := make(map[string]subnetScope, len(found))
	for _, sc := range found {
		byID[sc.ID] = sc
	}
	out := make([]subnetScope, 0, len(ids))
	for _, id := range ids {
		sc, ok := byID[strings.ToLower(id)]
		if !ok {
			return nil, httpx.Validation("subnetIds", "A chosen subnet doesn't exist or is archived.")
		}
		out = append(out, sc)
	}
	return out, nil
}

// blackedOut returns which of the CIDRs are in a maintenance blackout now.
func blackedOut(ctx context.Context, q querier, cidrs []string) (map[string]bool, error) {
	rows, err := q.Query(ctx, `SELECT c FROM unnest($1::text[]) AS t(c) WHERE EXISTS (
		SELECT 1 FROM discovery_blackouts b WHERE now() >= b.starts_at AND now() < b.ends_at AND (b.target IS NULL OR b.target && c::cidr))`, cidrs)
	if err != nil {
		return nil, err
	}
	hits, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return nil, err
	}
	out := make(map[string]bool, len(hits))
	for _, c := range hits {
		out[c] = true
	}
	return out, nil
}

// scopeFreezer builds a job's snapshot one subnet at a time.
type scopeFreezer struct {
	policy     Policy
	methods    []string
	allow      []netip.Prefix
	exclusions []Exclusion
	blackouts  map[string]bool
	methodSet  map[string]bool
	snap       Snapshot
}

func freezeScope(ctx context.Context, tx pgx.Tx, p Policy, methods []string, scopes []subnetScope) (Snapshot, error) {
	f := scopeFreezer{policy: p, methods: methods, methodSet: map[string]bool{}, snap: Snapshot{PolicyID: p.ID, PolicyName: p.Name, MaxConcurrency: p.MaxConcurrency,
		MaxPPS: p.MaxPPS, TimeoutMS: p.TimeoutMS, Retries: p.Retries, Windows: p.Windows, FrozenAt: time.Now().UTC(), Skipped: []string{}}}
	for _, a := range p.Allowlist {
		if pr, err := netip.ParsePrefix(a); err == nil {
			f.allow = append(f.allow, pr)
		}
	}
	var err error
	if f.exclusions, err = activeExclusions(ctx, tx); err != nil {
		return f.snap, err
	}
	cidrs := make([]string, len(scopes))
	for i, sc := range scopes {
		cidrs[i] = sc.CIDR
	}
	if f.blackouts, err = blackedOut(ctx, tx, cidrs); err != nil {
		return f.snap, err
	}
	for _, sc := range scopes {
		if err := f.add(ctx, tx, sc); err != nil {
			return f.snap, err
		}
	}
	return f.finish()
}

func (f *scopeFreezer) skip(reason string) { f.snap.Skipped = append(f.snap.Skipped, reason) }

func (f *scopeFreezer) add(ctx context.Context, tx pgx.Tx, sc subnetScope) error {
	pr := netip.MustParsePrefix(sc.CIDR)
	if !sc.Enabled {
		return targetError(fmt.Sprintf("Discovery is turned off for %s (%s).", sc.Name, sc.CIDR))
	}
	if !Probeable(pr) {
		return targetError(fmt.Sprintf("%s (%s) is a multicast, loopback or link-local network, which is never swept.", sc.Name, sc.CIDR))
	}
	if !Allowed(pr, f.allow) {
		return targetError(fmt.Sprintf("%s (%s) is outside the policy's approved networks.", sc.Name, sc.CIDR))
	}
	if f.blackouts[sc.CIDR] {
		f.skip(sc.CIDR + ": in a maintenance blackout")
		return nil
	}
	sensitive := sc.Sensitivity != "standard" || sc.ServiceSensitive
	allowed := FilterMethods(f.methods, sensitive, f.policy.IsSensitive)
	if len(allowed) == 0 {
		f.skip(sc.CIDR + ": sensitive network, passive methods only")
		return nil
	}
	for _, m := range allowed {
		f.methodSet[m] = true
	}
	parts := Subtract(pr, sc.RoutingDomainID, f.exclusions)
	if len(parts) == 0 {
		f.skip(sc.CIDR + ": fully excluded")
		return nil
	}
	for _, part := range parts {
		if err := f.addPart(ctx, tx, sc, part); err != nil {
			return err
		}
	}
	return nil
}

func (f *scopeFreezer) addPart(ctx context.Context, tx pgx.Tx, sc subnetScope, part netip.Prefix) error {
	if !TooLarge(part) {
		f.snap.Targets = append(f.snap.Targets, Target{sc.RoutingDomainID, part.String()})
		return nil
	}
	if !part.Addr().Is6() {
		return httpx.NewError(http.StatusUnprocessableEntity, "TARGET_TOO_LARGE",
			fmt.Sprintf("%s is larger than /%d. Split it into smaller subnets to discover it.", part, MaxIPv4Bits))
	}
	// IPv6 is never swept: only addresses already recorded are probed.
	rows, err := tx.Query(ctx, `SELECT host(address) FROM ip_addresses WHERE subnet_id = $1::uuid AND address <<= $2::cidr
		ORDER BY address LIMIT $3`, sc.ID, part.String(), MaxKnownTargets)
	if err != nil {
		return err
	}
	hosts, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return err
	}
	for _, h := range hosts {
		f.snap.Targets = append(f.snap.Targets, Target{sc.RoutingDomainID, h + "/128"})
	}
	if len(hosts) == 0 {
		f.skip(part.String() + ": IPv6 subnet with no recorded addresses to probe")
	}
	return nil
}

func (f *scopeFreezer) finish() (Snapshot, error) {
	if len(f.snap.Targets) == 0 {
		return f.snap, targetError("Nothing is left to discover after exclusions, blackouts and sensitivity rules: " + strings.Join(f.snap.Skipped, "; "))
	}
	if len(f.snap.Targets) > MaxTargets {
		return f.snap, targetError("This job would have too many targets. Choose fewer subnets.")
	}
	total := 0
	for _, t := range f.snap.Targets {
		total += addressesIn(t.CIDR)
	}
	if total > MaxJobAddresses {
		return f.snap, targetError(fmt.Sprintf("This job would probe %d addresses, and the limit is %d. Choose fewer subnets.", total, MaxJobAddresses))
	}
	for m := range f.methodSet {
		f.snap.Methods = append(f.snap.Methods, m)
	}
	return f.snap, nil
}
