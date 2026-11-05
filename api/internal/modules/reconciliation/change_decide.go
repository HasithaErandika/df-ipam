package reconciliation

import (
	"context"
	"dfipam/api/internal/audit"
	"dfipam/api/internal/auth"
	"dfipam/api/internal/modules/notifications"
	"dfipam/api/internal/platform/db"
	"dfipam/api/internal/platform/httpx"
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
)

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
		DedupeKey: "change-decided:" + r.ID, Title: "Your inventory change was " + outcome, Link: "/reconciliation/changes", Recipients: []string{*r.RequestedID}})
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

func (h *Handler) approveChange(c *gin.Context) {
	id, ok := httpx.PathID(c)
	if !ok {
		return
	}
	note, ok := decisionNote(c, false)
	if !ok {
		return
	}
	actor := auth.Current(c).UserID
	var out ChangeRequest
	err := db.InTx(c.Request.Context(), h.conn, func(tx pgx.Tx) (err error) {
		out, err = h.applyChange(c, tx, actor, id, note)
		return err
	})
	if err != nil {
		httpx.Fail(c, translateApply(err))
		return
	}
	httpx.OK(c, http.StatusOK, out)
}

func (h *Handler) applyChange(c *gin.Context, tx pgx.Tx, actor, id, note string) (ChangeRequest, error) {
	ctx := c.Request.Context()
	r, err := lockPending(c, tx, id, actor)
	if err != nil {
		return r, err
	}
	target, ok := h.targets[r.ObjectType]
	if !ok || r.ObjectID == nil {
		return r, errNoTarget
	}
	// Checked again here, because the rules may have changed since it was proposed.
	if err := h.validateChanges(r.ObjectType, r.Proposed); err != nil {
		return r, err
	}
	before, after, err := target.Apply(ctx, tx, *r.ObjectID, actor, r.Proposed)
	if err != nil {
		return r, err
	}
	if err := markApplied(ctx, tx, r, actor, id, note); err != nil {
		return r, err
	}
	if err := audit.Write(ctx, tx, audit.Entry{ActorID: actor, Action: r.ObjectType + ".change_applied", ObjectType: r.ObjectType, ObjectID: *r.ObjectID,
		Old: before, New: after, ApprovalRef: id}); err != nil {
		return r, err
	}
	if err := audit.Write(ctx, tx, audit.Entry{ActorID: actor, Action: "change_request.approve", ObjectType: "change_request", ObjectID: id, New: gin.H{"note": note}}); err != nil {
		return r, err
	}
	if err := tellRequester(c, tx, r, actor, "approved and applied"); err != nil {
		return r, err
	}
	return scanChange(tx.QueryRow(ctx, changeColumns+` WHERE c.id = $1::uuid`, id))
}

func markApplied(ctx context.Context, tx pgx.Tx, r ChangeRequest, actor, id, note string) error {
	if _, err := tx.Exec(ctx, `UPDATE change_requests SET status = 'applied', decided_by = $2::uuid, decided_at = now(), applied_at = now(), decision_note = $3
		WHERE id = $1::uuid`, id, actor, note); err != nil {
		return err
	}
	if r.FindingID == nil {
		return nil
	}
	_, err := tx.Exec(ctx, `UPDATE reconciliation_results SET review_status = 'accepted', reviewed_by = $2::uuid, reviewed_at = now() WHERE id = $1::uuid`,
		*r.FindingID, actor)
	return err
}
