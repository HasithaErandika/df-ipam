package catalog

import (
	"errors"
	"net/http"

	"dfipam/api/internal/auth"
	"dfipam/api/internal/platform/httpx"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
)

// checkOwner lets the owner of a record edit it, apart from its locked fields. It
// takes the row's lock, so ownership can't change while the edit is checked.
func checkOwner(c *gin.Context, tx pgx.Tx, e Entity, id string, body map[string]any) error {
	p := auth.Current(c)
	if e.OwnerCol == "" || p.Has(auth.InventoryEdit) {
		return nil
	}
	var owner *string
	err := tx.QueryRow(c.Request.Context(), "SELECT "+e.OwnerCol+"::text FROM "+e.Table+" WHERE id = $1::uuid FOR UPDATE", id).Scan(&owner)
	if errors.Is(err, pgx.ErrNoRows) {
		return httpx.ErrNotFound
	}
	if err != nil {
		return err
	}
	if owner == nil || *owner != p.UserID {
		return httpx.NewError(http.StatusForbidden, "NOT_ASSIGNED", "This record isn't assigned to you.")
	}
	for _, field := range e.OwnerLocked {
		if _, sent := body[field]; sent {
			return httpx.NewError(http.StatusForbidden, "ASSIGNED_FIELDS", "As its owner you can't change "+field+".")
		}
	}
	return nil
}
