// Package admin serves the permission matrix, system health and retention settings.
package admin

import (
	"net/http"

	"dfipam/api/internal/auth"
	"dfipam/api/internal/platform/db"
)

// Handler serves administration.
type Handler struct {
	conn          db.Conn
	encryptionSet bool
}

// NewHandler returns a handler. encryptionSet says whether the master key is loaded.
func NewHandler(conn db.Conn, encryptionSet bool) *Handler { return &Handler{conn, encryptionSet} }

// Register adds the routes.
func (h *Handler) Register(g *auth.Guard) {
	g.Handle(http.MethodGet, "/admin/permission-matrix", h.matrix, auth.UsersManage, auth.AuditView)
	g.Handle(http.MethodPost, "/roles", h.createRole, auth.UsersManage)
	g.Handle(http.MethodPut, "/roles/:id", h.updateRole, auth.UsersManage)
	g.Handle(http.MethodPut, "/roles/:id/permissions", h.setPermissions, auth.UsersManage)
	g.Handle(http.MethodDelete, "/roles/:id", h.deleteRole, auth.UsersManage)
	g.Handle(http.MethodGet, "/system/health", h.health, auth.SystemManage, auth.AuditView)
	g.Handle(http.MethodGet, "/system/retention", h.retention, auth.SystemManage, auth.AuditView)
	g.Handle(http.MethodPut, "/system/retention/:class", h.setRetention, auth.SystemManage)
}
