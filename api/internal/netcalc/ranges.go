package netcalc

import (
	"fmt"
	"math/big"
	"net/netip"
	"sort"
)

// Range is an inclusive span of addresses in one family.
type Range struct {
	First netip.Addr `json:"first"`
	Last  netip.Addr `json:"last"`
}

func toInt(a netip.Addr) *big.Int { return new(big.Int).SetBytes(a.AsSlice()) }

func fromInt(v *big.Int, is4 bool) netip.Addr {
	size := 16
	if is4 {
		size = 4
	}
	a, _ := netip.AddrFromSlice(v.FillBytes(make([]byte, size)))
	return a
}

// Size returns the number of addresses in the range.
func (r Range) Size() *big.Int {
	n := new(big.Int).Sub(toInt(r.Last), toInt(r.First))
	return n.Add(n, big.NewInt(1))
}

// Offset returns the address n places after a.
func Offset(a netip.Addr, n *big.Int) netip.Addr {
	return fromInt(new(big.Int).Add(toInt(a), n), a.Is4())
}

// PrefixRange returns every address in the prefix.
func PrefixRange(p netip.Prefix) Range { return Range{p.Masked().Addr(), lastAddress(p.Masked())} }

// UsableRange returns the assignable addresses in the prefix.
func UsableRange(p netip.Prefix) Range {
	r := PrefixRange(p)
	if p.Addr().Is4() && p.Bits() <= 30 {
		return Range{r.First.Next(), r.Last.Prev()}
	}
	return r
}

// Merge sorts ranges and joins any that overlap or touch.
func Merge(ranges []Range) []Range {
	sorted := append([]Range(nil), ranges...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].First.Less(sorted[j].First) })
	var out []Range
	for _, r := range sorted {
		if n := len(out); n > 0 {
			next := out[n-1].Last.Next()
			if !next.IsValid() || !next.Less(r.First) {
				if out[n-1].Last.Less(r.Last) {
					out[n-1].Last = r.Last
				}
				continue
			}
		}
		out = append(out, r)
	}
	return out
}

// Clip keeps only the parts of ranges inside outer.
func Clip(outer Range, ranges []Range) []Range {
	var out []Range
	for _, r := range ranges {
		if r.Last.Less(outer.First) || outer.Last.Less(r.First) {
			continue
		}
		if r.First.Less(outer.First) {
			r.First = outer.First
		}
		if outer.Last.Less(r.Last) {
			r.Last = outer.Last
		}
		out = append(out, r)
	}
	return out
}

// Gaps returns the parts of outer not covered by used. Its cost depends on the
// number of used ranges, never on the size of outer, so it is safe for IPv6.
func Gaps(outer Range, used []Range) []Range {
	var out []Range
	cursor := outer.First
	for _, r := range Merge(Clip(outer, used)) {
		if cursor.Less(r.First) {
			out = append(out, Range{cursor, r.First.Prev()})
		}
		cursor = r.Last.Next()
		if !cursor.IsValid() || outer.Last.Less(cursor) {
			return out
		}
	}
	return append(out, Range{cursor, outer.Last})
}

// Total adds up the sizes of ranges.
func Total(ranges []Range) *big.Int {
	sum := new(big.Int)
	for _, r := range ranges {
		sum.Add(sum, r.Size())
	}
	return sum
}

// Prefixes expresses a range as the fewest CIDRs, returning at most limit.
func Prefixes(r Range, limit int) []netip.Prefix {
	bits := r.First.BitLen()
	first, last := toInt(r.First), toInt(r.Last)
	var out []netip.Prefix
	for first.Cmp(last) <= 0 && len(out) < limit {
		host := bits
		if first.Sign() != 0 {
			host = int(first.TrailingZeroBits())
		}
		span := new(big.Int).Sub(last, first)
		span.Add(span, big.NewInt(1))
		if max := span.BitLen() - 1; host > max {
			host = max
		}
		out = append(out, netip.PrefixFrom(fromInt(first, r.First.Is4()), bits-host))
		first = new(big.Int).Add(first, new(big.Int).Lsh(big.NewInt(1), uint(host)))
	}
	return out
}

// Summarise returns the fewest CIDRs covering exactly the given prefixes.
func Summarise(prefixes []netip.Prefix) []netip.Prefix {
	ranges := make([]Range, len(prefixes))
	for i, p := range prefixes {
		ranges[i] = PrefixRange(p)
	}
	var out []netip.Prefix
	for _, family := range []bool{true, false} {
		var same []Range
		for _, r := range ranges {
			if r.First.Is4() == family {
				same = append(same, r)
			}
		}
		for _, r := range Merge(same) {
			out = append(out, Prefixes(r, 1<<16)...)
		}
	}
	return out
}

// Split divides a prefix into smaller ones, returning at most limit and the
// total number there would be.
func Split(p netip.Prefix, newBits, limit int) ([]netip.Prefix, string, error) {
	p = p.Masked()
	if newBits <= p.Bits() || newBits > p.Addr().BitLen() {
		return nil, "", fmt.Errorf("new prefix must be longer than /%d and at most /%d", p.Bits(), p.Addr().BitLen())
	}
	total := new(big.Int).Lsh(big.NewInt(1), uint(newBits-p.Bits()))
	step := new(big.Int).Lsh(big.NewInt(1), uint(p.Addr().BitLen()-newBits))
	cursor := toInt(p.Addr())
	var out []netip.Prefix
	for i := 0; i < limit && big.NewInt(int64(i)).Cmp(total) < 0; i++ {
		out = append(out, netip.PrefixFrom(fromInt(cursor, p.Addr().Is4()), newBits))
		cursor = new(big.Int).Add(cursor, step)
	}
	return out, total.String(), nil
}
