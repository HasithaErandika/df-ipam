package subnets

import (
	"context"
	"net/http"
	"slices"

	"dfipam/api/internal/auth"
	"dfipam/api/internal/platform/db"
	"dfipam/api/internal/platform/httpx"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
)

// hideInfrastructure leaves what a scoped reader needs. Their role covers a business
// service, not how the network behind it is built.
func (r *Response) hideInfrastructure() {
	r.Gateway, r.NetworkBlock, r.NetworkZone, r.VDOM, r.VLAN = nil, nil, nil, nil, nil
	r.SecurityZone, r.DiscoveryEnabled = "", false
}

// limit applies a reader's scope to one subnet. Out of scope reads as not found, so
// the answer doesn't confirm that it exists.
func (h *Handler) limit(c *gin.Context, row *Response) error {
	scope, err := auth.ServiceScope(c.Request.Context(), h.svc.pool, auth.Current(c))
	if err != nil || scope == nil {
		return err
	}
	if row.BusinessService == nil || !slices.Contains(*scope, row.BusinessService.ID) {
		return httpx.ErrNotFound
	}
	row.hideInfrastructure()
	return nil
}

// ownerOnly reports a change beyond what an assigned owner may make. Everything but
// the name and description decides how the network is run, or who answers for it.
func (r UpdateRequest) ownerOnly() bool {
	return r.Gateway != nil || r.Environment != nil || r.SecurityZone != nil || r.DiscoveryEnabled != nil || r.WarnThreshold != nil ||
		r.CriticalThreshold != nil || r.Sensitivity != nil || r.Mappings != (Mappings{})
}

func (h *Handler) write(c *gin.Context, id string, version int32, req UpdateRequest) (Response, error) {
	p, ctx := auth.Current(c), c.Request.Context()
	if p.Has(auth.InventoryEdit) {
		return h.svc.Update(ctx, p.UserID, id, version, req)
	}
	if req.ownerOnly() {
		return Response{}, httpx.NewError(http.StatusForbidden, "ASSIGNED_FIELDS", "As its owner you can change the name and description only.")
	}
	return h.svc.updateOwned(ctx, p.UserID, id, version, req)
}

func (s *Service) updateOwned(ctx context.Context, actor, id string, version int32, req UpdateRequest) (Response, error) {
	var updated Subnet
	err := db.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		old, err := s.load(ctx, tx, id, true)
		if err != nil {
			return err
		}
		if !owns(old, actor) {
			return httpx.NewError(http.StatusForbidden, "NOT_ASSIGNED", "This subnet isn't assigned to you.")
		}
		updated, err = s.update(ctx, tx, actor, id, version, req)
		return err
	})
	if err != nil {
		return Response{}, err
	}
	return toResponse(updated), nil
}

func owns(s Subnet, user string) bool {
	return (s.TechnicalOwnerID != nil && *s.TechnicalOwnerID == user) || (s.BusinessOwnerID != nil && *s.BusinessOwnerID == user)
}
