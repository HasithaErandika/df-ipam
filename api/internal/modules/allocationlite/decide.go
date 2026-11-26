package allocationlite

import (
	"net/http"
	"net/netip"

	"dfipam/api/internal/ipam"
	"dfipam/api/internal/notify"
	"dfipam/api/internal/platform/db"
	"dfipam/api/internal/platform/httpx"
	"dfipam/api/internal/validate"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
)

type decisionInput struct {
	Status string `json:"status"`
	IP     string `json:"ip"`
	Reason string `json:"reason"`
}

func (in *decisionInput) validate() error {
	var v validate.V
	v.OneOf("status", in.Status, []string{"approved", "rejected"}, "decision")
	v.Text("reason", &in.Reason, 500, in.Status == "rejected", "Reason")
	if in.Status == "approved" {
		v.IPv4("ip", &in.IP, false)
	}
	return v.Err()
}

type pendingRequest struct {
	old                               []byte
	requester, segmentID, name, ipTyp string
	requestedIP, environmentID        *string
}

// decideRequest approves with one address, creating its register entry in the
// same transaction, or rejects with a reason. One step, no chains.
func (h *Handler) decideRequest(c *gin.Context) {
	id, ok := httpx.PathID(c)
	if !ok {
		return
	}
	var in decisionInput
	if !httpx.Bind(c, &in) {
		return
	}
	if err := in.validate(); err != nil {
		httpx.Fail(c, err)
		return
	}
	var assigned string
	err := db.InTx(c, h.conn, func(tx pgx.Tx) error {
		var err error
		assigned, err = h.decide(c, tx, id, in)
		return err
	})
	if err != nil {
		httpx.Fail(c, friendly(err))
		return
	}
	httpx.OK(c, http.StatusOK, gin.H{"status": in.Status, "ip": assigned})
}

// decide records the decision on a pending request, assigning the address on
// approval, and tells the requester. It returns the assigned IP.
func (h *Handler) decide(c *gin.Context, tx pgx.Tx, id string, in decisionInput) (string, error) {
	var assigned string
	var r pendingRequest
	var status string
	if err := tx.QueryRow(c, `SELECT to_jsonb(r), requester_id::text, segment_id::text, name, ip_type, host(requested_ip), environment_id::text, status
		FROM ip_requests r WHERE id = $1::uuid FOR UPDATE`, id).Scan(&r.old, &r.requester, &r.segmentID, &r.name, &r.ipTyp,
		&r.requestedIP, &r.environmentID, &status); err != nil {
		return "", err
	}
	if status != "pending" {
		return "", errClosed
	}
	ipID := ""
	if in.Status == "approved" {
		var err error
		if assigned, ipID, err = h.assign(c, tx, r, in.IP); err != nil {
			return "", err
		}
	}
	if _, err := tx.Exec(c, `UPDATE ip_requests SET status = $2, decided_by = $3::uuid, decided_at = now(), decision_reason = $4,
		assigned_ip_id = NULLIF($5, '')::uuid WHERE id = $1::uuid`, id, in.Status, actor(c), in.Reason, ipID); err != nil {
		return "", err
	}
	if err := auditChange(c, tx, in.Status, "ip_request", id, rawJSON(r.old), map[string]string{"status": in.Status, "reason": in.Reason, "ip": assigned}); err != nil {
		return "", err
	}
	msg := "Your IP request for " + r.name + " was rejected: " + in.Reason
	if in.Status == "approved" {
		msg = "Your IP request for " + r.name + " was approved. Use IP " + assigned + "."
	}
	return assigned, notify.User(c, tx, r.requester, "request_decision", msg, "/requests", notify.Now)
}

// assign picks the address (chosen, requested, or the Available candidate), checks
// nothing else holds it, and registers it with the environment's defaults.
func (h *Handler) assign(c *gin.Context, tx pgx.Tx, r pendingRequest, chosen string) (string, string, error) {
	if _, err := tx.Exec(c, "SELECT 1 FROM segments WHERE id = $1::uuid FOR UPDATE", r.segmentID); err != nil {
		return "", "", err
	}
	ip := chosen
	if ip == "" && r.requestedIP != nil {
		ip = *r.requestedIP
	}
	if ip == "" {
		var err error
		if ip, err = ipam.Candidate(c, tx, r.segmentID); err != nil {
			return "", "", err
		}
	}
	addr, err := netip.ParseAddr(ip)
	if err != nil {
		return "", "", httpx.Validation("ip", "Enter a valid IPv4 address.")
	}
	st, err := ipam.Check(c, tx, r.segmentID, addr)
	if err != nil {
		return "", "", err
	}
	if issue := unavailable(st); issue != "" {
		return "", "", httpx.Validation("ip", issue)
	}
	var email, ipID string
	if err := tx.QueryRow(c, "SELECT email FROM users WHERE id = $1::uuid", r.requester).Scan(&email); err != nil {
		return "", "", err
	}
	env := ""
	if r.environmentID != nil {
		env = *r.environmentID
	}
	if err := tx.QueryRow(c, `INSERT INTO ip_addresses (segment_id, ip, status, ip_type, environment_id, assigned_name, owner, monitor, critical, created_by, updated_by)
		SELECT s.id, $2::inet, 'assigned', $3, e.id, $5, $6, COALESCE(e.default_monitor, false), COALESCE(e.default_critical, false), $7::uuid, $7::uuid
		FROM segments s LEFT JOIN environments e ON e.id = COALESCE(NULLIF($4, '')::uuid, s.default_environment_id)
		WHERE s.id = $1::uuid RETURNING id::text`, r.segmentID, ip, r.ipTyp, env, r.name, email, actor(c)).Scan(&ipID); err != nil {
		return "", "", err
	}
	return ip, ipID, auditChange(c, tx, "create", "ip_address", ipID, nil, map[string]string{"ip": ip, "name": r.name, "source": "ip_request"})
}

// unavailable explains why an address can't be assigned, or returns "".
func unavailable(s ipam.Status) string {
	switch {
	case !s.InSegment:
		return "This IP is not inside the request's segment."
	case !s.Usable:
		return "This is the network or broadcast address."
	case s.IsGateway:
		return "This is the segment's gateway."
	case s.InDHCPPool:
		return "This IP is inside the DHCP pool. Choose a static address outside it."
	case s.Registered:
		return "This IP is already in the register."
	case s.Reserved:
		return "This IP is held by a reservation."
	case s.SeenRecently:
		return "This IP is in use on the network (seen in the last 30 days outside the DHCP pool, or 24 hours inside it). Choose another."
	}
	return ""
}
