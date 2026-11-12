package allocationlite

import (
	"net/http"
	"net/netip"

	"dfipam/api/internal/platform/db"
	"dfipam/api/internal/platform/httpx"
	"dfipam/api/internal/validate"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
)

var errHoldClosed = httpx.NewError(http.StatusConflict, "RESERVATION_CLOSED", "Only an active reservation can be changed. Reload to see its current state.")

type heldRange struct {
	old                   []byte
	segmentID, start, end string
	reason, ownerName     string
	ownerID               *string
	status                string
}

// lockHold loads and locks a reservation the caller may manage.
func lockHold(c *gin.Context, tx pgx.Tx, id string) (heldRange, error) {
	var r heldRange
	err := tx.QueryRow(c, `SELECT to_jsonb(r), segment_id::text, host(start_ip), host(end_ip), reason, owner_name, owner_id::text, status
		FROM reservations r WHERE id = $1::uuid FOR UPDATE`, id).Scan(&r.old, &r.segmentID, &r.start, &r.end, &r.reason, &r.ownerName,
		&r.ownerID, &r.status)
	if err != nil {
		return r, err
	}
	if !canManage(c, r.ownerID) {
		return r, httpx.ErrNotFound
	}
	if r.status != "active" {
		return r, errHoldClosed
	}
	return r, nil
}

// updateReservation changes the reason, owner name or expiry (extend, or clear it).
func (h *Handler) updateReservation(c *gin.Context) {
	id, ok := httpx.PathID(c)
	if !ok {
		return
	}
	var in struct {
		Reason    string `json:"reason"`
		OwnerName string `json:"ownerName"`
		ExpiresOn string `json:"expiresOn"`
	}
	if !httpx.Bind(c, &in) {
		return
	}
	var v validate.V
	v.Text("reason", &in.Reason, 300, true, "Reason")
	v.Text("ownerName", &in.OwnerName, 120, false, "Owner")
	v.Date("expiresOn", &in.ExpiresOn, true, "Expiry")
	if err := v.Err(); err != nil {
		httpx.Fail(c, err)
		return
	}
	err := db.InTx(c, h.conn, func(tx pgx.Tx) error {
		r, err := lockHold(c, tx, id)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(c, `UPDATE reservations SET reason = $2, owner_name = $3, expires_at = NULLIF($4, '')::date + interval '1 day',
			reminder_sent_at = NULL, updated_by = $5::uuid WHERE id = $1::uuid`, id, in.Reason, in.OwnerName, in.ExpiresOn, actor(c)); err != nil {
			return err
		}
		return auditChange(c, tx, "update", "reservation", id, rawJSON(r.old), in)
	})
	if err != nil {
		httpx.Fail(c, friendly(err))
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *Handler) releaseReservation(c *gin.Context) {
	id, ok := httpx.PathID(c)
	if !ok {
		return
	}
	err := db.InTx(c, h.conn, func(tx pgx.Tx) error {
		r, err := lockHold(c, tx, id)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(c, "UPDATE reservations SET status = 'released', updated_by = $2::uuid WHERE id = $1::uuid", id, actor(c)); err != nil {
			return err
		}
		return auditChange(c, tx, "release", "reservation", id, rawJSON(r.old), map[string]string{"status": "released"})
	})
	if err != nil {
		httpx.Fail(c, friendly(err))
		return
	}
	c.Status(http.StatusNoContent)
}

// convertReservation turns one held address into a register entry. The rest of
// the range stays held, split around the converted address.
func (h *Handler) convertReservation(c *gin.Context) {
	id, ok := httpx.PathID(c)
	if !ok {
		return
	}
	var in struct {
		IP    string `json:"ip"`
		Type  string `json:"ipType"`
		Name  string `json:"name"`
		Owner string `json:"owner"`
	}
	if !httpx.Bind(c, &in) {
		return
	}
	var v validate.V
	addr := v.IPv4("ip", &in.IP, true)
	if in.Type == "" {
		in.Type = "other"
	}
	v.OneOf("ipType", in.Type, ipTypes, "type")
	v.Text("name", &in.Name, 120, true, "Assigned to")
	v.Text("owner", &in.Owner, 120, false, "Owner")
	if err := v.Err(); err != nil {
		httpx.Fail(c, err)
		return
	}
	var ipID string
	err := db.InTx(c, h.conn, func(tx pgx.Tx) error {
		r, err := lockHold(c, tx, id)
		if err != nil {
			return err
		}
		first, last := netip.MustParseAddr(r.start), netip.MustParseAddr(r.end)
		if addr.Less(first) || last.Less(addr) {
			return httpx.Validation("ip", "Choose an address inside "+r.start+" – "+r.end+".")
		}
		if in.Owner == "" {
			in.Owner = r.ownerName
		}
		if err := tx.QueryRow(c, `INSERT INTO ip_addresses (segment_id, ip, status, ip_type, environment_id, assigned_name, owner, reservation_id, created_by, updated_by)
			SELECT s.id, $2::inet, 'assigned', $3, s.default_environment_id, $4, $5, $6::uuid, $7::uuid, $7::uuid FROM segments s WHERE s.id = $1::uuid
			RETURNING id::text`, r.segmentID, in.IP, in.Type, in.Name, in.Owner, id, actor(c)).Scan(&ipID); err != nil {
			return err
		}
		if _, err := tx.Exec(c, "UPDATE reservations SET status = 'converted', updated_by = $2::uuid WHERE id = $1::uuid", id, actor(c)); err != nil {
			return err
		}
		// Keep the addresses either side held, as new reservations with the same terms.
		if _, err := tx.Exec(c, `INSERT INTO reservations (segment_id, start_ip, end_ip, reason, owner_id, owner_name, expires_at, created_by, updated_by)
			SELECT segment_id, a, b, reason, owner_id, owner_name, expires_at, $3::uuid, $3::uuid FROM reservations CROSS JOIN LATERAL
			(VALUES (start_ip, $2::inet - 1), ($2::inet + 1, end_ip)) AS parts(a, b)
			WHERE id = $1::uuid AND a <= b`, id, in.IP, actor(c)); err != nil {
			return err
		}
		if err := auditChange(c, tx, "convert", "reservation", id, rawJSON(r.old), map[string]string{"ip": in.IP, "ipAddressId": ipID}); err != nil {
			return err
		}
		return auditChange(c, tx, "create", "ip_address", ipID, nil, in)
	})
	if err != nil {
		httpx.Fail(c, friendly(err))
		return
	}
	httpx.OK(c, http.StatusOK, gin.H{"ipAddressId": ipID})
}
