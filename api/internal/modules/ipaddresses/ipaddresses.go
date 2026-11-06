// Package ipaddresses manages IP records. Allocation state only changes through a
// person's request here. Discovery has no path into these writes.
package ipaddresses

import (
	"errors"
	"net/http"
	"time"

	"dfipam/api/internal/auth"
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
	g.Handle(http.MethodGet, "/ip-addresses", h.list, auth.InventoryView, auth.InventoryViewScoped)
	g.Handle(http.MethodPost, "/ip-addresses", h.create, auth.InventoryCreate)
	g.Handle(http.MethodGet, "/ip-addresses/:id", h.get, auth.InventoryView, auth.InventoryViewScoped)
	g.Handle(http.MethodPatch, "/ip-addresses/:id", h.update, auth.InventoryEdit, auth.InventoryEditAssigned)
	g.Handle(http.MethodGet, "/ip-addresses/:id/history", h.history, auth.InventoryView)
	g.Handle(http.MethodGet, "/ip-addresses/:id/evidence", h.evidence, auth.InventoryView)
	g.Handle(http.MethodGet, "/ip-addresses/:id/device", h.device, auth.InventoryView)
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
	if err == nil {
		err = h.limit(c, &a)
	}
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, http.StatusOK, a)
}
