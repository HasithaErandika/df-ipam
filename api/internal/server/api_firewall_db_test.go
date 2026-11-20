package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"dfipam/api/internal/integrations/fortigate"
)

// fakeFortiGate answers like FortiOS: one WAN interface and one site-to-site
// tunnel over it. gateway can change, to show a difference on review.
func fakeFortiGate(t *testing.T, gateway *atomic.Value) *httptest.Server {
	answers := map[string]string{
		"/api/v2/monitor/system/status":            `{"version":"v7.4.3","results":{"hostname":"fw-core"}}`,
		"/api/v2/monitor/system/interface":         `{"results":{"port1":{"name":"port1","alias":"Dialog ILL","ip":"203.0.113.2","mask":30,"link":true,"speed":1000,"rx_bytes":1000,"tx_bytes":500}}}`,
		"/api/v2/monitor/vpn/ipsec":                `{"results":[{"name":"HO-Partner","rgwy":"198.51.100.4","incoming_bytes":10,"outgoing_bytes":20,"proxyid":[{"status":"up"}]}]}`,
		"/api/v2/monitor/virtual-wan/health-check": `{"results":{"Internet":{"port1":{"status":"up","latency":4.2,"jitter":0.3,"packet_loss":0}}}}`,
		"/api/v2/monitor/system/resource/usage":    `{"results":{"cpu":[{"current":7}],"mem":[{"current":41}],"session":[{"current":1200}]}}`,
		"/api/v2/cmdb/system/interface": `{"results":[{"name":"port1","alias":"Dialog ILL","role":"wan","type":"physical","mode":"static","ip":"203.0.113.2 255.255.255.252","estimated-downstream-bandwidth":200000,"estimated-upstream-bandwidth":200000,"status":"up","vdom":"root"},
			{"name":"internal","role":"lan","type":"hard-switch","mode":"static","ip":"192.168.1.1 255.255.255.0"}]}`,
		"/api/v2/cmdb/vpn.ipsec/phase1-interface": `{"results":[{"name":"HO-Partner","interface":"port1","remote-gw":"198.51.100.4","type":"static"}]}`,
		"/api/v2/cmdb/vpn.ipsec/phase2-interface": `{"results":[{"phase1name":"HO-Partner","src-subnet":"192.168.1.0 255.255.255.0","dst-subnet":"10.20.0.0 255.255.0.0"}]}`,
		"/api/v2/cmdb/system.dhcp/server": `{"results":[{"interface":"internal","status":"enable","ip-range":[{"start-ip":"192.168.1.100","end-ip":"192.168.1.200"}],
			"reserved-address":[{"ip":"192.168.1.20","mac":"00:11:22:33:44:55","description":"Printer HR"}]}]}`,
		"/api/v2/cmdb/firewall/address": `{"results":[{"name":"SRV-DB01","type":"ipmask","subnet":"192.168.1.10 255.255.255.255","comment":"Core DB"},
			{"name":"SSLVPN_TUNNEL_ADDR1","type":"iprange","start-ip":"10.212.134.200","end-ip":"10.212.134.210"},{"name":"all","type":"ipmask","subnet":"0.0.0.0 0.0.0.0"}]}`,
		"/api/v2/cmdb/firewall/vip":         `{"results":[{"name":"WEB-PUBLIC","extip":"203.0.113.5","mappedip":[{"range":"192.168.1.11"}]}]}`,
		"/api/v2/cmdb/vpn.ssl/settings":     `{"results":{"tunnel-ip-pools":[{"name":"SSLVPN_TUNNEL_ADDR1"}]}}`,
		"/api/v2/monitor/vpn/ssl":           `{"results":[{"user_name":"nimal","remote_host":"198.51.100.9","subsessions":[{"aip":"10.212.134.201"}]}]}`,
		"/api/v2/monitor/system/arp":        `{"results":[{"ip":"192.168.1.30","mac":"aa:bb:cc:00:00:01"}]}`,
		"/api/v2/monitor/system/dhcp":       `{"results":[]}`,
		"/api/v2/monitor/user/device/query": `{"results":[{"mac":"aa:bb:cc:00:00:01","ipv4_address":"192.168.1.30","hostname":"HR-PC-07","os_name":"Windows","os_version":"11","hardware_type":"Windows PC"}]}`,
	}
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.Header.Get("Authorization") != "Bearer tok" {
			t.Errorf("only GET with the token may reach the firewall: %s %s", r.Method, r.URL)
			w.WriteHeader(http.StatusForbidden)
			return
		}
		if r.URL.Path == "/api/v2/cmdb/router/static" {
			_, _ = w.Write([]byte(`{"results":[{"dst":"0.0.0.0 0.0.0.0","gateway":"` + gateway.Load().(string) + `","device":"port1"}]}`))
			return
		}
		body, ok := answers[r.URL.Path]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv
}

type fakeFirewall struct{ url string }

func (f fakeFirewall) Connect(context.Context) (*fortigate.Client, error) {
	return fortigate.NewClient(f.url, "tok", "", false)
}

func TestFirewallLinksAgainstDatabase(t *testing.T) {
	var gw atomic.Value
	gw.Store("203.0.113.1")
	srv := fakeFortiGate(t, &gw)
	e := newAPI(t, func(d *Deps) { d.LinkFirewall = fakeFirewall{srv.URL} })
	floor := id(e.call("POST", "/segments", map[string]any{"name": "Floor 1", "kind": "dhcp", "network": "192.168.1.0/24", "dhcpStart": "192.168.1.100", "dhcpEnd": "192.168.1.200"}, 200))

	// Review: nothing is saved, and only the WAN interface and the tunnel are proposed.
	review := e.call("GET", "/firewall/links/review", nil, 200)["data"].(map[string]any)
	props := review["proposals"].([]any)
	if len(props) != 2 || total(e.call("GET", "/network-links", nil, 200)) != 0 {
		t.Fatalf("want two proposals and no links yet: %v", props)
	}
	port1 := props[0].(map[string]any)["suggestion"].(map[string]any)
	if port1["wanIp"] != "203.0.113.2" || port1["gatewayIp"] != "203.0.113.1" || port1["downMbps"].(float64) != 200 {
		t.Fatalf("interface proposal: %v", port1)
	}
	tunnel := props[1].(map[string]any)["suggestion"].(map[string]any)
	if segs := tunnel["localSegments"].([]any); len(segs) != 1 || segs[0].(map[string]any)["id"] != floor {
		t.Fatalf("the tunnel's local subnet is the floor segment: %v", tunnel)
	}

	// Confirm both. The tunnel is listed first and still finds its line.
	business := map[string]any{"sourceSite": "Head Office", "purpose": "From the firewall", "linkRole": "primary", "criticality": "high"}
	items := []map[string]any{{"key": "ipsec:HO-Partner", "action": "add"}, {"key": "interface:port1", "action": "add", "name": "HO Primary ILL"}, {"key": "interface:gone", "action": "add"}}
	for _, it := range items {
		for k, v := range business {
			it[k] = v
		}
	}
	res := e.call("POST", "/firewall/links/apply", map[string]any{"items": items}, 200)["data"].(map[string]any)["results"].([]any)
	byKey := map[string]map[string]any{}
	for _, r := range res {
		byKey[r.(map[string]any)["key"].(string)] = r.(map[string]any)
	}
	if !byKey["interface:port1"]["ok"].(bool) || !byKey["ipsec:HO-Partner"]["ok"].(bool) || byKey["interface:gone"]["error"] != "It is no longer on the firewall. Fetch again." {
		t.Fatalf("results: %v", res)
	}
	vpnID := byKey["ipsec:HO-Partner"]["linkId"].(string)
	if rel := e.call("GET", "/network-links/"+vpnID, nil, 200)["data"].(map[string]any)["related"].([]any); len(rel) != 1 || rel[0].(map[string]any)["kind"] != "underlying" {
		t.Fatalf("the tunnel runs over the ILL: %v", rel)
	}

	// Live: rates need two samples; the interface and tunnel are matched to their links.
	live := e.call("GET", "/firewall/live", nil, 200)["data"].(map[string]any)
	iface := live["interfaces"].([]any)[0].(map[string]any)
	if iface["link"] == nil || live["tunnels"].([]any)[0].(map[string]any)["link"] == nil || live["cpu"].(float64) != 7 {
		t.Fatalf("live view: %v", live)
	}

	// The firewall changes: the review shows the difference, and an update applies only what is ticked.
	gw.Store("203.0.113.9")
	again := e.call("GET", "/firewall/links/review", nil, 200)["data"].(map[string]any)["proposals"].([]any)[0].(map[string]any)
	if again["existing"] == nil || !strings.Contains(mustJSONString(again["changes"]), "203.0.113.9") {
		t.Fatalf("want the new gateway as a change: %v", again)
	}
	e.call("POST", "/firewall/links/apply", map[string]any{"items": []map[string]any{{"key": "interface:port1", "action": "update", "fields": []string{"gatewayIp"}}}}, 200)
	if got := e.call("GET", "/network-links?ip=203.0.113.2", nil, 200)["data"].([]any)[0].(map[string]any); got["gatewayIp"] != "203.0.113.9" || got["name"] != "HO Primary ILL" {
		t.Fatalf("after update: %v", got)
	}
}

func mustJSONString(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

func TestFirewallInventoryAgainstDatabase(t *testing.T) {
	var gw atomic.Value
	gw.Store("203.0.113.1")
	srv := fakeFortiGate(t, &gw)
	e := newAPI(t, func(d *Deps) { d.LinkFirewall = fakeFirewall{srv.URL} })
	inv := e.call("GET", "/firewall/inventory", nil, 200)["data"].(map[string]any)
	nets := map[string]map[string]any{}
	for _, n := range inv["networks"].([]any) {
		nets[n.(map[string]any)["key"].(string)] = n.(map[string]any)
	}
	if lan := nets["net:internal"]; lan == nil || lan["network"] != "192.168.1.0/24" || lan["kind"] != "dhcp" || lan["dhcpStart"] != "192.168.1.100" || lan["canAdd"] != true {
		t.Fatalf("the LAN with its DHCP pool: %v", nets)
	}
	if ssl := nets["vpn:ssl:SSLVPN_TUNNEL_ADDR1"]; ssl == nil || ssl["kind"] != "vpn" || ssl["network"] != "10.212.134.192/27" {
		t.Fatalf("the SSL VPN pool as the network holding it: %v", nets)
	}
	if _, wan := nets["net:port1"]; wan {
		t.Fatal("a WAN interface is a network link, not a segment")
	}
	dev := inv["devices"].([]any)[0].(map[string]any)
	if dev["hostname"] != "HR-PC-07" || dev["platform"] != "windows" || dev["deviceStatus"] != "none" {
		t.Fatalf("device: %v", dev)
	}
	servers := map[string]map[string]any{}
	for _, s := range inv["servers"].([]any) {
		servers[s.(map[string]any)["ip"].(string)] = s.(map[string]any)
	}
	if len(servers) != 3 || servers["192.168.1.10"]["name"] != "SRV-DB01" || servers["192.168.1.20"]["mac"] != "00:11:22:33:44:55" || servers["192.168.1.11"] == nil {
		t.Fatalf("an address object, a virtual IP and a reservation: %v", servers)
	}
	live := e.call("GET", "/firewall/live", nil, 200)["data"].(map[string]any)
	if users := live["vpnUsers"].([]any); len(users) != 1 || users[0].(map[string]any)["user"] != "nimal" {
		t.Fatalf("VPN users: %v", live["vpnUsers"])
	}
}
