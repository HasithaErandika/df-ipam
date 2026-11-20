package inventory

import "testing"

func TestSegmentHostAddresses(t *testing.T) {
	cases := []struct {
		name                string
		gateway, start, end string
		ok                  bool
	}{
		{"valid", "192.168.4.1", "192.168.4.100", "192.168.4.250", true},
		{"gateway is the network address", "192.168.4.0", "", "", false},
		{"gateway is the broadcast address", "192.168.4.255", "", "", false},
		{"pool starts at the network address", "192.168.4.1", "192.168.4.0", "192.168.4.50", false},
		{"pool ends at the broadcast address", "192.168.4.1", "192.168.4.100", "192.168.4.255", false},
		{"gateway inside a pool that excludes it", "192.168.4.120", "192.168.4.1", "192.168.4.254", true},
	}
	for _, c := range cases {
		in := segmentInput{Name: "Floor 4", Kind: "dhcp", Network: "192.168.4.0/24", Gateway: c.gateway, DHCPStart: c.start, DHCPEnd: c.end}
		if err := in.validate(); (err == nil) != c.ok {
			t.Errorf("%s: got %v", c.name, err)
		}
	}
}

// A WAN hand-off or a single public address is a segment too.
func TestSmallSegments(t *testing.T) {
	cases := []struct {
		network, gateway string
		ok               bool
	}{
		{"122.255.36.14/32", "", true},
		{"122.255.36.14/32", "122.255.36.14", false},
		{"172.16.5.6/31", "172.16.5.7", true},
		{"172.16.5.4/30", "172.16.5.5", true},
		{"10.0.0.0/8", "", false},
	}
	for _, c := range cases {
		in := segmentInput{Name: "WAN", Kind: "server", Network: c.network, Gateway: c.gateway}
		if err := in.validate(); (err == nil) != c.ok {
			t.Errorf("%s gw %q: got %v", c.network, c.gateway, err)
		}
	}
	if got := parseSegmentText("DAP ILL 122.255.36.14/32\n"); len(got) != 1 || got[0].Skip != "" || got[0].Gateway.IsValid() {
		t.Errorf("a /32 line: %+v", got)
	}
}
