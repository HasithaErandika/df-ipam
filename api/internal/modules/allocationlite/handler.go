// Package allocationlite handles the single-step request and reservation workflows.
package allocationlite

import (
	"dfipam/api/internal/auth"
	"dfipam/api/internal/platform/db"
	"net/http"
)

type Handler struct{ conn db.Conn }

func New(conn db.Conn) *Handler { return &Handler{conn: conn} }
func (h *Handler) Register(g *auth.Guard) {
	g.Handle(http.MethodGet, "/ip-requests", h.listRequests, auth.IPRequest, auth.RequestsApprove)
	g.Handle(http.MethodPost, "/ip-requests", h.createRequest, auth.IPRequest)
	g.Handle(http.MethodPost, "/ip-requests/:id/decision", h.decideRequest, auth.RequestsApprove)
	g.Handle(http.MethodPost, "/ip-requests/:id/cancel", h.cancelRequest, auth.IPRequest)
	g.Handle(http.MethodGet, "/reservations", h.listReservations, auth.InventoryView)
	g.Handle(http.MethodPost, "/reservations", h.createReservation, auth.IPReserve)
	g.Handle(http.MethodPut, "/reservations/:id", h.updateReservation, auth.IPReserve)
	g.Handle(http.MethodPost, "/reservations/:id/release", h.releaseReservation, auth.IPReserve)
	g.Handle(http.MethodPost, "/reservations/:id/convert", h.convertReservation, auth.IPReserve)
	g.Handle(http.MethodGet, "/dashboard", h.dashboard, auth.InventoryView)
}
