package imports

import "net/netip"

func maskBits(mask string) (int, bool) {
	a, err := netip.ParseAddr(mask)
	if err != nil || !a.Is4() {
		return 0, false
	}
	b := a.As4()
	n, seenZero := 0, false
	for _, octet := range b {
		for i := 7; i >= 0; i-- {
			if octet&(1<<i) != 0 {
				if seenZero {
					return 0, false
				}
				n++
			} else {
				seenZero = true
			}
		}
	}
	return n, true
}
