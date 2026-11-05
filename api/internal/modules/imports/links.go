package imports

import (
	"context"
	"dfipam/api/internal/modules/subnets"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
)

// recordLinks maps the VDOM names and VLAN numbers in a file to their rows.
type recordLinks struct{ vdoms, vlans map[string]string }

func loadLinks(ctx context.Context, tx pgx.Tx, todo []pendingRecord, routingDomainID string) (recordLinks, error) {
	var names, numbers []string
	for _, p := range todo {
		if v := strings.TrimSpace(p.parsed.VDOM); v != "" {
			names = append(names, v)
		}
		if v := strings.TrimSpace(p.parsed.VLAN); v != "" {
			numbers = append(numbers, v)
		}
	}
	var links recordLinks
	var err error
	if links.vdoms, err = uniqueMatches(ctx, tx, `SELECT name, id::text FROM vdoms WHERE name = ANY($1::text[]) AND (routing_domain_id = $2::uuid OR routing_domain_id IS NULL)`,
		names, routingDomainID); err != nil {
		return links, err
	}
	links.vlans, err = uniqueMatches(ctx, tx, `SELECT vlan_id::text, id::text FROM vlans WHERE vlan_id::text = ANY($1::text[])`, numbers)
	return links, err
}

// uniqueMatches returns key to ID for the keys with exactly one match. A key that
// matches two rows is ambiguous, so it isn't linked at all.
func uniqueMatches(ctx context.Context, tx pgx.Tx, sql string, args ...any) (map[string]string, error) {
	rows, err := tx.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out, ambiguous := map[string]string{}, map[string]bool{}
	for rows.Next() {
		var key, id string
		if err := rows.Scan(&key, &id); err != nil {
			return nil, err
		}
		if _, dup := out[key]; dup {
			ambiguous[key] = true
		}
		out[key] = id
	}
	for key := range ambiguous {
		delete(out, key)
	}
	return out, rows.Err()
}

func (l recordLinks) request(b Batch, p pendingRecord) subnets.CreateRequest {
	req := subnets.CreateRequest{RoutingDomainID: *b.RoutingDomainID, CIDR: p.parsed.CIDR, Name: p.parsed.Name, Environment: *b.Environment,
		Description: fmt.Sprintf("Imported from %s line %d", b.FileName, p.line)}
	if p.parsed.Gateway != "" {
		g := p.parsed.Gateway
		req.Gateway = &g
	}
	if v, ok := l.vdoms[strings.TrimSpace(p.parsed.VDOM)]; ok {
		req.VDOMID = &v
	}
	if v, ok := l.vlans[strings.TrimSpace(p.parsed.VLAN)]; ok {
		req.VLANID = &v
	}
	return req
}
