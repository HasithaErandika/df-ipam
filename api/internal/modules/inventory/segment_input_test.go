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
