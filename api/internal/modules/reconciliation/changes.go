package reconciliation

import (
	"encoding/json"
	"net/http"
	"time"

	"dfipam/api/internal/platform/httpx"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
)

var (
	errNoTarget   = httpx.NewError(http.StatusUnprocessableEntity, "FINDING_NOT_CHANGEABLE", "This finding has no record a change request can act on.")
	errOpenChange = httpx.NewError(http.StatusConflict, "CHANGE_ALREADY_OPEN", "This finding already has a change request waiting for a decision.")
	errSelfDecide = httpx.NewError(http.StatusForbidden, "SELF_APPROVAL_DENIED", "Someone else must decide a change you proposed.")
	errDecided    = httpx.NewError(http.StatusConflict, "CHANGE_ALREADY_DECIDED", "This change request has already been decided.")
)

// ChangeRequest proposes a change to a record, drawn from a finding.
type ChangeRequest struct {
	ID           string            `json:"id"`
	FindingID    *string           `json:"findingId"`
	Finding      *string           `json:"finding"`
	ObjectType   string            `json:"objectType"`
	ObjectID     *string           `json:"objectId"`
	Proposed     map[string]string `json:"proposed"`
	Reason       string            `json:"reason"`
	Status       string            `json:"status"`
	RequestedBy  *string           `json:"requestedBy"`
	RequestedID  *string           `json:"requestedById"`
	DecidedBy    *string           `json:"decidedBy"`
	DecisionNote string            `json:"decisionNote"`
	CreatedAt    time.Time         `json:"createdAt"`
	DecidedAt    *time.Time        `json:"decidedAt"`
	AppliedAt    *time.Time        `json:"appliedAt"`
}

const changeColumns = `SELECT c.id::text, c.finding_id::text, f.explanation, c.object_type, c.object_id::text, c.proposed, c.reason, c.status,
	ru.email, c.requested_by::text, du.email, c.decision_note, c.created_at, c.decided_at, c.applied_at
	FROM change_requests c LEFT JOIN reconciliation_results f ON f.id = c.finding_id
	LEFT JOIN users ru ON ru.id = c.requested_by LEFT JOIN users du ON du.id = c.decided_by`

func scanChange(row pgx.Row) (ChangeRequest, error) {
	var r ChangeRequest
	var proposed []byte
	if err := row.Scan(&r.ID, &r.FindingID, &r.Finding, &r.ObjectType, &r.ObjectID, &proposed, &r.Reason, &r.Status, &r.RequestedBy, &r.RequestedID,
		&r.DecidedBy, &r.DecisionNote, &r.CreatedAt, &r.DecidedAt, &r.AppliedAt); err != nil {
		return r, err
	}
	return r, json.Unmarshal(proposed, &r.Proposed)
}

var changeStatuses = map[string]bool{"pending": true, "approved": true, "rejected": true, "applied": true}

func (h *Handler) listChanges(c *gin.Context) {
	p := httpx.ParsePage(c)
	status := c.Query("status")
	if status != "" && !changeStatuses[status] {
		httpx.Fail(c, httpx.Validation("status", "Choose a valid status."))
		return
	}
	ctx := c.Request.Context()
	var total int64
	if err := h.conn.QueryRow(ctx, `SELECT count(*) FROM change_requests WHERE $1 = '' OR status = $1`, status).Scan(&total); err != nil {
		httpx.Fail(c, err)
		return
	}
	rows, err := h.conn.Query(ctx, changeColumns+` WHERE $1 = '' OR c.status = $1 ORDER BY c.created_at DESC, c.id LIMIT $2 OFFSET $3`, status, p.Size, p.Offset())
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	out, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (ChangeRequest, error) { return scanChange(r) })
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.List(c, out, p, total)
}
