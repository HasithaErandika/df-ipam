package subnets

import (
	"context"
	"errors"
	"net/http"

	"dfipam/api/internal/auth"
	"dfipam/api/internal/platform/db"
	"dfipam/api/internal/platform/httpx"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
)

// MaxBulk bounds one bulk request.
const MaxBulk = 500

// BulkRow is one subnet in a bulk create. Fields left empty take the batch's values.
type BulkRow struct {
	CIDR        string  `json:"cidr"`
	Name        string  `json:"name"`
	Gateway     *string `json:"gateway"`
	Environment string  `json:"environment"`
	Description string  `json:"description"`
}

type BulkRequest struct {
	RoutingDomainID string    `json:"routingDomainId"`
	Environment     string    `json:"environment"`
	DryRun          bool      `json:"dryRun"`
	Subnets         []BulkRow `json:"subnets"`
}

// BulkResult reports one row: created (or would be), or why not.
type BulkResult struct {
	Row   int     `json:"row"`
	CIDR  string  `json:"cidr"`
	Name  string  `json:"name"`
	OK    bool    `json:"ok"`
	ID    *string `json:"id,omitempty"`
	Code  string  `json:"code,omitempty"`
	Issue string  `json:"issue,omitempty"`
}

type BulkResponse struct {
	Created bool         `json:"created"`
	Valid   int          `json:"valid"`
	Invalid int          `json:"invalid"`
	Results []BulkResult `json:"results"`
}

var errBulkRolledBack = errors.New("bulk rolled back")

func issue(err error) (string, string) {
	var e *httpx.Error
	if errors.As(err, &e) {
		if len(e.Details) > 0 {
			return e.Code, e.Details[0].Issue
		}
		return e.Code, e.Message
	}
	return "INTERNAL_ERROR", "Something went wrong."
}

// BulkCreate creates every row in one transaction, or none. Each row goes
// through the same checks as a single create, so overlaps within the batch are
// caught too. A dry run, or any failed row, rolls everything back.
func (s *Service) BulkCreate(ctx context.Context, actor string, req BulkRequest) (BulkResponse, error) {
	if len(req.Subnets) == 0 || len(req.Subnets) > MaxBulk {
		return BulkResponse{}, httpx.Validation("subnets", "Send between 1 and 500 subnets.")
	}
	out := BulkResponse{Results: make([]BulkResult, 0, len(req.Subnets))}
	err := db.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		inner := &Service{pool: tx}
		for i, row := range req.Subnets {
			env := row.Environment
			if env == "" {
				env = req.Environment
			}
			res := BulkResult{Row: i + 1, CIDR: row.CIDR, Name: row.Name}
			created, err := inner.Create(ctx, actor, CreateRequest{RoutingDomainID: req.RoutingDomainID, CIDR: row.CIDR, Name: row.Name,
				Gateway: row.Gateway, Environment: env, Description: row.Description})
			if err != nil {
				res.Code, res.Issue = issue(err)
				if res.Code == "INTERNAL_ERROR" {
					return err
				}
				out.Invalid++
			} else {
				res.OK, res.ID, res.CIDR = true, &created.ID, created.Calculated.CIDR
				out.Valid++
			}
			out.Results = append(out.Results, res)
		}
		if req.DryRun || out.Invalid > 0 {
			return errBulkRolledBack
		}
		return nil
	})
	if errors.Is(err, errBulkRolledBack) {
		for i := range out.Results {
			out.Results[i].ID = nil
		}
		return out, nil
	}
	out.Created = err == nil
	return out, err
}

// BulkArchive archives the listed subnets together. Any missing one stops all.
func (s *Service) BulkArchive(ctx context.Context, actor string, ids []string) (int, error) {
	if len(ids) == 0 || len(ids) > MaxBulk {
		return 0, httpx.Validation("ids", "Send between 1 and 500 subnet IDs.")
	}
	for _, id := range ids {
		if !httpx.IsUUID(id) {
			return 0, httpx.Validation("ids", "Every ID must be a valid subnet ID.")
		}
	}
	err := db.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		inner := &Service{pool: tx}
		for _, id := range ids {
			if err := inner.Archive(ctx, actor, id); err != nil {
				return err
			}
		}
		return nil
	})
	return len(ids), err
}

func (h *Handler) bulkCreate(c *gin.Context) {
	var req BulkRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Fail(c, httpx.ErrBadRequest)
		return
	}
	if !httpx.IsUUID(req.RoutingDomainID) {
		httpx.Fail(c, httpx.Validation("routingDomainId", "Choose a routing domain."))
		return
	}
	out, err := h.svc.BulkCreate(c.Request.Context(), auth.Current(c).UserID, req)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	status := http.StatusOK
	if out.Created {
		status = http.StatusCreated
	}
	httpx.OK(c, status, out)
}

func (h *Handler) bulkArchive(c *gin.Context) {
	var req struct {
		IDs []string `json:"ids"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Fail(c, httpx.ErrBadRequest)
		return
	}
	n, err := h.svc.BulkArchive(c.Request.Context(), auth.Current(c).UserID, req.IDs)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, http.StatusOK, gin.H{"archived": n})
}
