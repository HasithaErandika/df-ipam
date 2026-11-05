package allocation

import (
	"context"
	"fmt"
	"strings"

	"dfipam/api/internal/audit"
	"dfipam/api/internal/auth"
	"dfipam/api/internal/modules/notifications"
	"dfipam/api/internal/platform/db"
	"dfipam/api/internal/platform/httpx"

	"github.com/jackc/pgx/v5"
)

// decision is one person's verdict on one approval step.
type decision struct {
	approvalID string
	approve    bool
	comment    string
}

func (d decision) word() string {
	if d.approve {
		return "approved"
	}
	return "rejected"
}

// Decide approves or rejects one step. Self-approval, deciding two steps of one
// request, deciding out of order and lacking the step's role are all refused;
// the database refuses the first two as well.
func (s *Service) Decide(ctx context.Context, p *auth.Principal, approvalID string, approve bool, comment string) (Request, error) {
	comment = strings.TrimSpace(comment)
	if !approve && comment == "" {
		return Request{}, httpx.Validation("comment", "Say why you are rejecting this request.")
	}
	if len(comment) > 2000 {
		return Request{}, httpx.Validation("comment", "Keep the comment under 2,000 characters.")
	}
	d := decision{approvalID, approve, comment}
	var out Request
	err := db.InTx(ctx, s.conn, func(tx pgx.Tx) (err error) {
		out, err = s.decide(ctx, tx, p, d)
		return err
	})
	return out, err
}

func (s *Service) decide(ctx context.Context, tx pgx.Tx, p *auth.Principal, d decision) (Request, error) {
	ap, err := s.repo.GetApproval(ctx, tx, d.approvalID)
	if err != nil {
		return Request{}, err
	}
	r, err := s.repo.GetRequest(ctx, tx, ap.RequestID, true) // serialises decisions on one request
	if err != nil {
		return r, err
	}
	if r.RequestedBy == p.UserID {
		return r, errSelf
	}
	if r.Status != StatusPendingApproval {
		return r, errNotCurrent
	}
	current, next, err := s.openSteps(ctx, tx, r.ID, p.UserID)
	if err != nil {
		return r, err
	}
	if current == nil || current.ID != d.approvalID {
		return r, errNotCurrent
	}
	if err := checkEligible(ctx, tx, p.UserID, current); err != nil {
		return r, err
	}
	if err := recordDecision(ctx, tx, p.UserID, d); err != nil {
		return r, err
	}
	if err := s.applyDecision(ctx, tx, p, r, d, next); err != nil {
		return r, err
	}
	action := "ip_request.reject"
	if d.approve {
		action = "ip_request.approve"
	}
	if err := audit.Write(ctx, tx, audit.Entry{ActorID: p.UserID, Action: action, ObjectType: "ip_request", ObjectID: r.ID,
		ApprovalRef: d.approvalID, New: map[string]any{"step": current.StepOrder, "decision": d.word(), "comment": d.comment}}); err != nil {
		return r, err
	}
	return s.reload(ctx, tx, r.ID, p)
}

// openSteps returns the step to decide now and the one after it. Someone who
// decided an earlier step can't decide another.
func (s *Service) openSteps(ctx context.Context, tx pgx.Tx, requestID, user string) (current, next *Approval, err error) {
	steps, err := s.repo.Approvals(ctx, tx, requestID)
	if err != nil {
		return nil, nil, err
	}
	for i := range steps {
		st := &steps[i]
		if st.ApproverID != nil && *st.ApproverID == user {
			return nil, nil, errSeparation
		}
		if st.Decision != "pending" {
			continue
		}
		if current == nil {
			current = st
		} else if next == nil {
			next = st
		}
	}
	return current, next, nil
}

func checkEligible(ctx context.Context, tx pgx.Tx, user string, step *Approval) error {
	if step.ApproverRoleID == nil {
		return nil
	}
	var held bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM user_roles WHERE user_id = $1::uuid AND role_id = $2::uuid)`,
		user, *step.ApproverRoleID).Scan(&held); err != nil {
		return err
	}
	if !held {
		return errNotEligible
	}
	return nil
}

func recordDecision(ctx context.Context, tx pgx.Tx, user string, d decision) error {
	_, err := tx.Exec(ctx, `UPDATE ip_request_approvals SET approver_id = $2::uuid, decision = $3, comment = $4, decided_at = now()
		WHERE id = $1::uuid AND decision = 'pending'`, d.approvalID, user, d.word(), d.comment)
	switch db.ErrorCode(err) {
	case db.CodeUnique:
		return errSeparation
	case db.CodeCheck:
		return errSelf
	}
	return err
}

// applyDecision moves the request on: rejected, approved, or to its next step.
func (s *Service) applyDecision(ctx context.Context, tx pgx.Tx, p *auth.Principal, r Request, d decision, next *Approval) error {
	switch {
	case !d.approve:
		if err := s.repo.SetStatus(ctx, tx, r.ID, StatusRejected, "decided_at"); err != nil {
			return err
		}
		return notifications.Enqueue(ctx, tx, p.UserID, notifications.Event{Type: notifications.RequestRejected,
			ObjectType: "ip_request", ObjectID: r.ID, DedupeKey: "request_rejected:" + r.ID,
			Title: "Rejected: " + describe(r), Body: fmt.Sprintf("%s rejected your %s.\n\nReason: %s", p.Email, describe(r), d.comment),
			Link: requestLink(r.ID), Recipients: []string{r.RequestedBy}})
	case next == nil:
		if err := s.repo.SetStatus(ctx, tx, r.ID, StatusApproved, "decided_at"); err != nil {
			return err
		}
		return s.notifyApproved(ctx, tx, p, r)
	}
	return s.notifyStep(ctx, tx, p.UserID, r, int(next.StepOrder), next.ApproverRoleID)
}
