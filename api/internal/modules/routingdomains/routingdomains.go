// Package routingdomains manages routing domains and VRFs: the context in which
// an IP address is unique.
package routingdomains

import (
	"context"
	"errors"
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
	g.Handle(http.MethodGet, "/routing-domains/:id", h.get, auth.InventoryView)
	g.Handle(http.MethodPatch, "/routing-domains/:id", h.update, auth.InventoryEdit)
	g.Handle(http.MethodDelete, "/routing-domains/:id", h.remove, auth.InventoryDelete)
}

const selectDomain = `SELECT d.id::text, d.organisation_id::text, o.name, d.name, d.kind, d.route_distinguisher, d.is_isolated,
	(SELECT count(*) FROM subnets s WHERE s.routing_domain_id = d.id AND s.archived_at IS NULL)
	FROM routing_domains d JOIN organisations o ON o.id = d.organisation_id`

func scanDomain(r pgx.Row) (RoutingDomain, error) {
	var d RoutingDomain
	return d, r.Scan(&d.ID, &d.OrganisationID, &d.OrganisationName, &d.Name, &d.Kind, &d.RD, &d.IsIsolated, &d.SubnetCount)
}

func load(ctx context.Context, q interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}, id string) (RoutingDomain, error) {
	d, err := scanDomain(q.QueryRow(ctx, selectDomain+" WHERE d.id = $1::uuid", id))
	if errors.Is(err, pgx.ErrNoRows) {
		return d, httpx.ErrNotFound
	}
	return d, err
}

func (h *Handler) get(c *gin.Context) {
	id, ok := httpx.PathID(c)
	if !ok {
		return
	}
	d, err := load(c.Request.Context(), h.pool, id)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, http.StatusOK, d)
}

// update changes the descriptive fields. The organisation can't change, because
// the domain's subnets and addresses are scoped to it.
func (h *Handler) update(c *gin.Context) {
	id, ok := httpx.PathID(c)
	if !ok {
		return
	}
	var req struct {
		Name       *string `json:"name"`
		Kind       *string `json:"kind"`
		RD         *string `json:"routeDistinguisher"`
		IsIsolated *bool   `json:"isIsolated"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Fail(c, httpx.ErrBadRequest)
		return
	}
	ctx, actor := c.Request.Context(), auth.Current(c).UserID
	var out RoutingDomain
	err := db.InTx(ctx, h.pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `SELECT 1 FROM routing_domains WHERE id = $1::uuid FOR UPDATE`, id); err != nil {
			return err
		}
		old, err := load(ctx, tx, id)
		if err != nil {
			return err
		}
		next := old
		if req.Name != nil {
			if next.Name = strings.TrimSpace(*req.Name); next.Name == "" {
				return httpx.Validation("name", "Enter a name.")
			}
		}
		if req.Kind != nil {
			if !kinds[*req.Kind] {
				return httpx.Validation("kind", "Choose a type.")
			}
			next.Kind = *req.Kind
		}
		if req.RD != nil {
			next.RD = req.RD
			if strings.TrimSpace(*req.RD) == "" {
				next.RD = nil
			}
		}
		if req.IsIsolated != nil {
			next.IsIsolated = *req.IsIsolated
		}
		if _, err := tx.Exec(ctx, `UPDATE routing_domains SET name = $2, kind = $3, route_distinguisher = $4, is_isolated = $5,
			updated_by = $6::uuid WHERE id = $1::uuid`, id, next.Name, next.Kind, next.RD, next.IsIsolated, actor); err != nil {
			return err
		}
		out = next
		return audit.Write(ctx, tx, audit.Entry{ActorID: actor, Action: "routing_domain.update", ObjectType: "routing_domain", ObjectID: id, Old: old, New: next})
	})
	if db.ErrorCode(err) == db.CodeUnique {
		httpx.Fail(c, httpx.NewError(http.StatusConflict, "ALREADY_EXISTS", "This organisation already has a routing domain with that name."))
		return
	}
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, http.StatusOK, out)
}

// remove deletes a routing domain that nothing refers to, archived subnets included.
func (h *Handler) remove(c *gin.Context) {
	id, ok := httpx.PathID(c)
	if !ok {
		return
	}
	ctx, actor := c.Request.Context(), auth.Current(c).UserID
	err := db.InTx(ctx, h.pool, func(tx pgx.Tx) error {
		old, err := load(ctx, tx, id)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `DELETE FROM routing_domains WHERE id = $1::uuid`, id); err != nil {
			return err
		}
		return audit.Write(ctx, tx, audit.Entry{ActorID: actor, Action: "routing_domain.delete", ObjectType: "routing_domain", ObjectID: id, Old: old})
	})
	if db.ErrorCode(err) == db.CodeForeignKey {
		httpx.Fail(c, httpx.NewError(http.StatusConflict, "RELATED_RECORD", "This routing domain still has subnets, blocks or other linked records."))
		return
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
	rows, err := h.pool.Query(ctx, selectDomain+` ORDER BY o.name, d.name, d.id LIMIT $1 OFFSET $2`, p.Size, p.Offset())
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	out, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (RoutingDomain, error) { return scanDomain(r) })
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
