package reconciliation

import (
	"context"
	"dfipam/api/internal/audit"
	"dfipam/api/internal/auth"
	"dfipam/api/internal/modules/notifications"
	"dfipam/api/internal/platform/db"
	"dfipam/api/internal/platform/httpx"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
)

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
		if err := validateChange(field, value); err != nil {
			return err
		}
	}
	return nil
}

func validateChange(field, value string) error {
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
	return nil
}

type changeProposal struct {
	Changes map[string]string `json:"changes"`
	Reason  string            `json:"reason"`
}

func (h *Handler) proposeChange(c *gin.Context) {
	id, ok := httpx.PathID(c)
	if !ok {
		return
	}
	var req changeProposal
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
	err := db.InTx(ctx, h.conn, func(tx pgx.Tx) (err error) {
		out, err = h.storeProposal(ctx, tx, actor, id, req)
		return err
	})
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, http.StatusCreated, out)
}

func (h *Handler) storeProposal(ctx context.Context, tx pgx.Tx, actor, finding string, req changeProposal) (ChangeRequest, error) {
	var objectType, explanation string
	var objectID *string
	err := tx.QueryRow(ctx, `SELECT object_type, object_id::text, explanation FROM reconciliation_results WHERE id = $1::uuid FOR UPDATE`, finding).
		Scan(&objectType, &objectID, &explanation)
	if errors.Is(err, pgx.ErrNoRows) {
		return ChangeRequest{}, httpx.ErrNotFound
	}
	if err != nil {
		return ChangeRequest{}, err
	}
	if objectID == nil {
		return ChangeRequest{}, errNoTarget
	}
	if err := h.validateChanges(objectType, req.Changes); err != nil {
		return ChangeRequest{}, err
	}
	proposed, err := json.Marshal(req.Changes)
	if err != nil {
		return ChangeRequest{}, err
	}
	var changeID string
	err = tx.QueryRow(ctx, `INSERT INTO change_requests (object_type, object_id, proposed, requested_by, finding_id, reason)
		VALUES ($1, $2::uuid, $3::jsonb, $4::uuid, $5::uuid, $6) RETURNING id::text`, objectType, objectID, proposed, actor, finding, req.Reason).Scan(&changeID)
	if db.ErrorCode(err) == "23505" {
		return ChangeRequest{}, errOpenChange
	}
	if err != nil {
		return ChangeRequest{}, err
	}
	if _, err := tx.Exec(ctx, `UPDATE reconciliation_results SET review_status = 'investigating' WHERE id = $1::uuid AND review_status = 'open'`, finding); err != nil {
		return ChangeRequest{}, err
	}
	if err := audit.Write(ctx, tx, audit.Entry{ActorID: actor, Action: "change_request.create", ObjectType: "change_request", ObjectID: changeID,
		New: gin.H{"findingId": finding, "objectType": objectType, "objectId": objectID, "proposed": req.Changes, "reason": req.Reason}}); err != nil {
		return ChangeRequest{}, err
	}
	approvers, err := notifications.Approvers(ctx, tx, nil)
	if err != nil {
		return ChangeRequest{}, err
	}
	if err := notifications.Enqueue(ctx, tx, actor, notifications.Event{Type: notifications.ChangeRequested, ObjectType: "change_request", ObjectID: changeID,
		DedupeKey: "change-requested:" + changeID, Title: "An inventory change needs a decision", Body: explanation, Link: "/reconciliation/changes", Recipients: approvers}); err != nil {
		return ChangeRequest{}, err
	}
	return scanChange(tx.QueryRow(ctx, changeColumns+` WHERE c.id = $1::uuid`, changeID))
}
