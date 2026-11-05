package subnets

import (
	"strings"
	"testing"
)

func TestParseCIDR(t *testing.T) {
	for _, ok := range []string{"10.0.0.0/24", "10.0.0.0/31", "10.0.0.1/32", "2001:db8::/64", "2001:db8::1/128", " 192.168.0.0/16 "} {
		if _, err := ParseCIDR(ok); err != nil {
			t.Errorf("%q: %v", ok, err)
		}
	}
	for _, bad := range []string{"", "10.0.0.5/24", "10.0.0.0", "10.0.0.0/33", "::ffff:10.0.0.0/120", "2001:db8::1/64", "nonsense"} {
		if _, err := ParseCIDR(bad); err == nil {
			t.Errorf("%q: expected rejection", bad)
		}
	}
}

func TestValidateGateway(t *testing.T) {
	p, _ := ParseCIDR("10.0.0.0/24")
	inside, outside, junk, empty := "10.0.0.1", "10.0.1.1", "gateway", ""
	if validateGateway(&inside, p) != nil || validateGateway(&empty, p) != nil || validateGateway(nil, p) != nil {
		t.Error("valid gateway rejected")
	}
	if validateGateway(&outside, p) == nil || validateGateway(&junk, p) == nil {
		t.Error("invalid gateway accepted")
	}
}

func TestValidateThresholds(t *testing.T) {
	if validateThresholds(80, 95) != nil {
		t.Error("valid thresholds rejected")
	}
	for _, c := range [][2]int16{{95, 80}, {80, 80}, {0, 50}, {80, 101}} {
		if validateThresholds(c[0], c[1]) == nil {
			t.Errorf("%v accepted", c)
		}
	}
}

func TestOrderByIgnoresUnknownFields(t *testing.T) {
	if got := orderBy("name; DROP TABLE subnets"); got != "s.cidr ASC, s.id" {
		t.Fatalf("got %q", got)
	}
	if got := orderBy("-updatedAt"); got != "s.updated_at DESC, s.id" {
		t.Fatalf("got %q", got)
	}
}

func TestProviderReserved(t *testing.T) {
	for cidr, want := range map[string]string{
		"10.0.0.0/24":    "10.0.0.0-10.0.0.3,10.0.0.255-10.0.0.255",
		"10.1.0.0/29":    "10.1.0.0-10.1.0.3,10.1.0.7-10.1.0.7",
		"2001:db8::/64":  "2001:db8::-2001:db8::3,2001:db8::ffff:ffff:ffff:ffff-2001:db8::ffff:ffff:ffff:ffff",
		"10.2.0.0/30":    "",
		"2001:db8::/126": "",
	} {
		p, _ := ParseCIDR(cidr)
		var parts []string
		for _, r := range ProviderReserved(p) {
			parts = append(parts, r.First.String()+"-"+r.Last.String())
		}
		if got := strings.Join(parts, ","); got != want {
			t.Errorf("%s: got %q want %q", cidr, got, want)
		}
	}
}
