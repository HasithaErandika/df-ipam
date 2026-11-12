package inventory

import (
	"net/http"

	"dfipam/api/internal/platform/db"
	"dfipam/api/internal/platform/httpx"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
)

const ipConflict = "That IP is already in the register."

// createIP registers an address. An entry with no environment takes the segment's
// default one. Discovery never calls this: only a person registers addresses.
func (h *Handler) createIP(c *gin.Context) {
	var in ipInput
	if !httpx.Bind(c, &in) {
		return
	}
	if err := in.validate(true); err != nil {
		httpx.Fail(c, err)
		return
	}
	var id string
	err := db.InTx(c, h.conn, func(tx pgx.Tx) error {
		if err := tx.QueryRow(c, `INSERT INTO ip_addresses (segment_id, ip, status, ip_type, environment_id, device_id, assigned_name,
			expected_mac, owner, department, monitor, critical, check_port, notes, created_by, updated_by, platform, services)
			SELECT s.id, $2::inet, $3, $4, COALESCE(NULLIF($5, '')::uuid, s.default_environment_id), NULLIF($6, '')::uuid, $7,
				NULLIF($8, '')::macaddr, $9, $10, $11, $12, $13, $14, $15::uuid, $15::uuid, $16, $17
			FROM segments s WHERE s.id = $1::uuid RETURNING id::text`,
			in.SegmentID, in.IP, in.Status, in.Type, in.EnvironmentID, in.DeviceID, in.Name, in.MAC, in.Owner, in.Department,
			in.Monitor, in.Critical, in.Port, in.Notes, actor(c), in.Platform, in.Services).Scan(&id); err != nil {
			return err
		}
		return record(c, tx, "create", "ip_address", id, nil, in)
	})
	if err != nil {
		httpx.Fail(c, friendlyIP(err))
		return
	}
	h.recheck(c)
	httpx.OK(c, http.StatusCreated, gin.H{"id": id})
}

// updateIP changes what an entry is for. The address and segment stay fixed: to
// move an entry, release it and register the new address.
func (h *Handler) updateIP(c *gin.Context) {
	id, ok := httpx.PathID(c)
	if !ok {
		return
	}
	var in ipInput
	if !httpx.Bind(c, &in) {
		return
	}
	if err := in.validate(false); err != nil {
		httpx.Fail(c, err)
		return
	}
	err := db.InTx(c, h.conn, func(tx pgx.Tx) error {
		old, err := lockOld(c, tx, "ip_addresses", id)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(c, `UPDATE ip_addresses SET status = $2, ip_type = $3, environment_id = NULLIF($4, '')::uuid,
			device_id = NULLIF($5, '')::uuid, assigned_name = $6, expected_mac = NULLIF($7, '')::macaddr, owner = $8, department = $9,
			monitor = $10, critical = $11, check_port = $12, notes = $13, updated_by = $14::uuid, platform = $15, services = $16,
			health = CASE WHEN $10 THEN health ELSE 'unknown' END, server_check_failures = CASE WHEN $10 THEN server_check_failures ELSE 0 END
			WHERE id = $1::uuid`, id, in.Status, in.Type, in.EnvironmentID, in.DeviceID, in.Name, in.MAC, in.Owner, in.Department,
			in.Monitor, in.Critical, in.Port, in.Notes, actor(c), in.Platform, in.Services); err != nil {
			return err
		}
		return record(c, tx, "update", "ip_address", id, old, in)
	})
	if err != nil {
		httpx.Fail(c, friendlyIP(err))
		return
	}
	h.recheck(c)
	c.Status(http.StatusNoContent)
}

// releaseIP sets an address back to unassigned by removing its register entry.
// The audit row keeps everything it held.
func (h *Handler) releaseIP(c *gin.Context) {
	id, ok := httpx.PathID(c)
	if !ok {
		return
	}
	err := db.InTx(c, h.conn, func(tx pgx.Tx) error {
		old, err := lockOld(c, tx, "ip_addresses", id)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(c, "DELETE FROM ip_addresses WHERE id = $1::uuid", id); err != nil {
			return err
		}
		return record(c, tx, "release", "ip_address", id, old, nil)
	})
	if err != nil {
		httpx.Fail(c, friendlyIP(err))
		return
	}
	h.recheck(c)
	c.Status(http.StatusNoContent)
}

func friendlyIP(err error) error {
	if db.ErrorCode(err) == db.CodeCheck {
		return httpx.Validation("ip", "The IP must be inside the selected segment.")
	}
	return db.Friendly(err, ipConflict)
}
