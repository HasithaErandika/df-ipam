// Package ipaddresses manages IP records. Allocation state only changes through a
// person's request here. Discovery has no path into these writes.
package ipaddresses

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"strings"
	"time"

	"dfipam/api/internal/audit"
	"dfipam/api/internal/auth"
	"dfipam/api/internal/netcalc"
	"dfipam/api/internal/platform/db"
	"dfipam/api/internal/platform/httpx"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
)

var states = map[string]bool{"allocated": true, "reserved": true, "available_candidate": true, "quarantined": true, "retired": true}

type Address struct {
	ID                    string     `json:"id"`
	Address               string     `json:"address"`
	Family                int        `json:"family"`
	SubnetID              string     `json:"subnetId"`
	SubnetName            string     `json:"subnetName"`
	SubnetCIDR            string     `json:"subnetCidr"`
	RoutingDomainID       string     `json:"routingDomainId"`
	RoutingDomainName     string     `json:"routingDomainName"`
	Environment           string     `json:"environment"`
	SiteName              *string    `json:"siteName"`
	Hostname              string     `json:"hostname"`
	FQDN                  string     `json:"fqdn"`
	MAC                   *string    `json:"mac"`
	DeviceID              *string    `json:"deviceId"`
	DeviceName            *string    `json:"deviceName"`
	DeviceType            *string    `json:"deviceType"`
	Vendor                *string    `json:"vendor"`
	OS                    *string    `json:"operatingSystem"`
	OSFamily              *string    `json:"osFamily"`
	ApplicationID         *string    `json:"applicationId"`
	ApplicationName       *string    `json:"applicationName"`
	BusinessServiceID     *string    `json:"businessServiceId"`
	BusinessServiceName   *string    `json:"businessServiceName"`
	OwnerID               *string    `json:"ownerId"`
	OwnerEmail            *string    `json:"ownerEmail"`
	AllocationState       string     `json:"allocationState"`
	ObservationState      string     `json:"observationState"`
	ConfidenceScore       *int16     `json:"confidenceScore"`
	ConfidenceExplanation string     `json:"confidenceExplanation"`
	DiscoverySource       *string    `json:"discoverySource"`
	FirstSeen             *time.Time `json:"firstSeen"`
	LastSeen              *time.Time `json:"lastSeen"`
	LastVerified          *time.Time `json:"lastVerified"`
	Notes                 string     `json:"notes"`
	UpdatedAt             time.Time  `json:"updatedAt"`
}

const selectAddress = `SELECT i.id::text, host(i.address), family(i.address), i.subnet_id::text, s.name, s.cidr::text,
	i.routing_domain_id::text, rd.name, s.environment, st.name, i.hostname, i.fqdn, i.mac::text,
	i.device_id::text, d.name, d.device_type, NULLIF(d.vendor, ''), NULLIF(d.os_version, ''), NULLIF(d.os_family, 'unknown'),
	i.application_id::text, a.name, i.business_service_id::text, bs.name, i.owner_id::text, u.email,
	i.allocation_state, i.observation_state, i.confidence_score, i.confidence_explanation, os.name,
	i.first_seen, i.last_seen, i.last_verified, i.notes, i.updated_at`

const fromAddress = ` FROM ip_addresses i
	JOIN subnets s ON s.id = i.subnet_id
	JOIN routing_domains rd ON rd.id = i.routing_domain_id
	LEFT JOIN sites st ON st.id = s.site_id
	LEFT JOIN devices d ON d.id = i.device_id
	LEFT JOIN applications a ON a.id = i.application_id
	LEFT JOIN business_services bs ON bs.id = i.business_service_id
	LEFT JOIN users u ON u.id = i.owner_id
	LEFT JOIN observation_sources os ON os.id = i.discovery_source_id`

func scan(row pgx.CollectableRow) (Address, error) {
	var a Address
	err := row.Scan(&a.ID, &a.Address, &a.Family, &a.SubnetID, &a.SubnetName, &a.SubnetCIDR, &a.RoutingDomainID, &a.RoutingDomainName,
		&a.Environment, &a.SiteName, &a.Hostname, &a.FQDN, &a.MAC, &a.DeviceID, &a.DeviceName, &a.DeviceType, &a.Vendor, &a.OS, &a.OSFamily,
		&a.ApplicationID, &a.ApplicationName, &a.BusinessServiceID, &a.BusinessServiceName, &a.OwnerID, &a.OwnerEmail,
		&a.AllocationState, &a.ObservationState, &a.ConfidenceScore, &a.ConfidenceExplanation, &a.DiscoverySource,
		&a.FirstSeen, &a.LastSeen, &a.LastVerified, &a.Notes, &a.UpdatedAt)
	return a, err
}

type Handler struct{ conn db.Conn }

func NewHandler(conn db.Conn) *Handler { return &Handler{conn} }

func (h *Handler) Register(g *auth.Guard) {
	g.Handle(http.MethodGet, "/ip-addresses", h.list, auth.InventoryView)
	g.Handle(http.MethodPost, "/ip-addresses", h.create, auth.InventoryCreate)
	g.Handle(http.MethodGet, "/ip-addresses/:id", h.get, auth.InventoryView)
	g.Handle(http.MethodPatch, "/ip-addresses/:id", h.update, auth.InventoryEdit)
	g.Handle(http.MethodGet, "/ip-addresses/:id/history", h.history, auth.InventoryView)
	g.Handle(http.MethodGet, "/ip-addresses/:id/evidence", h.evidence, auth.InventoryView)
	g.Handle(http.MethodGet, "/ip-addresses/:id/device", h.device, auth.InventoryView)
}

var sorts = map[string]string{"address": "i.address", "hostname": "i.hostname", "updatedAt": "i.updated_at"}

func (h *Handler) list(c *gin.Context) {
	p := httpx.ParsePage(c)
	var where []string
	var args []any
	add := func(clause string, v any) {
		args = append(args, v)
		where = append(where, fmt.Sprintf(clause, len(args)))
	}
	if q := strings.TrimSpace(c.Query("q")); q != "" {
		add(`(host(i.address) ILIKE '%%' || $%[1]d || '%%' OR i.hostname ILIKE '%%' || $%[1]d || '%%'
			OR i.fqdn ILIKE '%%' || $%[1]d || '%%' OR i.mac::text ILIKE '%%' || $%[1]d || '%%')`, q)
	}
	for param, column := range map[string]string{"subnetId": "i.subnet_id", "routingDomainId": "i.routing_domain_id",
		"deviceId": "i.device_id", "ownerId": "i.owner_id", "siteId": "s.site_id", "businessServiceId": "i.business_service_id"} {
		if v := c.Query(param); v != "" {
			if !httpx.IsUUID(v) {
				httpx.Fail(c, httpx.Validation(param, "Must be a valid ID."))
				return
			}
			add(column+" = $%d::uuid", v)
		}
	}
	for param, column := range map[string]string{"allocationState": "i.allocation_state", "observationState": "i.observation_state", "environment": "s.environment"} {
		if v := c.Query(param); v != "" {
			add(column+" = $%d", v)
		}
	}
	clause := ""
	if len(where) > 0 {
		clause = " WHERE " + strings.Join(where, " AND ")
	}
	order := "i.address"
	sort := c.Query("sort")
	direction := ""
	if strings.HasPrefix(sort, "-") {
		sort, direction = sort[1:], " DESC"
	}
	if col, ok := sorts[sort]; ok {
		order = col + direction
	}
	ctx := c.Request.Context()
	var total int64
	if err := h.conn.QueryRow(ctx, "SELECT count(*)"+fromAddress+clause, args...).Scan(&total); err != nil {
		httpx.Fail(c, err)
		return
	}
	args = append(args, p.Size, p.Offset())
	rows, err := h.conn.Query(ctx, fmt.Sprintf("%s%s%s ORDER BY %s, i.id LIMIT $%d OFFSET $%d", selectAddress, fromAddress, clause, order, len(args)-1, len(args)), args...)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	out, err := pgx.CollectRows(rows, scan)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.List(c, out, p, total)
}

func one(c *gin.Context, q db.Conn, id string) (Address, error) {
	rows, err := q.Query(c.Request.Context(), selectAddress+fromAddress+" WHERE i.id = $1::uuid", id)
	if err != nil {
		return Address{}, err
	}
	a, err := pgx.CollectExactlyOneRow(rows, scan)
	if errors.Is(err, pgx.ErrNoRows) {
		return a, httpx.ErrNotFound
	}
	return a, err
}

func (h *Handler) get(c *gin.Context) {
	id, ok := httpx.PathID(c)
	if !ok {
		return
	}
	a, err := one(c, h.conn, id)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, http.StatusOK, a)
}

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
	if r.AllocationState != nil {
		if !states[*r.AllocationState] {
			return httpx.Validation("allocationState", "Choose a valid allocation state.")
		}
		p := auth.Current(c)
		if !p.Has(auth.IPAllocate) && !p.Has(auth.IPReserve) {
			return httpx.ErrForbidden
		}
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
func applyState(c *gin.Context, tx pgx.Tx, a Address, next, actor string) error {
	ctx := c.Request.Context()
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
	ctx, actor := c.Request.Context(), auth.Current(c).UserID
	var out Address
	err = db.InTx(ctx, h.conn, func(tx pgx.Tx) error {
		// An address inside overlapping subnets belongs to the most specific one.
		var subnetID, cidr string
		err := tx.QueryRow(ctx, `SELECT id::text, cidr::text FROM subnets WHERE routing_domain_id = $1::uuid AND archived_at IS NULL
			AND cidr >>= $2::text::inet ORDER BY masklen(cidr) DESC LIMIT 1 FOR UPDATE`, req.RoutingDomainID, addr.String()).Scan(&subnetID, &cidr)
		if errors.Is(err, pgx.ErrNoRows) {
			return httpx.NewError(http.StatusUnprocessableEntity, "ADDRESS_NOT_IN_SUBNET", "No active subnet in this routing domain contains this address.")
		}
		if err != nil {
			return err
		}
		usable := netcalc.UsableRange(netip.MustParsePrefix(cidr))
		if addr.Less(usable.First) || usable.Last.Less(addr) {
			return httpx.Validation("address", "This is the subnet's network or broadcast address.")
		}
		var id string
		if err := tx.QueryRow(ctx, `INSERT INTO ip_addresses (routing_domain_id, subnet_id, address, hostname, fqdn, mac, notes, device_id,
			application_id, business_service_id, owner_id, created_by, updated_by)
			VALUES ($1::uuid, $2::uuid, $3::text::inet, $4, $5, $6::text::macaddr, $7, $8::uuid, $9::uuid, $10::uuid, $11::uuid, $12::uuid, $12::uuid)
			RETURNING id::text`, req.RoutingDomainID, subnetID, addr.String(), text(req.Hostname), text(req.FQDN), emptyToNil(req.MAC), text(req.Notes),
			emptyToNil(req.DeviceID), emptyToNil(req.ApplicationID), emptyToNil(req.BusinessServiceID), emptyToNil(req.OwnerID), actor).Scan(&id); err != nil {
			return err
		}
		if req.AllocationState != nil && *req.AllocationState != "available_candidate" {
			if _, err := tx.Exec(ctx, `UPDATE ip_addresses SET allocation_state = $2 WHERE id = $1::uuid`, id, *req.AllocationState); err != nil {
				return err
			}
			if err := applyState(c, tx, Address{ID: id}, *req.AllocationState, actor); err != nil {
				return err
			}
		}
		if out, err = one(c, tx, id); err != nil {
			return err
		}
		return audit.Write(ctx, tx, audit.Entry{ActorID: actor, Action: "ip_address.create", ObjectType: "ip_address", ObjectID: id, New: out})
	})
	if err != nil {
		httpx.Fail(c, translate(err))
		return
	}
	httpx.OK(c, http.StatusCreated, out)
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
	ctx, actor := c.Request.Context(), auth.Current(c).UserID
	var out Address
	err := db.InTx(ctx, h.conn, func(tx pgx.Tx) error {
		var locked string
		if err := tx.QueryRow(ctx, `SELECT id::text FROM ip_addresses WHERE id = $1::uuid FOR UPDATE`, id).Scan(&locked); errors.Is(err, pgx.ErrNoRows) {
			return httpx.ErrNotFound
		} else if err != nil {
			return err
		}
		old, err := one(c, tx, id)
		if err != nil {
			return err
		}
		if req.AllocationState != nil && *req.AllocationState != old.AllocationState {
			held := old.AllocationState == "allocated" || old.AllocationState == "reserved"
			if held && strings.TrimSpace(req.Reason) == "" {
				return httpx.Validation("reason", "Give a reason for releasing this address.")
			}
			if err := applyState(c, tx, old, *req.AllocationState, actor); err != nil {
				return err
			}
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
			return err
		}
		if out, err = one(c, tx, id); err != nil {
			return err
		}
		return audit.Write(ctx, tx, audit.Entry{ActorID: actor, Action: "ip_address.update", ObjectType: "ip_address", ObjectID: id,
			Old: old, New: gin.H{"record": out, "reason": req.Reason}})
	})
	if err != nil {
		httpx.Fail(c, translate(err))
		return
	}
	httpx.OK(c, http.StatusOK, out)
}

func (h *Handler) history(c *gin.Context) {
	id, ok := httpx.PathID(c)
	if !ok {
		return
	}
	rows, err := h.conn.Query(c.Request.Context(), `SELECT a.action, a.occurred_at, u.email, a.result FROM audit_logs a LEFT JOIN users u ON u.id = a.actor_id
		WHERE a.object_type = 'ip_address' AND a.object_id = $1 ORDER BY a.occurred_at DESC, a.id DESC LIMIT 100`, id)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	type change struct {
		Action     string    `json:"action"`
		OccurredAt time.Time `json:"occurredAt"`
		Actor      *string   `json:"actorEmail"`
		Result     string    `json:"result"`
	}
	out, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (change, error) {
		var e change
		return e, r.Scan(&e.Action, &e.OccurredAt, &e.Actor, &e.Result)
	})
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, http.StatusOK, append([]change{}, out...))
}

func (h *Handler) evidence(c *gin.Context) {
	id, ok := httpx.PathID(c)
	if !ok {
		return
	}
	rows, err := h.conn.Query(c.Request.Context(), `
		SELECT os.name, o.mac::text, o.hostname, o.payload, o.observed_at
		FROM ip_addresses i JOIN discovery_observations o ON o.address = i.address AND o.routing_domain_id = i.routing_domain_id
		JOIN observation_sources os ON os.id = o.source_id
		WHERE i.id = $1::uuid ORDER BY o.observed_at DESC LIMIT 100`, id)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	type item struct {
		Source     string          `json:"source"`
		MAC        *string         `json:"mac"`
		Hostname   *string         `json:"hostname"`
		Payload    json.RawMessage `json:"payload"`
		ObservedAt time.Time       `json:"observedAt"`
	}
	out, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (item, error) {
		var e item
		return e, r.Scan(&e.Source, &e.MAC, &e.Hostname, &e.Payload, &e.ObservedAt)
	})
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, http.StatusOK, append([]item{}, out...))
}

// ChangeFields lists what a change request may set on an address. Allocation state
// isn't here: it moves only through the allocation workflow.
var ChangeFields = []db.Field{
	{Name: "hostname", Column: "hostname"}, {Name: "notes", Column: "notes"},
	{Name: "mac", Column: "mac", Cast: "macaddr", Nullable: true},
	{Name: "deviceId", Column: "device_id", Cast: "uuid", Nullable: true},
	{Name: "applicationId", Column: "application_id", Cast: "uuid", Nullable: true},
	{Name: "businessServiceId", Column: "business_service_id", Cast: "uuid", Nullable: true},
	{Name: "ownerId", Column: "owner_id", Cast: "uuid", Nullable: true},
}

// ApplyChange applies an approved change request.
func ApplyChange(ctx context.Context, tx pgx.Tx, id, actor string, changes map[string]string) (map[string]string, map[string]string, error) {
	return db.ApplyFields(ctx, tx, "ip_addresses", id, actor, "", ChangeFields, changes)
}
