package integrations

import (
	"net/http"

	"dfipam/api/internal/audit"
	"dfipam/api/internal/auth"
	"dfipam/api/internal/platform/db"
	"dfipam/api/internal/platform/httpx"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
)

var errNoToken = httpx.NewError(http.StatusNotFound, "TOKEN_NOT_FOUND", "No FortiGate API token is stored.")

// deleteToken removes the stored API token and switches the schedule off, so
// nothing keeps trying to read with a credential that is gone. The address
// and other settings stay, ready for a new token.
func (h *Handler) deleteToken(c *gin.Context) {
	if h.sync == nil {
		httpx.Fail(c, errOff)
		return
	}
	err := db.InTx(c, h.conn, func(tx pgx.Tx) error {
		tag, err := tx.Exec(c, `DELETE FROM settings WHERE key = 'fortigate.token'`)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return errNoToken
		}
		if _, err := tx.Exec(c, `UPDATE settings SET value = 'false'::jsonb, updated_at = now(), updated_by = $1::uuid
			WHERE key = 'fortigate.enabled'`, auth.Current(c).UserID); err != nil {
			return err
		}
		return audit.Write(c, tx, audit.Entry{ActorID: auth.Current(c).UserID, Action: "delete", ObjectType: "integration", ObjectID: "fortigate",
			New: gin.H{"token": "(deleted)", "enabled": false}})
	})
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	h.get(c)
}
