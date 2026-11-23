package dbrules

import "testing"

func TestSegments(t *testing.T) {
	e := setup(t)
	e.segment("Floor 1", "dhcp", "192.168.1.0/24")
	e.mustFail("overlapping network", `INSERT INTO segments (name, kind, network) VALUES ('Overlap', 'dhcp', '192.168.0.0/22')`)
	e.mustFail("same name", `INSERT INTO segments (name, kind, network) VALUES ('floor 1', 'dhcp', '192.168.9.0/24')`)
	e.mustFail("IPv6", `INSERT INTO segments (name, kind, network) VALUES ('V6', 'server', 'fd00::/64')`)
	e.mustFail("pool outside network", `INSERT INTO segments (name, kind, network, dhcp_start, dhcp_end)
		VALUES ('F2', 'dhcp', '192.168.2.0/24', '192.168.2.10', '192.168.3.10')`)
	e.mustFail("pool backwards", `INSERT INTO segments (name, kind, network, dhcp_start, dhcp_end)
		VALUES ('F2', 'dhcp', '192.168.2.0/24', '192.168.2.200', '192.168.2.10')`)
	e.mustFail("pool on a server segment", `INSERT INTO segments (name, kind, network, dhcp_start, dhcp_end)
		VALUES ('S1', 'server', '172.16.1.0/24', '172.16.1.10', '172.16.1.20')`)
	e.mustPass("valid floor", `INSERT INTO segments (name, kind, floor, vlan, network, gateway, dhcp_start, dhcp_end)
		VALUES ('F2', 'dhcp', 2, 20, '192.168.2.0/24', '192.168.2.1', '192.168.2.100', '192.168.2.250')`)
}

func TestRegister(t *testing.T) {
	e := setup(t)
	seg := e.segment("Prod servers", "server", "172.16.10.0/24")
	e.mustPass("IP in segment", `INSERT INTO ip_addresses (segment_id, ip, status) VALUES ($1, '172.16.10.5', 'assigned')`, seg)
	e.mustFail("same IP twice", `INSERT INTO ip_addresses (segment_id, ip, status) VALUES ($1, '172.16.10.5', 'reserved')`, seg)
	e.mustFail("IP outside segment", `INSERT INTO ip_addresses (segment_id, ip, status) VALUES ($1, '172.16.11.5', 'assigned')`, seg)
	e.mustFail("network not host", `INSERT INTO ip_addresses (segment_id, ip, status) VALUES ($1, '172.16.10.0/30', 'assigned')`, seg)
	e.mustFail("unknown status", `INSERT INTO ip_addresses (segment_id, ip, status) VALUES ($1, '172.16.10.6', 'free')`, seg)
	e.mustFail("segment shrinks away from its IPs", `UPDATE segments SET network = '172.16.11.0/24' WHERE id = $1`, seg)
	e.mustFail("reservation outside segment", `INSERT INTO reservations (segment_id, start_ip, end_ip, reason)
		VALUES ($1, '172.16.10.200', '172.16.11.10', 'x')`, seg)
	e.mustFail("reservation without reason", `INSERT INTO reservations (segment_id, start_ip, end_ip, reason)
		VALUES ($1, '172.16.10.200', '172.16.10.210', ' ')`, seg)
	e.mustPass("reservation", `INSERT INTO reservations (segment_id, start_ip, end_ip, reason)
		VALUES ($1, '172.16.10.200', '172.16.10.220', 'New printers')`, seg)
}

func TestRequests(t *testing.T) {
	e := setup(t)
	seg := e.segment("UAT servers", "server", "172.16.20.0/24")
	var user string
	e.row(&user, `INSERT INTO users (email, auth_provider, auth_subject, role) VALUES ('a@x.lk', 't', 'a', 'admin') RETURNING id`)
	var req string
	e.row(&req, `INSERT INTO ip_requests (requester_id, segment_id, name) VALUES ($1, $2, 'App server') RETURNING id`, user, seg)
	e.mustFail("rejected without reason", `UPDATE ip_requests SET status = 'rejected', decided_by = $2, decided_at = now() WHERE id = $1`, req, user)
	e.mustFail("approved without a decider", `UPDATE ip_requests SET status = 'approved' WHERE id = $1`, req)
	e.mustPass("rejected with reason", `UPDATE ip_requests SET status = 'rejected', decided_by = $2, decided_at = now(),
		decision_reason = 'Use the shared host' WHERE id = $1`, req, user)
	e.mustFail("unknown role", `INSERT INTO users (email, auth_provider, auth_subject, role) VALUES ('b@x.lk', 't', 'b', 'Super Administrator')`)
}

func TestAlertsAndSightings(t *testing.T) {
	e := setup(t)
	open := `INSERT INTO alerts (alert_type, severity, message, dedupe_key) VALUES ('unknown_device', 'medium', 'm', 'unknown:aa')`
	e.mustPass("first alert", open)
	e.mustFail("second open alert for the same cause", open)
	e.mustPass("close it", `UPDATE alerts SET status = 'closed', closed_at = now() WHERE dedupe_key = 'unknown:aa'`)
	e.mustPass("new alert after closing", open)
	e.mustFail("acknowledged without time", `UPDATE alerts SET status = 'acknowledged' WHERE status = 'open'`)

	seen := `INSERT INTO sightings (ip, mac, source) VALUES ('192.168.1.20', NULL, 'ping')`
	e.mustPass("ping sighting with no MAC", seen)
	e.mustFail("duplicate IP with no MAC", seen)
	e.mustPass("same IP with a MAC", `INSERT INTO sightings (ip, mac, source) VALUES ('192.168.1.20', '00:11:22:33:44:55', 'arp')`)
}

func TestAuditIsAppendOnly(t *testing.T) {
	e := setup(t)
	e.mustPass("insert", `INSERT INTO audit_logs (action, object_type) VALUES ('create', 'segment')`)
	e.mustFail("update", `UPDATE audit_logs SET action = 'x'`)
	e.mustFail("delete", `DELETE FROM audit_logs`)
}

func TestAppRoleGrants(t *testing.T) {
	e := setup(t)
	for _, c := range []struct {
		table, privilege string
		want             bool
	}{
		{"audit_logs", "INSERT", true}, {"audit_logs", "UPDATE", false}, {"audit_logs", "DELETE", false},
		{"users", "UPDATE", true}, {"users", "DELETE", false},
		{"ip_requests", "DELETE", false}, {"reservations", "DELETE", false},
		{"ip_addresses", "DELETE", true}, {"sightings", "UPDATE", true},
		{"devices", "DELETE", false},
	} {
		var got bool
		e.row(&got, `SELECT has_table_privilege('dfipam_app', $1, $2)`, c.table, c.privilege)
		if got != c.want {
			t.Errorf("dfipam_app %s on %s: got %t, want %t", c.privilege, c.table, got, c.want)
		}
	}
	var open bool
	e.row(&open, `SELECT bool_and(relrowsecurity) FROM pg_class WHERE relnamespace = 'public'::regnamespace AND relkind = 'r'
		AND relname <> 'schema_migrations'`)
	if !open {
		t.Error("a table is missing row level security")
	}
}
