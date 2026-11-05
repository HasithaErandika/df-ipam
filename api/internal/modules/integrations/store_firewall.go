package integrations

import (
	"context"
	"fmt"
	"net/netip"

	"github.com/jackc/pgx/v5"
)

func (s *store) firewall(ctx context.Context, r *Result) error {
	objects := map[string]string{}
	err := s.batched(ctx, len(r.AddressObjects), func(b *pgx.Batch, i int) { s.queueObject(b, r.AddressObjects[i], objects) })
	if err != nil {
		return err
	}
	groups := make([]string, len(r.AddressGroups))
	err = s.batched(ctx, len(r.AddressGroups), func(b *pgx.Batch, i int) { s.queueGroup(b, r.AddressGroups[i], &groups[i]) })
	if err != nil {
		return err
	}
	return s.batched(ctx, len(r.AddressGroups), func(b *pgx.Batch, i int) {
		if groups[i] != "" {
			s.queueMembers(b, r.AddressGroups[i], groups[i], objects)
		}
	})
}

func (s *store) queueObject(b *pgx.Batch, o AddressObject, objects map[string]string) {
	device, ok := s.devices[o.DeviceExternalID]
	if !ok {
		s.counts.Skipped++
		return
	}
	var network *string
	if p, err := netip.ParsePrefix(o.Network); err == nil {
		v := p.Masked().String()
		network = &v
	}
	b.Queue(`INSERT INTO firewall_address_objects (device_id, vdom_id, name, object_type, value, network, comment, last_seen)
		VALUES ($1::uuid, $2::uuid, $3, $4, $5, $6::text::cidr, $7, $8)
		ON CONFLICT (device_id, vdom_id, name) DO UPDATE SET object_type = EXCLUDED.object_type, value = EXCLUDED.value,
		  network = EXCLUDED.network, comment = EXCLUDED.comment, last_seen = EXCLUDED.last_seen
		RETURNING id::text`, device, s.vdomID(device, o.VDOM), truncate(o.Name, 255), truncate(o.Type, 50), truncate(o.Value, 500), network, truncate(o.Comment, 500), s.now).QueryRow(func(row pgx.Row) error {
		var id string
		if err := row.Scan(&id); err != nil {
			return fmt.Errorf("address object %s: %w", o.Name, err)
		}
		objects[device+"/"+o.VDOM+"/"+o.Name] = id
		s.counts.AddressObjects++
		return nil
	})
}

func (s *store) queueGroup(b *pgx.Batch, g AddressGroup, group *string) {
	device, ok := s.devices[g.DeviceExternalID]
	if !ok {
		s.counts.Skipped++
		return
	}
	b.Queue(`INSERT INTO firewall_address_groups (device_id, vdom_id, name, comment, last_seen)
		VALUES ($1::uuid, $2::uuid, $3, $4, $5)
		ON CONFLICT (device_id, vdom_id, name) DO UPDATE SET comment = EXCLUDED.comment, last_seen = EXCLUDED.last_seen
		RETURNING id::text`, device, s.vdomID(device, g.VDOM), truncate(g.Name, 255), truncate(g.Comment, 500), s.now).QueryRow(func(row pgx.Row) error {
		if err := row.Scan(group); err != nil {
			return fmt.Errorf("address group %s: %w", g.Name, err)
		}
		s.counts.AddressGroups++
		return nil
	})
}

// queueMembers replaces a group's members. A member that isn't listed is a nested
// group or an object from another VDOM.
func (s *store) queueMembers(b *pgx.Batch, g AddressGroup, group string, objects map[string]string) {
	device := s.devices[g.DeviceExternalID]
	var members []string
	for _, m := range g.Members {
		if object, ok := objects[device+"/"+g.VDOM+"/"+m]; ok {
			members = append(members, object)
		}
	}
	b.Queue(`DELETE FROM firewall_address_group_members WHERE group_id = $1::uuid`, group)
	if len(members) > 0 {
		b.Queue(`INSERT INTO firewall_address_group_members (group_id, object_id)
			SELECT $1::uuid, object FROM unnest($2::uuid[]) AS t(object) ON CONFLICT DO NOTHING`, group, members)
	}
}
