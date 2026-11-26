package notifications

import (
	"context"
	"time"

	"dfipam/api/internal/platform/db"
)

// facts is what the daily summary reports, read once for everyone. Each part
// lists only what needs attention, so a quiet day sends nothing.
type facts struct {
	Alerts    alertFacts
	Devices   int
	Requests  requestFacts
	Capacity  []capacity
	Failing   []failing
	Contracts []contract
	Holds     []hold
}

type alertFacts struct {
	Critical int `db:"critical"`
	High     int `db:"high"`
	Medium   int `db:"medium"`
	Low      int `db:"low"`
	New      int `db:"new"`
}

type requestFacts struct {
	Count  int        `db:"count"`
	Oldest *time.Time `db:"oldest"`
}

type openAlert struct {
	Severity string    `db:"severity"`
	Message  string    `db:"message"`
	At       time.Time `db:"opened_at"`
}

type capacity struct {
	ID   string `db:"id"`
	Name string `db:"name"`
	What string `db:"what"`
	Used int64  `db:"used"`
	Size int64  `db:"size"`
}

type failing struct {
	ID    string    `db:"id"`
	Name  string    `db:"name"`
	Since time.Time `db:"since"`
}

type contract struct {
	ID      string    `db:"id"`
	Code    string    `db:"code"`
	Name    string    `db:"name"`
	End     time.Time `db:"contract_end"`
	Officer *string   `db:"officer_id"`
}

type hold struct {
	Owner   string    `db:"owner_id"`
	Reason  string    `db:"reason"`
	Range   string    `db:"range"`
	Expires time.Time `db:"expires_at"`
}

// capacitySQL finds DHCP pools with 90% or more of their addresses seen in a
// day, and server networks with 90% or more of their addresses registered.
const capacitySQL = `SELECT s.id::text AS id, s.name, x.what, x.used, x.size FROM segments s,
	LATERAL (SELECT CASE WHEN s.kind = 'dhcp' THEN 'DHCP pool' ELSE 'Addresses registered' END AS what,
		CASE WHEN s.kind = 'dhcp' THEN (s.dhcp_end - s.dhcp_start + 1)
			ELSE GREATEST(power(2, 32 - masklen(s.network))::bigint - 2, 1) END AS size,
		CASE WHEN s.kind = 'dhcp' THEN (SELECT count(DISTINCT si.ip) FROM sightings si WHERE si.segment_id = s.id
				AND si.ip BETWEEN s.dhcp_start AND s.dhcp_end AND si.last_seen > now() - interval '24 hours')
			ELSE (SELECT count(*) FROM ip_addresses i WHERE i.segment_id = s.id) END AS used) x
	WHERE (s.kind = 'server' OR (s.kind = 'dhcp' AND s.dhcp_start IS NOT NULL)) AND x.used * 100 >= x.size * 90
	ORDER BY x.used::float / x.size DESC, s.name LIMIT 20`

// readFacts reads every part in a handful of queries, whatever the number of
// people the summary goes to.
func readFacts(ctx context.Context, conn db.Conn) (facts, []openAlert, error) {
	var f facts
	if err := conn.QueryRow(ctx, `SELECT count(*) FILTER (WHERE severity = 'critical'), count(*) FILTER (WHERE severity = 'high'),
		count(*) FILTER (WHERE severity = 'medium'), count(*) FILTER (WHERE severity = 'low'),
		count(*) FILTER (WHERE opened_at > now() - interval '24 hours') FROM alerts WHERE status = 'open'`).
		Scan(&f.Alerts.Critical, &f.Alerts.High, &f.Alerts.Medium, &f.Alerts.Low, &f.Alerts.New); err != nil {
		return f, nil, err
	}
	if err := conn.QueryRow(ctx, `SELECT (SELECT count(*) FROM devices WHERE status = 'pending'),
		count(*), min(created_at) FROM ip_requests WHERE status = 'pending'`).Scan(&f.Devices, &f.Requests.Count, &f.Requests.Oldest); err != nil {
		return f, nil, err
	}
	top, err := db.All[openAlert](ctx, conn, `SELECT severity, message, opened_at FROM alerts
		WHERE status = 'open' AND severity IN ('critical', 'high') ORDER BY severity = 'critical' DESC, opened_at DESC LIMIT 8`)
	if err != nil {
		return f, nil, err
	}
	if f.Capacity, err = db.All[capacity](ctx, conn, capacitySQL); err != nil {
		return f, nil, err
	}
	if f.Failing, err = db.All[failing](ctx, conn, `SELECT id::text AS id, name, scan_failed_at AS since FROM segments
		WHERE scan_failed_at IS NOT NULL ORDER BY name LIMIT 20`); err != nil {
		return f, nil, err
	}
	if f.Contracts, err = db.All[contract](ctx, conn, `SELECT id::text AS id, code, name, contract_end, officer_id::text AS officer_id
		FROM network_links WHERE archived_at IS NULL AND status <> 'terminated'
		AND contract_end BETWEEN `+today+` AND `+today+` + 30 ORDER BY contract_end, code`); err != nil {
		return f, nil, err
	}
	f.Holds, err = db.All[hold](ctx, conn, `SELECT owner_id::text AS owner_id, reason, expires_at,
		host(start_ip) || CASE WHEN start_ip <> end_ip THEN ' – ' || host(end_ip) ELSE '' END AS range
		FROM reservations WHERE status = 'active' AND owner_id IS NOT NULL AND expires_at > now()
		AND expires_at <= now() + make_interval(days => COALESCE((SELECT (value #>> '{}')::integer FROM settings WHERE key = 'reservations.reminder_days'), 7))
		ORDER BY expires_at`)
	return f, top, err
}

// today is the date in Sri Lanka.
const today = `(now() AT TIME ZONE 'Asia/Colombo')::date`
