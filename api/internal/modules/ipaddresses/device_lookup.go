package ipaddresses

import (
	"context"
	"dfipam/api/internal/platform/httpx"
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
)

// addressKeys is what identifies a record's device and evidence.
type addressKeys struct {
	id, address, routingDomain string
	device, iface, mac         *string
}

func (h *Handler) device(c *gin.Context) {
	id, ok := httpx.PathID(c)
	if !ok {
		return
	}
	ctx := c.Request.Context()
	k := addressKeys{id: id}
	err := h.conn.QueryRow(ctx, `SELECT host(address), routing_domain_id::text, device_id::text, interface_id::text, mac::text
		FROM ip_addresses WHERE id = $1::uuid`, id).Scan(&k.address, &k.routingDomain, &k.device, &k.iface, &k.mac)
	if errors.Is(err, pgx.ErrNoRows) {
		httpx.Fail(c, httpx.ErrNotFound)
		return
	}
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	out, err := h.deviceInfo(ctx, k)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, http.StatusOK, out)
}

func (h *Handler) deviceInfo(ctx context.Context, k addressKeys) (DeviceInfo, error) {
	out := DeviceInfo{Address: k.address}
	var err error
	if out.Device, err = h.deviceRecord(ctx, k); err != nil {
		return out, err
	}
	if out.Interface, err = h.interfaceInfo(ctx, k.iface); err != nil {
		return out, err
	}
	mac := k.mac
	if mac == nil && out.Interface != nil {
		mac = out.Interface.MAC
	}
	if mac != nil {
		out.MAC = DescribeMAC(*mac)
	}
	if out.DNS, err = h.dnsNames(ctx, k); err != nil {
		return out, err
	}
	if out.DHCP, err = h.dhcpLeases(ctx, k); err != nil {
		return out, err
	}
	if out.Sources, err = h.sourceSummaries(ctx, k); err != nil {
		return out, err
	}
	out.HasEvidence = len(out.Sources) > 0
	return out, nil
}

func (h *Handler) deviceRecord(ctx context.Context, k addressKeys) (*DeviceRecord, error) {
	if k.device == nil {
		return nil, nil
	}
	var d DeviceRecord
	err := h.conn.QueryRow(ctx, `SELECT d.id::text, d.name, d.hostname, d.device_type, d.vendor, d.model, d.serial_number, d.os_version, d.os_family,
		host(d.management_ip), d.status, d.sensitivity, st.name, d.source_system, d.last_discovered, d.last_verified,
		(SELECT count(*) FROM ip_addresses x WHERE x.device_id = d.id AND x.id <> $2::uuid), bs.name
		FROM devices d LEFT JOIN sites st ON st.id = d.site_id LEFT JOIN business_services bs ON bs.id = d.business_service_id
		WHERE d.id = $1::uuid`, *k.device, k.id).Scan(&d.ID, &d.Name, &d.Hostname, &d.DeviceType, &d.Vendor, &d.Model, &d.SerialNumber,
		&d.OSVersion, &d.OSFamily, &d.ManagementIP, &d.Status, &d.Sensitivity, &d.SiteName, &d.SourceSystem, &d.LastDiscovered, &d.LastVerified,
		&d.OtherAddresses, &d.BusinessService)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &d, nil
}

func (h *Handler) interfaceInfo(ctx context.Context, id *string) (*InterfaceInfo, error) {
	if id == nil {
		return nil, nil
	}
	var f InterfaceInfo
	err := h.conn.QueryRow(ctx, `SELECT di.name, di.mac::text, di.interface_type, di.admin_status, di.oper_status,
		(SELECT v.vlan_id::text || ' ' || v.name FROM vlans v WHERE v.id = di.vlan_id), (SELECT vd.name FROM vdoms vd WHERE vd.id = di.vdom_id),
		di.source, di.last_observed FROM device_interfaces di WHERE di.id = $1::uuid`, *id).
		Scan(&f.Name, &f.MAC, &f.Type, &f.AdminStatus, &f.OperStatus, &f.VLAN, &f.VDOM, &f.Source, &f.LastObserved)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &f, nil
}

// collectList reads rows into a list that is never nil, so it encodes as [].
func collectList[T any](rows pgx.Rows, err error, scan func(pgx.CollectableRow) (T, error)) ([]T, error) {
	if err != nil {
		return []T{}, err
	}
	out, err := pgx.AppendRows(make([]T, 0), rows, scan)
	return out, err
}

// dnsNames finds forward records naming the address, plus anything linked to the record.
func (h *Handler) dnsNames(ctx context.Context, k addressKeys) ([]DNSName, error) {
	rows, err := h.conn.Query(ctx, `SELECT r.name, r.record_type, r.value, os.name, r.last_seen FROM dns_records r
		LEFT JOIN observation_sources os ON os.id = r.source_id
		WHERE r.ip_address_id = $1::uuid
		   OR (r.record_type IN ('A','AAAA') AND r.value = $2 AND (r.routing_domain_id IS NULL OR r.routing_domain_id = $3::uuid))
		ORDER BY r.record_type, r.name LIMIT 50`, k.id, k.address, k.routingDomain)
	return collectList(rows, err, func(r pgx.CollectableRow) (DNSName, error) {
		var n DNSName
		return n, r.Scan(&n.Name, &n.RecordType, &n.Value, &n.Source, &n.LastSeen)
	})
}

func (h *Handler) dhcpLeases(ctx context.Context, k addressKeys) ([]DHCPLease, error) {
	rows, err := h.conn.Query(ctx, `SELECT sc.name, dr.mac::text, dr.hostname, os.name, dr.last_seen FROM dhcp_reservations dr
		JOIN dhcp_scopes sc ON sc.id = dr.scope_id JOIN subnets s ON s.id = sc.subnet_id
		LEFT JOIN observation_sources os ON os.id = dr.source_id
		WHERE dr.address = $1::text::inet AND s.routing_domain_id = $2::uuid LIMIT 20`, k.address, k.routingDomain)
	return collectList(rows, err, func(r pgx.CollectableRow) (DHCPLease, error) {
		var l DHCPLease
		return l, r.Scan(&l.Scope, &l.MAC, &l.Hostname, &l.Source, &l.LastSeen)
	})
}

func (h *Handler) sourceSummaries(ctx context.Context, k addressKeys) ([]SourceSummary, error) {
	rows, err := h.conn.Query(ctx, `SELECT os.name, os.reliability, count(*), max(o.observed_at) FROM discovery_observations o
		JOIN observation_sources os ON os.id = o.source_id
		WHERE o.address = $1::text::inet AND o.routing_domain_id = $2::uuid
		GROUP BY os.name, os.reliability ORDER BY max(o.observed_at) DESC`, k.address, k.routingDomain)
	return collectList(rows, err, func(r pgx.CollectableRow) (SourceSummary, error) {
		var s SourceSummary
		return s, r.Scan(&s.Source, &s.Reliability, &s.Observations, &s.LastSeen)
	})
}
