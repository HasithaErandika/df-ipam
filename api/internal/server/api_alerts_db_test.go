package server

import (
	"testing"

	"dfipam/api/internal/modules/monitoring"
)

// DHCP gives an address to another device all the time: that is not a
// conflict unless both answered in the same scan. A static address with two
// MACs is. A device that DHCP moves keeps its one alert.
func TestConflictsAndMovingDevicesAgainstDatabase(t *testing.T) {
	e := newAPI(t)
	floor := id(e.call("POST", "/segments", map[string]any{"name": "Floor 7", "kind": "dhcp", "network": "192.168.70.0/24",
		"gateway": "192.168.70.1", "dhcpStart": "192.168.70.100", "dhcpEnd": "192.168.70.200"}, 200))
	evaluate := func() {
		t.Helper()
		if err := monitoring.New(e.tx).Evaluate(e.ctx); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := e.tx.Exec(e.ctx, `INSERT INTO sightings (segment_id, ip, mac, source, is_static, first_seen, last_seen) VALUES
		($1, '192.168.70.150', '00:bb:00:00:00:01', 'arp', false, now() - interval '9 minutes', now() - interval '9 minutes'),
		($1, '192.168.70.150', '00:bb:00:00:00:02', 'arp', false, now(), now()),
		($1, '192.168.70.20', '00:bb:00:00:00:03', 'arp', true, now() - interval '8 minutes', now() - interval '8 minutes'),
		($1, '192.168.70.20', '00:bb:00:00:00:04', 'arp', true, now(), now())`, floor); err != nil {
		t.Fatal(err)
	}
	evaluate()
	if n := total(e.call("GET", "/alerts?type=ip_conflict&q=192.168.70.150", nil, 200)); n != 0 {
		t.Error("a DHCP address handed to another device raised a conflict")
	}
	if n := total(e.call("GET", "/alerts?type=ip_conflict&q=192.168.70.20", nil, 200)); n != 1 {
		t.Error("a static address with two MACs must raise a conflict")
	}

	if _, err := e.tx.Exec(e.ctx, `UPDATE sightings SET last_seen = now() - interval '1 day' WHERE mac = '00:bb:00:00:00:02'`); err != nil {
		t.Fatal(err)
	}
	if _, err := e.tx.Exec(e.ctx, `INSERT INTO sightings (segment_id, ip, mac, source, is_static) VALUES ($1, '192.168.70.160', '00:bb:00:00:00:02', 'arp', false)`, floor); err != nil {
		t.Fatal(err)
	}
	evaluate()
	var open int
	var ip string
	if err := e.tx.QueryRow(e.ctx, `SELECT count(*), max(host(ip)) FROM alerts WHERE alert_type = 'unknown_device'
		AND mac = '00:bb:00:00:00:02' AND status <> 'closed'`).Scan(&open, &ip); err != nil {
		t.Fatal(err)
	}
	if open != 1 || ip != "192.168.70.160" {
		t.Errorf("a moved device has %d open alerts at %s, want one at its new address", open, ip)
	}
}
