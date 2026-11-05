package integrations

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/netip"
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

func nullText(v string) *string {
	if v == "" {
		return nil
	}
	return &v
}

// externalID scopes a platform's identifier to this connection, because two
// vCenters can each have a vm-101.
func (s *store) externalID(id string) string { return s.conn + ":" + id }

func validMAC(v string) *string {
	hw, err := net.ParseMAC(v)
	if err != nil {
		return nil
	}
	out := hw.String()
	return &out
}

func validAddr(v string) (string, bool) {
	a, err := netip.ParseAddr(v)
	if err != nil {
		return "", false
	}
	return a.Unmap().WithZone("").String(), true
}

func oneOf(v string, allowed []string, fallback string) string {
	if slices.Contains(allowed, v) {
		return v
	}
	return fallback
}

var (
	deviceTypes = []string{"firewall", "router", "switch", "physical_server", "virtual_machine", "load_balancer",
		"network_appliance", "cloud_interface", "other"}
	osFamilies = []string{"unknown", "windows", "windows_server", "linux", "macos", "network_os", "hypervisor", "other"}
)

func (s *store) mapping(ctx context.Context, resource, external, object, id string) error {
	_, err := s.tx.Exec(ctx, `INSERT INTO external_resource_mappings (connection_id, resource_type, external_id, object_type, object_id, last_seen)
		VALUES ($1::uuid, $2, $3, $4, $5::uuid, $6)
		ON CONFLICT (connection_id, resource_type, external_id) DO UPDATE SET object_id = EXCLUDED.object_id, last_seen = EXCLUDED.last_seen`,
		s.conn, resource, external, object, id, s.now)
	return err
}

func (s *store) device(ctx context.Context, d Device) error {
	var mgmt *string
	if a, ok := validAddr(d.ManagementIP); ok {
		mgmt = &a
	}
	var id string
	err := s.tx.QueryRow(ctx, `INSERT INTO devices (name, hostname, management_ip, vendor, model, serial_number, device_type, os_version, os_family,
		  source_system, external_id, first_discovered, last_discovered, status)
		VALUES ($1, $2, $3::text::inet, $4, $5, $6, $7, $8, $9, $10, $11, $12, $12, 'active')
		ON CONFLICT (source_system, external_id) DO UPDATE SET name = EXCLUDED.name, hostname = EXCLUDED.hostname,
		  management_ip = COALESCE(EXCLUDED.management_ip, devices.management_ip), vendor = EXCLUDED.vendor, model = EXCLUDED.model,
		  serial_number = COALESCE(EXCLUDED.serial_number, devices.serial_number), os_version = EXCLUDED.os_version,
		  os_family = EXCLUDED.os_family, last_discovered = EXCLUDED.last_discovered
		RETURNING id::text`,
		truncate(d.Name, 255), truncate(d.Hostname, 255), mgmt, truncate(d.Vendor, 100), truncate(d.Model, 100), nullText(truncate(d.Serial, 100)),
		oneOf(d.DeviceType, deviceTypes, "other"), truncate(d.OSVersion, 255), oneOf(d.OSFamily, osFamilies, "unknown"),
		s.kind, s.externalID(d.ExternalID), s.now).Scan(&id)
	if err != nil {
		return fmt.Errorf("device %s: %w", d.ExternalID, err)
	}
	s.devices[d.ExternalID] = id
	s.counts.Devices++
	if err := s.mapping(ctx, "device", d.ExternalID, "device", id); err != nil {
		return err
	}
	for _, v := range d.VDOMs {
		var vdom string
		// A mapping fills an empty routing domain. A person's choice is never replaced.
		err := s.tx.QueryRow(ctx, `INSERT INTO vdoms (device_id, name, routing_domain_id) VALUES ($1::uuid, $2, $3::uuid)
			ON CONFLICT (device_id, name) DO UPDATE SET routing_domain_id = COALESCE(vdoms.routing_domain_id, EXCLUDED.routing_domain_id)
			RETURNING id::text`, id, truncate(v.Name, 100), s.domain(v.Context)).Scan(&vdom)
		if err != nil {
			return fmt.Errorf("vdom %s: %w", v.Name, err)
		}
		s.vdoms[id+"/"+v.Name] = vdom
	}
	for _, i := range d.Interfaces {
		_, err := s.tx.Exec(ctx, `INSERT INTO device_interfaces (device_id, name, mac, interface_type, admin_status, oper_status, vdom_id,
			  routing_domain_id, description, source, last_observed)
			VALUES ($1::uuid, $2, $3::macaddr, $4, $5, $6, $7::uuid, $8::uuid, $9, $10, $11)
			ON CONFLICT (device_id, vdom_id, name) DO UPDATE SET mac = EXCLUDED.mac, interface_type = EXCLUDED.interface_type,
			  admin_status = EXCLUDED.admin_status, oper_status = EXCLUDED.oper_status,
			  routing_domain_id = COALESCE(device_interfaces.routing_domain_id, EXCLUDED.routing_domain_id),
			  description = EXCLUDED.description, source = EXCLUDED.source, last_observed = EXCLUDED.last_observed`,
			id, truncate(i.Name, 255), validMAC(i.MAC), truncate(i.Type, 100), status(i.AdminStatus), status(i.OperStatus),
			nullText(s.vdoms[id+"/"+i.VDOM]), s.domain(i.Context), truncate(i.Description, 500), s.kind, s.now)
		if err != nil {
			return fmt.Errorf("interface %s: %w", i.Name, err)
		}
		s.counts.Interfaces++
	}
	return nil
}

func status(v string) string {
	if v == "" {
		return "unknown"
	}
	return truncate(v, 50)
}

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

func payloadJSON(p map[string]any) []byte {
	if len(p) == 0 {
		return []byte(`{}`)
	}
	raw, err := json.Marshal(p)
	if err != nil || len(raw) > maxPayload {
		return []byte(`{}`)
	}
	return raw
}

const chunk = 1000

// observations appends evidence, then refreshes the observation fields of records
// that already exist. Allocation state and ownership are never touched, and no
// record is created: an unknown address becomes a finding for a person to review.
func (s *store) observations(ctx context.Context, items []Observation) error {
	sources, err := s.sources(ctx)
	if err != nil {
		return err
	}
	type seen struct {
		domain, source string
		at             time.Time
	}
	alive := map[string]seen{}
	for start := 0; start < len(items); start += chunk {
		batch := &pgx.Batch{}
		for _, o := range items[start:min(start+chunk, len(items))] {
			source, known := sources[o.Source]
			var addr *string
			if a, ok := validAddr(o.Address); ok {
				addr = &a
			}
			if !known || (addr == nil && o.MAC == "" && o.Hostname == "") {
				s.counts.Skipped++
				continue
			}
			at := o.ObservedAt
			if at.IsZero() || at.After(s.now) {
				at = s.now
			}
			domain := s.domain(o.Context)
			if domain == nil {
				s.counts.Unmapped++
			}
			batch.Queue(`INSERT INTO discovery_observations (sync_job_id, source_id, routing_domain_id, address, mac, hostname, payload, observed_at)
				VALUES ($1::uuid, $2::uuid, $3::uuid, $4::text::inet, $5::macaddr, $6, $7::jsonb, $8)`,
				s.jobID, source, domain, addr, validMAC(o.MAC), nullText(truncate(o.Hostname, 255)), payloadJSON(o.Payload), at)
			s.counts.Observations++
			if o.Alive && domain != nil && addr != nil {
				key := *domain + "|" + *addr
				if prev, ok := alive[key]; !ok || at.After(prev.at) {
					alive[key] = seen{*domain, source, at}
				}
			}
		}
		if batch.Len() > 0 {
			if err := s.tx.SendBatch(ctx, batch).Close(); err != nil {
				return fmt.Errorf("observations: %w", err)
			}
		}
	}
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

func (s *store) cloud(ctx context.Context, r *Result) error {
	accounts := map[string]string{}
	for _, a := range r.Accounts {
		var id string
		err := s.tx.QueryRow(ctx, `INSERT INTO cloud_accounts (connection_id, provider, external_account_id, tenant_id, name, last_seen)
			VALUES ($1::uuid, $2, $3, $4, $5, $6)
			ON CONFLICT (connection_id, external_account_id) DO UPDATE SET tenant_id = EXCLUDED.tenant_id, name = EXCLUDED.name, last_seen = EXCLUDED.last_seen
			RETURNING id::text`, s.conn, a.Provider, a.ExternalID, nullText(a.TenantID), truncate(a.Name, 255), s.now).Scan(&id)
		if err != nil {
			return fmt.Errorf("cloud account: %w", err)
		}
		accounts[a.ExternalID] = id
		if err := s.mapping(ctx, "account", a.ExternalID, "cloud_account", id); err != nil {
			return err
		}
	}
	for _, n := range r.Networks {
		account, ok := accounts[n.AccountExternalID]
		if !ok {
			s.counts.Skipped++
			continue
		}
		var spaces []string
		for _, c := range n.AddressSpaces {
			if p, err := netip.ParsePrefix(c); err == nil {
				spaces = append(spaces, p.Masked().String())
			}
		}
		var id string
		err := s.tx.QueryRow(ctx, `INSERT INTO cloud_networks (cloud_account_id, routing_domain_id, region, resource_group, external_id, name, address_spaces, last_seen)
			VALUES ($1::uuid, $2::uuid, $3, $4, $5, $6, $7::cidr[], $8)
			ON CONFLICT (cloud_account_id, external_id) DO UPDATE SET routing_domain_id = EXCLUDED.routing_domain_id, region = EXCLUDED.region,
			  resource_group = EXCLUDED.resource_group, name = EXCLUDED.name, address_spaces = EXCLUDED.address_spaces, last_seen = EXCLUDED.last_seen
			RETURNING id::text`, account, s.domain(n.ExternalID), truncate(n.Region, 100), nullText(n.ResourceGroup), n.ExternalID, truncate(n.Name, 255), spaces, s.now).Scan(&id)
		if err != nil {
			return fmt.Errorf("cloud network: %w", err)
		}
		s.counts.Networks++
		if err := s.mapping(ctx, "network", n.ExternalID, "cloud_network", id); err != nil {
			return err
		}
	}
	return nil
}

// peerings records links between mapped networks. Domains are never merged.
func (s *store) peerings(ctx context.Context, items []Peering, source string) error {
	for _, p := range items {
		a, b := s.domain(p.ContextA), s.domain(p.ContextB)
		if a == nil || b == nil || *a == *b {
			continue
		}
		lo, hi := *a, *b
		if hi < lo {
			lo, hi = hi, lo
		}
		if _, err := s.tx.Exec(ctx, `INSERT INTO routing_domain_peerings (domain_a_id, domain_b_id, kind, source_id, last_seen)
			VALUES ($1::uuid, $2::uuid, $3, $4::uuid, $5)
			ON CONFLICT (domain_a_id, domain_b_id, kind) DO UPDATE SET last_seen = EXCLUDED.last_seen`,
			lo, hi, p.Kind, nullText(source), s.now); err != nil {
			return fmt.Errorf("peering: %w", err)
		}
	}
	return nil
}

func (s *store) vdomID(device, vdom string) *string {
	if vdom == "" {
		return nil
	}
	return nullText(s.vdoms[device+"/"+vdom])
}

func (s *store) firewall(ctx context.Context, r *Result) error {
	objects := map[string]string{}
	for _, o := range r.AddressObjects {
		device, ok := s.devices[o.DeviceExternalID]
		if !ok {
			s.counts.Skipped++
			continue
		}
		var network *string
		if p, err := netip.ParsePrefix(o.Network); err == nil {
			v := p.Masked().String()
			network = &v
		}
		var id string
		err := s.tx.QueryRow(ctx, `INSERT INTO firewall_address_objects (device_id, vdom_id, name, object_type, value, network, comment, last_seen)
			VALUES ($1::uuid, $2::uuid, $3, $4, $5, $6::text::cidr, $7, $8)
			ON CONFLICT (device_id, vdom_id, name) DO UPDATE SET object_type = EXCLUDED.object_type, value = EXCLUDED.value,
			  network = EXCLUDED.network, comment = EXCLUDED.comment, last_seen = EXCLUDED.last_seen
			RETURNING id::text`, device, s.vdomID(device, o.VDOM), truncate(o.Name, 255), truncate(o.Type, 50), truncate(o.Value, 500), network, truncate(o.Comment, 500), s.now).Scan(&id)
		if err != nil {
			return fmt.Errorf("address object %s: %w", o.Name, err)
		}
		objects[device+"/"+o.VDOM+"/"+o.Name] = id
		s.counts.AddressObjects++
	}
	for _, g := range r.AddressGroups {
		device, ok := s.devices[g.DeviceExternalID]
		if !ok {
			s.counts.Skipped++
			continue
		}
		var id string
		err := s.tx.QueryRow(ctx, `INSERT INTO firewall_address_groups (device_id, vdom_id, name, comment, last_seen)
			VALUES ($1::uuid, $2::uuid, $3, $4, $5)
			ON CONFLICT (device_id, vdom_id, name) DO UPDATE SET comment = EXCLUDED.comment, last_seen = EXCLUDED.last_seen
			RETURNING id::text`, device, s.vdomID(device, g.VDOM), truncate(g.Name, 255), truncate(g.Comment, 500), s.now).Scan(&id)
		if err != nil {
			return fmt.Errorf("address group %s: %w", g.Name, err)
		}
		s.counts.AddressGroups++
		if _, err := s.tx.Exec(ctx, `DELETE FROM firewall_address_group_members WHERE group_id = $1::uuid`, id); err != nil {
			return err
		}
		for _, m := range g.Members {
			object, ok := objects[device+"/"+g.VDOM+"/"+m]
			if !ok {
				continue // a nested group or an object from another VDOM
			}
			if _, err := s.tx.Exec(ctx, `INSERT INTO firewall_address_group_members (group_id, object_id) VALUES ($1::uuid, $2::uuid) ON CONFLICT DO NOTHING`, id, object); err != nil {
				return err
			}
		}
	}
	return nil
}

// routes replaces what this device or domain reported last time, because a route
// that has gone must not linger. Rows from other sources stay.
func (s *store) routes(ctx context.Context, items []Route, source string) error {
	devices := map[string]bool{}
	for _, r := range items {
		if d, ok := s.devices[r.DeviceExternalID]; ok && !devices[d] {
			devices[d] = true
			if _, err := s.tx.Exec(ctx, `DELETE FROM network_routes WHERE device_id = $1::uuid AND source_id = $2::uuid`, d, source); err != nil {
				return err
			}
		}
	}
	for _, r := range items {
		p, err := netip.ParsePrefix(r.Destination)
		device, ok := s.devices[r.DeviceExternalID]
		if err != nil || !ok {
			s.counts.Skipped++
			continue
		}
		var hop *string
		if a, ok := validAddr(r.NextHop); ok {
			hop = &a
		}
		if _, err := s.tx.Exec(ctx, `INSERT INTO network_routes (routing_domain_id, device_id, vdom_id, destination, next_hop, route_type, source_id, last_seen)
			VALUES ($1::uuid, $2::uuid, $3::uuid, $4::cidr, $5::text::inet, $6, $7::uuid, $8)`,
			s.domain(r.Context), device, s.vdomID(device, r.VDOM), p.Masked().String(), hop, truncate(r.Type, 50), source, s.now); err != nil {
			return fmt.Errorf("route: %w", err)
		}
		s.counts.Routes++
	}
	return nil
}

// scopes stores DHCP scopes that fall inside a subnet already in the mapped domain.
func (s *store) scopes(ctx context.Context, items []DHCPScope, source string) error {
	for _, d := range items {
		domain := s.domain(d.Context)
		start, ok1 := validAddr(d.RangeStart)
		end, ok2 := validAddr(d.RangeEnd)
		if domain == nil || !ok1 || !ok2 {
			s.counts.Skipped++
			continue
		}
		var subnet string
		err := s.tx.QueryRow(ctx, `SELECT id::text FROM subnets WHERE routing_domain_id = $1::uuid AND archived_at IS NULL
			AND cidr >>= $2::inet AND cidr >>= $3::inet ORDER BY masklen(cidr) DESC LIMIT 1`, *domain, start, end).Scan(&subnet)
		if err == pgx.ErrNoRows {
			s.counts.Skipped++
			continue
		}
		if err != nil {
			return err
		}
		device := s.devices[d.DeviceExternalID]
		tag, err := s.tx.Exec(ctx, `UPDATE dhcp_scopes SET name = $4, server_device_id = $5::uuid, source_id = $6::uuid, last_seen = $7
			WHERE subnet_id = $1::uuid AND range_start = $2::inet AND range_end = $3::inet`, subnet, start, end, truncate(d.Name, 255), nullText(device), source, s.now)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			if _, err := s.tx.Exec(ctx, `INSERT INTO dhcp_scopes (subnet_id, name, range_start, range_end, server_device_id, source_id, last_seen)
				VALUES ($1::uuid, $2, $3::inet, $4::inet, $5::uuid, $6::uuid, $7)`, subnet, truncate(d.Name, 255), start, end, nullText(device), source, s.now); err != nil {
				return err
			}
		}
		s.counts.DHCPScopes++
	}
	return nil
}

// sourceFor names the evidence source of a connection kind.
func sourceFor(kind string) string {
	return map[string]string{KindFortiGate: SourceFortiGate, KindVCenter: SourceVMware, KindAWS: SourceAWS, KindAzure: SourceAzure}[kind]
}

// save writes the whole result in the caller's transaction.
func save(ctx context.Context, tx pgx.Tx, connID, kind, jobID string, domains map[string]string, r *Result, now time.Time) (Counts, error) {
	s := &store{tx: tx, conn: connID, kind: kind, domains: domains, jobID: jobID, now: now, devices: map[string]string{}, vdoms: map[string]string{}}
	for _, d := range r.Devices {
		if err := s.device(ctx, d); err != nil {
			return s.counts, err
		}
	}
	if err := s.cloud(ctx, r); err != nil {
		return s.counts, err
	}
	if err := s.observations(ctx, r.Observations); err != nil {
		return s.counts, err
	}
	sources, err := s.sources(ctx)
	if err != nil {
		return s.counts, err
	}
	source := sources[sourceFor(kind)]
	if err := s.firewall(ctx, r); err != nil {
		return s.counts, err
	}
	if err := s.routes(ctx, r.Routes, source); err != nil {
		return s.counts, err
	}
	if err := s.peerings(ctx, r.Peerings, source); err != nil {
		return s.counts, err
	}
	if err := s.scopes(ctx, r.DHCPScopes, sources[SourceDHCP]); err != nil {
		return s.counts, err
	}
	s.counts.Warnings = len(r.Warnings)
	return s.counts, nil
}
