package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"dfipam/api/internal/auth"
	"dfipam/api/internal/modules/monitoring"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
)

// apiEnv drives the real router against a migrated database as an Admin, inside
// one transaction that is rolled back. It runs every query the API makes, under
// the app role's grants. Set TEST_APP_DATABASE_URL (make test-db), or it skips.
type apiEnv struct {
	t      *testing.T
	ctx    context.Context
	tx     pgx.Tx
	router *gin.Engine
	n      int
}

type adminResolver struct{ id string }

func (a adminResolver) Resolve(context.Context, auth.Identity) (*auth.Principal, error) {
	return auth.NewPrincipal(a.id, "admin@example.com", auth.RoleAdmin), nil
}

func newAPI(t *testing.T) *apiEnv {
	url := os.Getenv("TEST_APP_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_APP_DATABASE_URL is not set")
	}
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close(ctx) })
	tx, err := conn.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { tx.Rollback(ctx) })
	var uid string
	if err := tx.QueryRow(ctx, `INSERT INTO users (email, auth_provider, auth_subject, role) VALUES ('admin@example.com', 'test', 'admin', 'admin') RETURNING id`).Scan(&uid); err != nil {
		t.Fatal(err)
	}
	gin.SetMode(gin.TestMode)
	r, _ := New(Deps{Conn: tx, Verifier: stubVerifier{}, Resolver: adminResolver{uid}})
	return &apiEnv{t: t, ctx: ctx, tx: tx, router: r}
}

// call sends one request and checks the status. It paces itself under the
// per-user rate limit.
func (e *apiEnv) call(method, path string, body any, want int) map[string]any {
	e.t.Helper()
	time.Sleep(35 * time.Millisecond)
	var buf bytes.Buffer
	if body != nil {
		_ = json.NewEncoder(&buf).Encode(body)
	}
	req := httptest.NewRequest(method, "/api/v1"+path, &buf)
	req.Header.Set("Authorization", "Bearer token")
	req.Header.Set("Content-Type", "application/json")
	e.n++
	req.RemoteAddr = fmt.Sprintf("10.9.%d.%d:1000", e.n/250, e.n%250+1)
	w := httptest.NewRecorder()
	e.router.ServeHTTP(w, req)
	if w.Code != want {
		e.t.Fatalf("%s %s: got %d, want %d: %s", method, path, w.Code, want, w.Body.String())
	}
	out := map[string]any{}
	if strings.HasPrefix(w.Header().Get("Content-Type"), "application/json") {
		_ = json.Unmarshal(w.Body.Bytes(), &out)
	}
	return out
}

func id(m map[string]any) string { return m["data"].(map[string]any)["id"].(string) }

func total(m map[string]any) float64 { return m["meta"].(map[string]any)["total"].(float64) }

func TestAPIAgainstDatabase(t *testing.T) {
	e := newAPI(t)
	envs := e.call("GET", "/environments", nil, 200)["data"].([]any)
	prod := envs[0].(map[string]any)["id"].(string)

	e.call("POST", "/segments", map[string]any{"name": "Floor 1", "kind": "dhcp", "network": "192.168.1.1/24"}, 422)
	floor := id(e.call("POST", "/segments", map[string]any{"name": "Floor 1", "kind": "dhcp", "floor": 1, "vlan": 10,
		"network": "192.168.1.0/24", "gateway": "192.168.1.1", "dhcpStart": "192.168.1.100", "dhcpEnd": "192.168.1.200"}, 200))
	servers := id(e.call("POST", "/segments", map[string]any{"name": "Prod servers", "kind": "server", "network": "172.16.10.0/24",
		"defaultEnvironmentId": prod}, 200))
	e.call("POST", "/segments", map[string]any{"name": "Overlap", "kind": "dhcp", "network": "192.168.0.0/22"}, 409)
	e.call("POST", "/segments", map[string]any{"name": "AWS VPC", "kind": "cloud", "network": "10.50.0.0/24", "defaultEnvironmentId": prod}, 200)
	e.call("POST", "/segments", map[string]any{"name": "Cloud pool", "kind": "cloud", "network": "10.51.0.0/24", "dhcpStart": "10.51.0.10", "dhcpEnd": "10.51.0.20"}, 422)

	watched := id(e.call("POST", "/segments", map[string]any{"name": "Watched floor", "kind": "dhcp", "network": "192.168.9.0/24",
		"dhcpStart": "192.168.9.100", "dhcpEnd": "192.168.9.200", "alertPrivateMacs": true}, 200))
	if _, err := e.tx.Exec(e.ctx, `INSERT INTO sightings (segment_id, ip, mac, source, is_private_mac) VALUES
		($1, '192.168.9.150', '02:aa:bb:cc:dd:01', 'arp', true), ($2, '192.168.1.150', '02:aa:bb:cc:dd:02', 'arp', true)`, watched, floor); err != nil {
		t.Fatal(err)
	}
	// Evidence as discovery would record it: a static PC on the floor, an unknown server IP.
	if _, err := e.tx.Exec(e.ctx, `INSERT INTO sightings (segment_id, ip, mac, hostname, source, is_static) VALUES
		($1, '192.168.1.20', '00:11:22:33:44:55', 'pc-20', 'netbios', true),
		($2, '172.16.10.9', '00:11:22:33:44:66', '', 'ping', true)`, floor, servers); err != nil {
		t.Fatal(err)
	}
	if err := monitoring.New(e.tx).Evaluate(e.ctx); err != nil {
		t.Fatal(err)
	}
	if n := total(e.call("GET", "/alerts", nil, 200)); n < 3 {
		t.Fatalf("want unknown device, static and server alerts, got %v", n)
	}
	if n := total(e.call("GET", "/alerts?type=private_mac_device", nil, 200)); n != 1 {
		t.Fatalf("want one private-MAC alert, on the watched floor only; got %v", n)
	}
	for _, k := range []string{"", "unregistered", "registered", "static", "unknown", "rejected", "private", "alerting"} {
		e.call("GET", "/discovery/findings?seen=all&kind="+k, nil, 200)
	}
	for _, s := range []string{"", "unregistered", "registered", "static", "unknown"} {
		e.call("GET", "/segments/"+floor+"/addresses?state="+s, nil, 200)
	}
	if _, err := e.tx.Exec(e.ctx, `INSERT INTO sightings (segment_id, ip, mac, source, first_seen, last_seen)
		VALUES ($1, '192.168.1.60', '00:11:22:33:44:60', 'arp', now() - interval '2 days', now() - interval '1 day')`, floor); err != nil {
		t.Fatal(err)
	}
	connected := total(e.call("GET", "/discovery/findings", nil, 200))
	if all := total(e.call("GET", "/discovery/findings?seen=all", nil, 200)); all != connected+1 {
		t.Fatalf("a device that left must drop out of the default view: %v connected, %v in all", connected, all)
	}
	e.call("GET", "/discovery/findings?kind=bogus", nil, 422)
	e.call("GET", "/discovery/status", nil, 503)

	cand := e.call("GET", "/ip-addresses/available-candidate?segmentId="+floor, nil, 200)["data"].(map[string]any)["ip"]
	if cand != "192.168.1.2" {
		t.Fatalf("candidate %v: must skip the gateway", cand)
	}
	e.call("GET", "/ip-addresses/check?segmentId="+floor+"&ip=192.168.1.20", nil, 200)
	e.call("POST", "/ip-addresses", map[string]any{"segmentId": floor, "ip": "192.168.1.2", "assignedName": "HR printer", "ipType": "printer"}, 201)
	e.call("POST", "/ip-addresses", map[string]any{"segmentId": floor, "ip": "192.168.1.2", "assignedName": "Again"}, 409)
	e.call("POST", "/ip-addresses", map[string]any{"segmentId": floor, "ip": "10.0.0.1", "assignedName": "Outside"}, 422)
	srv := id(e.call("POST", "/ip-addresses", map[string]any{"segmentId": servers, "ip": "172.16.10.5", "assignedName": "app01", "ipType": "server", "monitor": true, "critical": true}, 201))
	e.call("PUT", "/ip-addresses/"+srv, map[string]any{"assignedName": "app01", "ipType": "server", "monitor": true, "checkPort": 443,
		"platform": "ubuntu", "services": []string{"apache", "mysql", "apache"}}, 204)
	e.call("PUT", "/ip-addresses/"+srv, map[string]any{"assignedName": "app01", "platform": "beos"}, 422)
	e.call("PUT", "/ip-addresses/"+srv, map[string]any{"assignedName": "app01", "services": []string{"telnet"}}, 422)
	e.call("GET", "/ip-addresses?view=servers&q=app", nil, 200)

	devices := e.call("GET", "/devices?status=pending", nil, 200)["data"].([]any)
	dev := devices[0].(map[string]any)["id"].(string)
	e.call("POST", "/devices/"+dev+"/decision", map[string]any{"status": "approved", "name": "Finance PC",
		"register": map[string]any{"segmentId": floor, "ip": "192.168.1.20", "ipType": "user_pc"}}, 204)

	req := id(e.call("POST", "/ip-requests", map[string]any{"segmentId": servers, "name": "db01", "purpose": "Database", "ipType": "server"}, 201))
	approved := e.call("POST", "/ip-requests/"+req+"/decision", map[string]any{"status": "approved"}, 200)["data"].(map[string]any)
	if approved["ip"] == "172.16.10.9" || approved["ip"] == "172.16.10.5" {
		t.Fatalf("approval took an address in use: %v", approved["ip"])
	}
	e.call("POST", "/ip-requests/"+req+"/decision", map[string]any{"status": "rejected", "reason": "late"}, 409)
	e.call("GET", "/ip-requests?status=approved", nil, 200)

	hold := id(e.call("POST", "/reservations", map[string]any{"segmentId": servers, "startIp": "172.16.10.200", "endIp": "172.16.10.210", "reason": "New racks"}, 201))
	e.call("POST", "/reservations", map[string]any{"segmentId": servers, "startIp": "172.16.10.205", "reason": "Overlap"}, 409)
	e.call("PUT", "/reservations/"+hold, map[string]any{"reason": "New racks", "expiresOn": time.Now().AddDate(0, 1, 0).Format(time.DateOnly)}, 204)
	e.call("POST", "/reservations/"+hold+"/convert", map[string]any{"ip": "172.16.10.205", "name": "rack-5"}, 200)
	if n := total(e.call("GET", "/reservations?view=active", nil, 200)); n != 2 {
		t.Fatalf("converting the middle of a range should leave two holds, got %v", n)
	}

	e.call("PUT", "/settings", map[string]any{"emailEnabled": true, "values": map[string]any{"sessionTimeoutMinutes": 45}}, 200)
	e.call("PUT", "/settings", map[string]any{"values": map[string]any{"scanIntervalMinutes": 0}}, 422)
	e.call("GET", "/users?q=admin", nil, 200)
	e.call("GET", "/dashboard", nil, 200)
	e.call("GET", "/alerts/summary", nil, 200)
	e.call("GET", "/notifications", nil, 200)
	e.call("GET", "/notification-preferences", nil, 200)
	if total(e.call("GET", "/audit-logs?q=192.168.1.2", nil, 200)) == 0 {
		t.Fatal("the audit log should find the printer's IP")
	}
	for _, f := range []string{"csv", "xlsx"} {
		e.call("GET", "/audit-logs/export?format="+f, nil, 200)
		e.call("GET", "/ip-addresses/export?format="+f, nil, 200)
		e.call("GET", "/devices/export?format="+f, nil, 200)
	}
	m := e.call("GET", "/segments/"+floor+"/map", nil, 200)["data"].(map[string]any)
	counts := m["counts"].(map[string]any)
	if counts["gateway"] != 1.0 || counts["dhcp_free"] != 100.0 || counts["dhcp_used"] != 1.0 || counts["assigned"] != 2.0 {
		t.Fatalf("address map counts: %v", counts)
	}
	if net := m["network"].(map[string]any); net["mask"] != "255.255.255.0" || net["usableAddresses"] != "254" {
		t.Fatalf("network facts: %v", net)
	}
	free := m["available"].([]any)
	if first := free[0].(map[string]any); first["start"] != "192.168.1.3" || first["end"] != "192.168.1.19" {
		t.Fatalf("free ranges must skip the gateway, register and seen hosts: %v", free[:2])
	}
	e.call("GET", "/integrations/fortigate", nil, 503)
	e.call("GET", "/discovery/findings?kind=online", nil, 200)
	for _, r := range e.call("GET", "/reports", nil, 200)["data"].([]any) {
		key := r.(map[string]any)["key"].(string)
		e.call("GET", "/reports/"+key+"?format=csv", nil, 200)
		e.call("GET", "/reports/"+key+"?format=pdf", nil, 200)
		e.call("GET", "/reports/"+key+"?format=xlsx", nil, 200)
	}
	e.call("DELETE", "/segments/"+floor, nil, 409)
}
