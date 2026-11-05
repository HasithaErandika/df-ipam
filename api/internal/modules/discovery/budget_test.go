package discovery

import (
	"net/netip"
	"testing"
)

func TestFitsBudget(t *testing.T) {
	for _, c := range []struct {
		running, job, ceiling int
		want                  bool
	}{
		{0, 50, 500, true},
		{450, 50, 500, true},
		{451, 50, 500, false},
		{0, 1000, 500, true}, // alone on the network, or it would never run
		{10, 1000, 500, false},
	} {
		if got := fitsBudget(c.running, c.job, c.ceiling); got != c.want {
			t.Errorf("%+v: got %v", c, got)
		}
	}
}

func TestGrantedRateNeverExceedsTheCeiling(t *testing.T) {
	for _, c := range []struct{ running, job, ceiling, want int }{
		{0, 50, 100, 50},
		{0, 1000, 100, 100}, // alone, it still can't pass the ceiling
		{80, 50, 100, 20},
		{100, 50, 100, 1}, // a resumed job keeps crawling, never bursting
	} {
		if got := grantedRate(c.running, c.job, c.ceiling); got != c.want {
			t.Errorf("%+v: got %d", c, got)
		}
	}
}

func TestProbeableRefusesFanOutRanges(t *testing.T) {
	for cidr, want := range map[string]bool{"10.0.0.0/24": true, "2001:db8::/120": true, "224.0.0.0/24": false, "ff02::/120": false,
		"127.0.0.0/24": false, "169.254.0.0/24": false, "0.0.0.0/24": false, "fe80::/120": false} {
		if got := Probeable(netip.MustParsePrefix(cidr)); got != want {
			t.Errorf("%s: got %v", cidr, got)
		}
	}
}
