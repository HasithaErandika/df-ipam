// Package devices finds devices that several sources reported separately and
// merges them into one record, moving their interfaces and address links.
// Create, read, update and delete of devices and interfaces live in the catalog.
package devices

import (
	"context"
	"errors"
	"net/http"

	"dfipam/api/internal/audit"
	"dfipam/api/internal/auth"
	"dfipam/api/internal/platform/db"
	"dfipam/api/internal/platform/httpx"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
)

// Match is a group of devices that look like one machine, and why.
type Match struct {
	Reason  string   `json:"reason"`
	Value   string   `json:"value"`
	Devices []Device `json:"devices"`
}

type Device struct {
	ID           string  `json:"id"`
	Name         string  `json:"name"`
	DeviceType   string  `json:"deviceType"`
	Vendor       string  `json:"vendor"`
	SourceSystem string  `json:"sourceSystem"`
	ManagementIP *string `json:"managementIp"`
	Interfaces   int64   `json:"interfaces"`
	Addresses    int64   `json:"addresses"`
}

type Handler struct{ conn db.Conn }

func NewHandler(conn db.Conn) *Handler { return &Handler{conn} }

func (h *Handler) Register(g *auth.Guard) {
	g.Handle(http.MethodGet, "/devices/duplicates", h.duplicates, auth.InventoryView)
	g.Handle(http.MethodPost, "/devices/:id/merge", h.merge, auth.InventoryEdit)
}

// duplicates groups devices sharing a serial number, a management address, a
// hostname or an interface MAC. Strong keys come first.
func (h *Handler) duplicates(c *gin.Context) {
	rows, err := h.conn.Query(c.Request.Context(), `
		WITH keys AS (
		  SELECT 'serial_number' AS reason, lower(serial_number) AS value, id FROM devices WHERE coalesce(serial_number, '') <> ''
		  UNION ALL SELECT 'interface_mac', di.mac::text, di.device_id FROM device_interfaces di WHERE di.mac IS NOT NULL
		  UNION ALL SELECT 'management_ip', host(management_ip), id FROM devices WHERE management_ip IS NOT NULL
		  UNION ALL SELECT 'hostname', lower(hostname), id FROM devices WHERE hostname <> ''
		), groups AS (
		  SELECT reason, value, array_agg(DISTINCT id) AS ids FROM keys GROUP BY reason, value HAVING count(DISTINCT id) > 1
		)
		SELECT g.reason, g.value, d.id::text, d.name, d.device_type, d.vendor, d.source_system, host(d.management_ip),
		       (SELECT count(*) FROM device_interfaces i WHERE i.device_id = d.id), (SELECT count(*) FROM ip_addresses a WHERE a.device_id = d.id)
		FROM groups g JOIN devices d ON d.id = ANY(g.ids)
		ORDER BY array_position(ARRAY['serial_number','interface_mac','management_ip','hostname'], g.reason), g.value, d.name
		LIMIT 500`)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	defer rows.Close()
	out := []Match{}
	index := map[[2]string]int{}
	for rows.Next() {
		var reason, value string
		var d Device
		if err := rows.Scan(&reason, &value, &d.ID, &d.Name, &d.DeviceType, &d.Vendor, &d.SourceSystem, &d.ManagementIP, &d.Interfaces, &d.Addresses); err != nil {
			httpx.Fail(c, err)
			return
		}
		k := [2]string{reason, value}
		i, ok := index[k]
		if !ok {
			index[k] = len(out)
			out = append(out, Match{Reason: reason, Value: value})
			i = len(out) - 1
		}
		out[i].Devices = append(out[i].Devices, d)
	}
	httpx.OK(c, http.StatusOK, out)
}

// ChangeFields lists what a change request may set on a device.
var ChangeFields = []db.Field{
	{Name: "ownerId", Column: "owner_id", Cast: "uuid", Nullable: true},
	{Name: "businessServiceId", Column: "business_service_id", Cast: "uuid", Nullable: true},
	{Name: "notes", Column: "notes"},
}

// ApplyChange applies an approved change request.
func ApplyChange(ctx context.Context, tx pgx.Tx, id, actor string, changes map[string]string) (map[string]string, map[string]string, error) {
	return db.ApplyFields(ctx, tx, "devices", id, actor, "", ChangeFields, changes)
}

// mergeStatements move everything from device $1 to device $2. Interfaces with the
// same name on the survivor keep the survivor's; the rest move. $3 is the actor.
var mergeStatements = []string{
	`UPDATE device_interfaces SET device_id = $2::uuid WHERE device_id = $1::uuid
	   AND NOT EXISTS (SELECT 1 FROM device_interfaces k WHERE k.device_id = $2::uuid AND k.name = device_interfaces.name
	                   AND k.vdom_id IS NOT DISTINCT FROM device_interfaces.vdom_id)`,
	`UPDATE ip_addresses SET interface_id = (SELECT k.id FROM device_interfaces k JOIN device_interfaces o ON o.name = k.name
	   AND o.vdom_id IS NOT DISTINCT FROM k.vdom_id WHERE o.id = ip_addresses.interface_id AND k.device_id = $2::uuid)
	 WHERE interface_id IN (SELECT id FROM device_interfaces WHERE device_id = $1::uuid)`,
	`UPDATE ip_addresses SET device_id = $2::uuid WHERE device_id = $1::uuid`,
	`UPDATE vdoms SET device_id = $2::uuid WHERE device_id = $1::uuid
	   AND NOT EXISTS (SELECT 1 FROM vdoms k WHERE k.device_id = $2::uuid AND k.name = vdoms.name)`,
	`UPDATE dhcp_scopes SET server_device_id = $2::uuid WHERE server_device_id = $1::uuid`,
	`UPDATE network_routes SET device_id = $2::uuid WHERE device_id = $1::uuid`,
}

const mergeFields = `UPDATE devices k SET
	   hostname = CASE WHEN k.hostname = '' THEN o.hostname ELSE k.hostname END,
	   management_ip = coalesce(k.management_ip, o.management_ip),
	   vendor = CASE WHEN k.vendor = '' THEN o.vendor ELSE k.vendor END,
	   model = CASE WHEN k.model = '' THEN o.model ELSE k.model END,
	   serial_number = coalesce(k.serial_number, o.serial_number),
	   os_version = CASE WHEN k.os_version = '' THEN o.os_version ELSE k.os_version END,
	   os_family = CASE WHEN k.os_family = 'unknown' THEN o.os_family ELSE k.os_family END,
	   site_id = coalesce(k.site_id, o.site_id), owner_id = coalesce(k.owner_id, o.owner_id),
	   business_service_id = coalesce(k.business_service_id, o.business_service_id),
	   first_discovered = least(k.first_discovered, o.first_discovered),
	   last_discovered = greatest(k.last_discovered, o.last_discovered), updated_by = $3::uuid
	 FROM devices o WHERE k.id = $2::uuid AND o.id = $1::uuid`

func (h *Handler) merge(c *gin.Context) {
	id, ok := httpx.PathID(c)
	if !ok {
		return
	}
	var req struct {
		IntoID string `json:"intoId"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Fail(c, httpx.ErrBadRequest)
		return
	}
	if !httpx.IsUUID(req.IntoID) || req.IntoID == id {
		httpx.Fail(c, httpx.Validation("intoId", "Choose a different device to keep."))
		return
	}
	ctx, actor := c.Request.Context(), auth.Current(c).UserID
	err := db.InTx(ctx, h.conn, func(tx pgx.Tx) error { return mergeDevice(ctx, tx, actor, id, req.IntoID) })
	if db.ErrorCode(err) == db.CodeForeignKey {
		err = httpx.NewError(http.StatusConflict, "DEVICE_IN_USE", "Another record still points at this device. Unlink it first.")
	}
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func mergeDevice(ctx context.Context, tx pgx.Tx, actor, id, into string) error {
	var old map[string]any
	err := tx.QueryRow(ctx, `SELECT to_jsonb(d) FROM devices d WHERE id = $1::uuid FOR UPDATE`, id).Scan(&old)
	if errors.Is(err, pgx.ErrNoRows) {
		return httpx.ErrNotFound
	}
	if err != nil {
		return err
	}
	var keep string
	if err := tx.QueryRow(ctx, `SELECT id::text FROM devices WHERE id = $1::uuid FOR UPDATE`, into).Scan(&keep); errors.Is(err, pgx.ErrNoRows) {
		return httpx.Validation("intoId", "That device doesn't exist.")
	} else if err != nil {
		return err
	}
	for _, s := range mergeStatements {
		if _, err := tx.Exec(ctx, s, id, into); err != nil {
			return err
		}
	}
	if _, err := tx.Exec(ctx, mergeFields, id, into, actor); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM devices WHERE id = $1::uuid`, id); err != nil {
		return err
	}
	return audit.Write(ctx, tx, audit.Entry{ActorID: actor, Action: "device.merge", ObjectType: "device", ObjectID: into,
		Old: old, New: map[string]string{"mergedInto": into, "removed": id}})
}
