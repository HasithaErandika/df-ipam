package allocationlite

import (
	"context"
	"errors"
	"net/netip"
	"strings"

	"dfipam/api/internal/audit"
	"dfipam/api/internal/auth"
	"dfipam/api/internal/netcalc"
	"dfipam/api/internal/platform/db"
	"dfipam/api/internal/platform/httpx"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
)

func actor(c *gin.Context) string    { return auth.Current(c).UserID }
func fail(c *gin.Context, err error) { httpx.Fail(c, domainError(err)) }
func domainError(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return httpx.ErrNotFound
	}
	switch db.ErrorCode(err) {
	case db.CodeUnique, db.CodeExclusion:
		return httpx.NewError(409, "CONFLICT", "That IP is already registered or reserved.")
	case db.CodeCheck, db.CodeBadValue:
		return httpx.NewError(422, "VALIDATION_FAILED", "The requested values are invalid for this segment.")
	}
	return err
}
func auditChange(c *gin.Context, tx pgx.Tx, action, kind, id string, old, new any) error {
	return audit.Write(c, tx, audit.Entry{ActorID: actor(c), Action: action, ObjectType: kind, ObjectID: id, Old: old, New: new})
}

func available(ctx context.Context, tx pgx.Tx, segmentID string) (string, error) {
	var network string
	var poolStart, poolEnd *string
	if err := tx.QueryRow(ctx, "SELECT network::text,dhcp_start::text,dhcp_end::text FROM segments WHERE id=$1::uuid", segmentID).Scan(&network, &poolStart, &poolEnd); err != nil {
		return "", err
	}
	prefix, err := netip.ParsePrefix(network)
	if err != nil {
		return "", err
	}
	usable := netcalc.UsableRange(prefix)
	rows, err := tx.Query(ctx, `SELECT ip::text,ip::text FROM ip_addresses WHERE segment_id=$1::uuid UNION ALL SELECT start_ip::text,end_ip::text FROM reservations WHERE segment_id=$1::uuid AND status='active' UNION ALL SELECT ip::text,ip::text FROM sightings WHERE segment_id=$1::uuid AND last_seen>now()-interval '24 hours' ORDER BY 1`, segmentID)
	if err != nil {
		return "", err
	}
	defer rows.Close()
	used := []netcalc.Range{}
	for rows.Next() {
		var a, b string
		if err := rows.Scan(&a, &b); err != nil {
			return "", err
		}
		first, e1 := netip.ParseAddr(a)
		last, e2 := netip.ParseAddr(b)
		if e1 == nil && e2 == nil {
			used = append(used, netcalc.Range{First: first, Last: last})
		}
	}
	if err := rows.Err(); err != nil {
		return "", err
	}
	if poolStart != nil && poolEnd != nil {
		first, e1 := netip.ParseAddr(*poolStart)
		last, e2 := netip.ParseAddr(*poolEnd)
		if e1 == nil && e2 == nil {
			used = append(used, netcalc.Range{First: first, Last: last})
		}
	}
	for _, gap := range netcalc.Gaps(usable, used) {
		if gap.First.IsValid() {
			return gap.First.String(), nil
		}
	}
	return "", httpx.ErrNotFound
}

func notifyUser(ctx context.Context, tx pgx.Tx, userID, kind, message, link string, email bool) error {
	_, err := tx.Exec(ctx, `INSERT INTO notifications(user_id,kind,message,link,email_status)
		SELECT u.id,$2,$3,$4,CASE WHEN $5 AND COALESCE(p.email,true) THEN 'pending' ELSE 'none' END
		FROM users u LEFT JOIN notification_preferences p ON p.user_id=u.id AND p.kind=$2 WHERE u.id=$1::uuid`, userID, kind, message, link, email)
	return err
}
func notifyAdmins(ctx context.Context, tx pgx.Tx, kind, message, link string) error {
	_, err := tx.Exec(ctx, `INSERT INTO notifications(user_id,kind,message,link,email_status)
		SELECT u.id,$1,$2,$3,CASE WHEN COALESCE(p.email,true) THEN 'pending' ELSE 'none' END
		FROM users u LEFT JOIN notification_preferences p ON p.user_id=u.id AND p.kind=$1 WHERE u.role='admin' AND u.is_active`, kind, message, link)
	return err
}
func clean(s string) string { return strings.TrimSpace(s) }
func nullable(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
