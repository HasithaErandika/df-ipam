package auth

import (
	"context"

	"github.com/jackc/pgx/v5"
)

type scopeQuerier interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
}

// ServiceScope says which business services a person may read. Nil means all of
// them. Empty means none, so a scoped view comes back empty rather than unfiltered.
func ServiceScope(ctx context.Context, q scopeQuerier, p *Principal) (*[]string, error) {
	if p.Has(InventoryView) {
		return nil, nil
	}
	rows, err := q.Query(ctx, `SELECT DISTINCT ur.business_service_id::text FROM user_roles ur
		JOIN role_permissions rp ON rp.role_id = ur.role_id JOIN permissions pm ON pm.id = rp.permission_id
		WHERE ur.user_id = $1::uuid AND ur.business_service_id IS NOT NULL AND pm.resource = 'inventory' AND pm.action = 'view_scoped'`, p.UserID)
	if err != nil {
		return nil, err
	}
	out, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if out == nil {
		out = []string{}
	}
	return &out, err
}
