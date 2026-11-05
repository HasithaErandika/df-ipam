package integrations

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// saveDevices stores devices first, then what hangs off them: VDOMs and mappings,
// then interfaces, which refer to the VDOMs.
func (s *store) saveDevices(ctx context.Context, list []Device) error {
	if err := s.batched(ctx, len(list), func(b *pgx.Batch, i int) { s.queueDevice(b, list[i]) }); err != nil {
		return err
	}
	err := s.batched(ctx, len(list), func(b *pgx.Batch, i int) {
		id := s.devices[list[i].ExternalID]
		s.queueMapping(b, "device", list[i].ExternalID, "device", id)
		for _, v := range list[i].VDOMs {
			s.queueVDOM(b, id, v)
		}
	})
	if err != nil {
		return err
	}
	return s.batched(ctx, len(list), func(b *pgx.Batch, i int) {
		for _, f := range list[i].Interfaces {
			s.queueInterface(b, s.devices[list[i].ExternalID], f)
		}
	})
}

func (s *store) queueDevice(b *pgx.Batch, d Device) {
	var mgmt *string
	if a, ok := validAddr(d.ManagementIP); ok {
		mgmt = &a
	}
	b.Queue(`INSERT INTO devices (name, hostname, management_ip, vendor, model, serial_number, device_type, os_version, os_family,
		  source_system, external_id, first_discovered, last_discovered, status)
		VALUES ($1, $2, $3::text::inet, $4, $5, $6, $7, $8, $9, $10, $11, $12, $12, 'active')
		ON CONFLICT (source_system, external_id) DO UPDATE SET name = EXCLUDED.name, hostname = EXCLUDED.hostname,
		  management_ip = COALESCE(EXCLUDED.management_ip, devices.management_ip), vendor = EXCLUDED.vendor, model = EXCLUDED.model,
		  serial_number = COALESCE(EXCLUDED.serial_number, devices.serial_number), os_version = EXCLUDED.os_version,
		  os_family = EXCLUDED.os_family, last_discovered = EXCLUDED.last_discovered
		RETURNING id::text`,
		truncate(d.Name, 255), truncate(d.Hostname, 255), mgmt, truncate(d.Vendor, 100), truncate(d.Model, 100), nullText(truncate(d.Serial, 100)),
		oneOf(d.DeviceType, deviceTypes, "other"), truncate(d.OSVersion, 255), oneOf(d.OSFamily, osFamilies, "unknown"),
		s.kind, s.externalID(d.ExternalID), s.now).QueryRow(func(row pgx.Row) error {
		var id string
		if err := row.Scan(&id); err != nil {
			return fmt.Errorf("device %s: %w", d.ExternalID, err)
		}
		s.devices[d.ExternalID] = id
		s.counts.Devices++
		return nil
	})
}

// queueVDOM stores a VDOM. A mapping fills an empty routing domain. A person's
// choice is never replaced.
func (s *store) queueVDOM(b *pgx.Batch, device string, v VDOM) {
	b.Queue(`INSERT INTO vdoms (device_id, name, routing_domain_id) VALUES ($1::uuid, $2, $3::uuid)
		ON CONFLICT (device_id, name) DO UPDATE SET routing_domain_id = COALESCE(vdoms.routing_domain_id, EXCLUDED.routing_domain_id)
		RETURNING id::text`, device, truncate(v.Name, 100), s.domain(v.Context)).QueryRow(func(row pgx.Row) error {
		var vdom string
		if err := row.Scan(&vdom); err != nil {
			return fmt.Errorf("vdom %s: %w", v.Name, err)
		}
		s.vdoms[device+"/"+v.Name] = vdom
		return nil
	})
}

func (s *store) queueInterface(b *pgx.Batch, device string, i Interface) {
	b.Queue(`INSERT INTO device_interfaces (device_id, name, mac, interface_type, admin_status, oper_status, vdom_id,
		  routing_domain_id, description, source, last_observed)
		VALUES ($1::uuid, $2, $3::macaddr, $4, $5, $6, $7::uuid, $8::uuid, $9, $10, $11)
		ON CONFLICT (device_id, vdom_id, name) DO UPDATE SET mac = EXCLUDED.mac, interface_type = EXCLUDED.interface_type,
		  admin_status = EXCLUDED.admin_status, oper_status = EXCLUDED.oper_status,
		  routing_domain_id = COALESCE(device_interfaces.routing_domain_id, EXCLUDED.routing_domain_id),
		  description = EXCLUDED.description, source = EXCLUDED.source, last_observed = EXCLUDED.last_observed`,
		device, truncate(i.Name, 255), validMAC(i.MAC), truncate(i.Type, 100), status(i.AdminStatus), status(i.OperStatus),
		nullText(s.vdoms[device+"/"+i.VDOM]), s.domain(i.Context), truncate(i.Description, 500), s.kind, s.now)
	s.counts.Interfaces++
}
