package allocationlite

import (
	"encoding/json"
	"net/http"
	"net/netip"
	"strings"
	"time"

	"dfipam/api/internal/auth"
	"dfipam/api/internal/platform/db"
	"dfipam/api/internal/platform/httpx"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
)

func (h *Handler) listReservations(c *gin.Context) {
	rows, err := h.conn.Query(c, `SELECT r.id::text,r.segment_id::text,s.name,r.start_ip::text,r.end_ip::text,r.reason,r.owner_id::text,r.owner_name,r.expires_at,r.status,r.created_at FROM reservations r JOIN segments s ON s.id=r.segment_id WHERE ($1='all' OR r.status=$1) ORDER BY r.expires_at NULLS LAST,r.created_at DESC LIMIT 200`, c.DefaultQuery("status", "active"))
	if err != nil {
		fail(c, err)
		return
	}
	defer rows.Close()
	out := []gin.H{}
	for rows.Next() {
		var id, sid, sname, start, end, reason, status string
		var ownerID *string
		var owner string
		var expires, created *time.Time
		if err := rows.Scan(&id, &sid, &sname, &start, &end, &reason, &ownerID, &owner, &expires, &status, &created); err != nil {
			fail(c, err)
			return
		}
		out = append(out, gin.H{"id": id, "segmentId": sid, "segmentName": sname, "startIp": start, "endIp": end, "reason": reason, "ownerId": ownerID, "ownerName": owner, "expiresAt": expires, "status": status, "createdAt": created})
	}
	if rows.Err() != nil {
		fail(c, rows.Err())
		return
	}
	httpx.OK(c, 200, out)
}

type reserveInput struct {
	SegmentID string `json:"segmentId"`
	StartIP   string `json:"startIp"`
	EndIP     string `json:"endIp"`
	Reason    string `json:"reason"`
	OwnerID   string `json:"ownerId"`
	OwnerName string `json:"ownerName"`
	ExpiresAt string `json:"expiresAt"`
}

func (h *Handler) createReservation(c *gin.Context) {
	var in reserveInput
	if c.ShouldBindJSON(&in) != nil {
		httpx.Fail(c, httpx.ErrBadRequest)
		return
	}
	in.Reason = clean(in.Reason)
	if in.EndIP == "" {
		in.EndIP = in.StartIP
	}
	if in.SegmentID == "" || in.StartIP == "" || in.Reason == "" {
		httpx.Fail(c, httpx.Validation("reason", "Choose a segment, address range and reason."))
		return
	}
	uid := auth.Current(c).UserID
	var id string
	err := db.InTx(c, h.conn, func(tx pgx.Tx) error {
		var locked string
		if err := tx.QueryRow(c, "SELECT id::text FROM segments WHERE id=$1::uuid FOR UPDATE", in.SegmentID).Scan(&locked); err != nil {
			return err
		}
		var conflict bool
		if err := tx.QueryRow(c, `SELECT EXISTS(SELECT 1 FROM reservations r WHERE r.segment_id=$1::uuid AND r.status='active' AND r.start_ip<=$3::inet AND r.end_ip>=$2::inet)
			OR EXISTS(SELECT 1 FROM ip_addresses i WHERE i.ip BETWEEN $2::inet AND $3::inet)
			OR EXISTS(SELECT 1 FROM sightings s WHERE s.segment_id=$1::uuid AND s.ip BETWEEN $2::inet AND $3::inet AND s.last_seen>now()-interval '24 hours')`, in.SegmentID, in.StartIP, in.EndIP).Scan(&conflict); err != nil {
			return err
		}
		if conflict {
			return httpx.NewError(409, "RANGE_IN_USE", "The selected range contains a registered, reserved or recently observed IP.")
		}
		if err := tx.QueryRow(c, `INSERT INTO reservations(segment_id,start_ip,end_ip,reason,owner_id,owner_name,expires_at,created_by,updated_by)
			VALUES($1::uuid,$2::inet,$3::inet,$4,NULLIF($5,'')::uuid,$6,NULLIF($7,'')::timestamptz,$8::uuid,$8::uuid) RETURNING id::text`, in.SegmentID, in.StartIP, in.EndIP, in.Reason, in.OwnerID, in.OwnerName, in.ExpiresAt, uid).Scan(&id); err != nil {
			return err
		}
		if err := auditChange(c, tx, "reserve", "reservation", id, nil, in); err != nil {
			return err
		}
		if in.OwnerID != "" {
			return notifyUser(c, tx, in.OwnerID, "reservation_expiry", "An IP reservation was created: "+in.Reason, "/reservations", false)
		}
		return nil
	})
	if err != nil {
		fail(c, err)
		return
	}
	httpx.OK(c, http.StatusCreated, gin.H{"id": id, "status": "active"})
}

func (h *Handler) updateReservation(c *gin.Context) {
	id, ok := httpx.PathID(c)
	if !ok {
		return
	}
	var in struct {
		Reason    string `json:"reason"`
		OwnerName string `json:"ownerName"`
		OwnerID   string `json:"ownerId"`
		ExpiresAt string `json:"expiresAt"`
	}
	if c.ShouldBindJSON(&in) != nil {
		httpx.Fail(c, httpx.ErrBadRequest)
		return
	}
	uid := auth.Current(c).UserID
	err := db.InTx(c, h.conn, func(tx pgx.Tx) error {
		var old []byte
		var owner *string
		var status string
		if err := tx.QueryRow(c, "SELECT to_jsonb(r),owner_id::text,status FROM reservations r WHERE id=$1::uuid FOR UPDATE", id).Scan(&old, &owner, &status); err != nil {
			return err
		}
		if !auth.Current(c).Has(auth.SystemManage) && (owner == nil || *owner != uid) {
			return httpx.ErrNotFound
		}
		if status != "active" {
			return httpx.NewError(409, "RESERVATION_CLOSED", "Only an active reservation can be changed.")
		}
		_, err := tx.Exec(c, `UPDATE reservations SET reason=COALESCE(NULLIF($2,''),reason),owner_id=COALESCE(NULLIF($3,'')::uuid,owner_id),owner_name=COALESCE(NULLIF($4,''),owner_name),expires_at=NULLIF($5,'')::timestamptz,updated_by=$6::uuid WHERE id=$1::uuid`, id, strings.TrimSpace(in.Reason), in.OwnerID, in.OwnerName, in.ExpiresAt, uid)
		if err != nil {
			return err
		}
		return auditChange(c, tx, "update", "reservation", id, json.RawMessage(old), in)
	})
	if err != nil {
		fail(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *Handler) releaseReservation(c *gin.Context) {
	id, ok := httpx.PathID(c)
	if !ok {
		return
	}
	uid := auth.Current(c).UserID
	err := db.InTx(c, h.conn, func(tx pgx.Tx) error {
		var old []byte
		var owner *string
		if err := tx.QueryRow(c, "SELECT to_jsonb(r),owner_id::text FROM reservations r WHERE id=$1::uuid AND status='active' FOR UPDATE", id).Scan(&old, &owner); err != nil {
			return err
		}
		if !auth.Current(c).Has(auth.SystemManage) && (owner == nil || *owner != uid) {
			return httpx.ErrNotFound
		}
		if _, err := tx.Exec(c, "UPDATE reservations SET status='released',updated_by=$2::uuid WHERE id=$1::uuid", id, uid); err != nil {
			return err
		}
		return auditChange(c, tx, "release", "reservation", id, json.RawMessage(old), map[string]string{"status": "released"})
	})
	if err != nil {
		fail(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

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
	if c.ShouldBindJSON(&in) != nil {
		httpx.Fail(c, httpx.ErrBadRequest)
		return
	}
	uid := auth.Current(c).UserID
	var assigned string
	err := db.InTx(c, h.conn, func(tx pgx.Tx) error {
		var old []byte
		var sid, start, end, reason, ownerName string
		var ownerID *string
		var expires *time.Time
		if err := tx.QueryRow(c, `SELECT to_jsonb(r),segment_id::text,start_ip::text,end_ip::text,reason,owner_id::text,owner_name,expires_at FROM reservations r WHERE id=$1::uuid AND status='active' FOR UPDATE`, id).Scan(&old, &sid, &start, &end, &reason, &ownerID, &ownerName, &expires); err != nil {
			return err
		}
		if !auth.Current(c).Has(auth.SystemManage) && (ownerID == nil || *ownerID != uid) {
			return httpx.ErrNotFound
		}
		if in.IP == "" {
			if start != end {
				return httpx.Validation("ip", "Choose an IP from the reserved range to assign.")
			}
			in.IP = start
		}
		var inside bool
		if err := tx.QueryRow(c, "SELECT $1::inet BETWEEN $2::inet AND $3::inet", in.IP, start, end).Scan(&inside); err != nil {
			return err
		}
		if !inside {
			return httpx.Validation("ip", "Choose an address inside this reservation.")
		}
		if in.Type == "" {
			in.Type = "other"
		}
		name := strings.TrimSpace(in.Name)
		if name == "" {
			name = reason
		}
		own := in.Owner
		if own == "" {
			own = ownerName
		}
		if err := tx.QueryRow(c, `INSERT INTO ip_addresses(segment_id,ip,status,ip_type,assigned_name,owner,reservation_id,created_by,updated_by)
			VALUES($1::uuid,$2::inet,'assigned',$3,$4,$5,$6::uuid,$7::uuid,$7::uuid) RETURNING id::text`, sid, in.IP, in.Type, name, own, id, uid).Scan(&assigned); err != nil {
			return err
		}
		if _, err := tx.Exec(c, "UPDATE reservations SET status='converted',updated_by=$2::uuid WHERE id=$1::uuid", id, uid); err != nil {
			return err
		}
		if _, err := tx.Exec(c, `INSERT INTO reservations(segment_id,start_ip,end_ip,reason,owner_id,owner_name,expires_at,created_by,updated_by)
			SELECT $1::uuid,$2::inet,$3::inet,$4,NULLIF($5,'')::uuid,$6,$7,$8::uuid,$8::uuid WHERE $2::inet<=$3::inet`, sid, start, previousIP(in.IP), reason, nullable(ownerID), ownerName, expires, uid); err != nil {
			return err
		}
		if _, err := tx.Exec(c, `INSERT INTO reservations(segment_id,start_ip,end_ip,reason,owner_id,owner_name,expires_at,created_by,updated_by)
			SELECT $1::uuid,$2::inet,$3::inet,$4,NULLIF($5,'')::uuid,$6,$7,$8::uuid,$8::uuid WHERE $2::inet<=$3::inet`, sid, nextIP(in.IP), end, reason, nullable(ownerID), ownerName, expires, uid); err != nil {
			return err
		}
		if err := auditChange(c, tx, "convert", "reservation", id, json.RawMessage(old), map[string]string{"ip": in.IP, "assignedIpId": assigned}); err != nil {
			return err
		}
		return auditChange(c, tx, "assign", "ip_address", assigned, nil, in)
	})
	if err != nil {
		fail(c, err)
		return
	}
	httpx.OK(c, 200, gin.H{"ipAddressId": assigned})
}

func previousIP(ip string) string { return adjustIP(ip, -1) }
func nextIP(ip string) string     { return adjustIP(ip, 1) }
func adjustIP(ip string, delta int) string {
	parsed, err := netip.ParseAddr(ip)
	if err != nil {
		return ""
	}
	if delta < 0 {
		return parsed.Prev().String()
	}
	return parsed.Next().String()
}
