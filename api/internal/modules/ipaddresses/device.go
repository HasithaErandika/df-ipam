package ipaddresses

import (
	"errors"
	"net"
	"net/http"
	"time"

	"dfipam/api/internal/platform/httpx"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
)

// DeviceInfo is what DF-IPAM legitimately knows about the device behind an
// address: its inventory records and what approved read-only sources
// reported. Nothing here comes from probing the device, and nothing is
// guessed: the operating system appears only when a record states it.
type DeviceInfo struct {
	Address     string          `json:"address"`
	Device      *DeviceRecord   `json:"device"`
	Interface   *InterfaceInfo  `json:"interface"`
	MAC         *MACInfo        `json:"mac"`
	DNS         []DNSName       `json:"dnsRecords"`
	DHCP        []DHCPLease     `json:"dhcpReservations"`
	Sources     []SourceSummary `json:"sources"`
	HasEvidence bool            `json:"hasEvidence"`
}

type DeviceRecord struct {
	ID              string     `json:"id"`
	Name            string     `json:"name"`
	Hostname        string     `json:"hostname"`
	DeviceType      string     `json:"deviceType"`
	Vendor          string     `json:"vendor"`
	Model           string     `json:"model"`
	SerialNumber    *string    `json:"serialNumber"`
	OSVersion       string     `json:"osVersion"`
	OSFamily        string     `json:"osFamily"`
	ManagementIP    *string    `json:"managementIp"`
	Status          string     `json:"status"`
	Sensitivity     string     `json:"sensitivity"`
	SiteName        *string    `json:"siteName"`
	SourceSystem    string     `json:"sourceSystem"`
	LastDiscovered  *time.Time `json:"lastDiscovered"`
	LastVerified    *time.Time `json:"lastVerified"`
	OtherAddresses  int64      `json:"otherAddresses"`
	BusinessService *string    `json:"businessService"`
}

type InterfaceInfo struct {
	Name         string     `json:"name"`
	MAC          *string    `json:"mac"`
	Type         string     `json:"interfaceType"`
	AdminStatus  string     `json:"adminStatus"`
	OperStatus   string     `json:"operStatus"`
	VLAN         *string    `json:"vlan"`
	VDOM         *string    `json:"vdom"`
	Source       string     `json:"source"`
	LastObserved *time.Time `json:"lastObserved"`
}

// MACInfo is read from the address itself, so it needs no lookup. A locally
// administered address is usually randomised or virtual.
type MACInfo struct {
	Address             string `json:"address"`
	LocallyAdministered bool   `json:"locallyAdministered"`
	Multicast           bool   `json:"multicast"`
	OUI                 string `json:"oui"`
}

type DNSName struct {
	Name       string     `json:"name"`
	RecordType string     `json:"recordType"`
	Value      string     `json:"value"`
	Source     *string    `json:"source"`
	LastSeen   *time.Time `json:"lastSeen"`
}

type DHCPLease struct {
	Scope    string     `json:"scope"`
	MAC      *string    `json:"mac"`
	Hostname string     `json:"hostname"`
	Source   *string    `json:"source"`
	LastSeen *time.Time `json:"lastSeen"`
}

type SourceSummary struct {
	Source       string    `json:"source"`
	Reliability  int16     `json:"reliability"`
	Observations int64     `json:"observations"`
	LastSeen     time.Time `json:"lastSeen"`
}

// DescribeMAC reads the flags in the first octet of a MAC address.
func DescribeMAC(value string) *MACInfo {
	hw, err := net.ParseMAC(value)
	if err != nil || len(hw) < 3 {
		return nil
	}
	return &MACInfo{Address: hw.String(), LocallyAdministered: hw[0]&0x02 != 0, Multicast: hw[0]&0x01 != 0,
		OUI: net.HardwareAddr(hw[:3]).String()}
}

func (h *Handler) device(c *gin.Context) {
	id, ok := httpx.PathID(c)
	if !ok {
		return
	}
	ctx := c.Request.Context()
	var addr, rd string
	var deviceID, interfaceID, mac *string
	err := h.conn.QueryRow(ctx, `SELECT host(address), routing_domain_id::text, device_id::text, interface_id::text, mac::text
		FROM ip_addresses WHERE id = $1::uuid`, id).Scan(&addr, &rd, &deviceID, &interfaceID, &mac)
	if errors.Is(err, pgx.ErrNoRows) {
		httpx.Fail(c, httpx.ErrNotFound)
		return
	}
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	out := DeviceInfo{Address: addr, DNS: []DNSName{}, DHCP: []DHCPLease{}, Sources: []SourceSummary{}}

	if deviceID != nil {
		var d DeviceRecord
		err := h.conn.QueryRow(ctx, `SELECT d.id::text, d.name, d.hostname, d.device_type, d.vendor, d.model, d.serial_number, d.os_version, d.os_family,
			host(d.management_ip), d.status, d.sensitivity, st.name, d.source_system, d.last_discovered, d.last_verified,
			(SELECT count(*) FROM ip_addresses x WHERE x.device_id = d.id AND x.id <> $2::uuid), bs.name
			FROM devices d LEFT JOIN sites st ON st.id = d.site_id LEFT JOIN business_services bs ON bs.id = d.business_service_id
			WHERE d.id = $1::uuid`, *deviceID, id).Scan(&d.ID, &d.Name, &d.Hostname, &d.DeviceType, &d.Vendor, &d.Model, &d.SerialNumber,
			&d.OSVersion, &d.OSFamily, &d.ManagementIP, &d.Status, &d.Sensitivity, &d.SiteName, &d.SourceSystem, &d.LastDiscovered, &d.LastVerified,
			&d.OtherAddresses, &d.BusinessService)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			httpx.Fail(c, err)
			return
		}
		if err == nil {
			out.Device = &d
		}
	}
	if interfaceID != nil {
		var f InterfaceInfo
		err := h.conn.QueryRow(ctx, `SELECT di.name, di.mac::text, di.interface_type, di.admin_status, di.oper_status,
			(SELECT v.vlan_id::text || ' ' || v.name FROM vlans v WHERE v.id = di.vlan_id), (SELECT vd.name FROM vdoms vd WHERE vd.id = di.vdom_id),
			di.source, di.last_observed FROM device_interfaces di WHERE di.id = $1::uuid`, *interfaceID).
			Scan(&f.Name, &f.MAC, &f.Type, &f.AdminStatus, &f.OperStatus, &f.VLAN, &f.VDOM, &f.Source, &f.LastObserved)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			httpx.Fail(c, err)
			return
		}
		if err == nil {
			out.Interface = &f
			if mac == nil {
				mac = f.MAC
			}
		}
	}
	if mac != nil {
		out.MAC = DescribeMAC(*mac)
	}

	// Forward records naming the address, plus anything linked to the record.
	rows, err := h.conn.Query(ctx, `SELECT r.name, r.record_type, r.value, os.name, r.last_seen FROM dns_records r
		LEFT JOIN observation_sources os ON os.id = r.source_id
		WHERE r.ip_address_id = $1::uuid
		   OR (r.record_type IN ('A','AAAA') AND r.value = $2 AND (r.routing_domain_id IS NULL OR r.routing_domain_id = $3::uuid))
		ORDER BY r.record_type, r.name LIMIT 50`, id, addr, rd)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	if out.DNS, err = pgx.CollectRows(rows, func(r pgx.CollectableRow) (DNSName, error) {
		var n DNSName
		return n, r.Scan(&n.Name, &n.RecordType, &n.Value, &n.Source, &n.LastSeen)
	}); err != nil {
		httpx.Fail(c, err)
		return
	}

	rows, err = h.conn.Query(ctx, `SELECT sc.name, dr.mac::text, dr.hostname, os.name, dr.last_seen FROM dhcp_reservations dr
		JOIN dhcp_scopes sc ON sc.id = dr.scope_id JOIN subnets s ON s.id = sc.subnet_id
		LEFT JOIN observation_sources os ON os.id = dr.source_id
		WHERE dr.address = $1::text::inet AND s.routing_domain_id = $2::uuid LIMIT 20`, addr, rd)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	if out.DHCP, err = pgx.CollectRows(rows, func(r pgx.CollectableRow) (DHCPLease, error) {
		var l DHCPLease
		return l, r.Scan(&l.Scope, &l.MAC, &l.Hostname, &l.Source, &l.LastSeen)
	}); err != nil {
		httpx.Fail(c, err)
		return
	}

	rows, err = h.conn.Query(ctx, `SELECT os.name, os.reliability, count(*), max(o.observed_at) FROM discovery_observations o
		JOIN observation_sources os ON os.id = o.source_id
		WHERE o.address = $1::text::inet AND o.routing_domain_id = $2::uuid
		GROUP BY os.name, os.reliability ORDER BY max(o.observed_at) DESC`, addr, rd)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	if out.Sources, err = pgx.CollectRows(rows, func(r pgx.CollectableRow) (SourceSummary, error) {
		var s SourceSummary
		return s, r.Scan(&s.Source, &s.Reliability, &s.Observations, &s.LastSeen)
	}); err != nil {
		httpx.Fail(c, err)
		return
	}
	out.HasEvidence = len(out.Sources) > 0
	if out.DNS == nil {
		out.DNS = []DNSName{}
	}
	if out.DHCP == nil {
		out.DHCP = []DHCPLease{}
	}
	if out.Sources == nil {
		out.Sources = []SourceSummary{}
	}
	httpx.OK(c, http.StatusOK, out)
}
