package integrations

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"golang.org/x/time/rate"
)

// FortiOS responses follow the published API shapes. They are not recordings from
// a real device, which is still to be checked: see docs/integrations.md.
var fortiFixtures = map[string]string{
	"/api/v2/monitor/system/status": `{"serial":"FGT60F0000000001","version":"v7.2.5","results":{"hostname":"edge-fw","model_name":"FortiGate","model_number":"60F"}}`,
	"/api/v2/cmdb/system/ha":        `{"results":{"mode":"standalone","group-name":""}}`,
	"/api/v2/cmdb/system/vdom":      `{"results":[{"name":"root"},{"name":"dmz"}]}`,
	"/api/v2/cmdb/system/interface": `{"results":[
	  {"name":"port1","vdom":"root","ip":"10.0.0.1 255.255.255.0","type":"physical","status":"up","alias":"core","macaddr":"00:09:0f:aa:00:01","vlanid":0,
	   "secondaryip":[{"ip":"10.0.1.1 255.255.255.0"}]},
	  {"name":"port2","vdom":"dmz","ip":"10.0.0.1 255.255.255.0","type":"physical","status":"down","macaddr":"00:09:0f:aa:00:02"},
	  {"name":"tun0","vdom":"root","ip":"0.0.0.0 0.0.0.0","type":"tunnel","status":"up","macaddr":"00:00:00:00:00:00"}]}`,
	"/api/v2/cmdb/firewall/address": `{"results":[
	  {"name":"lan","type":"ipmask","subnet":"10.0.0.0 255.255.255.0","comment":"LAN"},
	  {"name":"pool","type":"iprange","start-ip":"10.0.0.10","end-ip":"10.0.0.20"},
	  {"name":"web","type":"fqdn","fqdn":"web.example.com"}]}`,
	"/api/v2/cmdb/firewall/addrgrp":   `{"results":[{"name":"all-lan","comment":"","member":[{"name":"lan"},{"name":"pool"}]}]}`,
	"/api/v2/cmdb/router/static":      `{"results":[{"dst":"0.0.0.0 0.0.0.0","gateway":"10.0.0.254","device":"port1"}]}`,
	"/api/v2/monitor/network/arp":     `{"results":[{"ip":"10.0.0.50","mac":"00:11:22:33:44:55","interface":"port1"}]}`,
	"/api/v2/cmdb/system.dhcp/server": `{"results":[{"id":1,"interface":"port1","ip-range":[{"start-ip":"10.0.0.100","end-ip":"10.0.0.200"}]}]}`,
	"/api/v2/monitor/system/dhcp": `{"results":[{"ip":"10.0.0.101","mac":"00:11:22:33:44:66","hostname":"laptop-1","interface":"port1","status":"leased"},
	  {"ip":"10.0.0.102","mac":"00:11:22:33:44:77","interface":"port1","status":"expired"}]}`,
}

type fortiServer struct {
	*httptest.Server
	mu       sync.Mutex
	methods  []string
	override map[string]func(http.ResponseWriter)
}

func newFortiServer(t *testing.T) (*fortiServer, *fortiGate) {
	t.Helper()
	s := &fortiServer{override: map[string]func(http.ResponseWriter){}}
	s.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		s.methods = append(s.methods, r.Method)
		fn := s.override[r.URL.Path]
		s.mu.Unlock()
		if r.Header.Get("Authorization") != "Bearer secret-token" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if fn != nil {
			fn(w)
			return
		}
		body, ok := fortiFixtures[r.URL.Path]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		fmt.Fprint(w, body)
	}))
	t.Cleanup(s.Close)
	base, _ := url.Parse(s.URL)
	return s, &fortiGate{base, "secret-token", s.Client(), rate.NewLimiter(rate.Inf, 1), time.Millisecond}
}

func TestFortiGateCollect(t *testing.T) {
	s, f := newFortiServer(t)
	r, err := f.Collect(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range s.methods {
		if m != http.MethodGet {
			t.Fatalf("connector sent %s", m)
		}
	}
	if len(r.Warnings) != 0 {
		t.Fatalf("warnings: %+v", r.Warnings)
	}
	if len(r.Devices) != 1 || r.Devices[0].ExternalID != "FGT60F0000000001" || r.Devices[0].DeviceType != "firewall" || len(r.Devices[0].VDOMs) != 2 {
		t.Fatalf("device: %+v", r.Devices)
	}
	// The same IP in two VDOMs stays two observations with their own context.
	var contexts []string
	for _, o := range r.Observations {
		if o.Address == "10.0.0.1" {
			contexts = append(contexts, o.Context)
		}
	}
	slices.Sort(contexts)
	if strings.Join(contexts, ",") != "vdom:dmz,vdom:root" {
		t.Fatalf("contexts: %v", contexts)
	}
	byAddr := map[string]Observation{}
	for _, o := range r.Observations {
		byAddr[o.Context+o.Address] = o
	}
	if o := byAddr["vdom:root10.0.1.1"]; o.Payload["secondary"] != true {
		t.Errorf("secondary IP: %+v", o)
	}
	if byAddr["vdom:dmz10.0.0.1"].Alive {
		t.Error("an interface that is down counted as alive")
	}
	if _, ok := byAddr["vdom:root0.0.0.0"]; ok {
		t.Error("an unaddressed tunnel produced evidence")
	}
	if !byAddr["vdom:root10.0.0.50"].Alive || byAddr["vdom:root10.0.0.101"].Source != SourceDHCP || byAddr["vdom:root10.0.0.102"].Alive {
		t.Error("ARP or DHCP evidence is wrong")
	}
	if len(r.AddressObjects) != 6 || r.AddressObjects[0].Network != "10.0.0.0/24" || r.AddressObjects[1].Value != "10.0.0.10-10.0.0.20" {
		t.Errorf("address objects: %+v", r.AddressObjects)
	}
	if len(r.Routes) != 2 || r.Routes[0].Destination != "0.0.0.0/0" || len(r.DHCPScopes) != 2 || len(r.AddressGroups) != 2 {
		t.Errorf("routes %+v scopes %+v groups %+v", r.Routes, r.DHCPScopes, r.AddressGroups)
	}
}

func TestFortiGateHAMembersShareOneDevice(t *testing.T) {
	s, f := newFortiServer(t)
	s.override["/api/v2/cmdb/system/ha"] = func(w http.ResponseWriter) { fmt.Fprint(w, `{"results":{"mode":"a-p","group-name":"edge"}}`) }
	r, err := f.Collect(context.Background())
	if err != nil || r.Devices[0].ExternalID != "ha:edge" {
		t.Fatalf("%v %+v", err, r)
	}
}

func TestFortiGateMissingTableIsAWarning(t *testing.T) {
	s, f := newFortiServer(t)
	s.override["/api/v2/cmdb/firewall/address"] = func(w http.ResponseWriter) { w.WriteHeader(http.StatusForbidden) }
	r, err := f.Collect(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Warnings) != 2 || r.Warnings[0].Code != "ADDRESSES_UNAVAILABLE" {
		t.Fatalf("warnings: %+v", r.Warnings)
	}
}
