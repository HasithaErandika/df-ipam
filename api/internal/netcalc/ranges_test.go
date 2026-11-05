package netcalc

import (
	"net/netip"
	"testing"
)

func rng(a, b string) Range { return Range{netip.MustParseAddr(a), netip.MustParseAddr(b)} }

func strs(ps []netip.Prefix) string {
	out := ""
	for _, p := range ps {
		out += p.String() + " "
	}
	return out
}

func TestGaps(t *testing.T) {
	outer := rng("10.0.0.1", "10.0.0.10")
	got := Gaps(outer, []Range{rng("10.0.0.3", "10.0.0.4"), rng("10.0.0.4", "10.0.0.5"), rng("10.0.0.10", "10.0.0.20")})
	if len(got) != 2 || got[0] != rng("10.0.0.1", "10.0.0.2") || got[1] != rng("10.0.0.6", "10.0.0.9") {
		t.Fatalf("got %v", got)
	}
	if g := Gaps(outer, nil); len(g) != 1 || g[0] != outer {
		t.Fatalf("empty: %v", g)
	}
	if g := Gaps(outer, []Range{rng("10.0.0.0", "10.0.0.255")}); len(g) != 0 {
		t.Fatalf("full: %v", g)
	}
}

func TestGapsOnWholeIPv6Space(t *testing.T) {
	all := PrefixRange(netip.MustParsePrefix("::/0"))
	got := Gaps(all, []Range{rng("::", "::1"), rng("ffff:ffff:ffff:ffff:ffff:ffff:ffff:ffff", "ffff:ffff:ffff:ffff:ffff:ffff:ffff:ffff")})
	if len(got) != 1 || got[0].First.String() != "::2" {
		t.Fatalf("got %v", got)
	}
	if Total(got).String() != "340282366920938463463374607431768211453" {
		t.Fatalf("total %s", Total(got))
	}
}

func TestPrefixes(t *testing.T) {
	if got := strs(Prefixes(rng("10.0.0.1", "10.0.0.6"), 100)); got != "10.0.0.1/32 10.0.0.2/31 10.0.0.4/31 10.0.0.6/32 " {
		t.Fatal(got)
	}
	if got := strs(Prefixes(rng("0.0.0.0", "255.255.255.255"), 100)); got != "0.0.0.0/0 " {
		t.Fatal(got)
	}
	if got := Prefixes(rng("10.0.0.1", "10.0.0.6"), 2); len(got) != 2 {
		t.Fatal(got)
	}
}

func TestSummarise(t *testing.T) {
	in := []netip.Prefix{netip.MustParsePrefix("10.0.0.0/25"), netip.MustParsePrefix("10.0.0.128/25"),
		netip.MustParsePrefix("10.0.1.0/24"), netip.MustParsePrefix("2001:db8::/33"), netip.MustParsePrefix("2001:db8:8000::/33")}
	if got := strs(Summarise(in)); got != "10.0.0.0/23 2001:db8::/32 " {
		t.Fatal(got)
	}
}

func TestSplit(t *testing.T) {
	got, total, err := Split(netip.MustParsePrefix("10.0.0.0/24"), 26, 10)
	if err != nil || total != "4" || strs(got) != "10.0.0.0/26 10.0.0.64/26 10.0.0.128/26 10.0.0.192/26 " {
		t.Fatal(got, total, err)
	}
	got, total, err = Split(netip.MustParsePrefix("2001:db8::/32"), 64, 2)
	if err != nil || total != "4294967296" || len(got) != 2 {
		t.Fatal(got, total, err)
	}
	if _, _, err := Split(netip.MustParsePrefix("10.0.0.0/24"), 24, 2); err == nil {
		t.Fatal("expected error")
	}
}

func TestUsableRange(t *testing.T) {
	if got := UsableRange(netip.MustParsePrefix("10.0.0.0/30")); got != rng("10.0.0.1", "10.0.0.2") {
		t.Fatal(got)
	}
	if got := UsableRange(netip.MustParsePrefix("10.0.0.0/31")); got != rng("10.0.0.0", "10.0.0.1") {
		t.Fatal(got)
	}
}
