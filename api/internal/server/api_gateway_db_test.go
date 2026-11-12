package server

import (
	"testing"

	"dfipam/api/internal/modules/monitoring"
)

// A segment's gateway is infrastructure: it answers on every network, so it
// must never raise "unregistered static" or "unknown server IP".
func TestGatewaysRaiseNoAlertsAgainstDatabase(t *testing.T) {
	e := newAPI(t)
	wifi := id(e.call("POST", "/segments", map[string]any{"name": "Wi-Fi", "kind": "dhcp", "network": "192.168.50.0/24",
		"gateway": "192.168.50.1", "dhcpStart": "192.168.50.2", "dhcpEnd": "192.168.50.254"}, 200))
	dmz := id(e.call("POST", "/segments", map[string]any{"name": "DMZ", "kind": "server", "network": "172.20.14.192/29", "gateway": "172.20.14.193"}, 200))
	if _, err := e.tx.Exec(e.ctx, `INSERT INTO sightings (segment_id, ip, mac, source, is_static, is_gateway) VALUES
		($1, '192.168.50.1', '00:aa:00:00:00:01', 'arp', true, false),
		($2, '172.20.14.193', '00:aa:00:00:00:02', 'arp', true, false),
		($2, '172.20.14.194', '00:aa:00:00:00:03', 'arp', true, false)`, wifi, dmz); err != nil {
		t.Fatal(err)
	}
	if err := monitoring.New(e.tx).Evaluate(e.ctx); err != nil {
		t.Fatal(err)
	}
	for _, typ := range []string{"unregistered_static", "unknown_server_ip"} {
		for _, ip := range []string{"192.168.50.1", "172.20.14.193"} {
			if n := total(e.call("GET", "/alerts?type="+typ+"&q="+ip, nil, 200)); n != 0 {
				t.Errorf("gateway %s raised %s", ip, typ)
			}
		}
	}
	if n := total(e.call("GET", "/alerts?type=unknown_server_ip&q=172.20.14.194", nil, 200)); n != 1 {
		t.Error("an unregistered server in the DMZ must still raise an alert")
	}
	e.call("GET", "/dashboard", nil, 200)
}
