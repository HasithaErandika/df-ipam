package server

import (
	"testing"

	"dfipam/api/internal/modules/monitoring"
)

// The scheduled-work switches save and read back through Settings.
func TestAutomationSwitchesAgainstDatabase(t *testing.T) {
	e := newAPI(t)
	e.call("PUT", "/settings", map[string]any{"switches": map[string]bool{"autoScanDevices": false}}, 200)
	got := e.call("GET", "/settings", nil, 200)["data"].(map[string]any)["switches"].(map[string]any)
	if got["autoScanDevices"] != false || got["autoScanServers"] != true || got["serverChecks"] != true {
		t.Fatalf("switches: %v", got)
	}
}

// A monitored server that answers slower than the limit raises a Slow server
// alert, which closes once it answers in time again.
func TestSlowServerAgainstDatabase(t *testing.T) {
	e := newAPI(t)
	seg := id(e.call("POST", "/segments", map[string]any{"name": "Servers 5", "kind": "server", "network": "10.55.0.0/24"}, 200))
	if _, err := e.tx.Exec(e.ctx, `INSERT INTO ip_addresses (segment_id, ip, status, monitor, health, response_ms, assigned_name)
		VALUES ($1, '10.55.0.10', 'assigned', true, 'up', 450, 'APP-01')`, seg); err != nil {
		t.Fatal(err)
	}
	e.call("PUT", "/settings", map[string]any{"values": map[string]int{"slowMs": 200}}, 200)
	slow := func() float64 {
		t.Helper()
		if err := monitoring.New(e.tx).Evaluate(e.ctx); err != nil {
			t.Fatal(err)
		}
		return total(e.call("GET", "/alerts?type=server_slow&status=open&q=10.55.0.10", nil, 200))
	}
	if slow() != 1 {
		t.Fatal("a server answering in 450 ms raised no Slow server alert with a 200 ms limit")
	}
	if _, err := e.tx.Exec(e.ctx, `UPDATE ip_addresses SET response_ms = 3 WHERE ip = '10.55.0.10'`); err != nil {
		t.Fatal(err)
	}
	if slow() != 0 {
		t.Fatal("the Slow server alert stayed open after the server answered in 3 ms")
	}
}

// With automatic scanning of floor segments off, their alerts don't close:
// nobody is looking, so the cause may still be there.
func TestAutoScanOffKeepsAlertsOpenAgainstDatabase(t *testing.T) {
	e := newAPI(t)
	floor := id(e.call("POST", "/segments", map[string]any{"name": "Floor 10", "kind": "dhcp", "network": "192.168.100.0/24",
		"gateway": "192.168.100.1", "dhcpStart": "192.168.100.100", "dhcpEnd": "192.168.100.200"}, 200))
	open := func() int {
		t.Helper()
		if err := monitoring.New(e.tx).Evaluate(e.ctx); err != nil {
			t.Fatal(err)
		}
		var n int
		if err := e.tx.QueryRow(e.ctx, `SELECT count(*) FROM alerts WHERE mac = '00:ee:00:00:00:01' AND status <> 'closed'`).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	if _, err := e.tx.Exec(e.ctx, `INSERT INTO sightings (segment_id, ip, mac, source, is_static) VALUES ($1, '192.168.100.150', '00:ee:00:00:00:01', 'arp', false)`, floor); err != nil {
		t.Fatal(err)
	}
	if open() != 1 {
		t.Fatal("an unapproved device raised no alert")
	}
	e.call("PUT", "/settings", map[string]any{"switches": map[string]bool{"autoScanDevices": false}}, 200)
	if _, err := e.tx.Exec(e.ctx, `UPDATE sightings SET last_seen = now() - interval '1 day' WHERE mac = '00:ee:00:00:00:01'`); err != nil {
		t.Fatal(err)
	}
	if open() != 1 {
		t.Fatal("an alert closed while its segment wasn't being scanned")
	}
	e.call("PUT", "/settings", map[string]any{"switches": map[string]bool{"autoScanDevices": true}}, 200)
	if open() != 0 {
		t.Fatal("the alert stayed open once scanning was back on and found nothing")
	}
}
