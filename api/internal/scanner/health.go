package scanner

import (
	"context"
	"database/sql"
	"log/slog"
	"net"
	"net/netip"
	"strconv"
	"sync"
	"time"

	"dfipam/api/internal/modules/monitoring"
)

type monitoredIP struct {
	id, ip string
	port   sql.NullInt32
}

// checkAll checks every monitored address, 32 at a time, then records every
// result in one update and evaluates the alert rules.
func (s *Scanner) checkAll(ctx context.Context) error {
	targets, err := s.monitored(ctx)
	if err != nil || len(targets) == 0 {
		return err
	}
	icmp, err := newPinger()
	if err != nil {
		// Without ping, a server with no check port can't be checked at all.
		// Skip it rather than count a failure, or every one of them goes Down.
		slog.ErrorContext(ctx, "server checks can't ping; checking only servers with a check port", "error", err)
		icmp, targets = nil, withPort(targets)
		if len(targets) == 0 {
			return nil
		}
	} else {
		defer icmp.Close()
	}
	ids, results, times := make([]string, len(targets)), make([]string, len(targets)), make([]int32, len(targets))
	jobs := make(chan int)
	var wg sync.WaitGroup
	for range min(len(targets), 32) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range jobs {
				ids[i], results[i] = targets[i].id, "down"
				if ms, up := responseTime(ctx, icmp, targets[i]); up {
					results[i], times[i] = "up", ms
				}
			}
		}()
	}
	for i := range targets {
		if ctx.Err() != nil {
			break
		}
		jobs <- i
	}
	close(jobs)
	wg.Wait()
	if err := ctx.Err(); err != nil {
		return err
	}
	// A failed check only counts towards Down; the server is Down once it has
	// failed the configured number of checks in a row.
	if _, err := s.conn.Exec(ctx, healthSQL, ids, results, times); err != nil {
		return err
	}
	return monitoring.New(s.conn).Evaluate(ctx)
}

func withPort(targets []monitoredIP) []monitoredIP {
	var out []monitoredIP
	for _, t := range targets {
		if t.port.Valid {
			out = append(out, t)
		}
	}
	return out
}

func (s *Scanner) monitored(ctx context.Context) ([]monitoredIP, error) {
	rows, err := s.conn.Query(ctx, `SELECT id::text, host(ip), check_port FROM ip_addresses WHERE monitor ORDER BY ip`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []monitoredIP
	for rows.Next() {
		var item monitoredIP
		if err := rows.Scan(&item.id, &item.ip, &item.port); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

// healthSQL records one check per address: $1 the IDs, $2 "up" or "down", $3
// the response time in milliseconds (ignored when down).
const healthSQL = `WITH limits AS (
	SELECT COALESCE((SELECT (value #>> '{}')::integer FROM settings WHERE key = 'server_check.down_after_failures'), 3) AS n
), checks AS (
	SELECT v.id::uuid AS id, v.result, v.ms FROM unnest($1::text[], $2::text[], $3::integer[]) AS v(id, result, ms)
), next AS (
	SELECT i.id, CASE WHEN c.result = 'up' THEN c.ms END AS ms, CASE WHEN c.result = 'up' THEN 0 ELSE i.server_check_failures + 1 END AS failures,
		CASE WHEN c.result = 'up' THEN 'up' WHEN i.server_check_failures + 1 >= (SELECT n FROM limits) THEN 'down' ELSE i.health END AS health
	FROM ip_addresses i JOIN checks c ON c.id = i.id
)
UPDATE ip_addresses i SET server_check_failures = next.failures, health = next.health, response_ms = next.ms,
	health_changed_at = CASE WHEN i.health IS DISTINCT FROM next.health THEN now() ELSE i.health_changed_at END
FROM next WHERE i.id = next.id`

// responseTime reports whether the server answered, and how fast in
// milliseconds. It pings twice and keeps the faster reply, so one lost or late
// packet is neither a failure nor a slow server; a server that blocks ping is
// timed by connecting to its check port.
func responseTime(ctx context.Context, icmp *pinger, item monitoredIP) (int32, bool) {
	ip, err := netip.ParseAddr(item.ip)
	if err == nil && icmp != nil {
		best := time.Duration(-1)
		for range 2 {
			start := time.Now()
			if up, _, _ := icmp.ping(ctx, ip, 700*time.Millisecond); up && (best < 0 || time.Since(start) < best) {
				best = time.Since(start)
			}
		}
		if best >= 0 {
			return int32(best.Milliseconds()), true
		}
	}
	if !item.port.Valid {
		return 0, false
	}
	ctx, cancel := context.WithTimeout(ctx, 800*time.Millisecond)
	defer cancel()
	start := time.Now()
	conn, err := (&net.Dialer{}).DialContext(ctx, "tcp", net.JoinHostPort(item.ip, strconv.Itoa(int(item.port.Int32))))
	if err != nil {
		return 0, false
	}
	_ = conn.Close()
	return int32(time.Since(start).Milliseconds()), true
}
