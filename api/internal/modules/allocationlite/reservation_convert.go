package allocationlite

import (
	"net/http"
	"net/netip"

	"dfipam/api/internal/audit"
	"dfipam/api/internal/ipam"
	"dfipam/api/internal/platform/db"
	"dfipam/api/internal/platform/httpx"
	"dfipam/api/internal/validate"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
)

type convertInput struct {
	IP    string `json:"ip"`
	Type  string `json:"ipType"`
	Name  string `json:"name"`
	Owner string `json:"owner"`
	addr  netip.Addr
}

func (in *convertInput) validate() error {
	var v validate.V
	in.addr = v.IPv4("ip", &in.IP, true)
	if in.Type == "" {
		in.Type = "other"
	}
	v.OneOf("ipType", in.Type, ipTypes, "type")
	v.Text("name", &in.Name, 120, true, "Assigned to")
	v.Text("owner", &in.Owner, 120, false, "Owner")
	return v.Err()
}

// convertReservation turns one held address into a register entry. The rest of
// the range stays held, split around the converted address.
func (h *Handler) convertReservation(c *gin.Context) {
	id, ok := httpx.PathID(c)
	if !ok {
		return
	}
	var in convertInput
	if !httpx.Bind(c, &in) {
		return
	}
	if err := in.validate(); err != nil {
		httpx.Fail(c, err)
		return
	}
	var ipID string
	err := db.InTx(c, h.conn, func(tx pgx.Tx) error {
		var err error
		ipID, err = convertHold(c, tx, id, &in)
		return err
	})
	if err != nil {
		httpx.Fail(c, friendly(err))
		return
	}
	httpx.OK(c, http.StatusOK, gin.H{"ipAddressId": ipID})
}

func convertHold(c *gin.Context, tx pgx.Tx, id string, in *convertInput) (string, error) {
	r, err := lockHold(c, tx, id)
	if err != nil {
		return "", err
	}
	if err := checkConvertible(c, tx, r, in); err != nil {
		return "", err
	}
	if in.Owner == "" {
		in.Owner = r.ownerName
	}
	var ipID string
	if err := tx.QueryRow(c, `INSERT INTO ip_addresses (segment_id, ip, status, ip_type, environment_id, assigned_name, owner, reservation_id, created_by, updated_by)
		SELECT s.id, $2::inet, 'assigned', $3, s.default_environment_id, $4, $5, $6::uuid, $7::uuid, $7::uuid FROM segments s WHERE s.id = $1::uuid
		RETURNING id::text`, r.segmentID, in.IP, in.Type, in.Name, in.Owner, id, actor(c)).Scan(&ipID); err != nil {
		return "", err
	}
	if _, err := tx.Exec(c, "UPDATE reservations SET status = 'converted', updated_by = $2::uuid WHERE id = $1::uuid", id, actor(c)); err != nil {
		return "", err
	}
	if err := keepRest(c, tx, id, in.IP); err != nil {
		return "", err
	}
	if err := auditChange(c, tx, "convert", "reservation", id, rawJSON(r.old), map[string]string{"ip": in.IP, "ipAddressId": ipID}); err != nil {
		return "", err
	}
	return ipID, auditChange(c, tx, "create", "ip_address", ipID, nil, in)
}

// checkConvertible applies the register's rules to the address, apart from the
// hold being converted: it must be inside the range, not the network or
// broadcast address, outside the DHCP pool, and the gateway only as network.
func checkConvertible(c *gin.Context, tx pgx.Tx, r heldRange, in *convertInput) error {
	first, last := netip.MustParseAddr(r.start), netip.MustParseAddr(r.end)
	if in.addr.Less(first) || last.Less(in.addr) {
		return httpx.Validation("ip", "Choose an address inside "+r.start+" – "+r.end+".")
	}
	st, err := ipam.Check(c, tx, r.segmentID, in.addr)
	if err != nil {
		return err
	}
	st.Reserved = false // held by the reservation being converted
	if issue := ipam.RegisterIssue(st, in.Type); issue != "" {
		return httpx.Validation("ip", issue)
	}
	return nil
}

// keepRest keeps the addresses either side held, as new reservations with the
// same terms. A reminder already sent stays sent, so the expiry isn't announced
// twice. Each new reservation is audited.
func keepRest(c *gin.Context, tx pgx.Tx, id, ip string) error {
	rows, err := tx.Query(c, `INSERT INTO reservations (segment_id, start_ip, end_ip, reason, owner_id, owner_name, expires_at, reminder_sent_at, created_by, updated_by)
		SELECT segment_id, a, b, reason, owner_id, owner_name, expires_at, reminder_sent_at, $3::uuid, $3::uuid FROM reservations CROSS JOIN LATERAL
		(VALUES (start_ip, $2::inet - 1), ($2::inet + 1, end_ip)) AS parts(a, b)
		WHERE id = $1::uuid AND a <= b
		RETURNING id::text, host(start_ip), host(end_ip)`, id, ip, actor(c))
	if err != nil {
		return err
	}
	var entries []audit.Entry
	for rows.Next() {
		var newID, start, end string
		if err := rows.Scan(&newID, &start, &end); err != nil {
			rows.Close()
			return err
		}
		entries = append(entries, audit.Entry{ActorID: actor(c), Action: "create", ObjectType: "reservation", ObjectID: newID,
			New: map[string]string{"startIp": start, "endIp": end, "splitFrom": id}})
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	return audit.WriteMany(c, tx, entries)
}
