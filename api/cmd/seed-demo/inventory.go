package main

import (
	"fmt"
	"strings"
)

func slug(name string) string {
	return strings.ToLower(strings.NewReplacer(" ", "-", "and-", "", "/", "-").Replace(name))
}

func (s *sample) addDevices() {
	for _, d := range [][5]string{{"core-sw-01", "switch", "Cisco", "C9500", "dc"}, {"core-sw-02", "switch", "Cisco", "C9500", "dc"}, {"dr-core-sw-01", "switch", "Cisco", "C9300", "dr"},
		{"ho-core-sw-01", "switch", "Cisco", "C9300", "ho"}, {"lb-01", "load_balancer", "F5", "BIG-IP i5800", "dc"}, {"lb-02", "load_balancer", "F5", "BIG-IP i5800", "dc"}} {
		s.device(d[0], d[1], d[2], d[3], d[4], "network_os")
	}
	for i := range branches {
		site := fmt.Sprintf("br%d", i)
		id := s.device(fmt.Sprintf("%s-rtr-01", strings.ToLower(branchCode(i))), "router", "Cisco", "ISR 1111", site, "network_os")
		s.create("engineer", "/interfaces", "", H{"deviceId": id, "name": "GigabitEthernet0/0/0", "interfaceType": "wan", "description": "WAN link", "adminStatus": "up", "operStatus": "up"})
		s.create("engineer", "/interfaces", "", H{"deviceId": id, "name": "GigabitEthernet0/0/1", "interfaceType": "lan", "description": "Branch LAN", "adminStatus": "up", "operStatus": "up"})
		s.device(fmt.Sprintf("%s-sw-01", strings.ToLower(branchCode(i))), "switch", "Cisco", "C1000", site, "network_os")
	}
	for i := 0; i < 40; i++ {
		kind, family := "virtual_machine", "linux"
		if i%4 == 0 {
			kind, family = "physical_server", "windows_server"
		}
		s.ref[fmt.Sprintf("srv:%d", i)] = s.device(fmt.Sprintf("srv-%03d", i+1), kind, "Sample", "Server", "dc", family)
	}
	// The same box twice, for the Possible duplicates tab.
	s.create("engineer", "/devices", "", H{"name": "srv-001-old", "deviceType": "physical_server", "serialNumber": "SN-SAMPLE-1", "status": "inactive"})
	s.create("engineer", "/devices", "", H{"name": "srv-001-new", "deviceType": "physical_server", "serialNumber": "SN-SAMPLE-1"})
}

func (s *sample) device(name, kind, vendor, model, site, family string) string {
	return s.create("engineer", "/devices", "", H{"name": name, "hostname": name, "deviceType": kind, "vendor": vendor, "model": model, "osFamily": family,
		"siteId": s.ref["site:"+site], "environment": "production", "ownerId": s.people["engineer"], "status": "active", "notes": "Sample data"})
}

func stateFor(k int) string {
	switch {
	case k%17 == 5:
		return "quarantined"
	case k%23 == 7:
		return "retired"
	case k%11 == 3:
		return "available_candidate"
	}
	return "allocated"
}

func macFor(n, k int) string { return fmt.Sprintf("02:df:00:%02x:%02x:%02x", n>>8&255, n&255, k&255) }

func (s *sample) addAddresses() {
	server := 0
	for i, n := range s.nets {
		for k := 0; k < n.Size; k++ {
			host := fmt.Sprintf("%s-%02d", slug(n.Name), k+1)
			body := H{"routingDomainId": s.ref["rd:"+n.Domain], "address": fmt.Sprintf("%s%d", n.Prefix, n.Start+k), "hostname": host, "fqdn": host + ".sample.example",
				"mac": macFor(i, k), "notes": "Sample data", "ownerId": s.people["requester"], "allocationState": stateFor(k)}
			if id, ok := s.ref["app:"+n.App]; ok {
				body["applicationId"] = id
			}
			if strings.HasPrefix(n.Key, "dc") && k < 3 && server < 40 {
				body["deviceId"] = s.ref[fmt.Sprintf("srv:%d", server)]
				server++
			}
			s.must("engineer", "POST", "/ip-addresses", body)
		}
	}
	s.addDNS()
}

// addDNS has no API, because DNS is read-only evidence. One name disagrees on purpose.
func (s *sample) addDNS() {
	rows := [][3]string{{"portal.sample.example", "A", "172.16.1.10"}, {"api.sample.example", "A", "172.16.4.10"}, {"mail.sample.example", "A", "172.16.3.10"}, {"old-app.sample.example", "A", "10.10.10.250"}}
	for _, r := range rows {
		domain := s.ref["rd:dmz"]
		if strings.HasPrefix(r[2], "10.") {
			domain = s.ref["rd:corp"]
		}
		if _, err := s.pool.Exec(s.ctx, `INSERT INTO dns_records (name, record_type, value, routing_domain_id, last_seen) VALUES ($1, $2, $3, $4::uuid, now())`, r[0], r[1], r[2], domain); err != nil {
			panic(err)
		}
	}
}
