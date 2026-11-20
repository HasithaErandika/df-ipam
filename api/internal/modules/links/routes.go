// Package links serves Network Links: the company's internet lines, VPN
// tunnels, leased lines and other connectivity, with their providers,
// contracts and attachments. Records are administrative: nothing is polled.
package links

import (
	"encoding/json"
	"net/http"

	"dfipam/api/internal/audit"
	"dfipam/api/internal/auth"
	"dfipam/api/internal/platform/db"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
)

type Handler struct {
	conn     db.Conn
	files    *Store
	firewall Firewall
	sample   *sampler
}

// New takes the attachment store and the FortiGate connection; nil for either
// answers its routes with an error saying what to set up.
func New(conn db.Conn, files *Store, firewall Firewall) *Handler {
	return &Handler{conn: conn, files: files, firewall: firewall, sample: &sampler{}}
}

func (h *Handler) Register(g *auth.Guard) {
	view := auth.LinksView
	g.Handle(http.MethodGet, "/network-links", h.list, view)
	g.Handle(http.MethodPost, "/network-links", h.create, auth.LinksCreate)
	g.Handle(http.MethodGet, "/network-links/dashboard", h.dashboard, view)
	g.Handle(http.MethodGet, "/network-links/renewals", h.renewals, auth.LinksCommercialView)
	g.Handle(http.MethodGet, "/network-links/options", h.options, auth.LinksCreate, auth.LinksEdit)
	g.Handle(http.MethodGet, "/network-links/export", h.export, auth.LinksImportExport)
	g.Handle(http.MethodGet, "/network-links/template", h.template, auth.LinksImportExport)
	g.Handle(http.MethodPost, "/network-links/import", h.importLinks, auth.LinksImportExport)
	// The Firewall section: live state for anyone who sees the network or links,
	// and WAN lines and tunnels reviewed into Network Links.
	g.Handle(http.MethodGet, "/firewall/live", h.live, view, auth.InventoryView)
	g.Handle(http.MethodGet, "/firewall/links/review", h.firewallReview, auth.LinksCreate, auth.LinksEdit)
	g.Handle(http.MethodPost, "/firewall/links/apply", h.firewallApply, auth.LinksCreate, auth.LinksEdit)
	g.Handle(http.MethodGet, "/network-links/:id", h.detail, view)
	g.Handle(http.MethodPut, "/network-links/:id", h.update, auth.LinksEdit)
	g.Handle(http.MethodPost, "/network-links/:id/archive", h.archive, auth.LinksArchive)
	g.Handle(http.MethodPost, "/network-links/:id/restore", h.restore, auth.LinksArchive)
	g.Handle(http.MethodGet, "/network-links/:id/history", h.history, auth.AuditView)
	g.Handle(http.MethodPost, "/network-links/:id/attachments", h.upload, auth.LinksEdit)
	g.Handle(http.MethodGet, "/network-links/:id/attachments/:file", h.downloadFile, view)
	g.Handle(http.MethodDelete, "/network-links/:id/attachments/:file", h.removeFile, auth.LinksEdit)

	g.Handle(http.MethodGet, "/link-providers", h.providers, view)
	g.Handle(http.MethodPost, "/link-providers", h.createProvider, auth.LinksProviders)
	g.Handle(http.MethodPut, "/link-providers/:id", h.updateProvider, auth.LinksProviders)

	g.Handle(http.MethodGet, "/link-settings", h.getSettings, view, auth.NetworkManage)
	g.Handle(http.MethodPut, "/link-settings", h.saveSettings, auth.NetworkManage)
}

func actor(c *gin.Context) string { return auth.Current(c).UserID }

func can(c *gin.Context, permission string) bool { return auth.Current(c).Has(permission) }

func record(c *gin.Context, tx pgx.Tx, action, kind, id string, old, new any) error {
	return audit.Write(c, tx, audit.Entry{ActorID: actor(c), Action: action, ObjectType: kind, ObjectID: id, Old: old, New: new})
}

// linkRow is one link as stored, with its related links and local subnets,
// as JSON for the audit record. reminders_sent is bookkeeping, left out.
const linkRow = `SELECT to_jsonb(t) - 'reminders_sent' || jsonb_build_object(
	'relations', COALESCE((SELECT jsonb_agg(jsonb_build_object('related', l.code, 'kind', r.kind) ORDER BY l.code)
		FROM network_link_relations r JOIN network_links l ON l.id = r.related_id WHERE r.link_id = t.id), '[]'),
	'local_subnets', COALESCE((SELECT jsonb_agg(s.network::text ORDER BY s.network)
		FROM network_link_segments x JOIN segments s ON s.id = x.segment_id WHERE x.link_id = t.id), '[]'))
	FROM network_links t WHERE t.id = $1::uuid`

// lockLink locks one link and returns it as JSON for the audit record.
func lockLink(c *gin.Context, tx pgx.Tx, id string) (json.RawMessage, error) {
	if _, err := tx.Exec(c, `SELECT 1 FROM network_links WHERE id = $1::uuid FOR UPDATE`, id); err != nil {
		return nil, err
	}
	return storedLink(c, tx, id)
}

func storedLink(c *gin.Context, tx pgx.Tx, id string) (json.RawMessage, error) {
	var row []byte
	err := tx.QueryRow(c, linkRow, id).Scan(&row)
	return json.RawMessage(row), err
}
