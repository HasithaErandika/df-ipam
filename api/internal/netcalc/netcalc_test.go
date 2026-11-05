package netcalc

import (
	"math/big"
	"net/netip"
	"testing"
)

func TestCalculate(t *testing.T) {
	cases := []struct {
		cidr, first, last, broadcast, mask, total, usable string
	}{
		{"10.0.0.0/24", "10.0.0.1", "10.0.0.254", "10.0.0.255", "255.255.255.0", "256", "254"},
		{"10.0.0.0/30", "10.0.0.1", "10.0.0.2", "10.0.0.3", "255.255.255.252", "4", "2"},
		{"10.0.0.0/31", "10.0.0.0", "10.0.0.1", "", "255.255.255.254", "2", "2"},
		{"10.0.0.1/32", "10.0.0.1", "10.0.0.1", "", "255.255.255.255", "1", "1"},
		{"0.0.0.0/0", "0.0.0.1", "255.255.255.254", "255.255.255.255", "0.0.0.0", "4294967296", "4294967294"},
		{"2001:db8::/64", "2001:db8::", "2001:db8::ffff:ffff:ffff:ffff", "", "", "18446744073709551616", "18446744073709551616"},
		{"2001:db8::/127", "2001:db8::", "2001:db8::1", "", "", "2", "2"},
		{"2001:db8::1/128", "2001:db8::1", "2001:db8::1", "", "", "1", "1"},
		{"::/0", "::", "ffff:ffff:ffff:ffff:ffff:ffff:ffff:ffff", "", "", "340282366920938463463374607431768211456", "340282366920938463463374607431768211456"},
	}
	for _, c := range cases {
		t.Run(c.cidr, func(t *testing.T) {
			got := Calculate(netip.MustParsePrefix(c.cidr))
			if got.FirstUsable != c.first || got.LastUsable != c.last || got.Broadcast != c.broadcast ||
				got.Mask != c.mask || got.Total != c.total || got.Usable != c.usable {
				t.Fatalf("got %+v", got)
			}
		})
	}
}

func TestUtilisationPercent(t *testing.T) {
	for _, c := range []struct {
		used   int64
		usable string
		want   string
	}{
		{127, "254", "50.00"}, {0, "254", "0.00"}, {5, "0", "0.00"},
		{1, "340282366920938463463374607431768211456", "0.00"},
	} {
		if got := UtilisationPercent(big.NewInt(c.used), c.usable); got != c.want {
			t.Errorf("%d/%s = %s, want %s", c.used, c.usable, got, c.want)
		}
	}
}
