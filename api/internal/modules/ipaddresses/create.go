package ipaddresses

import (
	"context"
	"dfipam/api/internal/audit"
	"dfipam/api/internal/auth"
	"dfipam/api/internal/netcalc"
	"dfipam/api/internal/platform/db"
	"dfipam/api/internal/platform/httpx"
	"errors"
	"net/http"
	"net/netip"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
)

func (h *Handler) create(c *gin.Context) {
	var req writeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Fail(c, httpx.ErrBadRequest)
		return
	}
	addr, err := netip.ParseAddr(strings.TrimSpace(req.Address))
	if err != nil {
		httpx.Fail(c, httpx.Validation("address", "Enter a valid IP address."))
		return
	}
	if !httpx.IsUUID(req.RoutingDomainID) {
		httpx.Fail(c, httpx.Validation("routingDomainId", "Choose a routing domain."))
		return
	}
	if err := req.validate(c); err != nil {
		httpx.Fail(c, err)
		return
	}
	var out Address
	err = db.InTx(c.Request.Context(), h.conn, func(tx pgx.Tx) (err error) {
		out, err = insertAddress(c, tx, addr, req)
		return err
	})
	if err != nil {
		httpx.Fail(c, translate(err))
		return
	}
	httpx.OK(c, http.StatusCreated, out)
}

// holdingSubnet finds where a new address belongs. An address inside overlapping
// subnets belongs to the most specific one.
func holdingSubnet(ctx context.Context, tx pgx.Tx, routingDomainID string, addr netip.Addr) (string, error) {
	var subnetID, cidr string
	err := tx.QueryRow(ctx, `SELECT id::text, cidr::text FROM subnets WHERE routing_domain_id = $1::uuid AND archived_at IS NULL
		AND cidr >>= $2::text::inet ORDER BY masklen(cidr) DESC LIMIT 1 FOR UPDATE`, routingDomainID, addr.String()).Scan(&subnetID, &cidr)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", httpx.NewError(http.StatusUnprocessableEntity, "ADDRESS_NOT_IN_SUBNET", "No active subnet in this routing domain contains this address.")
	}
	if err != nil {
		return "", err
	}
	usable := netcalc.UsableRange(netip.MustParsePrefix(cidr))
	if addr.Less(usable.First) || usable.Last.Less(addr) {
		return "", httpx.Validation("address", "This is the subnet's network or broadcast address.")
	}
	return subnetID, nil
}

func insertAddress(c *gin.Context, tx pgx.Tx, addr netip.Addr, req writeRequest) (Address, error) {
	ctx, actor := c.Request.Context(), auth.Current(c).UserID
	subnetID, err := holdingSubnet(ctx, tx, req.RoutingDomainID, addr)
	if err != nil {
		return Address{}, err
	}
	var id string
	if err := tx.QueryRow(ctx, `INSERT INTO ip_addresses (routing_domain_id, subnet_id, address, hostname, fqdn, mac, notes, device_id,
		application_id, business_service_id, owner_id, created_by, updated_by)
		VALUES ($1::uuid, $2::uuid, $3::text::inet, $4, $5, $6::text::macaddr, $7, $8::uuid, $9::uuid, $10::uuid, $11::uuid, $12::uuid, $12::uuid)
		RETURNING id::text`, req.RoutingDomainID, subnetID, addr.String(), text(req.Hostname), text(req.FQDN), emptyToNil(req.MAC), text(req.Notes),
		emptyToNil(req.DeviceID), emptyToNil(req.ApplicationID), emptyToNil(req.BusinessServiceID), emptyToNil(req.OwnerID), actor).Scan(&id); err != nil {
		return Address{}, err
	}
	if req.AllocationState != nil && *req.AllocationState != "available_candidate" {
		if _, err := tx.Exec(ctx, `UPDATE ip_addresses SET allocation_state = $2 WHERE id = $1::uuid`, id, *req.AllocationState); err != nil {
			return Address{}, err
		}
		if err := applyState(c, tx, Address{ID: id}, *req.AllocationState, actor); err != nil {
			return Address{}, err
		}
	}
	out, err := one(c, tx, id)
	if err != nil {
		return out, err
	}
	return out, audit.Write(ctx, tx, audit.Entry{ActorID: actor, Action: "ip_address.create", ObjectType: "ip_address", ObjectID: id, New: out})
}
