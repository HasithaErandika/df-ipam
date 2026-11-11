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
	rows, err := s.conn.Query(ctx, `SELECT id::text,ip::text,check_port FROM ip_addresses WHERE monitor ORDER BY ip`)
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
				if _, err := s.conn.Exec(ctx, `UPDATE ip_addresses SET
					health_changed_at=CASE WHEN health IS DISTINCT FROM $2 THEN now() ELSE health_changed_at END,
					health=$2,server_check_failures=CASE WHEN $2='down' THEN server_check_failures+1 ELSE 0 END
					WHERE id=$1::uuid`, item.id, health); err != nil && ctx.Err() == nil {
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
