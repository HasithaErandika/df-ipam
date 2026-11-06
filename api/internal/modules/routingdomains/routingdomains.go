// Package routingdomains manages routing domains and VRFs: the context in which
// an IP address is unique.
package routingdomains

import (
	"errors"
	"net/http"
	"strings"

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
	g.Handle(http.MethodPatch, "/routing-domains/:id", h.update, auth.InventoryEdit)
	g.Handle(http.MethodDelete, "/routing-domains/:id", h.remove, auth.InventoryDelete)
}

const selectDomain = `SELECT d.id::text, d.organisation_id::text, o.name, d.name, d.kind, d.route_distinguisher, d.is_isolated,
	(SELECT count(*) FROM subnets s WHERE s.routing_domain_id = d.id AND s.archived_at IS NULL)
	FROM routing_domains d JOIN organisations o ON o.id = d.organisation_id`

func (h *Handler) one(c *gin.Context, tx pgx.Tx, id string) (RoutingDomain, error) {
	var d RoutingDomain
	err := tx.QueryRow(c.Request.Context(), selectDomain+` WHERE d.id = $1::uuid`, id).
		Scan(&d.ID, &d.OrganisationID, &d.OrganisationName, &d.Name, &d.Kind, &d.RD, &d.IsIsolated, &d.SubnetCount)
	if errors.Is(err, pgx.ErrNoRows) {
		return d, httpx.ErrNotFound
	}
	return d, err
}

// remove deletes a routing domain nothing refers to. Archived subnets still
// count, because their history refers to the domain.
func (h *Handler) remove(c *gin.Context) {
	id, ok := httpx.PathID(c)
	if !ok {
		return
	}
	ctx, actor := c.Request.Context(), auth.Current(c).UserID
	err := db.InTx(ctx, h.pool, func(tx pgx.Tx) error {
		old, err := h.one(c, tx, id)
		if err != nil {
			return err
		}
		return Remove(ctx, tx, actor, id, old)
	})
	if db.ErrorCode(err) == db.CodeForeignKey {
		err = httpx.NewError(http.StatusConflict, "ROUTING_DOMAIN_IN_USE", "Subnets, blocks or other records use this routing domain. Archive or move them first.")
	}
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	c.Status(http.StatusNoContent)
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
	var req NewDomain
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
	err := db.InTx(ctx, h.pool, func(tx pgx.Tx) (err error) {
		id, err = Insert(ctx, tx, actor, req)
		return err
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
