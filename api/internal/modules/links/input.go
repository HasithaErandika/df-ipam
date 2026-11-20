package links

import (
	"regexp"
	"slices"
	"strings"

	"dfipam/api/internal/validate"
)

type relationInput struct {
	RelatedID string `json:"relatedId"`
	Kind      string `json:"kind"`
}

// linkInput is what the link form sends. Bandwidth is in Mbps.
type linkInput struct {
	Name        string `json:"name"`
	Type        string `json:"linkType"`
	ProviderID  string `json:"providerId"`
	CircuitID   string `json:"circuitId"`
	SourceSite  string `json:"sourceSite"`
	Destination string `json:"destination"`
	Purpose     string `json:"purpose"`
	Environment string `json:"environment"`
	Role        string `json:"linkRole"`
	Criticality string `json:"criticality"`
	OfficerID   string `json:"officerId"`
	Status      string `json:"status"`
	technical
	vpn
	commercial
	support
	Remarks         string          `json:"remarks"`
	Relations       []relationInput `json:"relations"`
	LocalSegmentIDs []string        `json:"localSegmentIds"` // the link's segments, for any link type
}

var (
	environments  = []string{"", "production", "dr", "uat", "other"}
	roles         = []string{"primary", "secondary", "backup", "standalone"}
	criticalities = []string{"critical", "high", "medium", "low"}
	statuses      = []string{"active", "inactive", "pending", "terminated"}
	mediums       = []string{"", "fibre", "wireless", "copper", "other"}
	routings      = []string{"", "static", "bgp", "ospf", "other"}
	vpnTypes      = []string{"", "ipsec", "ssl", "other"}
	relationKinds = []string{"secondary", "backup", "underlying"}
	currencies    = regexp.MustCompile(`^[A-Z]{3}$`)
	phoneChars    = regexp.MustCompile(`^[0-9+()\-. /ext]*$`)
	hostName      = regexp.MustCompile(`^(?i)[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?(\.[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?)+$`)
)

// validate checks the form. vpnType says whether the chosen type shows the
// VPN section; when it doesn't, VPN details are cleared.
func (in *linkInput) validate(isVPN bool) error {
	var v validate.V
	in.basic(&v)
	in.technical.check(&v)
	if isVPN {
		in.vpn.check(&v)
	} else {
		in.vpn = vpn{}
	}
	in.commercial.check(&v)
	in.support.check(&v)
	v.Text("remarks", &in.Remarks, 4000, false, "Remarks")
	in.checkRelations(&v)
	return v.Err()
}

func (in *linkInput) basic(v *validate.V) {
	v.Text("name", &in.Name, 160, true, "Link name")
	v.Text("circuitId", &in.CircuitID, 120, false, "Circuit / service ID")
	v.Text("sourceSite", &in.SourceSite, 120, true, "Source location")
	v.Text("destination", &in.Destination, 160, false, "Destination")
	v.Text("purpose", &in.Purpose, 500, true, "Business purpose")
	v.OneOf("environment", in.Environment, environments, "environment")
	v.OneOf("linkRole", in.Role, roles, "link role")
	v.OneOf("criticality", in.Criticality, criticalities, "criticality")
	v.OneOf("status", in.Status, statuses, "status")
	v.UUID("providerId", in.ProviderID, false, "a listed provider")
	v.UUID("officerId", in.OfficerID, false, "a listed person")
}

func (t *technical) check(v *validate.V) {
	positive(v, "downMbps", t.DownMbps, "Download bandwidth")
	positive(v, "upMbps", t.UpMbps, "Upload bandwidth")
	v.OneOf("medium", t.Medium, mediums, "connection medium")
	hostAddress(v, "wanIp", &t.WANIP, "WAN IP")
	t.ExtraWANIPs = extraWANIPs(v, t.ExtraWANIPs, t.WANIP)
	hostAddress(v, "gatewayIp", &t.GatewayIP, "Gateway IP")
	t.PublicIPs = ipOrNetwork(v, "publicIps", t.PublicIPs, "Public IP or range")
	v.UUID("wanSegmentId", t.WANSegmentID, false, "a listed subnet")
	if len(t.DeviceIDs) > 50 {
		v.Add("deviceIds", "List at most 50 connected devices.")
	}
	t.DeviceIDs = slices.Compact(slices.Sorted(slices.Values(t.DeviceIDs)))
	for _, id := range t.DeviceIDs {
		v.UUID("deviceIds", id, true, "listed devices")
	}
	v.Text("deviceText", &t.DeviceText, 120, false, "Connected device")
	v.Text("interface", &t.Interface, 80, false, "Interface / port")
	v.IntRange("vlan", t.VLAN, 1, 4094, "VLAN ID")
	v.Text("vrf", &t.VRF, 80, false, "VDOM / VRF")
	v.OneOf("routing", t.Routing, routings, "routing type")
}

func (p *vpn) check(v *validate.V) {
	v.Text("vpnTunnel", &p.VPNTunnel, 120, false, "VPN tunnel name")
	v.OneOf("vpnType", p.VPNType, vpnTypes, "VPN type")
	peer(v, "localPeer", &p.LocalPeer, "Local peer")
	peer(v, "remotePeer", &p.RemotePeer, "Remote peer")
	if len(p.RemoteSubnets) > 100 {
		v.Add("remoteSubnets", "List at most 100 remote subnets.")
	}
	clean := make([]string, 0, len(p.RemoteSubnets))
	for _, s := range p.RemoteSubnets {
		if s = strings.TrimSpace(s); s != "" {
			clean = append(clean, ipOrNetwork(v, "remoteSubnets", s, "Each remote subnet"))
		}
	}
	p.RemoteSubnets = clean
	v.Text("remoteOrg", &p.RemoteOrg, 160, false, "Remote organisation")
	v.Text("remoteContact", &p.RemoteContact, 300, false, "Remote technical contact")
	v.Date("lastTestDate", &p.LastTestDate, false, "Last connectivity test")
	v.Text("vaultRef", &p.VaultRef, 200, false, "Credential vault reference")
}

func (m *commercial) check(v *validate.V) {
	v.Text("billingAccount", &m.BillingAccount, 80, false, "Billing account")
	money(v, "monthlyRental", m.MonthlyRental, "Monthly rental")
	money(v, "installCost", m.InstallCost, "Installation cost")
	if m.NoSeparateCost && m.MonthlyRental != nil && *m.MonthlyRental != 0 {
		v.Add("monthlyRental", "Leave the rental empty for a link with no separate cost.")
	}
	if m.Currency = strings.ToUpper(strings.TrimSpace(m.Currency)); m.Currency == "" {
		m.Currency = "LKR"
	}
	if !currencies.MatchString(m.Currency) {
		v.Add("currency", "Use a three-letter currency code such as LKR or USD.")
	}
	v.Text("contractRef", &m.ContractRef, 120, false, "Contract reference")
	v.Text("poRef", &m.PORef, 120, false, "Purchase order reference")
	v.Text("costCentre", &m.CostCentre, 80, false, "Cost centre")
	v.Date("activationDate", &m.ActivationDate, false, "Activation date")
	v.Date("contractStart", &m.ContractStart, false, "Contract start")
	v.Date("contractEnd", &m.ContractEnd, false, "Contract expiry")
	if m.ContractStart != "" && m.ContractEnd != "" && m.ContractEnd < m.ContractStart {
		v.Add("contractEnd", "The contract can't end before it starts.")
	}
	v.IntRange("renewalNoticeDays", m.RenewalNoticeDays, 0, 730, "Renewal notice period")
}

func (s *support) check(v *validate.V) {
	v.Text("supportContact", &s.SupportContact, 120, false, "Provider support contact")
	email(v, "supportEmail", &s.SupportEmail, "Provider support email")
	phone(v, "supportHotline", &s.SupportHotline, "Provider hotline")
	v.Text("accountManager", &s.AccountManager, 120, false, "Account manager")
	if s.SLAPct != nil && (*s.SLAPct < 0 || *s.SLAPct > 100) {
		v.Add("slaPct", "Availability is a percentage from 0 to 100.")
	}
	if s.ResolutionHours != nil && (*s.ResolutionHours < 0 || *s.ResolutionHours > 8760) {
		v.Add("resolutionHours", "Resolution time is from 0 to 8,760 hours.")
	}
}

func (in *linkInput) checkRelations(v *validate.V) {
	if len(in.Relations) > 20 || len(in.LocalSegmentIDs) > 100 {
		v.Add("relations", "Too many related links or segments.")
	}
	seen := map[string]bool{}
	for _, r := range in.Relations {
		v.UUID("relations", r.RelatedID, true, "listed links")
		v.OneOf("relations", r.Kind, relationKinds, "relationship")
		if seen[r.RelatedID] {
			v.Add("relations", "Each related link can be listed once.")
		}
		seen[r.RelatedID] = true
	}
	for _, id := range in.LocalSegmentIDs {
		v.UUID("localSegmentIds", id, true, "listed segments")
	}
}
