package allocation

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"dfipam/api/internal/audit"
	"dfipam/api/internal/auth"
	"dfipam/api/internal/platform/db"
	"dfipam/api/internal/platform/httpx"

	"github.com/jackc/pgx/v5"
)

// MaxSteps bounds how many levels one workflow may have.
const MaxSteps = 5

// WorkflowInput creates or replaces a workflow. Requests already submitted keep
// the steps they were given.
type WorkflowInput struct {
	Name       string     `json:"name"`
	MatchRules MatchRules `json:"matchRules"`
	IsActive   *bool      `json:"isActive"`
	Steps      []struct {
		ApproverRoleID string `json:"approverRoleId"`
	} `json:"steps"`
}

func (in *WorkflowInput) validate() error {
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" {
		return httpx.Validation("name", "Enter a workflow name.")
	}
	if len(in.Steps) == 0 || len(in.Steps) > MaxSteps {
		return httpx.Validation("steps", "A workflow needs one to five approval steps.")
	}
	for _, st := range in.Steps {
		if !httpx.IsUUID(st.ApproverRoleID) {
			return httpx.Validation("steps", "Choose a role for every step.")
		}
	}
	return ValidateRules(in.MatchRules)
}

func (s *Service) ListWorkflows(ctx context.Context) ([]Workflow, error) {
	out, err := s.repo.Workflows(ctx, s.conn, "")
	if out == nil {
		out = []Workflow{}
	}
	return out, err
}

func (s *Service) workflow(ctx context.Context, q querier, id string) (Workflow, error) {
	list, err := s.repo.Workflows(ctx, q, id)
	if err != nil {
		return Workflow{}, err
	}
	if len(list) == 0 {
		return Workflow{}, httpx.ErrNotFound
	}
	return list[0], nil
}

func (s *Service) writeWorkflow(ctx context.Context, tx pgx.Tx, id string, in WorkflowInput) error {
	rules, err := json.Marshal(in.MatchRules)
	if err != nil {
		return err
	}
	active := in.IsActive == nil || *in.IsActive
	if _, err := tx.Exec(ctx, `UPDATE approval_workflows SET name = $2, match_rules = $3::jsonb, is_active = $4 WHERE id = $1::uuid`,
		id, in.Name, rules, active); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM approval_workflow_steps WHERE workflow_id = $1::uuid`, id); err != nil {
		return err
	}
	roles := make([]string, len(in.Steps))
	for i, st := range in.Steps {
		roles[i] = st.ApproverRoleID
	}
	_, err = tx.Exec(ctx, `INSERT INTO approval_workflow_steps (workflow_id, step_order, approver_role_id)
		SELECT $1::uuid, n, role::uuid FROM unnest($2::text[]) WITH ORDINALITY AS t(role, n)`, id, roles)
	return err
}

func workflowError(err error) error {
	switch db.ErrorCode(err) {
	case db.CodeUnique:
		return httpx.NewError(http.StatusConflict, "ALREADY_EXISTS", "A workflow with this name already exists.")
	case db.CodeForeignKey:
		return httpx.Validation("steps", "A chosen role doesn't exist.")
	}
	return err
}

func (s *Service) CreateWorkflow(ctx context.Context, p *auth.Principal, in WorkflowInput) (Workflow, error) {
	if err := in.validate(); err != nil {
		return Workflow{}, err
	}
	var out Workflow
	err := db.InTx(ctx, s.conn, func(tx pgx.Tx) error {
		var id string
		if err := tx.QueryRow(ctx, `INSERT INTO approval_workflows (name) VALUES ($1) RETURNING id::text`, in.Name).Scan(&id); err != nil {
			return err
		}
		if err := s.writeWorkflow(ctx, tx, id, in); err != nil {
			return err
		}
		var err error
		if out, err = s.workflow(ctx, tx, id); err != nil {
			return err
		}
		return audit.Write(ctx, tx, audit.Entry{ActorID: p.UserID, Action: "approval_workflow.create", ObjectType: "approval_workflow", ObjectID: id, New: out})
	})
	return out, workflowError(err)
}

func (s *Service) UpdateWorkflow(ctx context.Context, p *auth.Principal, id string, in WorkflowInput) (Workflow, error) {
	if err := in.validate(); err != nil {
		return Workflow{}, err
	}
	var out Workflow
	err := db.InTx(ctx, s.conn, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `SELECT 1 FROM approval_workflows WHERE id = $1::uuid FOR UPDATE`, id); err != nil {
			return err
		}
		old, err := s.workflow(ctx, tx, id)
		if err != nil {
			return err
		}
		if err := s.writeWorkflow(ctx, tx, id, in); err != nil {
			return err
		}
		if out, err = s.workflow(ctx, tx, id); err != nil {
			return err
		}
		return audit.Write(ctx, tx, audit.Entry{ActorID: p.UserID, Action: "approval_workflow.update", ObjectType: "approval_workflow", ObjectID: id, Old: old, New: out})
	})
	return out, workflowError(err)
}

// DeleteWorkflow removes a workflow no request has used. Used ones are kept for
// history and can be deactivated instead.
func (s *Service) DeleteWorkflow(ctx context.Context, p *auth.Principal, id string) error {
	return db.InTx(ctx, s.conn, func(tx pgx.Tx) error {
		old, err := s.workflow(ctx, tx, id)
		if err != nil {
			return err
		}
		if old.InUse > 0 {
			return httpx.NewError(http.StatusConflict, "WORKFLOW_IN_USE", "Requests have used this workflow. Deactivate it instead.")
		}
		if _, err := tx.Exec(ctx, `DELETE FROM approval_workflows WHERE id = $1::uuid`, id); err != nil {
			return err
		}
		return audit.Write(ctx, tx, audit.Entry{ActorID: p.UserID, Action: "approval_workflow.delete", ObjectType: "approval_workflow", ObjectID: id, Old: old})
	})
}
