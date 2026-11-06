package allocation

import (
	"context"
	"dfipam/api/internal/platform/httpx"
	"encoding/json"
	"errors"

	"github.com/jackc/pgx/v5"
)

const approvalCols = `ap.id::text, ap.request_id::text, ap.step_order, ap.approver_role_id::text, ro.name, ap.approver_id::text, au.email,
	ap.decision, ap.comment, ap.decided_at`

const approvalFrom = ` FROM ip_request_approvals ap LEFT JOIN roles ro ON ro.id = ap.approver_role_id LEFT JOIN users au ON au.id = ap.approver_id`

func scanApproval(row pgx.Row) (Approval, error) {
	var a Approval
	err := row.Scan(&a.ID, &a.RequestID, &a.StepOrder, &a.ApproverRoleID, &a.ApproverRoleName, &a.ApproverID, &a.ApproverEmail,
		&a.Decision, &a.Comment, &a.DecidedAt)
	return a, err
}

func (Repository) Approvals(ctx context.Context, q querier, requestID string) ([]Approval, error) {
	rows, err := q.Query(ctx, "SELECT "+approvalCols+approvalFrom+" WHERE ap.request_id = $1::uuid ORDER BY ap.step_order", requestID)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (Approval, error) { return scanApproval(r) })
}

func (Repository) GetApproval(ctx context.Context, q querier, id string) (Approval, error) {
	a, err := scanApproval(q.QueryRow(ctx, "SELECT "+approvalCols+approvalFrom+" WHERE ap.id = $1::uuid", id))
	if errors.Is(err, pgx.ErrNoRows) {
		return a, httpx.ErrNotFound
	}
	return a, err
}

// eligible is the condition for "user $1 may decide approval ap now".
const eligible = `r.status = 'pending_approval' AND ap.decision = 'pending' AND r.requested_by <> $1::uuid
	AND ap.step_order = (SELECT min(x.step_order) FROM ip_request_approvals x WHERE x.request_id = r.id AND x.decision = 'pending')
	AND (ap.approver_role_id IS NULL OR EXISTS (SELECT 1 FROM user_roles ur WHERE ur.user_id = $1::uuid AND ur.role_id = ap.approver_role_id))
	AND NOT EXISTS (SELECT 1 FROM ip_request_approvals y WHERE y.request_id = r.id AND y.approver_id = $1::uuid)`

// PendingFor lists approval steps the user may decide now, oldest first.
func (Repository) PendingFor(ctx context.Context, q querier, user string, p httpx.Page) ([]string, int64, error) {
	base := ` FROM ip_request_approvals ap JOIN ip_requests r ON r.id = ap.request_id WHERE ` + eligible
	var total int64
	if err := q.QueryRow(ctx, "SELECT count(*)"+base, user).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := q.Query(ctx, "SELECT ap.id::text"+base+" ORDER BY r.submitted_at, ap.id LIMIT $2 OFFSET $3", user, p.Size, p.Offset())
	if err != nil {
		return nil, 0, err
	}
	ids, err := pgx.CollectRows(rows, pgx.RowTo[string])
	return ids, total, err
}

// Workflows loads workflows with their steps, in name order.
func (Repository) Workflows(ctx context.Context, q querier, id string) ([]Workflow, error) {
	var w where
	if id != "" {
		w.add("w.id = $%d::uuid", id)
	}
	rows, err := q.Query(ctx, `SELECT w.id::text, w.name, w.match_rules, w.is_active,
		COALESCE((SELECT jsonb_agg(jsonb_build_object('stepOrder', st.step_order, 'approverRoleId', st.approver_role_id, 'approverRoleName', ro.name)
		          ORDER BY st.step_order) FROM approval_workflow_steps st JOIN roles ro ON ro.id = st.approver_role_id
		          WHERE st.workflow_id = w.id), '[]'),
		(SELECT count(*) FROM ip_requests r WHERE r.workflow_id = w.id)
		FROM approval_workflows w`+w.sql()+` ORDER BY w.name`, w.args...)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (Workflow, error) {
		var wf Workflow
		var rules, steps []byte
		if err := r.Scan(&wf.ID, &wf.Name, &rules, &wf.IsActive, &steps, &wf.InUse); err != nil {
			return wf, err
		}
		if err := json.Unmarshal(rules, &wf.MatchRules); err != nil {
			return wf, err
		}
		return wf, json.Unmarshal(steps, &wf.Steps)
	})
}
