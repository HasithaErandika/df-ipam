package subnets

import "net/netip"

// searchClause turns a search into a filter. A network finds the subnets inside it
// and those around it, and an address finds the subnets holding it, which text
// matching can't do. Anything else matches name or CIDR as text.
func searchClause(q string) (string, string) {
	if p, err := netip.ParsePrefix(q); err == nil {
		return "(s.cidr <<= $%[1]d::text::cidr OR s.cidr >>= $%[1]d::text::cidr)", p.Masked().String()
	}
	if a, err := netip.ParseAddr(q); err == nil {
		return "s.cidr >>= $%[1]d::text::inet", a.String()
	}
	return "(s.name ILIKE '%%' || $%[1]d || '%%' OR s.cidr::text ILIKE '%%' || $%[1]d || '%%')", q
}
