package allocation

import (
	"context"
	"dfipam/api/internal/netcalc"
	"dfipam/api/internal/platform/httpx"
	"errors"
	"net/netip"

	"github.com/jackc/pgx/v5"
)

// subnet is what fulfilment needs to know about a subnet.
type subnet struct {
	ID, RoutingDomainID, CIDR, Environment, Sensitivity string
	Gateway                                             *string
	Archived                                            bool
}

func (Repository) Subnet(ctx context.Context, q querier, id string, lock bool) (subnet, error) {
	sql := `SELECT id::text, routing_domain_id::text, cidr::text, environment, sensitivity, host(gateway), archived_at IS NOT NULL
		FROM subnets WHERE id = $1::uuid`
	if lock {
		sql += " FOR UPDATE" // serialises automatic selection per subnet
	}
	var s subnet
	err := q.QueryRow(ctx, sql, id).Scan(&s.ID, &s.RoutingDomainID, &s.CIDR, &s.Environment, &s.Sensitivity, &s.Gateway, &s.Archived)
	if errors.Is(err, pgx.ErrNoRows) {
		return s, httpx.Validation("subnetId", "That subnet doesn't exist.")
	}
	return s, err
}

func rangeOf(first, last string) (netcalc.Range, bool) {
	a, errA := netip.ParseAddr(first)
	b, errB := netip.ParseAddr(last)
	return netcalc.Range{First: a, Last: b}, errA == nil && errB == nil && a.BitLen() == b.BitLen()
}

// Space loads every range in the subnet that can't be offered: held records,
// addresses seen in use or in conflict, reserved ranges, DHCP pools, the
// gateway, and more specific subnets that own their own addresses.
func (Repository) Space(ctx context.Context, q querier, s subnet) (Space, error) {
	sp := Space{Prefix: netip.MustParsePrefix(s.CIDR)}
	rows, err := q.Query(ctx, `
		SELECT host(address), host(address) FROM ip_addresses
		 WHERE subnet_id = $1::uuid AND (allocation_state <> 'available_candidate' OR observation_state IN ('observed_active','conflict_suspected'))
		UNION ALL SELECT host(range_start), host(range_end) FROM subnet_reserved_ranges WHERE subnet_id = $1::uuid
		UNION ALL SELECT host(range_start), host(range_end) FROM dhcp_scopes WHERE subnet_id = $1::uuid
		UNION ALL SELECT host(network(c.cidr)), host(broadcast(c.cidr)) FROM subnets c
		 WHERE c.routing_domain_id = $2::uuid AND c.id <> $1::uuid AND c.archived_at IS NULL AND c.cidr << $3::cidr`,
		s.ID, s.RoutingDomainID, s.CIDR)
	if err != nil {
		return sp, err
	}
	defer rows.Close()
	for rows.Next() {
		var first, last string
		if err := rows.Scan(&first, &last); err != nil {
			return sp, err
		}
		if r, ok := rangeOf(first, last); ok {
			sp.Blocked = append(sp.Blocked, r)
		}
	}
	if s.Gateway != nil {
		if r, ok := rangeOf(*s.Gateway, *s.Gateway); ok {
			sp.Blocked = append(sp.Blocked, r)
		}
	}
	return sp, rows.Err()
}

// Claim moves one address to state (allocated or reserved), creating its record
// if it has none. It succeeds only if the record is an available candidate in
// this subnet and isn't seen in use; otherwise it changes nothing and returns
// ok=false. The row lock this takes is what settles concurrent claims.
func (Repository) Claim(ctx context.Context, q querier, s subnet, addr, state, actor string, r Request) (string, bool, error) {
	var id string
	owner := r.TechnicalOwnerID
	if owner == nil {
		owner = &r.RequestedBy
	}
	err := q.QueryRow(ctx, `
		INSERT INTO ip_addresses (routing_domain_id, subnet_id, address, allocation_state, application_id, business_service_id,
		                          owner_id, created_by, updated_by)
		VALUES ($1::uuid, $2::uuid, $3::text::inet, $4, $5::uuid, $6::uuid, $7::uuid, $8::uuid, $8::uuid)
		ON CONFLICT (routing_domain_id, address) DO UPDATE SET allocation_state = EXCLUDED.allocation_state,
		  application_id = COALESCE(EXCLUDED.application_id, ip_addresses.application_id),
		  business_service_id = COALESCE(EXCLUDED.business_service_id, ip_addresses.business_service_id),
		  owner_id = EXCLUDED.owner_id, updated_by = EXCLUDED.updated_by
		WHERE ip_addresses.allocation_state = 'available_candidate' AND ip_addresses.subnet_id = EXCLUDED.subnet_id
		  AND ip_addresses.observation_state NOT IN ('observed_active','conflict_suspected')
		RETURNING id::text`,
		s.RoutingDomainID, s.ID, addr, state, r.ApplicationID, r.BusinessServiceID, owner, actor).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", false, nil
	}
	return id, err == nil, err
}

// SetState changes a record's state only if it is currently in from.
func (Repository) SetState(ctx context.Context, q querier, ipID, from, to, actor string, clearOwner bool) (bool, error) {
	extra := ""
	if clearOwner {
		extra = ", owner_id = NULL, application_id = NULL, business_service_id = NULL"
	}
	tag, err := q.Exec(ctx, `UPDATE ip_addresses SET allocation_state = $3, updated_by = NULLIF($4, '')::uuid`+extra+`
		WHERE id = $1::uuid AND allocation_state = $2`, ipID, from, to, actor)
	return tag.RowsAffected() == 1, err
}
