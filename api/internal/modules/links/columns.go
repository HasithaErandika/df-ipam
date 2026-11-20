package links

import (
	"fmt"
	"strings"
)

// col is one stored column with its value and SQL cast.
type col struct {
	name, cast string
	val        any
}

// nz is nil for an empty string, so optional typed columns store NULL.
func nz(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func (in *linkInput) baseCols() []col {
	return []col{
		{"name", "text", in.Name}, {"link_type", "text", in.Type}, {"provider_id", "uuid", nz(in.ProviderID)},
		{"circuit_id", "text", in.CircuitID}, {"source_site", "text", in.SourceSite}, {"destination", "text", in.Destination},
		{"purpose", "text", in.Purpose}, {"environment", "text", in.Environment}, {"link_role", "text", in.Role},
		{"criticality", "text", in.Criticality}, {"officer_id", "uuid", nz(in.OfficerID)}, {"status", "text", in.Status},
		{"down_mbps", "numeric", in.DownMbps}, {"up_mbps", "numeric", in.UpMbps}, {"medium", "text", in.Medium},
		{"wan_ip", "inet", nz(in.WANIP)}, {"wan_segment_id", "uuid", nz(in.WANSegmentID)}, {"public_ips", "cidr", nz(in.PublicIPs)},
		{"gateway_ip", "inet", nz(in.GatewayIP)}, {"device_id", "uuid", nz(in.DeviceID)}, {"device_text", "text", in.DeviceText},
		{"interface", "text", in.Interface}, {"vlan", "integer", in.VLAN}, {"vrf", "text", in.VRF}, {"routing", "text", in.Routing},
		{"vpn_tunnel", "text", in.VPNTunnel}, {"vpn_type", "text", in.VPNType}, {"local_peer", "text", in.LocalPeer},
		{"remote_peer", "text", in.RemotePeer}, {"remote_subnets", "cidr[]", nonNil(in.RemoteSubnets)}, {"remote_org", "text", in.RemoteOrg},
		{"remote_contact", "text", in.RemoteContact}, {"last_test_date", "date", nz(in.LastTestDate)}, {"vault_ref", "text", in.VaultRef},
		{"support_contact", "text", in.SupportContact}, {"support_email", "text", in.SupportEmail},
		{"support_hotline", "text", in.SupportHotline}, {"account_manager", "text", in.AccountManager}, {"sla_pct", "numeric", in.SLAPct},
		{"resolution_hours", "numeric", in.ResolutionHours}, {"remarks", "text", in.Remarks},
	}
}

func (in *linkInput) commercialCols() []col {
	return []col{
		{"no_separate_cost", "boolean", in.NoSeparateCost}, {"billing_account", "text", in.BillingAccount},
		{"monthly_rental", "numeric", in.MonthlyRental}, {"currency", "text", in.Currency}, {"install_cost", "numeric", in.InstallCost},
		{"contract_ref", "text", in.ContractRef}, {"po_ref", "text", in.PORef}, {"activation_date", "date", nz(in.ActivationDate)},
		{"contract_start", "date", nz(in.ContractStart)}, {"contract_end", "date", nz(in.ContractEnd)},
		{"renewal_notice_days", "integer", in.RenewalNoticeDays}, {"cost_centre", "text", in.CostCentre},
	}
}

func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

// insertSQL builds the INSERT for these columns and their values.
func insertSQL(cols []col, extra ...col) (string, []any) {
	cols = append(cols, extra...)
	names, marks, args := make([]string, len(cols)), make([]string, len(cols)), make([]any, len(cols))
	for i, c := range cols {
		names[i], marks[i], args[i] = c.name, fmt.Sprintf("$%d::%s", i+1, c.cast), c.val
	}
	return "INSERT INTO network_links (" + strings.Join(names, ", ") + ") VALUES (" + strings.Join(marks, ", ") + ") RETURNING id::text", args
}

// updateSQL builds the UPDATE of one link (the last argument is its id).
func updateSQL(cols []col, id string) (string, []any) {
	sets, args := make([]string, len(cols)), make([]any, 0, len(cols)+1)
	for i, c := range cols {
		sets[i] = fmt.Sprintf("%s = $%d::%s", c.name, i+1, c.cast)
		args = append(args, c.val)
	}
	return fmt.Sprintf("UPDATE network_links SET %s WHERE id = $%d::uuid", strings.Join(sets, ", "), len(cols)+1), append(args, id)
}
