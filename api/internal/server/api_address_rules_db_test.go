package server

import (
	"strings"
	"testing"
)

// The register refuses the network and broadcast addresses, DHCP pool
// addresses, and the gateway unless it is registered as network equipment, by
// hand, on approval and by import. Reservations refuse them too.
func TestRegisterAddressRulesAgainstDatabase(t *testing.T) {
	e := newAPI(t)
	seg := id(e.call("POST", "/segments", map[string]any{"name": "Floor 3", "kind": "dhcp", "network": "10.30.0.0/24",
		"gateway": "10.30.0.1", "dhcpStart": "10.30.0.100", "dhcpEnd": "10.30.0.200"}, 200))
	add := func(ip, ipType string, want int) {
		e.call("POST", "/ip-addresses", map[string]any{"segmentId": seg, "ip": ip, "ipType": ipType, "assignedName": "x", "status": "assigned"}, want)
	}
	add("10.30.0.0", "other", 422)   // network
	add("10.30.0.255", "other", 422) // broadcast
	add("10.30.0.150", "other", 422) // DHCP pool
	add("10.30.0.1", "server", 422)  // gateway, not as network equipment
	add("10.30.0.1", "network", 201)
	add("10.30.0.10", "printer", 201)

	dev := id(e.call("POST", "/devices", map[string]any{"mac": "00:ee:00:00:00:01", "name": "Laptop"}, 201))
	e.call("POST", "/devices/"+dev+"/decision", map[string]any{"status": "approved",
		"register": map[string]any{"segmentId": seg, "ip": "10.30.0.120", "ipType": "user_pc"}}, 422)

	e.call("POST", "/reservations", map[string]any{"segmentId": seg, "startIp": "10.30.0.95", "endIp": "10.30.0.105", "reason": "Overlaps pool"}, 422)
	e.call("POST", "/reservations", map[string]any{"segmentId": seg, "startIp": "10.30.0.0", "endIp": "10.30.0.3", "reason": "Network address"}, 422)
	e.call("POST", "/reservations", map[string]any{"segmentId": seg, "startIp": "10.30.0.20", "endIp": "10.30.0.25", "reason": "Fine"}, 201)

	got := e.importIPs("ip,status,assignedname,iptype\n10.30.0.160,assigned,In pool,other\n10.30.0.255,assigned,Broadcast,other\n10.30.0.30,assigned,Static,other\n")
	if got["imported"].(float64) != 1 {
		t.Fatalf("import should register only the static address: %v", got)
	}
	issues := ""
	for _, i := range got["issues"].([]any) {
		issues += i.(map[string]any)["issue"].(string) + "\n"
	}
	if !strings.Contains(issues, "DHCP pool") || !strings.Contains(issues, "broadcast") {
		t.Fatalf("import should say why: %s", issues)
	}
}

// Departments come from the list, in its spelling. Critical with Monitor blank
// imports as monitored.
func TestDepartmentsAndImportCriticalAgainstDatabase(t *testing.T) {
	e := newAPI(t)
	e.call("POST", "/departments", map[string]any{"name": "Treasury"}, 200)
	seg := id(e.call("POST", "/segments", map[string]any{"name": "Servers", "kind": "server", "network": "10.31.0.0/24"}, 200))
	e.call("POST", "/devices", map[string]any{"mac": "00:ee:00:00:00:02", "name": "PC", "department": "Nowhere"}, 422)
	dev := id(e.call("POST", "/devices", map[string]any{"mac": "00:ee:00:00:00:03", "name": "PC", "department": "treasury", "owner": "Nimal"}, 201))
	var dept string
	if err := e.tx.QueryRow(e.ctx, "SELECT department FROM devices WHERE id = $1", dev).Scan(&dept); err != nil || dept != "Treasury" {
		t.Fatalf("department stored as %q (%v), want the list's spelling", dept, err)
	}
	// Approving with owner and department left out keeps them; given blank clears them.
	e.call("POST", "/devices/"+dev+"/decision", map[string]any{"status": "approved"}, 204)
	e.call("POST", "/devices/"+dev+"/decision", map[string]any{"status": "approved", "owner": ""}, 204)
	var owner string
	if err := e.tx.QueryRow(e.ctx, "SELECT owner, department FROM devices WHERE id = $1", dev).Scan(&owner, &dept); err != nil || owner != "" || dept != "Treasury" {
		t.Fatalf("owner %q department %q (%v)", owner, dept, err)
	}

	got := e.importIPs("ip,status,assignedname,department,critical\n10.31.0.5,assigned,Core,Treasury,yes\n10.31.0.6,assigned,Other,Nowhere,\n")
	if got["imported"].(float64) != 1 {
		t.Fatalf("import: %v", got)
	}
	var monitor, critical bool
	if err := e.tx.QueryRow(e.ctx, "SELECT monitor, critical FROM ip_addresses WHERE ip = '10.31.0.5' AND segment_id = $1", seg).Scan(&monitor, &critical); err != nil || !monitor || !critical {
		t.Fatalf("monitor %v critical %v (%v), want both on", monitor, critical, err)
	}
}
