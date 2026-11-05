package subnets

import (
	"context"
	"fmt"
	"math/big"
	"net/netip"
	"time"

	"dfipam/api/internal/netcalc"
	"dfipam/api/internal/platform/db"

	"github.com/jackc/pgx/v5"
)

// SnapshotDay records today's utilisation for every active subnet, once a day. The
// figures come from address records and calculated ranges: no range is enumerated,
// and IPv6 counts stay exact through math/big.
func SnapshotDay(ctx context.Context, conn db.Conn, now time.Time) (int64, error) {
	day := now.UTC().Truncate(24 * time.Hour)
	var done bool
	if err := conn.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM subnet_utilisation_snapshots WHERE captured_at = $1)`, day).Scan(&done); err != nil {
		return 0, err
	}
	if done {
		return 0, nil
	}
	list, err := utilisationNow(ctx, conn)
	if err != nil {
		return 0, err
	}
	var written int64
	for start := 0; start < len(list); start += 500 {
		n, err := writeSnapshots(ctx, conn, day, list[start:min(start+500, len(list))])
		if err != nil {
			return written, err
		}
		written += n
	}
	return written, nil
}

type snapshot struct {
	id                 string
	allocated, reserve int64
	observed           int64
	pct                string
}

func utilisationNow(ctx context.Context, conn db.Conn) ([]snapshot, error) {
	rows, err := conn.Query(ctx, `
		SELECT s.id::text, s.cidr::text,
		  count(i.id) FILTER (WHERE i.allocation_state = 'allocated'),
		  count(i.id) FILTER (WHERE i.allocation_state = 'reserved'),
		  count(i.id) FILTER (WHERE i.allocation_state = 'quarantined'),
		  count(i.id) FILTER (WHERE i.observation_state = 'observed_active'),
		  COALESCE((SELECT array_agg(host(r.range_start) || '-' || host(r.range_end)) FROM subnet_reserved_ranges r WHERE r.subnet_id = s.id), '{}')
		FROM subnets s LEFT JOIN ip_addresses i ON i.subnet_id = s.id
		WHERE s.archived_at IS NULL GROUP BY s.id`)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, scanSnapshot)
}

func scanSnapshot(r pgx.CollectableRow) (snapshot, error) {
	var s snapshot
	var cidr string
	var quarantined int64
	var reserved []string
	if err := r.Scan(&s.id, &cidr, &s.allocated, &s.reserve, &quarantined, &s.observed, &reserved); err != nil {
		return s, err
	}
	p, err := netip.ParsePrefix(cidr)
	if err != nil {
		return s, fmt.Errorf("subnet %s: %w", s.id, err)
	}
	usable := netcalc.UsableRange(p)
	var ranges []netcalc.Range
	for _, v := range reserved {
		if rg, ok := parseRange(v); ok {
			ranges = append(ranges, rg)
		}
	}
	used := new(big.Int).Add(big.NewInt(s.allocated+s.reserve+quarantined), netcalc.Total(netcalc.Merge(netcalc.Clip(usable, ranges))))
	s.pct = netcalc.UtilisationPercent(used, netcalc.Calculate(p).Usable)
	return s, nil
}

func writeSnapshots(ctx context.Context, conn db.Conn, day time.Time, batch []snapshot) (int64, error) {
	ids, allocated, reserved, observed, pcts := make([]string, len(batch)), make([]int64, len(batch)), make([]int64, len(batch)), make([]int64, len(batch)), make([]string, len(batch))
	for i, s := range batch {
		ids[i], allocated[i], reserved[i], observed[i], pcts[i] = s.id, s.allocated, s.reserve, s.observed, s.pct
	}
	// Two workers can start together, so the second simply adds nothing.
	tag, err := conn.Exec(ctx, `INSERT INTO subnet_utilisation_snapshots (subnet_id, captured_at, allocated, reserved, observed_active, utilisation_pct)
		SELECT id, $1, a, r, o, least(p::numeric, 999.999) FROM unnest($2::uuid[], $3::bigint[], $4::bigint[], $5::bigint[], $6::text[]) AS t(id, a, r, o, p)
		ON CONFLICT (subnet_id, captured_at) DO NOTHING`, day, ids, allocated, reserved, observed, pcts)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

func parseRange(v string) (netcalc.Range, bool) {
	for i := 0; i < len(v); i++ {
		if v[i] != '-' {
			continue
		}
		start, err1 := netip.ParseAddr(v[:i])
		end, err2 := netip.ParseAddr(v[i+1:])
		if err1 == nil && err2 == nil {
			return netcalc.Range{First: start, Last: end}, true
		}
	}
	return netcalc.Range{}, false
}
