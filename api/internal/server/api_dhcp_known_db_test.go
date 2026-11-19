package server

import "testing"

// An approved device on a DHCP pool address is known without a register entry:
// pool addresses change, so they are never registered. A static address still
// needs its entry before the device counts as known.
func TestApprovedDHCPClientIsKnownAgainstDatabase(t *testing.T) {
	e := newAPI(t)
	wifi := id(e.call("POST", "/segments", map[string]any{"name": "DFHO_G", "kind": "dhcp", "network": "192.168.70.0/24",
		"gateway": "192.168.70.1", "dhcpStart": "192.168.70.100", "dhcpEnd": "192.168.70.200"}, 200))
	if _, err := e.tx.Exec(e.ctx, `INSERT INTO sightings (segment_id, ip, mac, source, is_static) VALUES
		($1, '192.168.70.150', '00:dd:00:00:00:01', 'arp', false),
		($1, '192.168.70.10', '00:dd:00:00:00:02', 'arp', true)`, wifi); err != nil {
		t.Fatal(err)
	}
	if _, err := e.tx.Exec(e.ctx, `INSERT INTO devices (mac, status, decided_at) VALUES
		('00:dd:00:00:00:01', 'approved', now()), ('00:dd:00:00:00:02', 'approved', now())`); err != nil {
		t.Fatal(err)
	}
	known := e.call("GET", "/discovery/findings?seen=all&kind=known&segmentId="+wifi, nil, 200)
	if n := total(known); n != 1 {
		t.Fatalf("known: got %v, want only the DHCP client", n)
	}
	if ip := known["data"].([]any)[0].(map[string]any)["ip"]; ip != "192.168.70.150" {
		t.Fatalf("known row is %v, want the pool address", ip)
	}
	if s := e.call("GET", "/discovery/summary?seen=all&segmentId="+wifi, nil, 200)["data"].(map[string]any); s["known"].(float64) != 1 {
		t.Fatalf("summary: %v", s)
	}
}
