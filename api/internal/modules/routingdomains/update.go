package routingdomains

import (
	"dfipam/api/internal/audit"
	"dfipam/api/internal/auth"
	"dfipam/api/internal/platform/db"
	"dfipam/api/internal/platform/httpx"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
)

type domainUpdate struct {
	Name       *string `json:"name"`
	RD         *string `json:"routeDistinguisher"`
	IsIsolated *bool   `json:"isIsolated"`
}

// apply returns the domain with the request's changes.
func (r domainUpdate) apply(next RoutingDomain) (RoutingDomain, error) {
	if r.Name != nil {
		if next.Name = strings.TrimSpace(*r.Name); next.Name == "" {
			return next, httpx.Validation("name", "Enter a name.")
		}
	}
	if r.RD != nil {
		next.RD = r.RD
		if strings.TrimSpace(*r.RD) == "" {
			next.RD = nil
		}
	}
	if r.IsIsolated != nil {
		next.IsIsolated = *r.IsIsolated
	}
	return next, nil
}

func (h *Handler) update(c *gin.Context) {
	id, ok := httpx.PathID(c)
	if !ok {
		return
	}
	var req domainUpdate
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
		old, err := h.one(c, tx, id)
		if err != nil {
			return err
		}
		next, err := req.apply(old)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE routing_domains SET name = $2, route_distinguisher = $3, is_isolated = $4, updated_by = $5::uuid
			WHERE id = $1::uuid`, id, next.Name, next.RD, next.IsIsolated, actor); err != nil {
			return err
		}
		if out, err = h.one(c, tx, id); err != nil {
			return err
		}
		return audit.Write(ctx, tx, audit.Entry{ActorID: actor, Action: "routing_domain.update", ObjectType: "routing_domain", ObjectID: id, Old: old, New: out})
	})
	if db.ErrorCode(err) == db.CodeUnique {
		err = httpx.NewError(http.StatusConflict, "ALREADY_EXISTS", "This organisation already has a routing domain with that name.")
	}
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, http.StatusOK, out)
}
