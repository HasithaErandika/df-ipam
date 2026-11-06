package imports

import (
	"cmp"
	"context"
	"net/netip"
	"slices"
	"strings"

	"github.com/jackc/pgx/v5"
)

// networks holds the subnets a file adds, by routing domain, so addresses further
// down the same file can be checked against them.
type networks struct {
	tx    pgx.Tx
	added map[string]map[netip.Prefix]bool
}

func newNetworks(tx pgx.Tx) *networks {
	return &networks{tx: tx, added: map[string]map[netip.Prefix]bool{}}
}

func (n *networks) check(ctx context.Context, s *sheet, rows []*rowCheck) error {
	switch s.Key {
	case "subnets":
		return n.checkSubnets(ctx, rows)
	case "ip_addresses":
		return n.checkAddresses(ctx, rows)
	case "reserved_ranges", "dhcp_scopes":
		checkRanges(rows)
	}
	return nil
}

func checkRanges(rows []*rowCheck) {
	for _, r := range rows {
		start, _ := netip.ParseAddr(r.row.fields["range_start"])
		end, _ := netip.ParseAddr(r.row.fields["range_end"])
		if r.valid() && (start.BitLen() != end.BitLen() || end.Less(start)) {
			r.fail("invalid", "range_end must come after range_start, in the same address family.")
		}
	}
}

func inInventory(id string) bool { return !strings.HasPrefix(id, "new:") }

// overlapsInFile marks a subnet that overlaps one above it in the same routing domain.
func overlapsInFile(rows []*rowCheck) {
	type item struct {
		r *rowCheck
		p netip.Prefix
	}
	groups := map[string][]item{}
	for _, r := range rows {
		if r.valid() {
			id := r.refs["routing_domain"].id
			groups[id] = append(groups[id], item{r, netip.MustParsePrefix(r.row.fields["cidr"])})
		}
	}
	for _, list := range groups {
		slices.SortFunc(list, func(a, b item) int {
			return cmp.Or(a.p.Addr().Compare(b.p.Addr()), cmp.Compare(a.p.Bits(), b.p.Bits()))
		})
		widest := 0
		for i := 1; i < len(list); i++ {
			if list[widest].p.Overlaps(list[i].p) {
				list[i].r.fail("overlap", "Overlaps row %d in this routing domain.", list[widest].r.row.number)
				continue
			}
			widest = i
		}
	}
}

func (n *networks) checkSubnets(ctx context.Context, rows []*rowCheck) error {
	for _, r := range rows {
		p := netip.MustParsePrefix(r.row.fields["cidr"])
		if gw, err := netip.ParseAddr(r.row.fields["gateway"]); err == nil && r.valid() && !p.Contains(gw) {
			r.fail("invalid", "gateway %s is outside %s.", gw, p)
		}
	}
	overlapsInFile(rows)
	if err := n.overlapsInventory(ctx, rows); err != nil {
		return err
	}
	for _, r := range rows {
		if r.valid() {
			id := r.refs["routing_domain"].id
			if n.added[id] == nil {
				n.added[id] = map[netip.Prefix]bool{}
			}
			n.added[id][netip.MustParsePrefix(r.row.fields["cidr"])] = true
		}
	}
	return nil
}

// overlapsInventory checks every subnet against the inventory in one query.
func (n *networks) overlapsInventory(ctx context.Context, rows []*rowCheck) error {
	var domains, cidrs []string
	var asked []*rowCheck
	for _, r := range rows {
		if id := r.refs["routing_domain"].id; r.valid() && inInventory(id) {
			domains, cidrs, asked = append(domains, id), append(cidrs, r.row.fields["cidr"]), append(asked, r)
		}
	}
	if len(asked) == 0 {
		return nil
	}
	found, err := pgx.CollectRows(mustRows(n.tx.Query(ctx, `SELECT t.n, (SELECT s.name || ' (' || s.cidr::text || ')' FROM subnets s
		WHERE s.routing_domain_id = t.d AND s.archived_at IS NULL AND s.cidr && t.c LIMIT 1)
		FROM unnest($1::uuid[], $2::cidr[]) WITH ORDINALITY AS t(d, c, n)`, domains, cidrs)), scanHit)
	if err != nil {
		return err
	}
	for _, h := range found {
		if h.detail != nil {
			asked[h.position-1].fail("overlap", "Overlaps %s in this routing domain.", *h.detail)
		}
	}
	return nil
}

type hit struct {
	position int
	detail   *string
}

func scanHit(r pgx.CollectableRow) (hit, error) {
	var h hit
	return h, r.Scan(&h.position, &h.detail)
}

type failedRows struct {
	pgx.Rows
	err error
}

func (f failedRows) Next() bool { return false }
func (f failedRows) Err() error { return f.err }
func (f failedRows) Close()     {}

// mustRows lets a query feed CollectRows directly, which reports its error.
func mustRows(rows pgx.Rows, err error) pgx.Rows {
	if err != nil {
		return failedRows{err: err}
	}
	return rows
}

func (n *networks) inFile(domain string, a netip.Addr) bool {
	for bits := a.BitLen(); bits >= 0 && n.added[domain] != nil; bits-- {
		if p, err := a.Prefix(bits); err == nil && n.added[domain][p] {
			return true
		}
	}
	return false
}

// checkAddresses needs each address to sit in a subnet, from the file or the
// inventory. Which one holds it, and whether it is usable, is settled on approval.
func (n *networks) checkAddresses(ctx context.Context, rows []*rowCheck) error {
	var domains, addrs []string
	var asked []*rowCheck
	for _, r := range rows {
		a, _ := netip.ParseAddr(r.row.fields["address"])
		id := r.refs["routing_domain"].id
		switch {
		case !r.valid() || n.inFile(id, a):
		case inInventory(id):
			domains, addrs, asked = append(domains, id), append(addrs, a.String()), append(asked, r)
		default:
			r.fail("invalid", "No subnet in this routing domain contains %s.", a)
		}
	}
	if len(asked) == 0 {
		return nil
	}
	found, err := pgx.CollectRows(mustRows(n.tx.Query(ctx, `SELECT t.n, CASE WHEN EXISTS (SELECT 1 FROM subnets s
		WHERE s.routing_domain_id = t.d AND s.archived_at IS NULL AND s.cidr >>= t.a) THEN NULL ELSE 'missing' END
		FROM unnest($1::uuid[], $2::inet[]) WITH ORDINALITY AS t(d, a, n)`, domains, addrs)), scanHit)
	if err != nil {
		return err
	}
	for _, h := range found {
		if h.detail != nil {
			r := asked[h.position-1]
			r.fail("invalid", "No subnet in this routing domain contains %s.", r.row.fields["address"])
		}
	}
	return nil
}
