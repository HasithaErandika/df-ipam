package allocationlite

import (
	"encoding/json"
	"net/http"
	"strings"

	"dfipam/api/internal/auth"
	"dfipam/api/internal/netcalc"
	"dfipam/api/internal/platform/db"
	"dfipam/api/internal/platform/httpx"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
	"net/netip"
)

func (h *Handler) listRequests(c *gin.Context) {
	p := auth.Current(c)
	status := c.DefaultQuery("status", "all")
	if status != "all" && status != "pending" && status != "approved" && status != "rejected" && status != "cancelled" {
		httpx.Fail(c, httpx.Validation("status", "Choose a request status."))
		return
	}
	rows, err := h.conn.Query(c, `SELECT r.id::text,r.requester_id::text,u.email,r.segment_id::text,s.name,r.ip_type,r.name,r.purpose,r.needed_by,r.requested_ip::text,r.status,r.decision_reason,r.assigned_ip_id::text,r.created_at::text FROM ip_requests r JOIN users u ON u.id=r.requester_id JOIN segments s ON s.id=r.segment_id WHERE ($1 OR r.requester_id=$2::uuid) AND ($3='all' OR r.status=$3) ORDER BY r.created_at DESC LIMIT 200`, p.Has(auth.RequestsApprove), p.UserID, status)
	if err != nil {
		fail(c, err)
		return
	}
	defer rows.Close()
	out := []gin.H{}
	for rows.Next() {
		var id, requester, email, segmentID, segment, name, kind, purpose, status, reason, created string
		var needed, requested, assigned *string
		if err := rows.Scan(&id, &requester, &email, &segmentID, &segment, &kind, &name, &purpose, &needed, &requested, &status, &reason, &assigned, &created); err != nil {
			fail(c, err)
			return
		}
		out = append(out, gin.H{"id": id, "requesterId": requester, "requesterEmail": email, "segmentId": segmentID, "segmentName": segment, "ipType": kind, "name": name, "purpose": purpose, "neededBy": needed, "requestedIp": requested, "status": status, "decisionReason": reason, "assignedIpId": assigned, "createdAt": created})
	}
	if rows.Err() != nil {
		fail(c, rows.Err())
		return
	}
	httpx.OK(c, 200, out)
}

type requestInput struct {
	SegmentID     string `json:"segmentId"`
	Type          string `json:"ipType"`
	EnvironmentID string `json:"environmentId"`
	Name          string `json:"name"`
	Purpose       string `json:"purpose"`
	NeededBy      string `json:"neededBy"`
	RequestedIP   string `json:"requestedIp"`
}

func (h *Handler) createRequest(c *gin.Context) {
	var in requestInput
	if c.ShouldBindJSON(&in) != nil {
		httpx.Fail(c, httpx.ErrBadRequest)
		return
	}
	in.Name = clean(in.Name)
	if in.SegmentID == "" || in.Name == "" {
		httpx.Fail(c, httpx.Validation("name", "Choose a segment and enter a device name."))
		return
	}
	if in.Type == "" {
		in.Type = "other"
	}
	uid := auth.Current(c).UserID
	var key string
	err := db.InTx(c, h.conn, func(tx pgx.Tx) error {
		if err := tx.QueryRow(c, `INSERT INTO ip_requests(requester_id,segment_id,ip_type,environment_id,name,purpose,needed_by,requested_ip) VALUES($1::uuid,$2::uuid,$3,NULLIF($4,'')::uuid,$5,$6,NULLIF($7,'')::date,NULLIF($8,'')::inet) RETURNING id::text`, uid, in.SegmentID, in.Type, in.EnvironmentID, in.Name, in.Purpose, in.NeededBy, in.RequestedIP).Scan(&key); err != nil {
			return err
		}
		if err := auditChange(c, tx, "create", "ip_request", key, nil, in); err != nil {
			return err
		}
		return notifyAdmins(c, tx, "request", "A new IP request needs review: "+in.Name, "/requests")
	})
	if err != nil {
		fail(c, err)
		return
	}
	httpx.OK(c, http.StatusCreated, gin.H{"id": key, "status": "pending"})
}

func (h *Handler) decideRequest(c *gin.Context) {
	id, ok := httpx.PathID(c)
	if !ok {
		return
	}
	var in struct {
		Status string `json:"status"`
		IP     string `json:"ip"`
		Reason string `json:"reason"`
	}
	if c.ShouldBindJSON(&in) != nil {
		httpx.Fail(c, httpx.ErrBadRequest)
		return
	}
	if in.Status != "approved" && in.Status != "rejected" {
		httpx.Fail(c, httpx.Validation("status", "Choose approved or rejected."))
		return
	}
	in.Reason = strings.TrimSpace(in.Reason)
	if in.Status == "rejected" && in.Reason == "" {
		httpx.Fail(c, httpx.Validation("reason", "Enter a reason for rejecting the request."))
		return
	}
	uid := auth.Current(c).UserID
	var resultIP *string
	err := db.InTx(c, h.conn, func(tx pgx.Tx) error {
		var old []byte
		var requester, segmentID, name, ipType, status string
		var requestedIP, environmentID *string
		if err := tx.QueryRow(c, `SELECT to_jsonb(r),r.requester_id::text,r.segment_id::text,r.name,r.ip_type,r.requested_ip::text,r.environment_id::text,r.status FROM ip_requests r WHERE id=$1::uuid FOR UPDATE`, id).Scan(&old, &requester, &segmentID, &name, &ipType, &requestedIP, &environmentID, &status); err != nil {
			return err
		}
		if status != "pending" {
			return httpx.NewError(409, "REQUEST_CLOSED", "Only a pending request can be decided.")
		}
		var assignedID string
		if in.Status == "approved" {
			if _, err := tx.Exec(c, "SELECT id FROM segments WHERE id=$1::uuid FOR UPDATE", segmentID); err != nil {
				return err
			}
			candidate := strings.TrimSpace(in.IP)
			if candidate == "" && requestedIP != nil {
				candidate = *requestedIP
			}
			if candidate == "" {
				var pickErr error
				candidate, pickErr = available(c, tx, segmentID)
				if pickErr != nil {
					return pickErr
				}
			}
			ok, err := isAvailable(c, tx, segmentID, candidate)
			if err != nil {
				return err
			}
			if !ok {
				return httpx.Validation("ip", "Choose an available IP outside the DHCP pool.")
			}
			var email string
			if err := tx.QueryRow(c, "SELECT email FROM users WHERE id=$1::uuid", requester).Scan(&email); err != nil {
				return err
			}
			if err := tx.QueryRow(c, `INSERT INTO ip_addresses(segment_id,ip,status,ip_type,environment_id,assigned_name,owner,monitor,critical,created_by,updated_by)
				SELECT $1::uuid,$2::inet,'assigned',$3,COALESCE(NULLIF($4,'')::uuid,s.default_environment_id),$5,$6,COALESCE(env.default_monitor,false),COALESCE(env.default_critical,false),$7::uuid,$7::uuid
				FROM segments s LEFT JOIN environments env ON env.id=COALESCE(NULLIF($4,'')::uuid,s.default_environment_id) WHERE s.id=$1::uuid RETURNING id::text`, segmentID, candidate, ipType, nullable(environmentID), name, email, uid).Scan(&assignedID); err != nil {
				return err
			}
			resultIP = &candidate
			if err := auditChange(c, tx, "assign", "ip_address", assignedID, nil, map[string]string{"ip": candidate, "name": name}); err != nil {
				return err
			}
		}
		if _, err := tx.Exec(c, `UPDATE ip_requests SET status=$2,decided_by=$3::uuid,decided_at=now(),decision_reason=$4,assigned_ip_id=NULLIF($5,'')::uuid WHERE id=$1::uuid AND status='pending'`, id, in.Status, uid, in.Reason, assignedID); err != nil {
			return err
		}
		if err := auditChange(c, tx, in.Status, "ip_request", id, json.RawMessage(old), map[string]any{"status": in.Status, "reason": in.Reason, "assignedIp": resultIP}); err != nil {
			return err
		}
		message := "Your IP request was rejected: " + in.Reason
		if in.Status == "approved" {
			message = "Your IP request was approved. Assigned IP: " + *resultIP
		}
		return notifyUser(c, tx, requester, "request_decision", message, "/requests", true)
	})
	if err != nil {
		fail(c, err)
		return
	}
	httpx.OK(c, 200, gin.H{"status": in.Status, "ip": resultIP})
}

func (h *Handler) cancelRequest(c *gin.Context) {
	id, ok := httpx.PathID(c)
	if !ok {
		return
	}
	uid := auth.Current(c).UserID
	err := db.InTx(c, h.conn, func(tx pgx.Tx) error {
		var old []byte
		var owner, status string
		if err := tx.QueryRow(c, "SELECT to_jsonb(r),requester_id::text,status FROM ip_requests r WHERE id=$1::uuid FOR UPDATE", id).Scan(&old, &owner, &status); err != nil {
			return err
		}
		if owner != uid {
			return httpx.ErrNotFound
		}
		if status != "pending" {
			return httpx.NewError(409, "REQUEST_CLOSED", "Only a pending request can be cancelled.")
		}
		if _, err := tx.Exec(c, "UPDATE ip_requests SET status='cancelled' WHERE id=$1::uuid", id); err != nil {
			return err
		}
		return auditChange(c, tx, "cancel", "ip_request", id, json.RawMessage(old), map[string]string{"status": "cancelled"})
	})
	if err != nil {
		fail(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func isAvailable(c *gin.Context, tx pgx.Tx, segmentID, ip string) (bool, error) {
	parsed, parseErr := netip.ParseAddr(ip)
	if parseErr != nil || !parsed.Is4() {
		return false, nil
	}
	var network string
	if err := tx.QueryRow(c, "SELECT network::text FROM segments WHERE id=$1::uuid", segmentID).Scan(&network); err != nil {
		return false, err
	}
	prefix, err := netip.ParsePrefix(network)
	if err != nil {
		return false, err
	}
	usable := netcalc.UsableRange(prefix)
	if parsed.Less(usable.First) || usable.Last.Less(parsed) {
		return false, nil
	}
	var ok bool
	err = tx.QueryRow(c, `SELECT EXISTS(SELECT 1 FROM segments s WHERE s.id=$1::uuid AND $2::inet << s.network AND (s.dhcp_start IS NULL OR $2::inet<s.dhcp_start OR $2::inet>s.dhcp_end)
		AND NOT EXISTS(SELECT 1 FROM ip_addresses i WHERE i.ip=$2::inet) AND NOT EXISTS(SELECT 1 FROM reservations r WHERE r.segment_id=s.id AND r.status='active' AND $2::inet BETWEEN r.start_ip AND r.end_ip)
		AND NOT EXISTS(SELECT 1 FROM sightings v WHERE v.segment_id=s.id AND v.ip=$2::inet AND v.last_seen>now()-interval '24 hours'))`, segmentID, ip).Scan(&ok)
	return ok, err
}

var _ = httpx.ErrBadRequest
