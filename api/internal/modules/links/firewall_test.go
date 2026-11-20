package links

import (
	"testing"

	"dfipam/api/internal/integrations/fortigate"
)

func TestFortiOSAddresses(t *testing.T) {
	if a, p := ipAndPrefix("203.0.113.2 255.255.255.252"); a != "203.0.113.2" || p != "/30" {
		t.Fatalf("got %s %s", a, p)
	}
	if a, _ := ipAndPrefix("0.0.0.0 0.0.0.0"); a != "" {
		t.Fatalf("an unset address must give nothing, got %q", a)
	}
	if n := network("10.20.0.5 255.255.0.0"); n != "10.20.0.0/16" {
		t.Fatalf("got %q", n)
	}
}

func TestTunnelProposal(t *testing.T) {
	p1 := fortigate.Phase1{Name: "HO-Partner", Interface: "port1", RemoteGW: "198.51.100.4", Type: "static"}
	p2 := []fortigate.Phase2{
		{Phase1: "HO-Partner", Src: "192.168.10.0 255.255.255.0", Dst: "10.20.0.0 255.255.0.0"},
		{Phase1: "HO-Partner", Src: "192.168.99.0 255.255.255.0", DstType: "name", Dst: ""},
		{Phase1: "Other", Src: "1.1.1.0 255.255.255.0", Dst: "2.2.2.0 255.255.255.0"},
	}
	segs := []segmentRef{{ID: "s1", Name: "Floor 1", Network: "192.168.10.0/24"}}
	p := tunnelProposal(p1, p2, map[string]string{"port1": "203.0.113.2"}, segs, "fw")
	s := p.Suggestion
	if p.Key != "ipsec:HO-Partner" || s.LinkType != "site_vpn" || s.LocalPeer != "203.0.113.2" || s.RemotePeer != "198.51.100.4" || s.Underlying != "interface:port1" {
		t.Fatalf("proposal: %+v", p)
	}
	if len(s.RemoteSubnets) != 1 || s.RemoteSubnets[0] != "10.20.0.0/16" || len(s.LocalSegments) != 1 {
		t.Fatalf("subnets: %v %v", s.RemoteSubnets, s.LocalSegments)
	}
	if len(p.Notes) != 2 {
		t.Fatalf("an unknown local subnet and a named remote address are noted: %v", p.Notes)
	}
	if d := tunnelProposal(fortigate.Phase1{Name: "dialup", Type: "dynamic", RemoteGW: "0.0.0.0"}, nil, nil, nil, "fw"); d.Kind != "dialup" || d.Suggestion.LinkType != "remote_vpn" || d.Suggestion.RemotePeer != "" {
		t.Fatalf("dial-up: %+v", d)
	}
}

func TestRates(t *testing.T) {
	if r := rate(2000, 1000, 2); r == nil || *r != 4000 {
		t.Fatalf("1000 bytes in 2 s is 4000 bit/s, got %v", r)
	}
	if rate(10, 1000, 2) != nil {
		t.Fatal("a counter that went back (a reboot) gives no rate")
	}
}
