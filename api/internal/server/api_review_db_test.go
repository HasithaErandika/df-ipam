package server

import (
	"context"
	"testing"

	"dfipam/api/internal/auth"
)

// roleSessions signs every request in as one person holding one role.
type roleSessions struct{ id, role string }

func (s roleSessions) Resolve(ctx context.Context, _ string) (*auth.Principal, error) {
	return auth.NewPrincipal(s.id, "manager@example.com", s.role, []string{auth.UsersManage, auth.InventoryView}, false), nil
}

// as returns the same database transaction driven by a person with that role.
func (e *apiEnv) as(userID, role string) *apiEnv {
	r, _ := New(Deps{Conn: e.tx, Sessions: roleSessions{userID, role}})
	return &apiEnv{t: e.t, ctx: e.ctx, tx: e.tx, router: r}
}

// Someone who manages users but isn't an Administrator can't hand out more
// than they hold: not to a role, not to their own role, not by giving a role.
func TestNoGrantBeyondOwnAccess(t *testing.T) {
	e := newAPI(t)
	e.call("POST", "/roles", map[string]any{"name": "User desk", "permissions": []string{auth.UsersManage, auth.InventoryView}}, 201)
	var mgr, other string
	if err := e.tx.QueryRow(e.ctx, `INSERT INTO users (email, auth_provider, auth_subject, role) VALUES ('manager@example.com', 'test', 'mgr', 'user_desk') RETURNING id`).Scan(&mgr); err != nil {
		t.Fatal(err)
	}
	if err := e.tx.QueryRow(e.ctx, `INSERT INTO users (email, auth_provider, auth_subject, role) VALUES ('someone@example.com', 'test', 'someone', NULL) RETURNING id`).Scan(&other); err != nil {
		t.Fatal(err)
	}
	m := e.as(mgr, "user_desk")

	// Adding a permission they lack, to any role, is refused.
	m.call("POST", "/roles", map[string]any{"name": "Sneaky", "permissions": []string{auth.SystemManage}}, 403)
	m.call("PUT", "/roles/user_desk", map[string]any{"name": "User desk", "permissions": []string{auth.UsersManage, auth.InventoryView, auth.SystemManage}}, 403)
	// Changing their own role's permissions is refused even within their access.
	m.call("PUT", "/roles/user_desk", map[string]any{"name": "User desk", "permissions": []string{auth.UsersManage}}, 409)
	// Renaming it without changing permissions is fine.
	m.call("PUT", "/roles/user_desk", map[string]any{"name": "User desk team", "permissions": []string{auth.UsersManage, auth.InventoryView}}, 200)
	// Giving the Administrator role, or a role beyond their own, is refused.
	m.call("PUT", "/users/"+other, map[string]any{"role": "admin", "isActive": true}, 403)
	m.call("PUT", "/users/"+other, map[string]any{"role": "network_engineer", "isActive": true}, 403)
	// A role within their access can be given.
	e.call("POST", "/roles", map[string]any{"name": "Look only", "permissions": []string{auth.InventoryView}}, 201)
	m.call("PUT", "/users/"+other, map[string]any{"role": "look_only", "isActive": true}, 204)
}

// A role that may change link costs can always see them: otherwise its holders
// were sent the costs blank and wiped them on save.
func TestCommercialManageBringsView(t *testing.T) {
	e := newAPI(t)
	e.call("POST", "/roles", map[string]any{"name": "Cost keeper", "permissions": []string{auth.LinksView, auth.LinksEdit, auth.LinksCommercialManage}}, 201)
	var perms []string
	if err := e.tx.QueryRow(e.ctx, "SELECT permissions FROM roles WHERE key = 'cost_keeper'").Scan(&perms); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, p := range perms {
		found = found || p == auth.LinksCommercialView
	}
	if !found {
		t.Fatalf("manage without view saved: %v", perms)
	}
}

// A segment edit can't move the DHCP pool over register entries, and a segment
// a link uses can't be deleted.
func TestSegmentEditAndDeleteRules(t *testing.T) {
	e := newAPI(t)
	seg := id(e.call("POST", "/segments", map[string]any{"name": "Review floor", "kind": "dhcp", "network": "10.77.0.0/24",
		"gateway": "10.77.0.1", "dhcpStart": "10.77.0.100", "dhcpEnd": "10.77.0.200"}, 200))
	e.call("POST", "/ip-addresses", map[string]any{"segmentId": seg, "ip": "10.77.0.20", "assignedName": "Printer"}, 201)
	e.call("PUT", "/segments/"+seg, map[string]any{"name": "Review floor", "kind": "dhcp", "network": "10.77.0.0/24",
		"gateway": "10.77.0.1", "dhcpStart": "10.77.0.10", "dhcpEnd": "10.77.0.200"}, 422)
	e.call("PUT", "/segments/"+seg, map[string]any{"name": "Review floor", "kind": "dhcp", "network": "10.77.0.0/24",
		"gateway": "10.77.0.20", "dhcpStart": "10.77.0.100", "dhcpEnd": "10.77.0.200"}, 422)

	wan := id(e.call("POST", "/segments", map[string]any{"name": "Review WAN", "kind": "cloud", "network": "203.0.113.8/32"}, 200))
	e.call("POST", "/network-links", link("Review ILL", "ill", map[string]any{"wanIp": "203.0.113.8", "wanSegmentId": wan}), 201)
	got := e.call("DELETE", "/segments/"+wan, nil, 409)
	if msg := got["error"].(map[string]any)["message"].(string); msg == "" || msg[:13] != "Network links" {
		t.Fatalf("delete should name the links: %q", msg)
	}
}

// A save that points at something missing is the request's fault (422), not a
// conflict with other records (409).
func TestMissingReferenceIs422(t *testing.T) {
	e := newAPI(t)
	e.call("POST", "/network-links", link("Ghost provider", "ill", map[string]any{"providerId": "00000000-0000-4000-8000-000000000001"}), 422)
}

// Exports are audited.
func TestExportsAreAudited(t *testing.T) {
	e := newAPI(t)
	e.call("GET", "/devices/export?format=csv", nil, 200)
	var n int
	if err := e.tx.QueryRow(e.ctx, "SELECT count(*) FROM audit_logs WHERE action = 'export' AND object_type = 'devices'").Scan(&n); err != nil || n != 1 {
		t.Fatalf("want one export audit row, got %d (%v)", n, err)
	}
}

// Converting the middle of a hold leaves two holds, each audited, and a reminder
// already sent stays sent.
func TestConvertKeepsTheRestAudited(t *testing.T) {
	e := newAPI(t)
	seg := id(e.call("POST", "/segments", map[string]any{"name": "Review servers", "kind": "server", "network": "10.78.0.0/24", "gateway": "10.78.0.1"}, 200))
	hold := id(e.call("POST", "/reservations", map[string]any{"segmentId": seg, "startIp": "10.78.0.2", "endIp": "10.78.0.20", "reason": "Racks"}, 201))
	if _, err := e.tx.Exec(e.ctx, "UPDATE reservations SET reminder_sent_at = now() WHERE id = $1::uuid", hold); err != nil {
		t.Fatal(err)
	}
	e.call("POST", "/reservations/"+hold+"/convert", map[string]any{"ip": "10.78.0.10", "name": "rack-10"}, 200)
	var audited, reminded int
	if err := e.tx.QueryRow(e.ctx, `SELECT
			(SELECT count(*) FROM audit_logs a JOIN reservations r ON r.id::text = a.object_id WHERE a.object_type = 'reservation' AND a.action = 'create' AND r.segment_id = $1::uuid AND r.status = 'active'),
			(SELECT count(*) FROM reservations WHERE segment_id = $1::uuid AND status = 'active' AND reminder_sent_at IS NOT NULL)`, seg).Scan(&audited, &reminded); err != nil {
		t.Fatal(err)
	}
	if audited != 2 || reminded != 2 {
		t.Fatalf("want 2 audited holds with the reminder kept, got %d audited, %d reminded", audited, reminded)
	}
}
