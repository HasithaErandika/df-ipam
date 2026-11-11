package scanner

import (
	"context"
	"database/sql"
	"fmt"
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

func (s *Scanner) checkAll(ctx context.Context) error {
	rows, err := s.conn.Query(ctx, `SELECT id::text,host(ip),check_port FROM ip_addresses WHERE monitor ORDER BY ip`)
	if err != nil {
		return err
	}
	var targets []monitoredIP
	for rows.Next() {
		var item monitoredIP
		if err := rows.Scan(&item.id, &item.ip, &item.port); err != nil {
			rows.Close()
			return err
		}
		targets = append(targets, item)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()
	jobs := make(chan monitoredIP)
	var workers int
	if len(targets) < 32 {
		workers = len(targets)
	} else {
		workers = 32
	}
	var wg sync.WaitGroup
	errs := make(chan error, len(targets))
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for item := range jobs {
				if ctx.Err() != nil {
					continue
				}
				up := isUp(ctx, item)
				health := "down"
				if up {
					health = "up"
				}
				// A failed check only counts towards Down; the server is Down once it has
				// failed the configured number of checks in a row (docs/new.md).
				if _, err := s.conn.Exec(ctx, healthSQL, item.id, health); err != nil && ctx.Err() == nil {
					errs <- err
				}
			}
		}()
	}
	for _, item := range targets {
		select {
		case <-ctx.Done():
			close(jobs)
			wg.Wait()
			return ctx.Err()
		case jobs <- item:
		}
	}
	close(jobs)
	wg.Wait()
	close(errs)
	for err := range errs {
		return fmt.Errorf("update monitored IP health: %w", err)
	}
	return monitoring.New(s.conn).Evaluate(ctx)
}

const healthSQL = `WITH limits AS (
	SELECT COALESCE((SELECT (value #>> '{}')::integer FROM settings WHERE key = 'server_check.down_after_failures'), 3) AS n
), next AS (
	SELECT i.id, CASE WHEN $2 = 'up' THEN 0 ELSE i.server_check_failures + 1 END AS failures,
		CASE WHEN $2 = 'up' THEN 'up' WHEN i.server_check_failures + 1 >= (SELECT n FROM limits) THEN 'down' ELSE i.health END AS health
	FROM ip_addresses i WHERE i.id = $1::uuid
)
UPDATE ip_addresses i SET server_check_failures = next.failures, health = next.health,
	health_changed_at = CASE WHEN i.health IS DISTINCT FROM next.health THEN now() ELSE i.health_changed_at END
FROM next WHERE i.id = next.id`

func isUp(ctx context.Context, item monitoredIP) bool {
	ip, err := netip.ParseAddr(item.ip)
	if err == nil {
		if up, pingErr := ping(ctx, ip, 700*time.Millisecond); pingErr == nil && up {
			return true
		}
	}
	if !item.port.Valid {
		return false
	}
	ctx, cancel := context.WithTimeout(ctx, 800*time.Millisecond)
	defer cancel()
	conn, err := (&net.Dialer{}).DialContext(ctx, "tcp", net.JoinHostPort(item.ip, strconv.Itoa(int(item.port.Int32))))
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}
