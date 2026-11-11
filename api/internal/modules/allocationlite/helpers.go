package allocationlite

import (
	"encoding/json"
	"net/http"

	"dfipam/api/internal/audit"
	"dfipam/api/internal/auth"
	"dfipam/api/internal/platform/db"
	"dfipam/api/internal/platform/httpx"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
)

var errClosed = httpx.NewError(http.StatusConflict, "REQUEST_CLOSED", "Only a pending request can be changed. Reload to see its current state.")

func actor(c *gin.Context) string { return auth.Current(c).UserID }

func friendly(err error) error {
	if db.ErrorCode(err) == db.CodeCheck {
		return httpx.Validation("ip", "The address must be inside the selected segment.")
	}
	return db.Friendly(err, "That IP is already registered or reserved.")
}

func rawJSON(b []byte) any { return json.RawMessage(b) }

func auditChange(c *gin.Context, tx pgx.Tx, action, kind, id string, old, new any) error {
	return audit.Write(c, tx, audit.Entry{ActorID: actor(c), Action: action, ObjectType: kind, ObjectID: id, Old: old, New: new})
}

// canManage reports whether this person may change a reservation: its owner, or
// someone who manages the system.
func canManage(c *gin.Context, owner *string) bool {
	p := auth.Current(c)
	return p.Has(auth.SystemManage) || (owner != nil && *owner == p.UserID)
}
