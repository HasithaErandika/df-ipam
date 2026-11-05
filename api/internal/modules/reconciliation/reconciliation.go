// Package reconciliation compares inventory with evidence and records findings.
// It never changes inventory: administrators review findings, then act.
package reconciliation

import (
	"context"
	"net/http"

	"dfipam/api/internal/auth"
	"dfipam/api/internal/platform/db"
	"dfipam/api/internal/platform/httpx"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
)

// Applier makes an approved change to a record in its owning module, inside tx. It
// returns the fields before and after, for the audit row.
type Applier func(ctx context.Context, tx pgx.Tx, id, actor string, changes map[string]string) (before, after map[string]string, err error)

// Target says what a person may change on one kind of record. Allocation state isn't
// here on purpose: it changes only through the allocation workflow.
type Target struct {
	Fields map[string]bool
	Apply  Applier
}

type Handler struct {
	conn    db.Conn
	targets map[string]Target
}

// NewHandler takes the record types change requests can act on, keyed by the
// finding's object type.
func NewHandler(conn db.Conn, targets map[string]Target) *Handler { return &Handler{conn, targets} }

func (h *Handler) Register(g *auth.Guard) {
	g.Handle(http.MethodGet, "/reconciliation/results", h.list, auth.InventoryView)
	g.Handle(http.MethodGet, "/reconciliation/summary", h.summary, auth.InventoryView)
	g.Handle(http.MethodPost, "/reconciliation/run", h.run, auth.DiscoveryRun)
	g.Handle(http.MethodPost, "/reconciliation/results/:id/review", h.review, auth.InventoryEdit)
	g.Handle(http.MethodGet, "/reconciliation/rules", h.listSettings, auth.InventoryView)
	g.Handle(http.MethodPut, "/reconciliation/rules/:classification", h.saveSettings, auth.DiscoveryManage)
	g.Handle(http.MethodPost, "/reconciliation/results/:id/change-requests", h.proposeChange, auth.InventoryEdit)
	g.Handle(http.MethodGet, "/change-requests", h.listChanges, auth.InventoryView)
	g.Handle(http.MethodPost, "/change-requests/:id/approve", h.approveChange, auth.RequestsApprove)
	g.Handle(http.MethodPost, "/change-requests/:id/reject", h.rejectChange, auth.RequestsApprove)
}

func (h *Handler) run(c *gin.Context) {
	out, err := Run(c.Request.Context(), h.conn, auth.Current(c).UserID)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, http.StatusOK, out)
}
