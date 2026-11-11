package fortigate

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func fakeFirewall(t *testing.T, token string) *httptest.Server {
	answers := map[string]string{
		"/api/v2/monitor/system/status": `{"version":"v7.4.3","serial":"FG100F","results":{"hostname":"fw-core"}}`,
		"/api/v2/monitor/system/arp":    `{"results":[{"ip":"192.168.1.20","mac":"00:11:22:33:44:55"},{"ip":"fe80::1","mac":"00:11:22:33:44:56"}]}`,
		"/api/v2/monitor/system/dhcp":   `{"results":[{"ip":"192.168.1.120","mac":"aa:bb:cc:dd:ee:01","hostname":"LAPTOP-7","vci":"MSFT 5.0","status":"leased"}]}`,
		"/api/v2/monitor/vpn/ssl":       `{"results":[{"user_name":"nimal","remote_host":"203.0.113.9","subsessions":[{"aip":"10.212.134.200"}]}]}`,
		"/api/v2/monitor/vpn/ipsec":     `{"results":[{"name":"dialup_0","rgwy":"198.51.100.4","xauth_user":"kamal","proxyid":[{"status":"up","proxy_dst":[{"subnet":"10.10.10.5-10.10.10.5"}]}]}]}`,
	}
	return httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("DF-IPAM sent %s: it must only read", r.Method)
		}
		if r.Header.Get("Authorization") != "Bearer "+token {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		body, ok := answers[r.URL.Path]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_, _ = w.Write([]byte(body))
	}))
}

func TestReadAll(t *testing.T) {
	srv := fakeFirewall(t, "good")
	defer srv.Close()
	c, err := NewClient(srv.URL, "good", "root", false)
	if err != nil {
		t.Fatal(err)
	}
	name, err := c.Status(context.Background())
	if err != nil || name != "fw-core v7.4.3" {
		t.Fatalf("status: %q %v", name, err)
	}
	entries, counts, err := c.ReadAll(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if counts != (Counts{ARP: 1, DHCP: 1, VPN: 2}) {
		t.Fatalf("counts %+v: IPv6 ARP must be skipped", counts)
	}
	var lease, vpn bool
	for _, e := range entries {
		lease = lease || (e.Source == "dhcp" && e.Hostname == "LAPTOP-7" && e.Vendor == "MSFT 5.0")
		vpn = vpn || (e.Source == "vpn" && e.IP == "10.10.10.5" && strings.Contains(e.Hostname, "kamal"))
	}
	if !lease || !vpn {
		t.Fatalf("missing lease or IPsec client: %+v", entries)
	}
}

func TestRefusals(t *testing.T) {
	if _, err := NewClient("http://10.0.0.1", "t", "", true); err == nil {
		t.Fatal("accepted plain http: the token would travel unencrypted")
	}
	srv := fakeFirewall(t, "good")
	defer srv.Close()
	c, _ := NewClient(srv.URL, "wrong", "", false)
	if _, err := c.Status(context.Background()); err == nil || !strings.Contains(err.Error(), "refused the API token") {
		t.Fatalf("a bad token should say so: %v", err)
	}
	strict, _ := NewClient(srv.URL, "good", "", true)
	if _, err := strict.Status(context.Background()); err == nil {
		t.Fatal("an untrusted certificate was accepted with verification on")
	}
}
