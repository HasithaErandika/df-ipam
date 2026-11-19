package fortigate

import "testing"

// A lease alone doesn't mean the device is there; one the ARP table confirms does,
// and its details win over the bare ARP entry.
func TestSplitEvidence(t *testing.T) {
	present, enrich := splitEvidence([]Entry{
		{IP: "192.168.1.10", MAC: "00:11:22:33:44:55", Source: "arp"},
		{IP: "192.168.1.10", MAC: "00-11-22-33-44-55", Hostname: "PC-10", Source: "dhcp"},
		{IP: "192.168.1.11", MAC: "00:11:22:33:44:66", Hostname: "LAPTOP-11", Source: "dhcp"},
		{IP: "10.212.1.5", User: "amal", VPNKind: "ssl", Source: "vpn"},
	})
	if len(present) != 2 || present[0].Source != "dhcp" || present[0].Hostname != "PC-10" || present[1].Source != "vpn" {
		t.Fatalf("present: %+v", present)
	}
	if len(enrich) != 1 || enrich[0].IP != "192.168.1.11" {
		t.Fatalf("enrich: %+v", enrich)
	}
}
