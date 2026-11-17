package scanner

import (
	"context"
	"os"
	"testing"

	"github.com/jackc/pgx/v5"
)

// Saving a sweep stores what was learned, records when a host was identified
// in full, and joins a later sighting without a MAC outside the DHCP pool to
// the MAC already seen, instead of listing the host twice. Inside the pool the
// address may have gone to another device, so the MAC is not borrowed. Needs TEST_DATABASE_URL (make test-db).
func TestSaveSightingsAgainstDatabase(t *testing.T) {
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
	var seg string
	if err := tx.QueryRow(ctx, `INSERT INTO segments (name, kind, network, dhcp_start, dhcp_end)
		VALUES ('Save test', 'dhcp', '10.250.0.0/24', '10.250.0.100', '10.250.0.200') RETURNING id`).Scan(&seg); err != nil {
		t.Fatal(err)
	}
	s := New(tx)
	first := []sighting{{ip: "10.250.0.48", mac: "00:11:22:33:44:55", hostname: "PC-48", source: "netbios", full: true,
		ports: []int{445}, portsChecked: true, static: true},
		{ip: "10.250.0.150", mac: "00:11:22:33:44:66", source: "arp"}}
	if err := s.saveSightings(ctx, seg, first); err != nil {
		t.Fatal(err)
	}
	later := []sighting{{ip: "10.250.0.48", source: "ping", static: true}, {ip: "10.250.0.150", source: "ping"}}
	if err := s.saveSightings(ctx, seg, later); err != nil {
		t.Fatal(err)
	}
	var rows int
	var mac, name string
	var identified bool
	var ports []int32
	if err := tx.QueryRow(ctx, `SELECT count(*) OVER (), mac::text, hostname, identified_at IS NOT NULL, open_ports
		FROM sightings WHERE ip = '10.250.0.48'`).Scan(&rows, &mac, &name, &identified, &ports); err != nil {
		t.Fatal(err)
	}
	if rows != 1 || mac != "00:11:22:33:44:55" || name != "PC-48" || !identified || len(ports) != 1 || ports[0] != 445 {
		t.Fatalf("got %d rows, mac %s, name %s, identified %v, ports %v", rows, mac, name, identified, ports)
	}
	var bare int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM sightings WHERE ip = '10.250.0.150' AND mac IS NULL`).Scan(&bare); err != nil || bare != 1 {
		t.Fatalf("a DHCP pool address borrowed the MAC last seen there (%d MAC-less rows, %v)", bare, err)
	}
	s.loadFullMarks(ctx)
	if s.dueFull(segment{id: seg}, "10.250.0.48", false) {
		t.Fatal("a host identified moments ago is due again after a restart")
	}
}
