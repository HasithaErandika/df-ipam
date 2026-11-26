package inventory

import (
	"net/netip"
	"strings"
	"testing"
	"unicode/utf8"
)

const fortiSample = `POT-IDC-00-FW-01 $ show system interface
name    Name.
COLOTR-PUB   static   0.0.0.0 0.0.0.0  10.121.11.108 255.255.255.248  up   disable   vlan  disable
CTF - CISCO   static   0.0.0.0 0.0.0.0  0.0.0.0 0.0.0.0  up   disable   tunnel  disable
CTF_IT_Loopback   static   0.0.0.0 0.0.0.0  123.231.68.146 255.255.255.255  up   disable   loopback  disable
Sub_vlan 60   static   0.0.0.0 0.0.0.0  172.20.16.94 255.255.255.248  up   disable   vlan  disable
Sub_vlan_55   static   0.0.0.0 0.0.0.0  172.20.16.62 255.255.255.240  down   disable   vlan  disable
`

func TestParseFortiGateInterfaces(t *testing.T) {
	got := parseSegmentText(fortiSample)
	if len(got) != 5 {
		t.Fatalf("got %d rows: %+v", len(got), got)
	}
	pub := got[0]
	if pub.Name != "COLOTR-PUB" || pub.Network.String() != "10.121.11.104/29" || pub.Gateway.String() != "10.121.11.108" || pub.Firewall != "POT-IDC-00-FW-01" || pub.Skip != "" {
		t.Errorf("COLOTR-PUB: %+v", pub)
	}
	if got[1].Name != "CTF - CISCO" || got[1].Skip == "" {
		t.Errorf("a tunnel with no address must be skipped: %+v", got[1])
	}
	if got[2].Skip == "" {
		t.Errorf("a loopback must be skipped: %+v", got[2])
	}
	if got[3].Name != "Sub_vlan 60" || got[3].VLAN == nil || *got[3].VLAN != 60 || got[3].Network.String() != "172.20.16.88/29" {
		t.Errorf("a name with a space: %+v", got[3])
	}
	if !got[4].Down || got[4].Network.String() != "172.20.16.48/28" {
		t.Errorf("a down interface: %+v", got[4])
	}
}

func TestParseCIDRLines(t *testing.T) {
	got := parseSegmentText("Floor 3 192.168.3.0/24\n10.9.0.1/30, link to branch\n10.1.1.0/24\nnot a network\n")
	if len(got) != 3 {
		t.Fatalf("got %+v", got)
	}
	if got[0].Name != "Floor 3" || got[0].Network.String() != "192.168.3.0/24" || got[0].Gateway.IsValid() {
		t.Errorf("%+v", got[0])
	}
	if got[1].Name != "link to branch" || got[1].Gateway.String() != "10.9.0.1" {
		t.Errorf("%+v", got[1])
	}
	if got[2].Name != "10.1.1.0/24" {
		t.Errorf("a bare CIDR names itself: %+v", got[2])
	}
}

func TestMaskBits(t *testing.T) {
	for mask, want := range map[string]int{"255.255.255.248": 29, "255.255.255.0": 24, "255.255.255.252": 30, "0.0.0.0": 0} {
		if got, err := maskBits(mask); err != nil || got != want {
			t.Errorf("%s = %d, %v", mask, got, err)
		}
	}
	if _, err := maskBits("255.0.255.0"); err == nil {
		t.Error("a broken mask must fail")
	}
}

func TestPlanSegments(t *testing.T) {
	existing := []existingSegment{{"Wi-Fi", netip.MustParsePrefix("192.168.1.0/24")}, {"DMZ", netip.MustParsePrefix("172.20.14.192/29")}}
	text := `FW-A $ show system interface
DMZ          static 0.0.0.0 0.0.0.0 172.20.14.193 255.255.255.248 up disable vlan disable
Sub_vlan_3   static 0.0.0.0 0.0.0.0 172.20.14.14 255.255.255.248 up disable vlan disable
Big          static 0.0.0.0 0.0.0.0 192.168.0.1 255.255.0.0 up disable vlan disable
Sub_vlan_4   static 0.0.0.0 0.0.0.0 172.20.14.22 255.255.255.248 up disable vlan disable
FW-B # show system interface
Sub_vlan_3   static 0.0.0.0 0.0.0.0 172.20.14.30 255.255.255.248 up disable vlan disable
`
	rows := planSegments(parseSegmentText(text), existing, []int{5})
	want := []struct {
		add    bool
		name   string
		reason string
	}{
		{false, "DMZ", "Already a segment: DMZ"},
		{true, "Sub_vlan_3", ""},
		{false, "Big", "Overlaps Wi-Fi (192.168.1.0/24)"},
		{false, "Sub_vlan_4", "Left out"},
		{true, "Sub_vlan_3 (FW-B)", ""},
	}
	if len(rows) != len(want) {
		t.Fatalf("got %+v", rows)
	}
	for i, w := range want {
		if rows[i].Add != w.add || rows[i].Name != w.name || rows[i].Reason != w.reason {
			t.Errorf("row %d: got %+v, want %+v", i, rows[i], w)
		}
	}
}

// Pasted rows follow the form's rules: no prefix wider than /16, no gateway on
// the broadcast address, and a long name cut by character, not by byte.
func TestPlanAppliesTheFormRules(t *testing.T) {
	long := strings.Repeat("é", 81)
	rows := planSegments([]textSegment{
		{Line: 1, Name: "wide", Network: netip.MustParsePrefix("10.0.0.0/8")},
		{Line: 2, Name: "bcast", Network: netip.MustParsePrefix("10.1.0.0/24"), Gateway: netip.MustParseAddr("10.1.0.255")},
		{Line: 3, Name: long, Network: netip.MustParsePrefix("10.2.0.0/24")},
	}, nil, nil)
	if rows[0].Add || rows[0].Reason == "" {
		t.Errorf("a /8 was added: %+v", rows[0])
	}
	if !rows[1].Add || rows[1].Gateway != "" {
		t.Errorf("broadcast gateway kept: %+v", rows[1])
	}
	if !utf8.ValidString(rows[2].Name) || utf8.RuneCountInString(rows[2].Name) != 80 {
		t.Errorf("name cut badly: %q", rows[2].Name)
	}
}
