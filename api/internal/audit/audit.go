// Package audit writes the audit trail. Rows are written in the same transaction
// as the change they describe, so neither can exist without the other.
package audit

import (
	"context"
	"encoding/json"
	"fmt"

	"dfipam/api/internal/platform/httpx"

	"github.com/jackc/pgx/v5"
)

type Entry struct {
	ActorID     string
	ActorType   string // defaults to user
	Action      string
	ObjectType  string
	ObjectID    string
	Old, New    any
	ApprovalRef string
}

func marshal(v any) ([]byte, error) {
	if v == nil {
		return nil, nil
	}
	return json.Marshal(v)
}

func nullable(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func Write(ctx context.Context, tx pgx.Tx, e Entry) error {
	oldJSON, err := marshal(e.Old)
	if err != nil {
		return fmt.Errorf("audit old values: %w", err)
	}
	newJSON, err := marshal(e.New)
	if err != nil {
		return fmt.Errorf("audit new values: %w", err)
	}
	m := httpx.MetaFrom(ctx)
	actorType := e.ActorType
	if actorType == "" {
		actorType = "user"
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO audit_logs (actor_id, actor_type, action, object_type, object_id, old_values, new_values,
		                        source_ip, correlation_id, approval_ref)
		VALUES ($1, $2, $3, $4, $5, $6::jsonb, $7::jsonb, $8::text::inet, $9, $10)`,
		nullable(e.ActorID), actorType, e.Action, e.ObjectType, nullable(e.ObjectID), oldJSON, newJSON,
		nullable(m.SourceIP), nullable(m.CorrelationID), nullable(e.ApprovalRef))
	if err != nil {
		return fmt.Errorf("write audit: %w", err)
	}
	return nil
}
