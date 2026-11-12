package server

import (
	"strings"
	"testing"
)

func TestRolesAgainstDatabase(t *testing.T) {
	e := newAPI(t)
	if n := len(e.call("GET", "/roles", nil, 200)["data"].([]any)); n != 6 {
		t.Fatalf("want the six starting roles, got %d", n)
	}
	e.call("POST", "/roles", map[string]any{"name": "Floor support", "permissions": []string{"inventory:view", "made:up"}}, 422)
	key := e.call("POST", "/roles", map[string]any{"name": "Floor support", "description": "Floors only",
		"permissions": []string{"inventory:view", "inventory:edit"}}, 201)["data"].(map[string]any)["key"].(string)
	if key != "floor_support" {
		t.Fatalf("key %q", key)
	}
	e.call("POST", "/roles", map[string]any{"name": "floor SUPPORT"}, 409)
	e.call("PUT", "/roles/"+key, map[string]any{"name": "Floor support", "permissions": []string{"inventory:view"}}, 200)
	// The full-access role keeps every permission whatever is sent.
	e.call("PUT", "/roles/admin", map[string]any{"name": "Administrator", "permissions": []string{}}, 200)
	var full bool
	if err := e.tx.QueryRow(e.ctx, "SELECT full_access FROM roles WHERE key = 'admin'").Scan(&full); err != nil || !full {
		t.Fatalf("admin must keep full access: %v %v", full, err)
	}
	e.call("DELETE", "/roles/admin", nil, 409)
	e.call("DELETE", "/roles/nobody", nil, 404)

	var me string
	if err := e.tx.QueryRow(e.ctx, "SELECT id FROM users WHERE email = 'admin@example.com'").Scan(&me); err != nil {
		t.Fatal(err)
	}
	e.call("PUT", "/users/"+me, map[string]any{"role": "viewer", "isActive": true}, 409)
	var other string
	if err := e.tx.QueryRow(e.ctx, `INSERT INTO users (email, auth_provider, auth_subject, role) VALUES ('ne@example.com', 'test', 'ne', NULL) RETURNING id`).Scan(&other); err != nil {
		t.Fatal(err)
	}
	e.call("PUT", "/users/"+other, map[string]any{"role": "no_such_role", "isActive": true}, 422)
	e.call("PUT", "/users/"+other, map[string]any{"role": key, "isActive": true}, 204)
	e.call("DELETE", "/roles/"+key, nil, 409)
	e.call("PUT", "/users/"+other, map[string]any{"role": "network_engineer", "isActive": true}, 204)
	e.call("DELETE", "/roles/"+key, nil, 204)
	if got := e.call("GET", "/users?role=network_engineer", nil, 200); total(got) != 1 {
		t.Fatalf("want one network engineer, got %v", total(got))
	}
}

func TestSegmentImportAgainstDatabase(t *testing.T) {
	e := newAPI(t)
	text := strings.Join([]string{
		"FW-01 $ show system interface",
		"Sub_vlan_3   static   0.0.0.0 0.0.0.0  172.20.14.14 255.255.255.248  up   disable   vlan  disable",
		"Tunnel-1     static   0.0.0.0 0.0.0.0  0.0.0.0 0.0.0.0  up   disable   tunnel  disable",
		"Floor 3 192.168.3.0/24",
	}, "\n")
	before := len(e.call("GET", "/segments", nil, 200)["data"].([]any))
	dry := e.call("POST", "/segments/import?dryRun=true", map[string]any{"text": text}, 200)["data"].(map[string]any)
	if dry["added"].(float64) != 2 || dry["skipped"].(float64) != 1 || !dry["dryRun"].(bool) {
		t.Fatalf("dry run: %v", dry)
	}
	if n := len(e.call("GET", "/segments", nil, 200)["data"].([]any)); n != before {
		t.Fatal("a dry run must not keep anything")
	}
	got := e.call("POST", "/segments/import", map[string]any{"text": text, "kind": "server", "skip": []int{4}}, 200)["data"].(map[string]any)
	if got["added"].(float64) != 1 {
		t.Fatalf("want only Sub_vlan_3, the floor left out: %v", got)
	}
	again := e.call("POST", "/segments/import?dryRun=true", map[string]any{"text": text}, 200)["data"].(map[string]any)
	rows := again["rows"].([]any)
	if r := rows[0].(map[string]any); r["add"].(bool) || !strings.HasPrefix(r["reason"].(string), "Already a segment") {
		t.Fatalf("a repeat must be skipped: %v", r)
	}
	e.call("POST", "/segments/import", map[string]any{"text": "  "}, 422)
	for _, k := range []string{"needs", "new", "known"} {
		e.call("GET", "/discovery/findings?seen=all&kind="+k, nil, 200)
	}
	e.call("GET", "/discovery/summary?seen=all", nil, 200)
}
