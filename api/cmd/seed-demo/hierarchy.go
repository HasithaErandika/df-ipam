package main

import "fmt"

var branches = []string{"Galle", "Matara", "Negombo", "Jaffna", "Kurunegala", "Anuradhapura", "Batticaloa", "Trincomalee", "Ratnapura", "Badulla",
	"Kalutara", "Gampaha", "Nuwara Eliya", "Hambantota", "Vavuniya", "Ampara", "Puttalam", "Chilaw", "Kegalle", "Polonnaruwa"}

func branchCode(i int) string { return fmt.Sprintf("BR-%02d", i+1) }

func (s *sample) addHierarchy() {
	var org, domain string
	err := s.pool.QueryRow(s.ctx, `SELECT o.id::text, coalesce((SELECT d.id::text FROM routing_domains d WHERE d.organisation_id = o.id AND d.kind = 'default' LIMIT 1), '')
		FROM organisations o WHERE o.code = 'DF'`).Scan(&org, &domain)
	if err != nil {
		org = s.create("admin", "/organisations", "", H{"name": "Dialog Finance", "code": "DF"})
	}
	s.ref["org"] = org
	if domain == "" {
		domain = s.create("admin", "/routing-domains", "", H{"organisationId": org, "name": "Default", "kind": "default"})
	}
	s.ref["rd:corp"] = domain
	for _, d := range [][3]string{{"dmz", "DMZ VRF", "vrf"}, {"core", "Core Banking VRF", "vrf"}, {"pay", "Payments VRF", "vrf"}, {"lab", "Lab VRF", "isolated"},
		{"guest", "Guest Wi-Fi", "isolated"}, {"aws", "AWS Production VPC", "vpc"}, {"azure", "Azure Production VNet", "vnet"}} {
		s.create("admin", "/routing-domains", "rd:"+d[0], H{"organisationId": org, "name": d[1], "kind": d[2]})
	}
	for _, u := range [][2]string{{"Retail Banking", "RB"}, {"Corporate Banking", "CB"}, {"Treasury", "TR"}, {"Information Technology", "IT"}, {"Finance", "FN"}, {"Risk and Compliance", "RC"}} {
		s.create("admin", "/business-units", "unit:"+u[1], H{"organisationId": org, "name": u[0], "code": u[1]})
	}
	s.addServices()
}

func (s *sample) addServices() {
	for _, v := range []struct {
		key, name, level string
		sensitive        bool
	}{{"core", "Core Banking", "critical", true}, {"pay", "Payment Switch", "critical", true}, {"ib", "Internet Banking", "high", false}, {"atm", "ATM Network", "high", true},
		{"mail", "Email and Collaboration", "standard", false}, {"corp", "Corporate IT", "standard", false}, {"branch", "Branch Operations", "high", false}} {
		s.create("admin", "/business-services", "svc:"+v.key, H{"name": v.name, "criticality": v.level, "isSensitive": v.sensitive})
	}
	for _, a := range [][3]string{{"teller", "Teller System", "core"}, {"loans", "Loan Origination", "core"}, {"switch", "Card Switch", "pay"}, {"mobile", "Mobile Banking API", "ib"}, {"web", "Customer Portal", "ib"},
		{"exchange", "Mail Gateway", "mail"}, {"erp", "Finance ERP", "corp"}, {"hr", "HR System", "corp"}, {"monitor", "Monitoring Platform", "corp"}, {"atmc", "ATM Controller", "atm"}} {
		s.create("admin", "/applications", "app:"+a[0], H{"name": a[1], "businessServiceId": s.ref["svc:"+a[2]]})
	}
	for _, t := range [][2]string{{"Network Operations", "netops@example.com"}, {"Server Operations", "serverops@example.com"}, {"Security Operations", "secops@example.com"}, {"Branch Support", "branch@example.com"}} {
		s.create("admin", "/teams", "", H{"name": t[0], "email": t[1]})
	}
}

// addSites adds three main sites and the branches, with their zones and VLANs.
func (s *sample) addSites() {
	org := s.ref["org"]
	for _, m := range [][4]string{{"dc", "Colombo Data Centre", "CMB-DC1", "data_centre"}, {"dr", "Kandy DR Site", "KDY-DR1", "data_centre"}, {"ho", "Colombo Head Office", "CMB-HQ", "site"}} {
		s.create("admin", "/sites", "site:"+m[0], H{"organisationId": org, "name": m[1], "code": m[2], "siteType": m[3], "address": "Sample address"})
	}
	for _, m := range [][3]string{{"dc", "Core", "core"}, {"dc", "Server", "internal"}, {"dc", "DMZ", "dmz"}, {"dc", "Management", "management"}, {"dr", "Server", "internal"}, {"ho", "User", "user"}} {
		s.create("admin", "/network-zones", "zone:"+m[0]+":"+m[1], H{"name": m[1], "siteId": s.ref["site:"+m[0]], "securityZone": m[2]})
	}
	for i, name := range branches {
		key := fmt.Sprintf("site:br%d", i)
		s.create("admin", "/sites", key, H{"organisationId": org, "name": name + " Branch", "code": branchCode(i), "siteType": "branch", "address": "Sample address"})
		for _, v := range []struct {
			id   int
			name string
		}{{10, "Users"}, {20, "Voice"}, {30, "ATM and devices"}, {99, "Management"}} {
			s.create("admin", "/vlans", fmt.Sprintf("vlan:br%d:%d", i, v.id), H{"vlanId": v.id, "name": v.name, "siteId": s.ref[key]})
		}
	}
	for i, n := range dcTiers {
		s.create("admin", "/vlans", fmt.Sprintf("vlan:dc:%d", i), H{"vlanId": 100 + i, "name": n.Name, "siteId": s.ref["site:dc"]})
	}
}

func (s *sample) addBlocks() {
	for _, b := range [][4]string{{"corp", "corp", "10.0.0.0/8", "Corporate private range"}, {"dc", "corp", "10.10.0.0/16", "Colombo DC servers"}, {"dr", "corp", "10.20.0.0/16", "Kandy DR servers"},
		{"ho", "corp", "10.30.0.0/16", "Head office"}, {"br", "corp", "10.100.0.0/16", "Branches"}, {"wan", "corp", "10.255.0.0/24", "WAN links"}, {"v6", "corp", "2001:db8:10::/48", "Corporate IPv6"},
		{"dmz", "dmz", "172.16.0.0/16", "DMZ"}, {"core", "core", "10.200.0.0/16", "Core banking"}, {"pay", "pay", "10.210.0.0/16", "Payments"}, {"aws", "aws", "10.50.0.0/16", "AWS VPC"}, {"azure", "azure", "10.60.0.0/16", "Azure VNet"}} {
		body := H{"routingDomainId": s.ref["rd:"+b[1]], "cidr": b[2], "name": b[3], "purpose": "Sample data"}
		if b[0] != "corp" && b[1] == "corp" && b[0] != "v6" {
			body["parentId"] = s.ref["block:corp"]
		}
		s.create("admin", "/network-blocks", "block:"+b[0], body)
	}
}
