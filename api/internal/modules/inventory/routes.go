// Package inventory serves segments, devices and the IP register.
package inventory

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"dfipam/api/internal/audit"
	"dfipam/api/internal/auth"
	"dfipam/api/internal/modules/monitoring"
	"dfipam/api/internal/platform/db"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
)

type Handler struct{ conn db.Conn }

func NewHandler(conn db.Conn) *Handler { return &Handler{conn: conn} }

func (h *Handler) Register(g *auth.Guard) {
	read, edit := auth.InventoryView, auth.InventoryEdit
	g.Handle(http.MethodGet, "/segments", h.segments, read)
	g.Handle(http.MethodPost, "/segments", h.createSegment, edit)
	g.Handle(http.MethodPut, "/segments/:id", h.updateSegment, edit)
	g.Handle(http.MethodDelete, "/segments/:id", h.deleteSegment, edit)
	g.Handle(http.MethodGet, "/segments/:id/addresses", h.addresses, read)
	g.Handle(http.MethodGet, "/segments/:id/map", h.addressMap, read)

	g.Handle(http.MethodGet, "/devices", h.devices, read)
	g.Handle(http.MethodPost, "/devices", h.createDevice, edit)
	g.Handle(http.MethodPut, "/devices/:id", h.updateDevice, edit)
	g.Handle(http.MethodPost, "/devices/:id/decision", h.decideDevice, edit)
	g.Handle(http.MethodGet, "/devices/export", h.exportDevices, read)
	g.Handle(http.MethodPost, "/devices/import", h.importDevices, edit)

	g.Handle(http.MethodGet, "/ip-addresses", h.register, read)
	g.Handle(http.MethodPost, "/ip-addresses", h.createIP, edit)
	g.Handle(http.MethodPut, "/ip-addresses/:id", h.updateIP, edit)
	g.Handle(http.MethodDelete, "/ip-addresses/:id", h.releaseIP, edit)
	g.Handle(http.MethodGet, "/ip-addresses/available-candidate", h.candidate, edit, auth.RequestsApprove)
	g.Handle(http.MethodGet, "/ip-addresses/check", h.check, edit, auth.RequestsApprove, auth.IPReserve)
	g.Handle(http.MethodGet, "/ip-addresses/export", h.exportRegister, read)
	g.Handle(http.MethodPost, "/ip-addresses/import", h.importRegister, edit)
}

// recheck re-evaluates alerts after a committed change, before the reply, so an
// approved or registered device stops warning at once instead of after the next
// scan. The change already stands, so a failure here is only logged.
func (h *Handler) recheck(c *gin.Context) {
	if err := monitoring.New(h.conn).Evaluate(c); err != nil {
		slog.ErrorContext(c, "alert re-check after a change failed", "error", err)
	}
}

func actor(c *gin.Context) string { return auth.Current(c).UserID }

// lockOld locks one row and returns it as JSON for the audit record. table is a constant.
func lockOld(c *gin.Context, tx pgx.Tx, table, id string) (json.RawMessage, error) {
	var old []byte
	err := tx.QueryRow(c, "SELECT to_jsonb(t) FROM "+table+" t WHERE id = $1::uuid FOR UPDATE", id).Scan(&old)
	return json.RawMessage(old), err
}

func record(c *gin.Context, tx pgx.Tx, action, kind, id string, old, new any) error {
	return audit.Write(c, tx, audit.Entry{ActorID: actor(c), Action: action, ObjectType: kind, ObjectID: id, Old: old, New: new})
}
