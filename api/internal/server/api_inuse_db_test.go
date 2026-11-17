package server

import (
	"testing"

	"dfipam/api/internal/modules/monitoring"
)

// A static device switched off for a weekend keeps its address: outside the
// DHCP pool an address seen in the last 30 days is not an Available candidate.
func TestStaticAddressStaysInUseAgainstDatabase(t *testing.T) {
	e := newAPI(t)
	floor := id(e.call("POST", "/segments", map[string]any{"name": "Floor 8", "kind": "dhcp", "network": "192.168.80.0/24",
		"gateway": "192.168.80.1", "dhcpStart": "192.168.80.100", "dhcpEnd": "192.168.80.200"}, 200))
	if _, err := e.tx.Exec(e.ctx, `INSERT INTO sightings (segment_id, ip, mac, source, is_static, first_seen, last_seen) VALUES
		($1, '192.168.80.2', '00:cc:00:00:00:01', 'arp', true, now() - interval '3 days', now() - interval '3 days'),
		($1, '192.168.80.3', '00:cc:00:00:00:02', 'arp', true, now() - interval '40 days', now() - interval '40 days')`, floor); err != nil {
		t.Fatal(err)
	}
	cand := e.call("GET", "/ip-addresses/available-candidate?segmentId="+floor, nil, 200)["data"].(map[string]any)["ip"]
	if cand != "192.168.80.3" {
		t.Fatalf("candidate %v: want 192.168.80.3 (.2 was seen 3 days ago, .3 not for 40)", cand)
	}
	check := e.call("GET", "/ip-addresses/check?segmentId="+floor+"&ip=192.168.80.2", nil, 200)["data"].(map[string]any)
	if check["status"].(map[string]any)["seenRecently"] != true {
		t.Fatalf("check: %v", check)
	}
}

// Alerts on a segment whose last scan failed stay open: nobody looked, so the
// cause may still be there. They close once a scan succeeds again.
func TestFailedScanKeepsAlertsOpenAgainstDatabase(t *testing.T) {
	e := newAPI(t)
	floor := id(e.call("POST", "/segments", map[string]any{"name": "Floor 9", "kind": "dhcp", "network": "192.168.90.0/24",
		"gateway": "192.168.90.1", "dhcpStart": "192.168.90.100", "dhcpEnd": "192.168.90.200"}, 200))
	open := func() int {
		t.Helper()
		if err := monitoring.New(e.tx).Evaluate(e.ctx); err != nil {
			t.Fatal(err)
		}
		var n int
		if err := e.tx.QueryRow(e.ctx, `SELECT count(*) FROM alerts WHERE mac = '00:dd:00:00:00:01' AND status <> 'closed'`).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	if _, err := e.tx.Exec(e.ctx, `INSERT INTO sightings (segment_id, ip, mac, source, is_static) VALUES ($1, '192.168.90.150', '00:dd:00:00:00:01', 'arp', false)`, floor); err != nil {
		t.Fatal(err)
	}
	if open() != 1 {
		t.Fatal("an unapproved device raised no alert")
	}
	if _, err := e.tx.Exec(e.ctx, `UPDATE sightings SET last_seen = now() - interval '1 day' WHERE mac = '00:dd:00:00:00:01';
		UPDATE segments SET scan_failed_at = now() WHERE id = '`+floor+`'`); err != nil {
		t.Fatal(err)
	}
	if open() != 1 {
		t.Fatal("an alert closed while its segment couldn't be scanned")
	}
	if _, err := e.tx.Exec(e.ctx, `UPDATE segments SET scan_failed_at = NULL WHERE id = $1`, floor); err != nil {
		t.Fatal(err)
	}
	if open() != 0 {
		t.Fatal("the alert stayed open after a successful scan found nothing")
	}
}
