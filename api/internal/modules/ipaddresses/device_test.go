package ipaddresses

import "testing"

func TestDescribeMAC(t *testing.T) {
	m := DescribeMAC("00:1A:2B:3C:4D:5E")
	if m == nil || m.LocallyAdministered || m.Multicast || m.OUI != "00:1a:2b" || m.Address != "00:1a:2b:3c:4d:5e" {
		t.Fatalf("universal: %+v", m)
	}
	if m := DescribeMAC("02:00:00:00:00:01"); m == nil || !m.LocallyAdministered {
		t.Fatalf("locally administered: %+v", m)
	}
	if m := DescribeMAC("01:00:5e:00:00:01"); m == nil || !m.Multicast {
		t.Fatalf("multicast: %+v", m)
	}
	if DescribeMAC("nonsense") != nil {
		t.Fatal("bad MAC accepted")
	}
}
