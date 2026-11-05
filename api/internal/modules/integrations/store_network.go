package integrations

import (
	"context"
	"errors"
	"fmt"
	"net/netip"

	"github.com/jackc/pgx/v5"
)

func (s *store) cloud(ctx context.Context, r *Result) error {
	accounts := map[string]string{}
	err := s.batched(ctx, len(r.Accounts), func(b *pgx.Batch, i int) { s.queueAccount(b, r.Accounts[i], accounts) })
	if err != nil {
		return err
	}
	err = s.batched(ctx, len(r.Accounts), func(b *pgx.Batch, i int) {
		a := r.Accounts[i]
		s.queueMapping(b, "account", a.ExternalID, "cloud_account", accounts[a.ExternalID])
	})
	if err != nil {
		return err
	}
	networks := map[string]string{}
	err = s.batched(ctx, len(r.Networks), func(b *pgx.Batch, i int) { s.queueNetwork(b, r.Networks[i], accounts, networks) })
	if err != nil {
		return err
	}
	return s.batched(ctx, len(r.Networks), func(b *pgx.Batch, i int) {
		n := r.Networks[i]
		if id, ok := networks[n.ExternalID]; ok {
			s.queueMapping(b, "network", n.ExternalID, "cloud_network", id)
		}
	})
}

func (s *store) queueAccount(b *pgx.Batch, a CloudAccount, accounts map[string]string) {
	b.Queue(`INSERT INTO cloud_accounts (connection_id, provider, external_account_id, tenant_id, name, last_seen)
		VALUES ($1::uuid, $2, $3, $4, $5, $6)
		ON CONFLICT (connection_id, external_account_id) DO UPDATE SET tenant_id = EXCLUDED.tenant_id, name = EXCLUDED.name, last_seen = EXCLUDED.last_seen
		RETURNING id::text`, s.conn, a.Provider, a.ExternalID, nullText(a.TenantID), truncate(a.Name, 255), s.now).QueryRow(func(row pgx.Row) error {
		var id string
		if err := row.Scan(&id); err != nil {
			return fmt.Errorf("cloud account: %w", err)
		}
		accounts[a.ExternalID] = id
		return nil
	})
}

func (s *store) queueNetwork(b *pgx.Batch, n CloudNetwork, accounts, networks map[string]string) {
	account, ok := accounts[n.AccountExternalID]
	if !ok {
		s.counts.Skipped++
		return
	}
	var spaces []string
	for _, c := range n.AddressSpaces {
		if p, err := netip.ParsePrefix(c); err == nil {
			spaces = append(spaces, p.Masked().String())
		}
	}
	b.Queue(`INSERT INTO cloud_networks (cloud_account_id, routing_domain_id, region, resource_group, external_id, name, address_spaces, last_seen)
		VALUES ($1::uuid, $2::uuid, $3, $4, $5, $6, $7::cidr[], $8)
		ON CONFLICT (cloud_account_id, external_id) DO UPDATE SET routing_domain_id = EXCLUDED.routing_domain_id, region = EXCLUDED.region,
		  resource_group = EXCLUDED.resource_group, name = EXCLUDED.name, address_spaces = EXCLUDED.address_spaces, last_seen = EXCLUDED.last_seen
		RETURNING id::text`, account, s.domain(n.ExternalID), truncate(n.Region, 100), nullText(n.ResourceGroup), n.ExternalID, truncate(n.Name, 255), spaces, s.now).QueryRow(func(row pgx.Row) error {
		var id string
		if err := row.Scan(&id); err != nil {
			return fmt.Errorf("cloud network: %w", err)
		}
		networks[n.ExternalID] = id
		s.counts.Networks++
		return nil
	})
}

// peerings records links between mapped networks. Domains are never merged.
func (s *store) peerings(ctx context.Context, items []Peering, source string) error {
	err := s.batched(ctx, len(items), func(b *pgx.Batch, i int) {
		p := items[i]
		x, y := s.domain(p.ContextA), s.domain(p.ContextB)
		if x == nil || y == nil || *x == *y {
			return
		}
		lo, hi := *x, *y
		if hi < lo {
			lo, hi = hi, lo
		}
		b.Queue(`INSERT INTO routing_domain_peerings (domain_a_id, domain_b_id, kind, source_id, last_seen)
			VALUES ($1::uuid, $2::uuid, $3, $4::uuid, $5)
			ON CONFLICT (domain_a_id, domain_b_id, kind) DO UPDATE SET last_seen = EXCLUDED.last_seen`,
			lo, hi, p.Kind, nullText(source), s.now)
	})
	if err != nil {
		return fmt.Errorf("peering: %w", err)
	}
	return nil
}

// routes replaces what this device or domain reported last time, because a route
// that has gone must not linger. Rows from other sources stay.
func (s *store) routes(ctx context.Context, items []Route, source string) error {
	var devices []string
	seen := map[string]bool{}
	for _, r := range items {
		if d, ok := s.devices[r.DeviceExternalID]; ok && !seen[d] {
			seen[d] = true
			devices = append(devices, d)
		}
	}
	if len(devices) > 0 {
		if _, err := s.tx.Exec(ctx, `DELETE FROM network_routes WHERE device_id = ANY($1::uuid[]) AND source_id = $2::uuid`, devices, source); err != nil {
			return err
		}
	}
	err := s.batched(ctx, len(items), func(b *pgx.Batch, i int) { s.queueRoute(b, items[i], source) })
	if err != nil {
		return fmt.Errorf("route: %w", err)
	}
	return nil
}

func (s *store) queueRoute(b *pgx.Batch, r Route, source string) {
	p, err := netip.ParsePrefix(r.Destination)
	device, ok := s.devices[r.DeviceExternalID]
	if err != nil || !ok {
		s.counts.Skipped++
		return
	}
	var hop *string
	if a, ok := validAddr(r.NextHop); ok {
		hop = &a
	}
	b.Queue(`INSERT INTO network_routes (routing_domain_id, device_id, vdom_id, destination, next_hop, route_type, source_id, last_seen)
		VALUES ($1::uuid, $2::uuid, $3::uuid, $4::cidr, $5::text::inet, $6, $7::uuid, $8)`,
		s.domain(r.Context), device, s.vdomID(device, r.VDOM), p.Masked().String(), hop, truncate(r.Type, 50), source, s.now)
	s.counts.Routes++
}

// scopes stores DHCP scopes that fall inside a subnet already in the mapped domain.
func (s *store) scopes(ctx context.Context, items []DHCPScope, source string) error {
	subnets := make([]string, len(items))
	err := s.batched(ctx, len(items), func(b *pgx.Batch, i int) { s.queueSubnetOf(b, items[i], &subnets[i]) })
	if err != nil {
		return err
	}
	return s.batched(ctx, len(items), func(b *pgx.Batch, i int) {
		if subnets[i] != "" {
			s.queueScope(b, items[i], subnets[i], source)
		}
	})
}

// queueSubnetOf finds the smallest subnet holding a scope. A scope with nowhere to
// go is skipped.
func (s *store) queueSubnetOf(b *pgx.Batch, d DHCPScope, subnet *string) {
	domain := s.domain(d.Context)
	start, ok1 := validAddr(d.RangeStart)
	end, ok2 := validAddr(d.RangeEnd)
	if domain == nil || !ok1 || !ok2 {
		s.counts.Skipped++
		return
	}
	b.Queue(`SELECT id::text FROM subnets WHERE routing_domain_id = $1::uuid AND archived_at IS NULL
		AND cidr >>= $2::inet AND cidr >>= $3::inet ORDER BY masklen(cidr) DESC LIMIT 1`, *domain, start, end).QueryRow(func(row pgx.Row) error {
		err := row.Scan(subnet)
		if errors.Is(err, pgx.ErrNoRows) {
			s.counts.Skipped++
			return nil
		}
		return err
	})
}

// queueScope updates the scope, or inserts it if the update found nothing. It is
// one statement so that a scope listed twice lands on the same row.
func (s *store) queueScope(b *pgx.Batch, d DHCPScope, subnet, source string) {
	start, _ := validAddr(d.RangeStart)
	end, _ := validAddr(d.RangeEnd)
	b.Queue(`WITH changed AS (
		  UPDATE dhcp_scopes SET name = $4, server_device_id = $5::uuid, source_id = $6::uuid, last_seen = $7
		  WHERE subnet_id = $1::uuid AND range_start = $2::inet AND range_end = $3::inet RETURNING 1)
		INSERT INTO dhcp_scopes (subnet_id, name, range_start, range_end, server_device_id, source_id, last_seen)
		SELECT $1::uuid, $4, $2::inet, $3::inet, $5::uuid, $6::uuid, $7 WHERE NOT EXISTS (SELECT 1 FROM changed)`,
		subnet, start, end, truncate(d.Name, 255), nullText(s.devices[d.DeviceExternalID]), source, s.now)
	s.counts.DHCPScopes++
}
