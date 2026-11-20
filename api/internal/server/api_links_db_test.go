package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"dfipam/api/internal/auth"
	"dfipam/api/internal/modules/links"
)

func link(name, kind string, extra map[string]any) map[string]any {
	m := map[string]any{"name": name, "linkType": kind, "sourceSite": "Head Office", "purpose": "Testing", "linkRole": "primary",
		"criticality": "high", "status": "active"}
	for k, v := range extra {
		m[k] = v
	}
	return m
}

func TestNetworkLinksAgainstDatabase(t *testing.T) {
	e := newAPI(t)
	provider := id(e.call("POST", "/link-providers", map[string]any{"name": "Dialog Business", "supportEmail": "noc@dialog.lk"}, 201))
	e.call("POST", "/link-providers", map[string]any{"name": "dialog business"}, 409)
	e.call("POST", "/link-providers", map[string]any{"name": "SLT", "website": "slt.lk"}, 422)

	end := time.Now().AddDate(0, 0, 20).Format(time.DateOnly)
	ill := id(e.call("POST", "/network-links", link("HO Primary ILL", "ill", map[string]any{"providerId": provider, "circuitId": "ILL-778",
		"downMbps": 200, "upMbps": 200, "wanIp": "203.0.113.2", "publicIps": "203.0.113.0/29", "monthlyRental": 250000, "currency": "LKR",
		"contractStart": time.Now().Format(time.DateOnly), "contractEnd": end}), 201))
	e.call("POST", "/network-links", link("Bad", "ill", map[string]any{"wanIp": "2001:db8::1"}), 422)
	e.call("POST", "/network-links", link("Bad", "ill", map[string]any{"sourceSite": "Nowhere"}), 422)
	e.call("POST", "/network-links", link("Dup circuit", "ill", map[string]any{"providerId": provider, "circuitId": "ill-778"}), 409)
	e.call("POST", "/network-links", link("Paid twice", "site_vpn", map[string]any{"noSeparateCost": true, "monthlyRental": 10}), 422)

	// Remote subnets of different VPNs may overlap: they are never local space.
	vpnA := id(e.call("POST", "/network-links", link("Partner A VPN", "site_vpn", map[string]any{"remoteSubnets": []string{"10.0.0.0/16"},
		"remotePeer": "vpn.partner.lk", "noSeparateCost": true, "relations": []map[string]string{{"relatedId": ill, "kind": "underlying"}}}), 201))
	e.call("POST", "/network-links", link("Partner B VPN", "site_vpn", map[string]any{"remoteSubnets": []string{"10.0.0.0/16"}}), 201)
	e.call("PUT", "/network-links/"+vpnA, link("Partner A VPN", "site_vpn", map[string]any{"relations": []map[string]string{{"relatedId": vpnA, "kind": "backup"}}}), 422)
	e.call("PUT", "/network-links/"+ill, link("HO Primary ILL", "ill", map[string]any{"providerId": provider, "circuitId": "ILL-778",
		"relations": []map[string]string{{"relatedId": vpnA, "kind": "backup"}}}), 422)

	if got := e.call("GET", "/network-links?q=ill-778", nil, 200); total(got) != 1 {
		t.Fatalf("search by circuit: %v", got)
	}
	if got := e.call("GET", "/network-links?ip=203.0.113.5", nil, 200); total(got) != 1 {
		t.Fatalf("a public address finds its link: %v", got)
	}
	if got := e.call("GET", "/network-links?vpn=true", nil, 200); total(got) != 2 {
		t.Fatalf("two VPNs: %v", got)
	}
	detail := e.call("GET", "/network-links/"+ill, nil, 200)["data"].(map[string]any)
	if rel := detail["related"].([]any); len(rel) != 1 || rel[0].(map[string]any)["direction"] != "incoming" {
		t.Fatalf("the ILL carries the VPN: %v", rel)
	}

	// Reminders: one notice per reminder day, never twice.
	for i := 0; i < 2; i++ {
		if err := links.QueueReminders(e.ctx, e.tx); err != nil {
			t.Fatal(err)
		}
	}
	var sent int
	if err := e.tx.QueryRow(e.ctx, `SELECT count(*) FROM notifications WHERE kind = 'link_contract_expiry'`).Scan(&sent); err != nil || sent != 1 {
		t.Fatalf("want one reminder, got %d (%v)", sent, err)
	}

	e.call("PUT", "/network-links/"+ill, link("HO Primary ILL", "ill", map[string]any{"providerId": provider, "circuitId": "ILL-778",
		"monthlyRental": 275000, "contractEnd": time.Now().AddDate(1, 0, 0).Format(time.DateOnly)}), 200)
	history := e.call("GET", "/network-links/"+ill+"/history", nil, 200)["data"].([]any)
	if a := history[0].(map[string]any)["action"]; a != "renew" {
		t.Fatalf("a later contract end is a renewal: %v", history[0])
	}

	dash := e.call("GET", "/network-links/dashboard", nil, 200)["data"].(map[string]any)
	if dash["totals"].(map[string]any)["vpn"].(float64) != 2 {
		t.Fatalf("dashboard: %v", dash["totals"])
	}
	e.call("PUT", "/link-settings", map[string]any{"types": []map[string]any{{"key": "other", "label": "Other", "active": true}},
		"sites": []string{"Head Office"}, "reminderDays": []int{30}}, 422)

	e.call("POST", "/network-links/"+vpnA+"/archive", nil, 204)
	e.call("POST", "/network-links/"+vpnA+"/archive", nil, 409)
	e.call("PUT", "/network-links/"+vpnA, link("Partner A VPN", "site_vpn", nil), 409)
	if got := e.call("GET", "/network-links?vpn=true", nil, 200); total(got) != 1 {
		t.Fatalf("an archived link leaves the list: %v", got)
	}
	e.call("POST", "/network-links/"+vpnA+"/restore", nil, 204)

	for _, key := range []string{"network-links", "network-link-costs", "network-link-renewals"} {
		e.call("GET", "/reports/"+key+"?format=pdf&from="+time.Now().Format(time.DateOnly)+"&to="+time.Now().AddDate(1, 0, 0).Format(time.DateOnly), nil, 200)
		e.call("GET", "/reports/"+key+"?format=xlsx", nil, 200)
	}
	e.call("GET", "/network-links/export?format=xlsx", nil, 200)
	e.call("GET", "/network-links/template", nil, 200)
	e.call("POST", "/network-links/"+ill+"/attachments", nil, 503) // no store in tests
	checkImport(t, e)
}

func checkImport(t *testing.T, e *apiEnv) {
	file := "Link ID,Link Name,Link Type,Source Location,Business Purpose,Link Role,Criticality,Service Provider,Circuit ID\n" +
		",Branch broadband,Broadband / Fibre,Head Office,Branch,backup,low,,\n" +
		",HO ILL again,ill,Head Office,Dup,primary,high,Dialog Business,ILL-778\n" +
		"DF-LNK-9999,Ghost,ill,Head Office,x,primary,high,,\n"
	got := upload(t, e, "/network-links/import?dryRun=true", file)
	if got["added"].(float64) != 1 || got["skipped"].(float64) != 2 {
		t.Fatalf("one new, one existing (needs confirming), one unknown ID: %v", got)
	}
	got = upload(t, e, "/network-links/import?update=true", file)
	if got["added"].(float64) != 1 || got["updated"].(float64) != 1 {
		t.Fatalf("confirmed import: %v", got)
	}
}

func upload(t *testing.T, e *apiEnv, path, csv string) map[string]any {
	t.Helper()
	time.Sleep(35 * time.Millisecond)
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	part, _ := w.CreateFormFile("file", "links.csv")
	_, _ = part.Write([]byte(csv))
	_ = w.Close()
	req := httptest.NewRequest(http.MethodPost, "/api/v1"+path, &buf)
	req.AddCookie(&http.Cookie{Name: auth.SessionCookiePlain, Value: "test"})
	req.Header.Set(auth.CSRFHeader, "DF-IPAM")
	req.Header.Set("Content-Type", w.FormDataContentType())
	e.n++
	req.RemoteAddr = fmt.Sprintf("10.8.%d.%d:1000", e.n/250, e.n%250+1)
	rec := httptest.NewRecorder()
	e.router.ServeHTTP(rec, req)
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), "dryRun") {
		t.Fatalf("%s: %d %s", path, rec.Code, rec.Body.String())
	}
	out := map[string]any{}
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	return out["data"].(map[string]any)
}

// DAP_ILL at head office: one link, two public /32 addresses.
func TestLinkWithTwoWANAddresses(t *testing.T) {
	e := newAPI(t)
	dap := id(e.call("POST", "/network-links", link("DAP_ILL", "ill", map[string]any{"wanIp": "122.255.36.14 255.255.255.255",
		"extraWanIps": []string{"122.255.35.1", "122.255.36.14"}, "interface": "WAN 1", "circuitId": "994621202"}), 201))
	l := e.call("GET", "/network-links/"+dap, nil, 200)["data"].(map[string]any)["link"].(map[string]any)
	if l["wanIp"] != "122.255.36.14" || fmt.Sprint(l["extraWanIps"]) != "[122.255.35.1/32]" {
		t.Fatalf("stored: %v %v", l["wanIp"], l["extraWanIps"])
	}
	wan := e.call("GET", "/network-links/"+dap, nil, 200)["data"].(map[string]any)["wanAddresses"].([]any)
	if len(wan) != 2 || wan[0].(map[string]any)["main"] != true || wan[1].(map[string]any)["network"] != "122.255.35.1/32" {
		t.Fatalf("WAN addresses by subnet: %v", wan)
	}
	if got := e.call("GET", "/network-links?ip=122.255.35.1", nil, 200); total(got) != 1 {
		t.Fatalf("the second address finds its link: %v", got)
	}
	if got := e.call("GET", "/network-links?q=122.255.35", nil, 200); total(got) != 1 {
		t.Fatalf("search by the second address: %v", got)
	}
	e.call("PUT", "/network-links/"+dap, link("DAP_ILL", "ill", map[string]any{"extraWanIps": []string{"172,22,60.145"}}), 422)
}

// Devices the scan saw on a link's addresses are offered; a link keeps several,
// and only approved ones can be added.
func TestLinkDevicesFromTheScan(t *testing.T) {
	e := newAPI(t)
	if _, err := e.tx.Exec(e.ctx, `INSERT INTO sightings (ip, mac, source) VALUES
		('10.10.40.253', '00:ee:00:00:00:01', 'arp'), ('10.10.40.254', '00:ee:00:00:00:02', 'arp'), ('10.10.40.1', '00:ee:00:00:00:03', 'arp')`); err != nil {
		t.Fatal(err)
	}
	var fw, cpe, pending string
	if err := e.tx.QueryRow(e.ctx, `WITH d AS (INSERT INTO devices (mac, name, status, decided_at) VALUES
		('00:ee:00:00:00:01', 'FW-HO', 'approved', now()), ('00:ee:00:00:00:02', 'Fintech CPE', 'approved', now()),
		('00:ee:00:00:00:03', '', 'pending', NULL) RETURNING id::text, mac)
		SELECT (SELECT id FROM d WHERE mac = '00:ee:00:00:00:01'), (SELECT id FROM d WHERE mac = '00:ee:00:00:00:02'),
		(SELECT id FROM d WHERE mac = '00:ee:00:00:00:03')`).Scan(&fw, &cpe, &pending); err != nil {
		t.Fatal(err)
	}
	seen := e.call("GET", "/network-links/seen-devices?ip=10.10.40.253&ip=10.10.40.254&ip=nonsense", nil, 200)["data"].([]any)
	if len(seen) != 2 || seen[0].(map[string]any)["name"] != "FW-HO" {
		t.Fatalf("seen: %v", seen)
	}
	vpn := id(e.call("POST", "/network-links", link("Fintech_VPN", "site_vpn", map[string]any{"wanIp": "10.10.40.253 /24",
		"gatewayIp": "10.10.40.254", "deviceIds": []string{fw, cpe, fw}, "deviceText": "Provider switch"}), 201))
	d := e.call("GET", "/network-links/"+vpn, nil, 200)["data"].(map[string]any)
	if l := d["link"].(map[string]any); len(l["deviceIds"].([]any)) != 2 || len(l["deviceNames"].([]any)) != 2 {
		t.Fatalf("devices: %v %v", l["deviceIds"], l["deviceNames"])
	}
	if devs := d["devices"].([]any); len(devs) != 2 || devs[0].(map[string]any)["lastIp"] == nil {
		t.Fatalf("connected devices with where they were seen: %v", devs)
	}
	if s := d["seenDevices"].([]any); len(s) != 2 {
		t.Fatalf("the link page shows what the scan saw: %v", s)
	}
	e.call("PUT", "/network-links/"+vpn, link("Fintech_VPN", "site_vpn", map[string]any{"deviceIds": []string{fw, pending}}), 422)
}
