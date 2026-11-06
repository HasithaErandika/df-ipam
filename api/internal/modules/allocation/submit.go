package allocation

import (
	"context"
	"dfipam/api/internal/audit"
	"dfipam/api/internal/auth"
	"dfipam/api/internal/platform/db"
	"dfipam/api/internal/platform/httpx"
	"net/http"
	"slices"
	"strings"

	"github.com/jackc/pgx/v5"
)

func inputOf(r Request) RequestInput {
	return RequestInput{RequestType: r.RequestType, SubnetID: r.SubnetID, SelectionMode: r.SelectionMode,
		RequestedAddress: r.RequestedAddress, AllocationID: r.AllocationID, ReservationID: r.ReservationID,
		ApplicationID: r.ApplicationID, BusinessServiceID: r.BusinessServiceID, TechnicalOwnerID: r.TechnicalOwnerID,
		BusinessOwnerID: r.BusinessOwnerID, Justification: r.Justification, Department: r.Department, Environment: r.Environment,
		RequiredDate: r.RequiredDate, ExpiryDate: r.ExpiryDate, Comments: r.Comments}
}

// own loads and locks a request the caller made, in one of the given statuses.
func (s *Service) own(ctx context.Context, q querier, p *auth.Principal, id string, statuses ...string) (Request, error) {
	r, err := s.repo.GetRequest(ctx, q, id, true)
	if err != nil {
		return r, err
	}
	if r.RequestedBy != p.UserID {
		if CanSeeAll(p) {
			return r, httpx.NewError(http.StatusForbidden, "NOT_REQUESTER", "Only the person who made this request can change it.")
		}
		return r, httpx.ErrNotFound
	}
	if slices.Contains(statuses, r.Status) {
		return r, nil
	}
	return r, badState("This request can't be changed in its current status (" + strings.ReplaceAll(r.Status, "_", " ") + ").")
}

func (s *Service) DeleteRequest(ctx context.Context, p *auth.Principal, id string) error {
	return db.InTx(ctx, s.conn, func(tx pgx.Tx) error {
		old, err := s.own(ctx, tx, p, id, StatusDraft)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `DELETE FROM ip_requests WHERE id = $1::uuid`, id); err != nil {
			return err
		}
		return audit.Write(ctx, tx, audit.Entry{ActorID: p.UserID, Action: "ip_request.delete", ObjectType: "ip_request", ObjectID: id, Old: old})
	})
}

// submit freezes the approval steps from the matching workflow and tells the
// first step's approvers.
func (s *Service) submit(ctx context.Context, tx pgx.Tx, p *auth.Principal, r Request) error {
	if _, err := s.resolve(ctx, tx, inputOf(r), r.ID); err != nil {
		return err
	}
	sub, err := s.repo.Subnet(ctx, tx, r.SubnetID, false)
	if err != nil {
		return err
	}
	env := sub.Environment
	if r.Environment != nil {
		env = *r.Environment
	}
	workflows, err := s.repo.Workflows(ctx, tx, "")
	if err != nil {
		return err
	}
	wf := ChooseWorkflow(workflows, Target{RequestType: r.RequestType, Environment: env, Sensitivity: sub.Sensitivity})
	var workflowID *string
	steps := []*string{nil} // default: one step for anyone with the approve permission
	if wf != nil {
		workflowID = &wf.ID
		steps = steps[:0]
		for _, st := range wf.Steps {
			role := st.ApproverRoleID
			steps = append(steps, &role)
		}
	}
	if _, err := tx.Exec(ctx, `INSERT INTO ip_request_approvals (request_id, step_order, approver_role_id)
		SELECT $1::uuid, n, role FROM unnest($2::uuid[]) WITH ORDINALITY AS t(role, n)`, r.ID, steps); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE ip_requests SET workflow_id = $2::uuid WHERE id = $1::uuid`, r.ID, workflowID); err != nil {
		return err
	}
	if err := s.repo.SetStatus(ctx, tx, r.ID, StatusPendingApproval, "submitted_at"); err != nil {
		return err
	}
	if err := audit.Write(ctx, tx, audit.Entry{ActorID: p.UserID, Action: "ip_request.submit", ObjectType: "ip_request", ObjectID: r.ID,
		New: map[string]any{"workflowId": workflowID, "steps": len(steps)}}); err != nil {
		return err
	}
	return s.notifyStep(ctx, tx, p.UserID, r, 1, steps[0])
}
