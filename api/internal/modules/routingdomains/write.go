package routingdomains

import (
	"context"

	"dfipam/api/internal/audit"

	"github.com/jackc/pgx/v5"
)

// NewDomain is a routing domain to create.
type NewDomain struct {
	OrganisationID string  `json:"organisationId"`
	Name           string  `json:"name"`
	Kind           string  `json:"kind"`
	RD             *string `json:"routeDistinguisher"`
	IsIsolated     bool    `json:"isIsolated"`
}

// Insert creates a routing domain. The request must already be checked.
func Insert(ctx context.Context, tx pgx.Tx, actor string, req NewDomain) (string, error) {
	var id string
	if err := tx.QueryRow(ctx, `INSERT INTO routing_domains (organisation_id, name, kind, route_distinguisher, is_isolated, created_by, updated_by)
		VALUES ($1::uuid, $2, $3, $4, $5, $6::uuid, $6::uuid) RETURNING id::text`,
		req.OrganisationID, req.Name, req.Kind, req.RD, req.IsIsolated, actor).Scan(&id); err != nil {
		return "", err
	}
	return id, audit.Write(ctx, tx, audit.Entry{ActorID: actor, Action: "routing_domain.create", ObjectType: "routing_domain", ObjectID: id, New: req})
}

// Remove deletes a routing domain. old is the audit copy of what is going.
func Remove(ctx context.Context, tx pgx.Tx, actor, id string, old any) error {
	if _, err := tx.Exec(ctx, `DELETE FROM routing_domains WHERE id = $1::uuid`, id); err != nil {
		return err
	}
	return audit.Write(ctx, tx, audit.Entry{ActorID: actor, Action: "routing_domain.delete", ObjectType: "routing_domain", ObjectID: id, Old: old})
}
