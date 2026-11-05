package reconciliation

import (
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"strings"
	"time"

	"dfipam/api/internal/audit"
	"dfipam/api/internal/auth"
	"dfipam/api/internal/modules/notifications"
	"dfipam/api/internal/platform/db"
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

func (h *Handler) validateChanges(objectType string, changes map[string]string) error {
	target, ok := h.targets[objectType]
	if !ok {
		return errNoTarget
	}
	if len(changes) == 0 {
		return httpx.Validation("changes", "Propose at least one change.")
	}
	for field, value := range changes {
		if !target.Fields[field] {
			return httpx.Validation(field, "This field can't be changed through a change request.")
		}
		if strings.HasSuffix(field, "Id") && value != "" && !httpx.IsUUID(value) {
			return httpx.Validation(field, "Choose a valid record.")
		}
		if field == "mac" && value != "" {
			if _, err := net.ParseMAC(value); err != nil {
				return httpx.Validation(field, "Enter a valid MAC address.")
			}
		}
		if len(value) > 2000 {
			return httpx.Validation(field, "Use at most 2000 characters.")
		}
	}
	return nil
}

func (h *Handler) proposeChange(c *gin.Context) {
	id, ok := httpx.PathID(c)
	if !ok {
		return
	}
	var req struct {
		Changes map[string]string `json:"changes"`
		Reason  string            `json:"reason"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Fail(c, httpx.ErrBadRequest)
		return
	}
	req.Reason = strings.TrimSpace(req.Reason)
	if req.Reason == "" {
		httpx.Fail(c, httpx.Validation("reason", "Say why this change is needed."))
		return
	}
	ctx, actor := c.Request.Context(), auth.Current(c).UserID
	var out ChangeRequest
	err := db.InTx(ctx, h.conn, func(tx pgx.Tx) error {
		var objectType, explanation string
		var objectID *string
		err := tx.QueryRow(ctx, `SELECT object_type, object_id::text, explanation FROM reconciliation_results WHERE id = $1::uuid FOR UPDATE`, id).
			Scan(&objectType, &objectID, &explanation)
		if errors.Is(err, pgx.ErrNoRows) {
			return httpx.ErrNotFound
		}
		if err != nil {
			return err
		}
		if objectID == nil {
			return errNoTarget
		}
		if err := h.validateChanges(objectType, req.Changes); err != nil {
			return err
		}
		proposed, err := json.Marshal(req.Changes)
		if err != nil {
			return err
		}
		var changeID string
		err = tx.QueryRow(ctx, `INSERT INTO change_requests (object_type, object_id, proposed, requested_by, finding_id, reason)
			VALUES ($1, $2::uuid, $3::jsonb, $4::uuid, $5::uuid, $6) RETURNING id::text`, objectType, objectID, proposed, actor, id, req.Reason).Scan(&changeID)
		if db.ErrorCode(err) == "23505" {
			return errOpenChange
		}
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE reconciliation_results SET review_status = 'investigating' WHERE id = $1::uuid AND review_status = 'open'`, id); err != nil {
			return err
		}
		if err := audit.Write(ctx, tx, audit.Entry{ActorID: actor, Action: "change_request.create", ObjectType: "change_request", ObjectID: changeID,
			New: gin.H{"findingId": id, "objectType": objectType, "objectId": objectID, "proposed": req.Changes, "reason": req.Reason}}); err != nil {
			return err
		}
		approvers, err := notifications.Approvers(ctx, tx, nil)
		if err != nil {
			return err
		}
		if err := notifications.Enqueue(ctx, tx, actor, notifications.Event{Type: notifications.ChangeRequested, ObjectType: "change_request", ObjectID: changeID,
			DedupeKey: "change-requested:" + changeID, Title: "An inventory change needs a decision", Body: explanation, Link: "/reconciliation?tab=changes", Recipients: approvers}); err != nil {
			return err
		}
		out, err = scanChange(tx.QueryRow(ctx, changeColumns+` WHERE c.id = $1::uuid`, changeID))
		return err
	})
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, http.StatusCreated, out)
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

// lockPending reads a pending request for a decision by actor, and blocks the person
// who proposed it. The database constraint backs this up.
func lockPending(c *gin.Context, tx pgx.Tx, id, actor string) (ChangeRequest, error) {
	r, err := scanChange(tx.QueryRow(c.Request.Context(), changeColumns+` WHERE c.id = $1::uuid FOR UPDATE OF c`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return r, httpx.ErrNotFound
	}
	if err != nil {
		return r, err
	}
	if r.Status != "pending" {
		return r, errDecided
	}
	if r.RequestedID != nil && *r.RequestedID == actor {
		return r, errSelfDecide
	}
	return r, nil
}

func decisionNote(c *gin.Context, required bool) (string, bool) {
	var req struct {
		Note string `json:"note"`
	}
	if c.Request.ContentLength != 0 {
		if err := c.ShouldBindJSON(&req); err != nil {
			httpx.Fail(c, httpx.ErrBadRequest)
			return "", false
		}
	}
	req.Note = strings.TrimSpace(req.Note)
	if required && req.Note == "" {
		httpx.Fail(c, httpx.Validation("note", "Say why this change is rejected."))
		return "", false
	}
	return req.Note, true
}

func tellRequester(c *gin.Context, tx pgx.Tx, r ChangeRequest, actor, outcome string) error {
	if r.RequestedID == nil {
		return nil
	}
	return notifications.Enqueue(c.Request.Context(), tx, actor, notifications.Event{Type: notifications.ChangeDecided, ObjectType: "change_request", ObjectID: r.ID,
		DedupeKey: "change-decided:" + r.ID, Title: "Your inventory change was " + outcome, Link: "/reconciliation?tab=changes", Recipients: []string{*r.RequestedID}})
}

// approveChange decides and applies in one transaction, so inventory, the request,
// the finding and the audit rows change together or not at all.
func (h *Handler) approveChange(c *gin.Context) {
	id, ok := httpx.PathID(c)
	if !ok {
		return
	}
	note, ok := decisionNote(c, false)
	if !ok {
		return
	}
	ctx, actor := c.Request.Context(), auth.Current(c).UserID
	var out ChangeRequest
	err := db.InTx(ctx, h.conn, func(tx pgx.Tx) error {
		r, err := lockPending(c, tx, id, actor)
		if err != nil {
			return err
		}
		target, ok := h.targets[r.ObjectType]
		if !ok || r.ObjectID == nil {
			return errNoTarget
		}
		// Checked again here, because the rules may have changed since it was proposed.
		if err := h.validateChanges(r.ObjectType, r.Proposed); err != nil {
			return err
		}
		before, after, err := target.Apply(ctx, tx, *r.ObjectID, actor, r.Proposed)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE change_requests SET status = 'applied', decided_by = $2::uuid, decided_at = now(), applied_at = now(), decision_note = $3
			WHERE id = $1::uuid`, id, actor, note); err != nil {
			return err
		}
		if r.FindingID != nil {
			if _, err := tx.Exec(ctx, `UPDATE reconciliation_results SET review_status = 'accepted', reviewed_by = $2::uuid, reviewed_at = now() WHERE id = $1::uuid`,
				*r.FindingID, actor); err != nil {
				return err
			}
		}
		if err := audit.Write(ctx, tx, audit.Entry{ActorID: actor, Action: r.ObjectType + ".change_applied", ObjectType: r.ObjectType, ObjectID: *r.ObjectID,
			Old: before, New: after, ApprovalRef: id}); err != nil {
			return err
		}
		if err := audit.Write(ctx, tx, audit.Entry{ActorID: actor, Action: "change_request.approve", ObjectType: "change_request", ObjectID: id, New: gin.H{"note": note}}); err != nil {
			return err
		}
		if err := tellRequester(c, tx, r, actor, "approved and applied"); err != nil {
			return err
		}
		out, err = scanChange(tx.QueryRow(ctx, changeColumns+` WHERE c.id = $1::uuid`, id))
		return err
	})
	if err != nil {
		httpx.Fail(c, translateApply(err))
		return
	}
	httpx.OK(c, http.StatusOK, out)
}

func translateApply(err error) error {
	if db.ErrorCode(err) == "23503" || db.ErrorCode(err) == "22P02" {
		return httpx.Validation("changes", "A proposed value no longer points at a valid record.")
	}
	return err
}

func (h *Handler) rejectChange(c *gin.Context) {
	id, ok := httpx.PathID(c)
	if !ok {
		return
	}
	note, ok := decisionNote(c, true)
	if !ok {
		return
	}
	ctx, actor := c.Request.Context(), auth.Current(c).UserID
	var out ChangeRequest
	err := db.InTx(ctx, h.conn, func(tx pgx.Tx) error {
		r, err := lockPending(c, tx, id, actor)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE change_requests SET status = 'rejected', decided_by = $2::uuid, decided_at = now(), decision_note = $3 WHERE id = $1::uuid`, id, actor, note); err != nil {
			return err
		}
		if err := audit.Write(ctx, tx, audit.Entry{ActorID: actor, Action: "change_request.reject", ObjectType: "change_request", ObjectID: id, New: gin.H{"note": note}}); err != nil {
			return err
		}
		if err := tellRequester(c, tx, r, actor, "rejected"); err != nil {
			return err
		}
		out, err = scanChange(tx.QueryRow(ctx, changeColumns+` WHERE c.id = $1::uuid`, id))
		return err
	})
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, http.StatusOK, out)
}
