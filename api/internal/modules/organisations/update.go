package organisations

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

type organisationUpdate struct {
	Name *string `json:"name"`
	Code *string `json:"code"`
}

// apply returns the organisation with the request's changes.
func (r organisationUpdate) apply(next Organisation) (Organisation, error) {
	if r.Name != nil {
		if next.Name = strings.TrimSpace(*r.Name); next.Name == "" {
			return next, httpx.Validation("name", "Enter a name.")
		}
	}
	if r.Code != nil {
		if next.Code = strings.ToUpper(strings.TrimSpace(*r.Code)); next.Code == "" {
			return next, httpx.Validation("code", "Enter a short code.")
		}
	}
	return next, nil
}

func (h *Handler) update(c *gin.Context) {
	id, ok := httpx.PathID(c)
	if !ok {
		return
	}
	var req organisationUpdate
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Fail(c, httpx.ErrBadRequest)
		return
	}
	ctx, actor := c.Request.Context(), auth.Current(c).UserID
	var out Organisation
	err := db.InTx(ctx, h.pool, func(tx pgx.Tx) error {
		old, err := h.one(c, tx, id, true)
		if err != nil {
			return err
		}
		next, err := req.apply(old)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE organisations SET name = $2, code = $3, updated_by = $4::uuid WHERE id = $1::uuid`,
			id, next.Name, next.Code, actor); err != nil {
			return err
		}
		if out, err = h.one(c, tx, id, false); err != nil {
			return err
		}
		return audit.Write(ctx, tx, audit.Entry{ActorID: actor, Action: "organisation.update", ObjectType: "organisation", ObjectID: id, Old: old, New: out})
	})
	if db.ErrorCode(err) == db.CodeUnique {
		err = httpx.NewError(http.StatusConflict, "ALREADY_EXISTS", "An organisation with this name or code already exists.")
	}
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, http.StatusOK, out)
}
