package scanner

import (
	"context"
	"database/sql"
	"net"
	"os"
	"testing"

	"github.com/jackc/pgx/v5"
)

// A server that refuses ping but listens on its check port is Up; with the
// port closed as well it is Down.
func TestIsUpUsesTheCheckPort(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			_ = c.Close()
		}
	}()
	// 192.0.2.1 is reserved for documentation, so ping never answers there.
	if isUp(context.Background(), monitoredIP{ip: "192.0.2.1", port: sql.NullInt32{Int32: int32(port), Valid: true}}) {
		t.Fatal("an address that answers nothing was reported Up")
	}
	if !isUp(context.Background(), monitoredIP{ip: "127.0.0.1", port: sql.NullInt32{Int32: int32(port), Valid: true}}) {
		t.Fatal("a listening check port was reported Down")
	}
	_ = ln.Close()
	if isUp(context.Background(), monitoredIP{ip: "127.0.0.1", port: sql.NullInt32{Int32: int32(port), Valid: true}}) && !pingWorks() {
		t.Fatal("a closed port was reported Up")
	}
}

// pingWorks says whether this machine may send ICMP, which decides whether
// 127.0.0.1 answers ping in the test above.
func pingWorks() bool {
	conn, _, err := openICMP()
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}

// Down only after the configured failures in a row; one success is Up again.
// Needs TEST_DATABASE_URL (make test-db); every change is rolled back.
func TestHealthNeedsFailuresInARow(t *testing.T) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close(ctx)
	tx, err := conn.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	var id string
	if err := tx.QueryRow(ctx, `WITH s AS (INSERT INTO segments (name, kind, network) VALUES ('Health test', 'server', '172.31.250.0/24') RETURNING id)
		INSERT INTO ip_addresses (segment_id, ip, status, monitor, health) SELECT id, '172.31.250.10', 'assigned', true, 'up' FROM s RETURNING id`).Scan(&id); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO settings (key, value) VALUES ('server_check.down_after_failures', '3')
		ON CONFLICT (key) DO UPDATE SET value = '3'`); err != nil {
		t.Fatal(err)
	}
	step := func(result, want string, failures int) {
		t.Helper()
		if _, err := tx.Exec(ctx, healthSQL, id, result); err != nil {
			t.Fatal(err)
		}
		var health string
		var n int
		if err := tx.QueryRow(ctx, `SELECT health, server_check_failures FROM ip_addresses WHERE id = $1::uuid`, id).Scan(&health, &n); err != nil {
			t.Fatal(err)
		}
		if health != want || n != failures {
			t.Fatalf("after %s: health %s with %d failures, want %s with %d", result, health, n, want, failures)
		}
	}
	step("down", "up", 1) // one missed check is not an outage
	step("down", "up", 2)
	step("down", "down", 3)
	step("down", "down", 4)
	step("up", "up", 0)
}
