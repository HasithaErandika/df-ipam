package organisations

import (
	"context"

	"dfipam/api/internal/audit"

	"github.com/jackc/pgx/v5"
)

// Insert creates an organisation and its Default routing domain, so subnets always
// have a routing context. Name and code must already be clean.
func Insert(ctx context.Context, tx pgx.Tx, actor, name, code string) (Organisation, error) {
	var o Organisation
	if err := tx.QueryRow(ctx, `INSERT INTO organisations (name, code, created_by, updated_by)
		VALUES ($1, $2, $3::uuid, $3::uuid) RETURNING id::text, name, code, created_at`, name, code, actor).
		Scan(&o.ID, &o.Name, &o.Code, &o.CreatedAt); err != nil {
		return o, err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO routing_domains (organisation_id, name, kind, created_by, updated_by)
		VALUES ($1::uuid, 'Default', 'default', $2::uuid, $2::uuid)`, o.ID, actor); err != nil {
		return o, err
	}
	return o, audit.Write(ctx, tx, audit.Entry{ActorID: actor, Action: "organisation.create", ObjectType: "organisation", ObjectID: o.ID, New: o})
}

// Remove deletes an organisation and its empty Default routing domain. old is the
// audit copy of what is going.
func Remove(ctx context.Context, tx pgx.Tx, actor, id string, old any) error {
	if _, err := tx.Exec(ctx, `DELETE FROM routing_domains rd WHERE rd.organisation_id = $1::uuid AND rd.kind = 'default'
		AND NOT EXISTS (SELECT 1 FROM subnets s WHERE s.routing_domain_id = rd.id)`, id); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM organisations WHERE id = $1::uuid`, id); err != nil {
		return err
	}
	return audit.Write(ctx, tx, audit.Entry{ActorID: actor, Action: "organisation.delete", ObjectType: "organisation", ObjectID: id, Old: old})
}
