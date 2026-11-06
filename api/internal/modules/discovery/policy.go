package discovery

import (
	"context"
	"dfipam/api/internal/audit"
	"dfipam/api/internal/platform/db"
	"dfipam/api/internal/platform/httpx"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"
)

// Policy is a discovery policy.
type Policy struct {
	ID             string     `json:"id"`
	Name           string     `json:"name"`
	Description    string     `json:"description"`
	Methods        []string   `json:"methods"`
	Allowlist      []string   `json:"targetAllowlist"`
	MaxConcurrency int        `json:"maxConcurrency"`
	MaxPPS         int        `json:"maxPps"`
	TimeoutMS      int        `json:"timeoutMs"`
	Retries        int        `json:"retries"`
	Windows        []Window   `json:"windows"`
	IsSensitive    bool       `json:"isSensitive"`
	ApprovedBy     *string    `json:"approvedByEmail"`
	ApprovedAt     *time.Time `json:"approvedAt"`
	CreatedBy      *string    `json:"createdBy"`
	UpdatedBy      *string    `json:"updatedBy"`
	UpdatedAt      time.Time  `json:"updatedAt"`
}

const policyCols = `p.id::text, p.name, p.description, p.methods, p.target_allowlist::text[], p.max_concurrency, p.max_pps, p.timeout_ms,
	p.retries, p.windows, p.is_sensitive, u.email, p.approved_at, p.created_by::text, p.updated_by::text, p.updated_at`

func scanPolicy(r pgx.Row) (Policy, error) {
	var p Policy
	var windows []byte
	err := r.Scan(&p.ID, &p.Name, &p.Description, &p.Methods, &p.Allowlist, &p.MaxConcurrency, &p.MaxPPS, &p.TimeoutMS, &p.Retries,
		&windows, &p.IsSensitive, &p.ApprovedBy, &p.ApprovedAt, &p.CreatedBy, &p.UpdatedBy, &p.UpdatedAt)
	if err == nil {
		err = json.Unmarshal(windows, &p.Windows)
	}
	return p, err
}

func (s *Service) Policies(ctx context.Context) ([]Policy, error) {
	rows, err := s.conn.Query(ctx, "SELECT "+policyCols+" FROM discovery_policies p LEFT JOIN users u ON u.id = p.approved_by ORDER BY p.name")
	if err != nil {
		return nil, err
	}
	out, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (Policy, error) { return scanPolicy(r) })
	if out == nil {
		out = []Policy{}
	}
	return out, err
}

func (s *Service) policy(ctx context.Context, q querier, id string, lock bool) (Policy, error) {
	sql := "SELECT " + policyCols + " FROM discovery_policies p LEFT JOIN users u ON u.id = p.approved_by WHERE p.id = $1::uuid"
	if lock {
		sql += " FOR UPDATE OF p"
	}
	p, err := scanPolicy(q.QueryRow(ctx, sql, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return p, httpx.ErrNotFound
	}
	return p, err
}

func (s *Service) SavePolicy(ctx context.Context, actor, id string, in PolicyInput) (Policy, error) {
	if err := in.validate(); err != nil {
		return Policy{}, err
	}
	var out Policy
	err := db.InTx(ctx, s.conn, func(tx pgx.Tx) (err error) {
		out, err = s.savePolicy(ctx, tx, actor, id, in)
		return err
	})
	if db.ErrorCode(err) == db.CodeUnique {
		err = httpx.NewError(http.StatusConflict, "ALREADY_EXISTS", "A policy with this name already exists.")
	}
	return out, err
}

func (s *Service) savePolicy(ctx context.Context, tx pgx.Tx, actor, id string, in PolicyInput) (Policy, error) {
	var old *Policy
	action := "discovery_policy.update"
	if id == "" {
		action = "discovery_policy.create"
		if err := tx.QueryRow(ctx, `INSERT INTO discovery_policies (name, created_by, updated_by) VALUES ($1, $2::uuid, $2::uuid) RETURNING id::text`,
			in.Name, actor).Scan(&id); err != nil {
			return Policy{}, err
		}
	} else {
		p, err := s.policy(ctx, tx, id, true)
		if err != nil {
			return p, err
		}
		old = &p
	}
	windows, _ := json.Marshal(in.Windows)
	if _, err := tx.Exec(ctx, `UPDATE discovery_policies SET name = $2, description = $3, methods = $4, target_allowlist = $5::cidr[],
		max_concurrency = $6, max_pps = $7, timeout_ms = $8, retries = $9, windows = $10::jsonb, is_sensitive = $11,
		approved_by = NULL, approved_at = NULL, updated_by = $12::uuid WHERE id = $1::uuid`,
		id, in.Name, in.Description, in.Methods, in.Allowlist, in.MaxConcurrency, in.MaxPPS, in.TimeoutMS, in.Retries, windows, in.IsSensitive, actor); err != nil {
		return Policy{}, err
	}
	out, err := s.policy(ctx, tx, id, false)
	if err != nil {
		return out, err
	}
	return out, audit.Write(ctx, tx, audit.Entry{ActorID: actor, Action: action, ObjectType: "discovery_policy", ObjectID: id, Old: old, New: out})
}

// ApprovePolicy records approval by someone who didn't write the settings.
func (s *Service) ApprovePolicy(ctx context.Context, actor, id string) (Policy, error) {
	var out Policy
	err := db.InTx(ctx, s.conn, func(tx pgx.Tx) error {
		p, err := s.policy(ctx, tx, id, true)
		if err != nil {
			return err
		}
		if (p.CreatedBy != nil && *p.CreatedBy == actor) || (p.UpdatedBy != nil && *p.UpdatedBy == actor) {
			return httpx.NewError(http.StatusForbidden, "SELF_APPROVAL_DENIED", "Someone other than the person who wrote this policy must approve it.")
		}
		if _, err := tx.Exec(ctx, `UPDATE discovery_policies SET approved_by = $2::uuid, approved_at = now() WHERE id = $1::uuid`, id, actor); err != nil {
			return err
		}
		if out, err = s.policy(ctx, tx, id, false); err != nil {
			return err
		}
		return audit.Write(ctx, tx, audit.Entry{ActorID: actor, Action: "discovery_policy.approve", ObjectType: "discovery_policy", ObjectID: id, ApprovalRef: id, New: out})
	})
	return out, err
}
