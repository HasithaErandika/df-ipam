package discovery

import (
	"errors"
	"net/http"

	"dfipam/api/internal/audit"
	"dfipam/api/internal/auth"
	"dfipam/api/internal/platform/db"
	"dfipam/api/internal/platform/httpx"
	"dfipam/api/internal/scanner"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
)

// stop ends the running scan, whoever started it. Segments already swept keep
// their results; the one in progress saves nothing.
func (h *Handler) stop(c *gin.Context) {
	if h.scan == nil {
		httpx.Fail(c, errUnavailable)
		return
	}
	err := db.InTx(c, h.conn, func(tx pgx.Tx) error {
		if err := audit.Write(c, tx, audit.Entry{ActorID: auth.Current(c).UserID, Action: "scan_stop", ObjectType: "discovery"}); err != nil {
			return err
		}
		return h.scan.Stop() // last, so a refusal rolls the audit row back
	})
	switch {
	case errors.Is(err, scanner.ErrNotRunning):
		httpx.Fail(c, httpx.NewError(http.StatusConflict, "SCAN_NOT_RUNNING", "No scan is running. It may have just finished."))
	case err != nil:
		httpx.Fail(c, db.Friendly(err))
	default:
		httpx.OK(c, http.StatusOK, gin.H{"stopped": true})
	}
}
