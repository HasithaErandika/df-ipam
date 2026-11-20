package links

import (
	"encoding/json"
	"time"
)

// linkDTO is one network link. Money fields are cleared unless the person may
// see commercial information; CommercialHidden then says so.
type linkDTO struct {
	ID           string     `db:"id" json:"id"`
	Code         string     `db:"code" json:"code"`
	Name         string     `db:"name" json:"name"`
	Type         string     `db:"link_type" json:"linkType"`
	ProviderID   *string    `db:"provider_id" json:"providerId"`
	ProviderName *string    `db:"provider_name" json:"providerName"`
	CircuitID    string     `db:"circuit_id" json:"circuitId"`
	SourceSite   string     `db:"source_site" json:"sourceSite"`
	Destination  string     `db:"destination" json:"destination"`
	Purpose      string     `db:"purpose" json:"purpose"`
	Environment  string     `db:"environment" json:"environment"`
	Role         string     `db:"link_role" json:"linkRole"`
	Criticality  string     `db:"criticality" json:"criticality"`
	OfficerID    *string    `db:"officer_id" json:"officerId"`
	OfficerEmail *string    `db:"officer_email" json:"officerEmail"`
	OfficerName  *string    `db:"officer_name" json:"officerName"`
	Status       string     `db:"status" json:"status"`
	ArchivedAt   *time.Time `db:"archived_at" json:"archivedAt"`

	DownMbps     *float64 `db:"down_mbps" json:"downMbps"`
	UpMbps       *float64 `db:"up_mbps" json:"upMbps"`
	Medium       string   `db:"medium" json:"medium"`
	WANIP        *string  `db:"wan_ip" json:"wanIp"`
	WANSegmentID *string  `db:"wan_segment_id" json:"wanSegmentId"`
	WANSegment   *string  `db:"wan_segment" json:"wanSegment"`
	ExtraWANIPs  []string `db:"extra_wan_ips" json:"extraWanIps"`
	PublicIPs    *string  `db:"public_ips" json:"publicIps"`
	GatewayIP    *string  `db:"gateway_ip" json:"gatewayIp"`
	DeviceIDs    []string `db:"device_ids" json:"deviceIds"`
	DeviceNames  []string `db:"device_names" json:"deviceNames"` // in the order of DeviceIDs
	DeviceText   string   `db:"device_text" json:"deviceText"`
	Interface    string   `db:"interface" json:"interface"`
	VLAN         *int32   `db:"vlan" json:"vlan"`
	VRF          string   `db:"vrf" json:"vrf"`
	Routing      string   `db:"routing" json:"routing"`

	VPNTunnel     string   `db:"vpn_tunnel" json:"vpnTunnel"`
	VPNType       string   `db:"vpn_type" json:"vpnType"`
	LocalPeer     string   `db:"local_peer" json:"localPeer"`
	RemotePeer    string   `db:"remote_peer" json:"remotePeer"`
	RemoteSubnets []string `db:"remote_subnets" json:"remoteSubnets"`
	RemoteOrg     string   `db:"remote_org" json:"remoteOrg"`
	RemoteContact string   `db:"remote_contact" json:"remoteContact"`
	LastTestDate  *string  `db:"last_test_date" json:"lastTestDate"`
	VaultRef      string   `db:"vault_ref" json:"vaultRef"`

	CommercialHidden  bool     `db:"-" json:"commercialHidden"`
	NoSeparateCost    bool     `db:"no_separate_cost" json:"noSeparateCost"`
	BillingAccount    string   `db:"billing_account" json:"billingAccount"`
	MonthlyRental     *float64 `db:"monthly_rental" json:"monthlyRental"`
	Currency          string   `db:"currency" json:"currency"`
	InstallCost       *float64 `db:"install_cost" json:"installCost"`
	AnnualCost        *float64 `db:"annual_cost" json:"annualCost"`
	ContractRef       string   `db:"contract_ref" json:"contractRef"`
	PORef             string   `db:"po_ref" json:"poRef"`
	ActivationDate    *string  `db:"activation_date" json:"activationDate"`
	ContractStart     *string  `db:"contract_start" json:"contractStart"`
	ContractEnd       *string  `db:"contract_end" json:"contractEnd"`
	RenewalNoticeDays *int32   `db:"renewal_notice_days" json:"renewalNoticeDays"`
	CostCentre        string   `db:"cost_centre" json:"costCentre"`

	SupportContact  string   `db:"support_contact" json:"supportContact"`
	SupportEmail    string   `db:"support_email" json:"supportEmail"`
	SupportHotline  string   `db:"support_hotline" json:"supportHotline"`
	AccountManager  string   `db:"account_manager" json:"accountManager"`
	SLAPct          *float64 `db:"sla_pct" json:"slaPct"`
	ResolutionHours *float64 `db:"resolution_hours" json:"resolutionHours"`
	Remarks         string   `db:"remarks" json:"remarks"`

	CreatedAt time.Time `db:"created_at" json:"createdAt"`
	UpdatedAt time.Time `db:"updated_at" json:"updatedAt"`
}

// linkColumns selects a linkDTO from network_links l.
const linkColumns = `SELECT l.id::text AS id, l.code, l.name, l.link_type, l.provider_id::text AS provider_id, p.name AS provider_name,
	l.circuit_id, l.source_site, l.destination, l.purpose, l.environment, l.link_role, l.criticality,
	l.officer_id::text AS officer_id, u.email AS officer_email, NULLIF(u.display_name, '') AS officer_name, l.status, l.archived_at,
	l.down_mbps::float8 AS down_mbps, l.up_mbps::float8 AS up_mbps, l.medium, host(l.wan_ip) AS wan_ip,
	l.wan_segment_id::text AS wan_segment_id, ws.name AS wan_segment, l.extra_wan_ips::text[] AS extra_wan_ips, l.public_ips::text AS public_ips, host(l.gateway_ip) AS gateway_ip,
	l.device_ids::text[] AS device_ids, array(SELECT COALESCE(NULLIF(d.name, ''), d.mac::text) FROM unnest(l.device_ids) WITH ORDINALITY u(id, n)
		JOIN devices d ON d.id = u.id ORDER BY u.n)::text[] AS device_names, l.device_text, l.interface, l.vlan, l.vrf, l.routing,
	l.vpn_tunnel, l.vpn_type, l.local_peer, l.remote_peer, l.remote_subnets::text[] AS remote_subnets, l.remote_org, l.remote_contact,
	l.last_test_date::text AS last_test_date, l.vault_ref,
	l.no_separate_cost, l.billing_account, l.monthly_rental::float8 AS monthly_rental, l.currency, l.install_cost::float8 AS install_cost,
	(l.monthly_rental * 12)::float8 AS annual_cost, l.contract_ref, l.po_ref, l.activation_date::text AS activation_date,
	l.contract_start::text AS contract_start, l.contract_end::text AS contract_end, l.renewal_notice_days, l.cost_centre,
	l.support_contact, l.support_email, l.support_hotline, l.account_manager, l.sla_pct::float8 AS sla_pct,
	l.resolution_hours::float8 AS resolution_hours, l.remarks, l.created_at, l.updated_at`

const linkFrom = ` FROM network_links l
	LEFT JOIN network_link_providers p ON p.id = l.provider_id
	LEFT JOIN users u ON u.id = l.officer_id
	LEFT JOIN segments ws ON ws.id = l.wan_segment_id`

// hideCommercial clears what only commercial viewers may see.
func (l *linkDTO) hideCommercial() {
	*l = linkDTO{
		ID: l.ID, Code: l.Code, Name: l.Name, Type: l.Type, ProviderID: l.ProviderID, ProviderName: l.ProviderName, CircuitID: l.CircuitID,
		SourceSite: l.SourceSite, Destination: l.Destination, Purpose: l.Purpose, Environment: l.Environment, Role: l.Role,
		Criticality: l.Criticality, OfficerID: l.OfficerID, OfficerEmail: l.OfficerEmail, OfficerName: l.OfficerName, Status: l.Status,
		ArchivedAt: l.ArchivedAt, DownMbps: l.DownMbps, UpMbps: l.UpMbps, Medium: l.Medium, WANIP: l.WANIP, WANSegmentID: l.WANSegmentID,
		WANSegment: l.WANSegment, ExtraWANIPs: l.ExtraWANIPs, PublicIPs: l.PublicIPs, GatewayIP: l.GatewayIP, DeviceIDs: l.DeviceIDs, DeviceNames: l.DeviceNames,
		DeviceText: l.DeviceText, Interface: l.Interface, VLAN: l.VLAN, VRF: l.VRF, Routing: l.Routing, VPNTunnel: l.VPNTunnel,
		VPNType: l.VPNType, LocalPeer: l.LocalPeer, RemotePeer: l.RemotePeer, RemoteSubnets: l.RemoteSubnets, RemoteOrg: l.RemoteOrg,
		RemoteContact: l.RemoteContact, LastTestDate: l.LastTestDate, VaultRef: l.VaultRef, CommercialHidden: true,
		SupportContact: l.SupportContact, SupportEmail: l.SupportEmail, SupportHotline: l.SupportHotline, AccountManager: l.AccountManager,
		SLAPct: l.SLAPct, ResolutionHours: l.ResolutionHours, Remarks: l.Remarks, CreatedAt: l.CreatedAt, UpdatedAt: l.UpdatedAt,
	}
}

// commercialKeys are the stored columns only commercial viewers may see, also
// in the change history.
var commercialKeys = []string{"no_separate_cost", "billing_account", "monthly_rental", "currency", "install_cost", "contract_ref", "po_ref",
	"activation_date", "contract_start", "contract_end", "renewal_notice_days", "cost_centre",
	"noSeparateCost", "billingAccount", "monthlyRental", "installCost", "contractRef", "poRef", "activationDate", "contractStart",
	"contractEnd", "renewalNoticeDays", "costCentre"}

func rawJSON(b []byte) json.RawMessage {
	if len(b) == 0 {
		return nil
	}
	return json.RawMessage(b)
}
