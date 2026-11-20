package links

import (
	"slices"
	"strconv"
	"strings"
)

// ioRow is one link as the export writes it and the import reads it, with the
// names people recognise instead of ids.
type ioRow struct {
	linkDTO
	TypeLabel    string
	LocalSubnets []string
	Related      []string // "DF-LNK-0002:backup"
}

// oldHeaders are columns' names in older files, still read on import.
var oldHeaders = map[string]string{"Segments": "local subnets", "Connected Devices": "connected device"}

// ioColumn is one spreadsheet column: its header, how a link fills it, and how
// an imported cell sets the form. Commercial columns need commercial access.
type ioColumn struct {
	Header     string
	Commercial bool
	get        func(r *ioRow) string
	set        func(in *linkInput, v string, x *resolver)
}

func str(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

func num(p *float64) string {
	if p == nil {
		return ""
	}
	return strconv.FormatFloat(*p, 'f', -1, 64)
}

func i32(p *int32) string {
	if p == nil {
		return ""
	}
	return strconv.Itoa(int(*p))
}

func text(get func(r *ioRow) string, set func(in *linkInput, v string)) (func(*ioRow) string, func(*linkInput, string, *resolver)) {
	return get, func(in *linkInput, v string, _ *resolver) { set(in, v) }
}

func col2(header string, commercial bool, get func(*ioRow) string, set func(*linkInput, string, *resolver)) ioColumn {
	return ioColumn{Header: header, Commercial: commercial, get: get, set: set}
}

func plain(header string, get func(r *ioRow) string, set func(in *linkInput, v string)) ioColumn {
	g, s := text(get, set)
	return col2(header, false, g, s)
}

func paid(header string, get func(r *ioRow) string, set func(in *linkInput, v string)) ioColumn {
	g, s := text(get, set)
	return col2(header, true, g, s)
}

// ioColumns are the export and import columns, in order. The Link ID decides
// whether an imported row adds a link or updates one.
var ioColumns = []ioColumn{
	plain("Link ID", func(r *ioRow) string { return r.Code }, func(*linkInput, string) {}),
	plain("Link Name", func(r *ioRow) string { return r.Name }, func(in *linkInput, v string) { in.Name = v }),
	col2("Link Type", false, func(r *ioRow) string { return r.TypeLabel }, func(in *linkInput, v string, x *resolver) { in.Type = x.linkType(v) }),
	col2("Service Provider", false, func(r *ioRow) string { return str(r.ProviderName) }, func(in *linkInput, v string, x *resolver) { in.ProviderID = x.find("providers", v) }),
	plain("Circuit ID", func(r *ioRow) string { return r.CircuitID }, func(in *linkInput, v string) { in.CircuitID = v }),
	plain("Source Location", func(r *ioRow) string { return r.SourceSite }, func(in *linkInput, v string) { in.SourceSite = v }),
	plain("Destination", func(r *ioRow) string { return r.Destination }, func(in *linkInput, v string) { in.Destination = v }),
	plain("Business Purpose", func(r *ioRow) string { return r.Purpose }, func(in *linkInput, v string) { in.Purpose = v }),
	plain("Environment", func(r *ioRow) string { return r.Environment }, func(in *linkInput, v string) { in.Environment = lower(v) }),
	plain("Link Role", func(r *ioRow) string { return r.Role }, func(in *linkInput, v string) { in.Role = lower(v) }),
	plain("Criticality", func(r *ioRow) string { return r.Criticality }, func(in *linkInput, v string) { in.Criticality = lower(v) }),
	col2("Responsible Officer", false, func(r *ioRow) string { return str(r.OfficerEmail) }, func(in *linkInput, v string, x *resolver) { in.OfficerID = x.find("officers", v) }),
	plain("Status", func(r *ioRow) string { return r.Status }, func(in *linkInput, v string) { in.Status = lower(v) }),
	plain("Download Mbps", func(r *ioRow) string { return num(r.DownMbps) }, func(in *linkInput, v string) { in.DownMbps = parseNum(v) }),
	plain("Upload Mbps", func(r *ioRow) string { return num(r.UpMbps) }, func(in *linkInput, v string) { in.UpMbps = parseNum(v) }),
	plain("Connection Medium", func(r *ioRow) string { return r.Medium }, func(in *linkInput, v string) { in.Medium = lower(v) }),
	plain("WAN IP", func(r *ioRow) string { return str(r.WANIP) }, func(in *linkInput, v string) { in.WANIP = v }),
	col2("WAN Subnet", false, func(r *ioRow) string { return str(r.WANSegment) }, func(in *linkInput, v string, x *resolver) {
		if strings.TrimSpace(v) != "" {
			in.WANSegmentID = x.find("segments", v)
		} else {
			in.WANSegmentID = x.ids["segments"][wanNetwork(in.WANIP)] // a guess: no issue when it isn't a segment
		}
	}),
	plain("More WAN IPs", func(r *ioRow) string { return strings.Join(r.ExtraWANIPs, "; ") }, func(in *linkInput, v string) { in.ExtraWANIPs = split(v) }),
	plain("Public IPs", func(r *ioRow) string { return strings.TrimSuffix(str(r.PublicIPs), "/32") }, func(in *linkInput, v string) { in.PublicIPs = v }),
	plain("Gateway IP", func(r *ioRow) string { return str(r.GatewayIP) }, func(in *linkInput, v string) { in.GatewayIP = v }),
	col2("Connected Devices", false, func(r *ioRow) string {
		return strings.Join(append(slices.Clone(r.DeviceNames), split(r.DeviceText)...), "; ")
	}, setDevices),
	plain("Interface", func(r *ioRow) string { return r.Interface }, func(in *linkInput, v string) { in.Interface = v }),
	plain("VLAN", func(r *ioRow) string { return i32(r.VLAN) }, func(in *linkInput, v string) { in.VLAN = parseInt(v) }),
	plain("VDOM / VRF", func(r *ioRow) string { return r.VRF }, func(in *linkInput, v string) { in.VRF = v }),
	plain("Routing", func(r *ioRow) string { return r.Routing }, func(in *linkInput, v string) { in.Routing = lower(v) }),
	plain("VPN Tunnel", func(r *ioRow) string { return r.VPNTunnel }, func(in *linkInput, v string) { in.VPNTunnel = v }),
	plain("VPN Type", func(r *ioRow) string { return r.VPNType }, func(in *linkInput, v string) { in.VPNType = lower(v) }),
	plain("Local Peer", func(r *ioRow) string { return r.LocalPeer }, func(in *linkInput, v string) { in.LocalPeer = v }),
	plain("Remote Peer", func(r *ioRow) string { return r.RemotePeer }, func(in *linkInput, v string) { in.RemotePeer = v }),
	col2("Segments", false, func(r *ioRow) string { return strings.Join(r.LocalSubnets, "; ") }, func(in *linkInput, v string, x *resolver) { in.LocalSegmentIDs = x.findAll("segments", v) }),
	plain("Remote Subnets", func(r *ioRow) string { return strings.Join(r.RemoteSubnets, "; ") }, func(in *linkInput, v string) { in.RemoteSubnets = split(v) }),
	plain("Remote Organisation", func(r *ioRow) string { return r.RemoteOrg }, func(in *linkInput, v string) { in.RemoteOrg = v }),
	plain("Remote Contact", func(r *ioRow) string { return r.RemoteContact }, func(in *linkInput, v string) { in.RemoteContact = v }),
	plain("Last Test Date", func(r *ioRow) string { return str(r.LastTestDate) }, func(in *linkInput, v string) { in.LastTestDate = v }),
	plain("Vault Reference", func(r *ioRow) string { return r.VaultRef }, func(in *linkInput, v string) { in.VaultRef = v }),
	paid("No Separate Cost", func(r *ioRow) string { return yesNo(r.NoSeparateCost) }, func(in *linkInput, v string) { in.NoSeparateCost = isYes(v) }),
	paid("Billing Account", func(r *ioRow) string { return r.BillingAccount }, func(in *linkInput, v string) { in.BillingAccount = v }),
	paid("Monthly Rental", func(r *ioRow) string { return num(r.MonthlyRental) }, func(in *linkInput, v string) { in.MonthlyRental = parseNum(v) }),
	paid("Currency", func(r *ioRow) string { return r.Currency }, func(in *linkInput, v string) { in.Currency = v }),
	paid("Installation Cost", func(r *ioRow) string { return num(r.InstallCost) }, func(in *linkInput, v string) { in.InstallCost = parseNum(v) }),
	paid("Annual Recurring Cost", func(r *ioRow) string { return num(r.AnnualCost) }, func(*linkInput, string) {}),
	paid("Contract Reference", func(r *ioRow) string { return r.ContractRef }, func(in *linkInput, v string) { in.ContractRef = v }),
	paid("PO Reference", func(r *ioRow) string { return r.PORef }, func(in *linkInput, v string) { in.PORef = v }),
	paid("Activation Date", func(r *ioRow) string { return str(r.ActivationDate) }, func(in *linkInput, v string) { in.ActivationDate = v }),
	paid("Contract Start", func(r *ioRow) string { return str(r.ContractStart) }, func(in *linkInput, v string) { in.ContractStart = v }),
	paid("Contract Expiry", func(r *ioRow) string { return str(r.ContractEnd) }, func(in *linkInput, v string) { in.ContractEnd = v }),
	paid("Renewal Notice Days", func(r *ioRow) string { return i32(r.RenewalNoticeDays) }, func(in *linkInput, v string) { in.RenewalNoticeDays = parseInt(v) }),
	paid("Cost Centre", func(r *ioRow) string { return r.CostCentre }, func(in *linkInput, v string) { in.CostCentre = v }),
	plain("Support Contact", func(r *ioRow) string { return r.SupportContact }, func(in *linkInput, v string) { in.SupportContact = v }),
	plain("Support Email", func(r *ioRow) string { return r.SupportEmail }, func(in *linkInput, v string) { in.SupportEmail = v }),
	plain("Support Hotline", func(r *ioRow) string { return r.SupportHotline }, func(in *linkInput, v string) { in.SupportHotline = v }),
	plain("Account Manager", func(r *ioRow) string { return r.AccountManager }, func(in *linkInput, v string) { in.AccountManager = v }),
	plain("SLA Availability %", func(r *ioRow) string { return num(r.SLAPct) }, func(in *linkInput, v string) { in.SLAPct = parseNum(v) }),
	plain("Resolution Hours", func(r *ioRow) string { return num(r.ResolutionHours) }, func(in *linkInput, v string) { in.ResolutionHours = parseNum(v) }),
	plain("Remarks", func(r *ioRow) string { return r.Remarks }, func(in *linkInput, v string) { in.Remarks = v }),
	col2("Related Links", false, func(r *ioRow) string { return strings.Join(r.Related, "; ") }, func(in *linkInput, v string, x *resolver) { in.Relations = x.relations(v) }),
}
