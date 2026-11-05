package subnets

import (
	"context"
	"fmt"
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
	cases := []struct {
		kind, cidr, provider string
		ranges               []string
	}{
		{"vpc", "10.0.0.0/24", "aws", []string{"10.0.0.0-10.0.0.3", "10.0.0.255-10.0.0.255"}},
		{"vnet", "10.1.0.0/29", "azure", []string{"10.1.0.0-10.1.0.3", "10.1.0.7-10.1.0.7"}},
		{"vpc", "2001:db8::/64", "aws", []string{"2001:db8::-2001:db8::3", "2001:db8::ffff:ffff:ffff:ffff-2001:db8::ffff:ffff:ffff:ffff"}},
		{"vpc", "10.0.0.0/30", "", nil},
		{"default", "10.0.0.0/24", "", nil},
		{"vrf", "10.0.0.0/24", "", nil},
	}
	for _, c := range cases {
		p, _ := ParseCIDR(c.cidr)
		provider, ranges := ProviderReserved(c.kind, p)
		var got []string
		for _, r := range ranges {
			got = append(got, r.First.String()+"-"+r.Last.String())
		}
		if provider != c.provider || fmt.Sprint(got) != fmt.Sprint(c.ranges) {
			t.Errorf("%s %s: got %q %v", c.kind, c.cidr, provider, got)
		}
	}
}

func TestBulkValidation(t *testing.T) {
	s := &Service{}
	name := "x"
	for _, req := range []BulkRequest{
		{Action: "archive"},
		{IDs: []string{"nope"}, Action: "archive"},
		{IDs: []string{"00000000-0000-0000-0000-000000000001"}, Action: "explode"},
		{IDs: []string{"00000000-0000-0000-0000-000000000001"}, Action: "update"},
		{IDs: []string{"00000000-0000-0000-0000-000000000001"}, Action: "update", Changes: UpdateRequest{Name: &name}},
	} {
		if _, err := s.Bulk(context.Background(), "u", req); err == nil {
			t.Errorf("%+v: expected rejection", req)
		}
	}
}
