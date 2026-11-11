// Package monitoring evaluates sightings and server health into deduplicated alerts.
package monitoring

import (
	"context"
	"fmt"

	"dfipam/api/internal/platform/db"
	"github.com/jackc/pgx/v5"
)

type Engine struct{ conn db.Conn }

func New(conn db.Conn) *Engine { return &Engine{conn: conn} }

type candidate struct {
	typeName, severity, segmentID, ipID, deviceID, ip, mac, message, key string
}

const candidatesSQL = `
WITH recent AS (
 SELECT si.*,sg.kind,sg.name AS segment_name,COALESCE(env.name,'') AS environment_name,COALESCE(env.default_severity,'medium') AS environment_severity,dev.id AS device_id,dev.status AS device_status,
        ipr.id AS register_id,ipr.status AS register_status,ipr.expected_mac,ipr.monitor,ipr.server_check_failures
 FROM sightings si JOIN segments sg ON sg.id=si.segment_id
 LEFT JOIN devices dev ON dev.mac=si.mac
 LEFT JOIN ip_addresses ipr ON ipr.ip=si.ip
 LEFT JOIN environments env ON env.id=COALESCE(ipr.environment_id,sg.default_environment_id)
 WHERE si.last_seen > now()-interval '10 minutes'
), rules AS (
 SELECT CASE WHEN r.device_status='rejected' THEN 'rejected_device' ELSE 'unknown_device' END AS typ,
        CASE WHEN r.device_status='rejected' THEN 'high' WHEN r.kind='server' THEN 'high' ELSE 'medium' END AS severity,
        r.segment_id,r.register_id AS ipid,r.device_id,r.ip::text,r.mac::text,
        CASE WHEN r.device_status='rejected' THEN 'A rejected device is active on ' ELSE 'An unapproved device is active on ' END || r.segment_name AS message
 FROM recent r WHERE r.mac IS NOT NULL AND r.device_status IS DISTINCT FROM 'approved'
 UNION ALL
 SELECT 'unregistered_static','medium',r.segment_id,NULL::uuid,NULL::uuid,r.ip::text,r.mac::text,'Unregistered static IP is active on '||r.segment_name
 FROM recent r WHERE r.kind='dhcp' AND r.is_static AND r.register_id IS NULL
 UNION ALL
 SELECT 'unknown_server_ip',CASE WHEN r.environment_name='Production' THEN 'critical' ELSE 'high' END,r.segment_id,NULL::uuid,NULL::uuid,r.ip::text,r.mac::text,'Unregistered IP is active in server segment '||r.segment_name
 FROM recent r WHERE r.kind='server' AND r.register_id IS NULL
 UNION ALL
 SELECT 'register_misuse','high',r.segment_id,r.register_id,r.device_id,r.ip::text,r.mac::text,'Registered IP is being used in a way that conflicts with its register entry'
 FROM recent r WHERE r.register_id IS NOT NULL AND (r.register_status IN ('blocked','reserved') OR (r.register_status='assigned' AND r.expected_mac IS NOT NULL AND r.mac IS NOT NULL AND r.expected_mac::text<>r.mac::text))
 UNION ALL
 SELECT 'register_misuse','high',r.segment_id,NULL::uuid,NULL::uuid,r.ip::text,r.mac::text,'A reserved IP is active on the network'
 FROM recent r JOIN reservations hold ON hold.segment_id=r.segment_id AND hold.status='active' AND r.ip BETWEEN hold.start_ip AND hold.end_ip
 UNION ALL
	 SELECT 'ip_conflict','high',(array_agg(segment_id))[1],NULL::uuid,NULL::uuid,ip::text,NULL::text,'Multiple MAC addresses were seen for one IP'
 FROM recent WHERE mac IS NOT NULL GROUP BY ip HAVING count(DISTINCT mac)>1
 UNION ALL
 SELECT 'server_mac_changed',r.environment_severity,r.segment_id,r.register_id,r.device_id,r.ip::text,r.mac::text,'Server IP answered with a different MAC address'
 FROM recent r WHERE r.kind='server' AND r.expected_mac IS NOT NULL AND r.mac IS NOT NULL AND r.expected_mac::text<>r.mac::text
 UNION ALL
 SELECT 'server_down',COALESCE(env.default_severity,'medium'),sg.id,ip.id,NULL::uuid,ip.ip::text,ip.expected_mac::text,'Monitored server has failed three checks'
 FROM ip_addresses ip JOIN segments sg ON sg.id=ip.segment_id LEFT JOIN environments env ON env.id=ip.environment_id OR (ip.environment_id IS NULL AND env.id=sg.default_environment_id)
 WHERE ip.monitor AND ip.server_check_failures>=COALESCE((SELECT (value #>> '{}')::integer FROM settings WHERE key='server_check.down_after_failures'),3)
)
SELECT typ,severity,COALESCE(segment_id::text,''),COALESCE(ipid::text,''),COALESCE(device_id::text,''),COALESCE(ip,''),COALESCE(mac,''),message FROM rules`

func (e *Engine) Evaluate(ctx context.Context) error {
	return db.InTx(ctx, e.conn, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `INSERT INTO devices(mac,status,first_seen,last_seen)
			SELECT mac,'pending',min(first_seen),max(last_seen) FROM sightings
			WHERE mac IS NOT NULL AND last_seen>now()-interval '10 minutes' GROUP BY mac
			ON CONFLICT(mac) DO UPDATE SET first_seen=COALESCE(devices.first_seen,EXCLUDED.first_seen),
				last_seen=GREATEST(COALESCE(devices.last_seen,EXCLUDED.last_seen),EXCLUDED.last_seen)`); err != nil {
			return err
		}
		rows, err := tx.Query(ctx, candidatesSQL)
		if err != nil {
			return err
		}
		var found []candidate
		for rows.Next() {
			var x candidate
			if err := rows.Scan(&x.typeName, &x.severity, &x.segmentID, &x.ipID, &x.deviceID, &x.ip, &x.mac, &x.message); err != nil {
				rows.Close()
				return err
			}
			x.key = fmt.Sprintf("%s:%s:%s", x.typeName, x.ip, x.mac)
			found = append(found, x)
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return err
		}
		rows.Close()
		active := make(map[string]struct{}, len(found))
		for _, x := range found {
			active[x.key] = struct{}{}
			var id string
			var inserted bool
			err := tx.QueryRow(ctx, `INSERT INTO alerts(alert_type,severity,segment_id,ip_address_id,device_id,ip,mac,message,dedupe_key)
				VALUES($1,$2,NULLIF($3,'')::uuid,NULLIF($4,'')::uuid,NULLIF($5,'')::uuid,NULLIF($6,'')::inet,NULLIF($7,'')::macaddr,$8,$9)
				ON CONFLICT(dedupe_key) WHERE status<>'closed' DO UPDATE SET last_seen_at=now(),message=EXCLUDED.message
				RETURNING id::text,(xmax=0)`, x.typeName, x.severity, x.segmentID, x.ipID, x.deviceID, x.ip, x.mac, x.message, x.key).Scan(&id, &inserted)
			if err != nil {
				return err
			}
			if inserted && (x.severity == "high" || x.severity == "critical") {
				if err := notifyAdmins(ctx, tx, "alert", x.message, "/alerts", true); err != nil {
					return err
				}
			}
		}
		if len(active) == 0 {
			_, err := tx.Exec(ctx, `WITH closed AS (
				UPDATE alerts SET status='closed',closed_at=now() WHERE status<>'closed' RETURNING alert_type,message
			) INSERT INTO notifications(user_id,kind,message,link,email_status)
			SELECT u.id,'server_up','A monitored server is back up: '||closed.message,'/alerts',
				CASE WHEN COALESCE(p.email,true) THEN 'pending' ELSE 'none' END
			FROM closed CROSS JOIN users u LEFT JOIN notification_preferences p ON p.user_id=u.id AND p.kind='server_up'
			WHERE closed.alert_type='server_down' AND u.role='admin' AND u.is_active`)
			if err != nil {
				return err
			}
		} else {
			keys := make([]string, 0, len(active))
			for key := range active {
				keys = append(keys, key)
			}
			_, err := tx.Exec(ctx, `WITH closed AS (
				UPDATE alerts SET status='closed',closed_at=now() WHERE status<>'closed' AND dedupe_key<>ALL($1::text[]) RETURNING alert_type,message
			) INSERT INTO notifications(user_id,kind,message,link,email_status)
			SELECT u.id,'server_up','A monitored server is back up: '||closed.message,'/alerts',
				CASE WHEN COALESCE(p.email,true) THEN 'pending' ELSE 'none' END
			FROM closed CROSS JOIN users u LEFT JOIN notification_preferences p ON p.user_id=u.id AND p.kind='server_up'
			WHERE closed.alert_type='server_down' AND u.role='admin' AND u.is_active`, keys)
			if err != nil {
				return err
			}
		}
		return nil
	})
}

func notifyAdmins(ctx context.Context, tx pgx.Tx, kind, message, link string, email bool) error {
	_, err := tx.Exec(ctx, `INSERT INTO notifications(user_id,kind,message,link,email_status)
		SELECT u.id,$1,$2,$3,CASE WHEN $4 AND COALESCE(p.email,true) THEN 'pending' ELSE 'none' END
		FROM users u LEFT JOIN notification_preferences p ON p.user_id=u.id AND p.kind=$1
		WHERE u.role='admin' AND u.is_active`, kind, message, link, email)
	return err
}
