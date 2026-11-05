// Package routingdomains manages routing domains and VRFs: the context in which
// an IP address is unique.
package routingdomains

import (
	"net/http"
	"strings"

	"dfipam/api/internal/audit"
	"dfipam/api/internal/auth"
	"dfipam/api/internal/platform/db"
	"dfipam/api/internal/platform/httpx"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
)

type RoutingDomain struct {
	ID               string  `json:"id"`
	OrganisationID   string  `json:"organisationId"`
	OrganisationName string  `json:"organisationName"`
	Name             string  `json:"name"`
	Kind             string  `json:"kind"`
	RD               *string `json:"routeDistinguisher"`
	IsIsolated       bool    `json:"isIsolated"`
	SubnetCount      int64   `json:"subnetCount"`
}

var kinds = map[string]bool{"default": true, "vrf": true, "vpc": true, "vnet": true, "isolated": true}

type Handler struct{ pool db.Conn }

func NewHandler(pool db.Conn) *Handler { return &Handler{pool} }

func (h *Handler) Register(g *auth.Guard) {
	g.Handle(http.MethodGet, "/routing-domains", h.list, auth.InventoryView)
	g.Handle(http.MethodPost, "/routing-domains", h.create, auth.InventoryCreate)
}

func (h *Handler) list(c *gin.Context) {
	p := httpx.ParsePage(c)
	ctx := c.Request.Context()
	var total int64
	if err := h.pool.QueryRow(ctx, `SELECT count(*) FROM routing_domains`).Scan(&total); err != nil {
		httpx.Fail(c, err)
		return
	}
	rows, err := h.pool.Query(ctx, `
		SELECT d.id::text, d.organisation_id::text, o.name, d.name, d.kind, d.route_distinguisher, d.is_isolated,
		       (SELECT count(*) FROM subnets s WHERE s.routing_domain_id = d.id AND s.archived_at IS NULL)
		FROM routing_domains d JOIN organisations o ON o.id = d.organisation_id
		ORDER BY o.name, d.name, d.id LIMIT $1 OFFSET $2`, p.Size, p.Offset())
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	out, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (RoutingDomain, error) {
		var d RoutingDomain
		return d, r.Scan(&d.ID, &d.OrganisationID, &d.OrganisationName, &d.Name, &d.Kind, &d.RD, &d.IsIsolated, &d.SubnetCount)
	})
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.List(c, out, p, total)
}

func (h *Handler) create(c *gin.Context) {
	var req struct {
		OrganisationID string  `json:"organisationId"`
		Name           string  `json:"name"`
		Kind           string  `json:"kind"`
		RD             *string `json:"routeDistinguisher"`
		IsIsolated     bool    `json:"isIsolated"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Fail(c, httpx.ErrBadRequest)
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		httpx.Fail(c, httpx.Validation("name", "Enter a name."))
		return
	}
	if !kinds[req.Kind] {
		httpx.Fail(c, httpx.Validation("kind", "Choose a type."))
		return
	}
	ctx, actor := c.Request.Context(), auth.Current(c).UserID
	var id string
	err := db.InTx(ctx, h.pool, func(tx pgx.Tx) error {
		if err := tx.QueryRow(ctx, `INSERT INTO routing_domains (organisation_id, name, kind, route_distinguisher, is_isolated, created_by, updated_by)
			VALUES ($1::uuid, $2, $3, $4, $5, $6::uuid, $6::uuid) RETURNING id::text`,
			req.OrganisationID, req.Name, req.Kind, req.RD, req.IsIsolated, actor).Scan(&id); err != nil {
			return err
		}
		return audit.Write(ctx, tx, audit.Entry{ActorID: actor, Action: "routing_domain.create", ObjectType: "routing_domain", ObjectID: id, New: req})
	})
	switch db.ErrorCode(err) {
	case db.CodeUnique:
		httpx.Fail(c, httpx.NewError(http.StatusConflict, "ALREADY_EXISTS", "This organisation already has a routing domain with that name."))
		return
	case db.CodeForeignKey, "22P02":
		httpx.Fail(c, httpx.Validation("organisationId", "Choose an organisation."))
		return
	}
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, http.StatusCreated, gin.H{"id": id})
}
