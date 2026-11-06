package ipaddresses

import (
	"fmt"
	"net/http"

	"dfipam/api/internal/auth"
	"dfipam/api/internal/platform/httpx"

	"github.com/gin-gonic/gin"
)

// An address is in scope through its own business service, or its subnet's.
const inScope = `(i.business_service_id = ANY($%[1]d::text[]::uuid[]) OR s.business_service_id = ANY($%[1]d::text[]::uuid[]))`

// textSearch matches on what this person may read. Searching a hidden field would
// let someone work out its value from which rows come back.
func textSearch(p *auth.Principal) string {
	mac := ""
	if p.Has(auth.InventoryView) {
		mac = ` OR i.mac::text ILIKE '%%' || $%[1]d || '%%'`
	}
	return `(host(i.address) ILIKE '%%' || $%[1]d || '%%' OR i.hostname ILIKE '%%' || $%[1]d || '%%'
		OR i.fqdn ILIKE '%%' || $%[1]d || '%%'` + mac + `)`
}

// hideInfrastructure leaves what a scoped reader needs: the address, what it is for
// and who owns it, but not the machine behind it or the evidence about it.
func (a *Address) hideInfrastructure() {
	a.MAC, a.DeviceID, a.DeviceName, a.DeviceType, a.Vendor, a.OS, a.OSFamily, a.DiscoverySource = nil, nil, nil, nil, nil, nil, nil, nil
	a.ConfidenceScore, a.ConfidenceExplanation, a.Notes = nil, "", ""
}

// limit applies a reader's scope to one address. Out of scope reads as not found, so
// the answer doesn't confirm that it exists.
func (h *Handler) limit(c *gin.Context, a *Address) error {
	ctx := c.Request.Context()
	scope, err := auth.ServiceScope(ctx, h.conn, auth.Current(c))
	if err != nil || scope == nil {
		return err
	}
	var allowed bool
	err = h.conn.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM ip_addresses i JOIN subnets s ON s.id = i.subnet_id
		WHERE i.id = $2::uuid AND `+fmt.Sprintf(inScope, 1)+`)`, *scope, a.ID).Scan(&allowed)
	if err != nil {
		return err
	}
	if !allowed {
		return httpx.ErrNotFound
	}
	a.hideInfrastructure()
	return nil
}

// checkAssigned lets the owner of a record look after it. Who owns it, and what it
// belongs to, stay with people who may edit everything, or a record could be handed
// on. Allocation state has its own permission.
func checkAssigned(p *auth.Principal, old Address, req writeRequest) error {
	if p.Has(auth.InventoryEdit) {
		return nil
	}
	if old.OwnerID == nil || *old.OwnerID != p.UserID {
		return httpx.NewError(http.StatusForbidden, "NOT_ASSIGNED", "This address isn't assigned to you.")
	}
	if req.OwnerID != nil || req.BusinessServiceID != nil || req.ApplicationID != nil {
		return httpx.NewError(http.StatusForbidden, "ASSIGNED_FIELDS", "As its owner you can't change who owns this address, or what it belongs to.")
	}
	return nil
}
