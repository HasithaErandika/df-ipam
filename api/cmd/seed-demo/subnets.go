package main

import (
	"fmt"
	"net/netip"
)

// network is one subnet in the plan, kept for the steps that fill it.
type network struct {
	ID, Name, Domain, Prefix string // Prefix is the address up to the last octet
	Start, Size              int    // first last-octet to use, and how many to fill
	Key, App                 string
	Discover                 bool
}

var dcTiers = []struct{ Name, App string }{{"Web tier", "web"}, {"Application tier", "mobile"}, {"Database tier", "erp"}, {"Directory services", "hr"}, {"Mail", "exchange"}, {"Virtualisation", "monitor"},
	{"Backup", "monitor"}, {"Storage", "monitor"}, {"Monitoring", "monitor"}, {"Jump hosts", "monitor"}, {"Finance systems", "erp"}, {"HR systems", "hr"}, {"Integration", "mobile"}, {"Test", "web"}, {"UAT", "web"}, {"Build", "monitor"}}

type plan struct {
	key, domain, cidr, name, gateway, env, service, unit, block, site, vlan, zone, app string
	discover                                                                           bool
	fill                                                                               int
}

func (s *sample) subnet(p plan) {
	body := H{"routingDomainId": s.ref["rd:"+p.domain], "cidr": p.cidr, "name": p.name, "environment": p.env, "description": "Sample data", "discoveryEnabled": p.discover,
		"technicalOwnerId": s.owner(p), "businessOwnerId": s.people["requester2"]}
	if p.gateway != "" {
		body["gateway"] = p.gateway
	}
	for field, key := range map[string]string{"businessServiceId": "svc:" + p.service, "businessUnitId": "unit:" + p.unit, "networkBlockId": "block:" + p.block, "siteId": "site:" + p.site, "vlanId": p.vlan, "networkZoneId": p.zone} {
		if id, ok := s.ref[key]; ok {
			body[field] = id
		}
	}
	id := s.create("engineer", "/subnets", "subnet:"+p.key, body)
	s.nets = append(s.nets, network{ID: id, Name: p.name, Domain: p.domain, Prefix: prefixOf(p.cidr), Start: firstHost(p.cidr), Size: p.fill, Key: p.key, App: p.app, Discover: p.discover})
}

// owner leaves a few subnets without one, for the Ownership missing finding.
func (s *sample) owner(p plan) any {
	if p.domain == "lab" {
		return nil
	}
	return s.people["engineer"]
}

// firstHost leaves room for the gateway, and in larger subnets for what providers keep.
func firstHost(cidr string) int {
	p, err := netip.ParsePrefix(cidr)
	if err != nil || !p.Addr().Is4() {
		return 0
	}
	last := int(p.Addr().As4()[3])
	if p.Bits() >= 26 {
		return last + 2
	}
	return last + 10
}

func prefixOf(cidr string) string {
	for i := len(cidr) - 1; i >= 0; i-- {
		if cidr[i] == '.' || cidr[i] == ':' {
			return cidr[:i+1]
		}
	}
	return cidr
}

func (s *sample) addSubnets() {
	s.dataCentres()
	s.headOffice()
	s.branchNetworks()
	s.otherDomains()
	s.protect()
}

func (s *sample) dataCentres() {
	for i, t := range dcTiers {
		env, fill := "production", 12+i%5*6
		if t.Name == "Test" || t.Name == "UAT" {
			env = map[string]string{"Test": "testing", "UAT": "uat"}[t.Name]
		}
		s.subnet(plan{key: fmt.Sprintf("dc%d", i), domain: "corp", cidr: fmt.Sprintf("10.10.%d.0/24", 10+i), name: "DC " + t.Name, gateway: fmt.Sprintf("10.10.%d.1", 10+i), env: env, service: "corp", unit: "IT",
			block: "dc", site: "dc", vlan: fmt.Sprintf("vlan:dc:%d", i), zone: "zone:dc:Server", discover: i < 8, fill: fill, app: t.App})
		if i < 8 {
			s.subnet(plan{key: fmt.Sprintf("dr%d", i), domain: "corp", cidr: fmt.Sprintf("10.20.%d.0/24", 10+i), name: "DR " + t.Name, gateway: fmt.Sprintf("10.20.%d.1", 10+i), env: "disaster_recovery", service: "corp", unit: "IT", block: "dr", site: "dr", zone: "zone:dr:Server", fill: 6})
		}
	}
	s.subnet(plan{key: "dcfull", domain: "corp", cidr: "10.10.99.0/28", name: "DC Load balancer VIPs", gateway: "10.10.99.1", env: "production", service: "ib", unit: "IT", block: "dc", site: "dc", fill: 13})
}

func (s *sample) headOffice() {
	for floor := 1; floor <= 6; floor++ {
		s.subnet(plan{key: fmt.Sprintf("hou%d", floor), domain: "corp", cidr: fmt.Sprintf("10.30.%d.0/24", floor), name: fmt.Sprintf("HO Floor %d Users", floor), gateway: fmt.Sprintf("10.30.%d.1", floor), env: "production", service: "corp", unit: "IT", block: "ho", site: "ho", zone: "zone:ho:User", discover: floor < 3, fill: 18})
		s.subnet(plan{key: fmt.Sprintf("hov%d", floor), domain: "corp", cidr: fmt.Sprintf("10.30.%d.0/24", 20+floor), name: fmt.Sprintf("HO Floor %d Voice", floor), gateway: fmt.Sprintf("10.30.%d.1", 20+floor), env: "production", service: "corp", unit: "IT", block: "ho", site: "ho", zone: "zone:ho:User", fill: 6})
	}
	s.subnet(plan{key: "howifi", domain: "corp", cidr: "10.30.40.0/23", name: "HO Staff Wi-Fi", gateway: "10.30.40.1", env: "production", service: "corp", unit: "IT", block: "ho", site: "ho", fill: 8})
}

func (s *sample) branchNetworks() {
	for i, name := range branches {
		base := fmt.Sprintf("10.100.%d.", i+1)
		common := plan{domain: "corp", env: "production", unit: "RB", block: "br", site: fmt.Sprintf("br%d", i)}
		for _, n := range []struct {
			suffix, label, cidr, gw, service string
			vlan, fill                       int
		}{{"u", "Users", "0/25", "1", "branch", 10, 10}, {"v", "Voice", "128/26", "129", "branch", 20, 4}, {"a", "ATM and devices", "192/27", "193", "atm", 30, 3}, {"m", "Management", "224/28", "225", "branch", 99, 3}} {
			p := common
			p.key, p.name, p.cidr, p.gateway, p.service = fmt.Sprintf("br%d%s", i, n.suffix), name+" "+n.label, base+n.cidr, base+n.gw, n.service
			p.vlan, p.fill, p.discover = fmt.Sprintf("vlan:br%d:%d", i, n.vlan), n.fill, i < 5 && n.suffix == "u"
			s.subnet(p)
		}
		s.subnet(plan{key: fmt.Sprintf("wan%d", i), domain: "corp", cidr: fmt.Sprintf("10.255.0.%d/31", i*2), name: name + " WAN link", env: "production", service: "branch", unit: "IT", block: "wan", site: fmt.Sprintf("br%d", i)})
	}
}

func (s *sample) otherDomains() {
	for i, n := range []string{"Web", "Reverse proxy", "Mail relay", "API gateway", "VPN", "DNS", "SFTP", "Partner"} {
		s.subnet(plan{key: fmt.Sprintf("dmz%d", i), domain: "dmz", cidr: fmt.Sprintf("172.16.%d.0/24", i+1), name: "DMZ " + n, gateway: fmt.Sprintf("172.16.%d.1", i+1), env: "production", service: "ib", unit: "IT", block: "dmz", site: "dc", zone: "zone:dc:DMZ", fill: 8})
	}
	for i, n := range []string{"Core app", "Core database", "Core batch", "Teller gateway", "Core DR replication", "Core management"} {
		s.subnet(plan{key: fmt.Sprintf("core%d", i), domain: "core", cidr: fmt.Sprintf("10.200.%d.0/24", i+1), name: n, gateway: fmt.Sprintf("10.200.%d.1", i+1), env: "production", service: "core", unit: "IT", block: "core", site: "dc", fill: 10})
	}
	for i, n := range []string{"Card switch", "HSM", "ATM host", "Payment gateway"} {
		s.subnet(plan{key: fmt.Sprintf("pay%d", i), domain: "pay", cidr: fmt.Sprintf("10.210.%d.0/24", i+1), name: n, gateway: fmt.Sprintf("10.210.%d.1", i+1), env: "production", service: "pay", unit: "IT", block: "pay", site: "dc", fill: 8})
	}
	for i, n := range []string{"Web", "App", "Data", "Tools", "Shared", "Egress"} {
		s.subnet(plan{key: fmt.Sprintf("aws%d", i), domain: "aws", cidr: fmt.Sprintf("10.50.%d.0/24", i+1), name: "AWS " + n, env: "production", service: "ib", unit: "IT", block: "aws", fill: 6})
	}
	for i, n := range []string{"Web", "App", "Data", "Shared", "Gateway"} {
		s.subnet(plan{key: fmt.Sprintf("az%d", i), domain: "azure", cidr: fmt.Sprintf("10.60.%d.0/24", i+1), name: "Azure " + n, env: "production", service: "corp", unit: "IT", block: "azure", fill: 5})
	}
	s.subnet(plan{key: "lab", domain: "lab", cidr: "10.10.10.0/24", name: "Lab copy of DC Web tier", gateway: "10.10.10.1", env: "testing", fill: 4})
	s.subnet(plan{key: "guest", domain: "guest", cidr: "10.90.0.0/22", name: "Guest Wi-Fi", gateway: "10.90.0.1", env: "production", service: "corp", unit: "IT"})
	for i, n := range []string{"DC Servers", "DC Web", "HO Users", "HO Wi-Fi"} {
		s.subnet(plan{key: fmt.Sprintf("v6%d", i), domain: "corp", cidr: fmt.Sprintf("2001:db8:10:%x::/64", i+1), name: "IPv6 " + n, env: "production", service: "corp", unit: "IT", block: "v6"})
	}
	s.subnet(plan{key: "v6link", domain: "corp", cidr: "2001:db8:10:ff::/127", name: "IPv6 core link", env: "production", service: "corp", unit: "IT", block: "v6"})
}

// protect marks core banking and payments as sensitive, through the normal edit.
func (s *sample) protect() {
	for _, n := range s.nets {
		if n.Domain != "core" && n.Domain != "pay" {
			continue
		}
		cur := s.must("engineer", "GET", "/subnets/"+n.ID, nil)
		s.must("engineer", "PATCH", "/subnets/"+n.ID, H{"sensitivity": "restricted"}, "If-Match", fmt.Sprint(cur["version"]))
	}
}
