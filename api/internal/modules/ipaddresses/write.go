package ipaddresses

import (
	"context"
	"dfipam/api/internal/audit"
	"dfipam/api/internal/auth"
	"dfipam/api/internal/platform/db"
	"dfipam/api/internal/platform/httpx"
	"errors"
	"net"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
)

type writeRequest struct {
	RoutingDomainID   string  `json:"routingDomainId"`
	Address           string  `json:"address"`
	Hostname          *string `json:"hostname"`
	FQDN              *string `json:"fqdn"`
	MAC               *string `json:"mac"`
	Notes             *string `json:"notes"`
	DeviceID          *string `json:"deviceId"`
	ApplicationID     *string `json:"applicationId"`
	BusinessServiceID *string `json:"businessServiceId"`
	OwnerID           *string `json:"ownerId"`
	AllocationState   *string `json:"allocationState"`
	Reason            string  `json:"reason"`
}

func (r *writeRequest) validate(c *gin.Context) error {
	if err := r.checkValues(); err != nil {
		return err
	}
	if r.AllocationState != nil {
		p := auth.Current(c)
		if !p.Has(auth.IPAllocate) && !p.Has(auth.IPReserve) {
			return httpx.ErrForbidden
		}
	}
	return nil
}

// checkValues checks what the request says, whoever is sending it.
func (r *writeRequest) checkValues() error {
	for name, field := range map[string]**string{"deviceId": &r.DeviceID, "applicationId": &r.ApplicationID, "businessServiceId": &r.BusinessServiceID, "ownerId": &r.OwnerID} {
		if v := *field; v != nil && *v != "" && !httpx.IsUUID(*v) {
			return httpx.Validation(name, "Choose a valid record.")
		}
	}
	if r.MAC != nil && *r.MAC != "" {
		if _, err := net.ParseMAC(*r.MAC); err != nil {
			return httpx.Validation("mac", "Enter a valid MAC address.")
		}
	}
	if r.AllocationState != nil && !states[*r.AllocationState] {
		return httpx.Validation("allocationState", "Choose a valid allocation state.")
	}
	return nil
}

func emptyToNil(v *string) *string {
	if v == nil || *v == "" {
		return nil
	}
	return v
}

func text(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}

// applyState keeps ip_allocations and ip_reservations in step with the record.
// The caller holds the row, and the unique indexes are the final guarantee.
func applyState(ctx context.Context, tx pgx.Tx, a Address, next, actor string) error {
	switch a.AllocationState {
	case "allocated":
		if _, err := tx.Exec(ctx, `UPDATE ip_allocations SET status = 'released', released_by = $2::uuid, released_at = now()
			WHERE ip_address_id = $1::uuid AND status = 'active'`, a.ID, actor); err != nil {
			return err
		}
	case "reserved":
		if _, err := tx.Exec(ctx, `UPDATE ip_reservations SET status = 'released' WHERE ip_address_id = $1::uuid AND status = 'active'`, a.ID); err != nil {
			return err
		}
	}
	switch next {
	case "allocated":
		_, err := tx.Exec(ctx, `INSERT INTO ip_allocations (ip_address_id, subnet_id, routing_domain_id, address, allocated_by)
			SELECT id, subnet_id, routing_domain_id, address, $2::uuid FROM ip_addresses WHERE id = $1::uuid`, a.ID, actor)
		return err
	case "reserved":
		_, err := tx.Exec(ctx, `INSERT INTO ip_reservations (ip_address_id, subnet_id, routing_domain_id, address, reserved_by)
			SELECT id, subnet_id, routing_domain_id, address, $2::uuid FROM ip_addresses WHERE id = $1::uuid`, a.ID, actor)
		return err
	}
	return nil
}

func translate(err error) error {
	switch db.ErrorCode(err) {
	case db.CodeUnique:
		return httpx.NewError(http.StatusConflict, "ADDRESS_UNAVAILABLE", "This address already has a record in this routing domain.")
	case db.CodeForeignKey, db.CodeBadValue:
		return httpx.Validation("body", "A chosen record doesn't exist.")
	case db.CodeCheck:
		return httpx.NewError(http.StatusUnprocessableEntity, "ADDRESS_NOT_IN_SUBNET", "This address is outside its subnet.")
	}
	return err
}

func (h *Handler) update(c *gin.Context) {
	id, ok := httpx.PathID(c)
	if !ok {
		return
	}
	var req writeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Fail(c, httpx.ErrBadRequest)
		return
	}
	if err := req.validate(c); err != nil {
		httpx.Fail(c, err)
		return
	}
	var out Address
	err := db.InTx(c.Request.Context(), h.conn, func(tx pgx.Tx) (err error) {
		out, err = updateAddress(c, tx, id, req)
		return err
	})
	if err != nil {
		httpx.Fail(c, translate(err))
		return
	}
	httpx.OK(c, http.StatusOK, out)
}

// changeState moves the record to a new allocation state. Letting go of a held
// address needs a reason.
func changeState(c *gin.Context, tx pgx.Tx, old Address, req writeRequest, actor string) error {
	if req.AllocationState == nil || *req.AllocationState == old.AllocationState {
		return nil
	}
	held := old.AllocationState == "allocated" || old.AllocationState == "reserved"
	if held && strings.TrimSpace(req.Reason) == "" {
		return httpx.Validation("reason", "Give a reason for releasing this address.")
	}
	return applyState(c.Request.Context(), tx, old, *req.AllocationState, actor)
}

func updateAddress(c *gin.Context, tx pgx.Tx, id string, req writeRequest) (Address, error) {
	ctx, actor := c.Request.Context(), auth.Current(c).UserID
	var locked string
	if err := tx.QueryRow(ctx, `SELECT id::text FROM ip_addresses WHERE id = $1::uuid FOR UPDATE`, id).Scan(&locked); errors.Is(err, pgx.ErrNoRows) {
		return Address{}, httpx.ErrNotFound
	} else if err != nil {
		return Address{}, err
	}
	old, err := one(c, tx, id)
	if err != nil {
		return old, err
	}
	if err := changeState(c, tx, old, req, actor); err != nil {
		return old, err
	}
	if _, err := tx.Exec(ctx, `UPDATE ip_addresses SET
		hostname = COALESCE($2, hostname), fqdn = COALESCE($3, fqdn), notes = COALESCE($4, notes),
		mac = CASE WHEN $5::text IS NULL THEN mac ELSE NULLIF($5, '')::macaddr END,
		device_id = CASE WHEN $6::text IS NULL THEN device_id ELSE NULLIF($6, '')::uuid END,
		application_id = CASE WHEN $7::text IS NULL THEN application_id ELSE NULLIF($7, '')::uuid END,
		business_service_id = CASE WHEN $8::text IS NULL THEN business_service_id ELSE NULLIF($8, '')::uuid END,
		owner_id = CASE WHEN $9::text IS NULL THEN owner_id ELSE NULLIF($9, '')::uuid END,
		allocation_state = COALESCE($10, allocation_state), updated_by = $11::uuid
		WHERE id = $1::uuid`, id, req.Hostname, req.FQDN, req.Notes, req.MAC, req.DeviceID, req.ApplicationID,
		req.BusinessServiceID, req.OwnerID, req.AllocationState, actor); err != nil {
		return old, err
	}
	out, err := one(c, tx, id)
	if err != nil {
		return out, err
	}
	return out, audit.Write(ctx, tx, audit.Entry{ActorID: actor, Action: "ip_address.update", ObjectType: "ip_address", ObjectID: id,
		Old: old, New: gin.H{"record": out, "reason": req.Reason}})
}
