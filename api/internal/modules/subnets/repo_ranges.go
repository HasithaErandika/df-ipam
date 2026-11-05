package subnets

import (
	"context"
	"dfipam/api/internal/netcalc"
	"errors"
	"net/netip"

	"github.com/jackc/pgx/v5"
)

// Overlaps includes exception subnets, so a warning never hides a real clash.
func (Repository) Overlaps(ctx context.Context, q querier, routingDomainID, cidr string) ([]Conflict, error) {
	rows, err := q.Query(ctx, `SELECT id::text, name, cidr::text FROM subnets
		WHERE routing_domain_id = $1::uuid AND archived_at IS NULL AND cidr && $2::text::cidr
		ORDER BY masklen(cidr) DESC LIMIT 10`, routingDomainID, cidr)
	if err != nil {
		return nil, err
	}
	out, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (Conflict, error) {
		var c Conflict
		return c, r.Scan(&c.ID, &c.Name, &c.CIDR)
	})
	if out == nil {
		out = []Conflict{}
	}
	return out, err
}

// ReservedRanges loads reserved ranges for many subnets in one query.
func (Repository) ReservedRanges(ctx context.Context, q querier, ids []string) (map[string][]netcalc.Range, error) {
	rows, err := q.Query(ctx, `SELECT subnet_id::text, host(range_start), host(range_end) FROM subnet_reserved_ranges WHERE subnet_id = ANY($1::uuid[])`, ids)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string][]netcalc.Range{}
	for rows.Next() {
		var id, first, last string
		if err := rows.Scan(&id, &first, &last); err != nil {
			return nil, err
		}
		a, errA := netip.ParseAddr(first)
		b, errB := netip.ParseAddr(last)
		if errA != nil || errB != nil {
			continue
		}
		out[id] = append(out[id], netcalc.Range{First: a, Last: b})
	}
	return out, rows.Err()
}

// HeldAddresses returns addresses that can't be offered: records in a held state.
func (Repository) HeldAddresses(ctx context.Context, q querier, id string) ([]netip.Addr, error) {
	rows, err := q.Query(ctx, `SELECT host(address) FROM ip_addresses
		WHERE subnet_id = $1::uuid AND allocation_state IN ('allocated','reserved','quarantined','retired') ORDER BY address`, id)
	if err != nil {
		return nil, err
	}
	hosts, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return nil, err
	}
	out := make([]netip.Addr, 0, len(hosts))
	for _, h := range hosts {
		if a, err := netip.ParseAddr(h); err == nil {
			out = append(out, a)
		}
	}
	return out, nil
}

// DHCPRanges returns the subnet's DHCP pools.
func (Repository) DHCPRanges(ctx context.Context, q querier, id string) ([]netcalc.Range, error) {
	rows, err := q.Query(ctx, `SELECT host(range_start), host(range_end) FROM dhcp_scopes WHERE subnet_id = $1::uuid`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []netcalc.Range
	for rows.Next() {
		var first, last string
		if err := rows.Scan(&first, &last); err != nil {
			return nil, err
		}
		a, errA := netip.ParseAddr(first)
		b, errB := netip.ParseAddr(last)
		if errA == nil && errB == nil {
			out = append(out, netcalc.Range{First: a, Last: b})
		}
	}
	return out, rows.Err()
}

// Cell is a recorded address in a loaded range.
type Cell struct {
	ID, Address, AllocationState, ObservationState, Hostname string
}

// Cells loads only the records inside one range of the subnet.
func (Repository) Cells(ctx context.Context, q querier, id, first, last string) ([]Cell, error) {
	rows, err := q.Query(ctx, `SELECT id::text, host(address), allocation_state, observation_state, hostname FROM ip_addresses
		WHERE subnet_id = $1::uuid AND address BETWEEN $2::text::inet AND $3::text::inet`, id, first, last)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (Cell, error) {
		var c Cell
		return c, r.Scan(&c.ID, &c.Address, &c.AllocationState, &c.ObservationState, &c.Hostname)
	})
}

// MappingProblem reports a mapping that contradicts the subnet's own context.
func (Repository) MappingProblem(ctx context.Context, q querier, routingDomainID, cidr string, m Mappings) (string, string, error) {
	var field, issue string
	err := q.QueryRow(ctx, `
		SELECT field, issue FROM (
		  SELECT 'networkBlockId' field, 'The network block must be in the same routing domain and contain this subnet.' issue
		  FROM network_blocks b WHERE b.id = $3::uuid AND (b.routing_domain_id <> $1::uuid OR NOT $2::text::cidr <<= b.cidr)
		  UNION ALL
		  SELECT 'vdomId', 'This VDOM belongs to a different routing domain.'
		  FROM vdoms v WHERE v.id = $4::uuid AND v.routing_domain_id IS NOT NULL AND v.routing_domain_id <> $1::uuid
		  UNION ALL
		  SELECT 'vlanId', 'This VLAN belongs to a different site.'
		  FROM vlans l WHERE l.id = $5::uuid AND $6::uuid IS NOT NULL AND l.site_id IS NOT NULL AND l.site_id <> $6::uuid
		  UNION ALL
		  SELECT 'networkZoneId', 'This zone belongs to a different site.'
		  FROM network_zones z WHERE z.id = $7::uuid AND $6::uuid IS NOT NULL AND z.site_id IS NOT NULL AND z.site_id <> $6::uuid
		) p LIMIT 1`, routingDomainID, cidr, m.NetworkBlockID, m.VDOMID, m.VLANID, m.SiteID, m.NetworkZoneID).Scan(&field, &issue)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", "", nil
	}
	return field, issue, err
}
