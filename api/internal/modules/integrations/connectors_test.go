package integrations

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"regexp"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"dfipam/api/internal/platform/safehttp"

	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/network/armnetwork/v6"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/vmware/govmomi/simulator"
	"github.com/vmware/govmomi/vim25"
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

func TestFortiGateRetriesAreBounded(t *testing.T) {
	s, f := newFortiServer(t)
	var calls int
	s.override["/api/v2/monitor/system/status"] = func(w http.ResponseWriter) { calls++; w.WriteHeader(http.StatusTooManyRequests) }
	if _, err := f.Collect(context.Background()); err == nil {
		t.Fatal("expected a failure")
	}
	if calls != fortiRetries {
		t.Fatalf("%d calls", calls)
	}
}

func TestFortiGatePagination(t *testing.T) {
	var starts []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		starts = append(starts, r.URL.Query().Get("start"))
		n := fortiPageSize
		if r.URL.Query().Get("start") != "0" {
			n = 3
		}
		items := make([]map[string]string, n)
		for i := range items {
			items[i] = map[string]string{"name": "x"}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"results": items})
	}))
	defer srv.Close()
	base, _ := url.Parse(srv.URL)
	f := &fortiGate{base, "t", srv.Client(), rate.NewLimiter(rate.Inf, 1), time.Millisecond}
	out, err := fortiList[fortiGroup](context.Background(), f, "/x", "root")
	if err != nil || len(out) != fortiPageSize+3 || len(starts) != 2 {
		t.Fatalf("%d items, %v, %v", len(out), starts, err)
	}
}

func TestErrorsNeverCarryTheToken(t *testing.T) {
	s, f := newFortiServer(t)
	s.override["/api/v2/monitor/system/status"] = func(w http.ResponseWriter) { w.WriteHeader(http.StatusInternalServerError) }
	_, err := f.Collect(context.Background())
	if err == nil || strings.Contains(err.Error(), "secret-token") {
		t.Fatalf("got %v", err)
	}
	f.token = "wrong"
	if err := f.Test(context.Background()); err == nil || strings.Contains(err.Error(), "wrong") {
		t.Fatalf("got %v", err)
	}
}

func TestNewFortiGateAppliesTheOutboundPolicy(t *testing.T) {
	c := Connection{BaseURL: "https://169.254.169.254"}
	if _, err := newFortiGate(c, nil, safehttp.Policy{}); !errors.Is(err, safehttp.ErrBlocked) {
		t.Fatalf("got %v", err)
	}
	if _, err := newVCenter(c, nil, safehttp.Policy{}); !errors.Is(err, safehttp.ErrBlocked) {
		t.Fatalf("got %v", err)
	}
}

func TestSplitIPMask(t *testing.T) {
	a, p, ok := splitIPMask("10.1.2.3 255.255.255.192")
	if !ok || a.String() != "10.1.2.3" || p.Masked().String() != "10.1.2.0/26" {
		t.Fatal(a, p, ok)
	}
	for _, bad := range []string{"", "10.1.2.3", "x 255.0.0.0", "10.0.0.1 255.0.255.0", "10.0.0.1 nonsense"} {
		if _, _, ok := splitIPMask(bad); ok {
			t.Errorf("%q accepted", bad)
		}
	}
}

func TestVCenterAgainstSimulator(t *testing.T) {
	simulator.Test(func(ctx context.Context, c *vim25.Client) {
		r, err := collectVCenter(ctx, c, time.Now())
		if err != nil {
			t.Fatal(err)
		}
		var hosts, vms int
		for _, d := range r.Devices {
			switch d.DeviceType {
			case "physical_server":
				hosts++
			case "virtual_machine":
				vms++
			}
			if !strings.HasPrefix(d.ExternalID, c.ServiceContent.About.InstanceUuid+":") {
				t.Errorf("VM not keyed by vCenter instance: %s", d.ExternalID)
			}
		}
		if hosts == 0 || vms == 0 {
			t.Fatalf("%d hosts, %d VMs", hosts, vms)
		}
	})
}

func TestGuestFamily(t *testing.T) {
	for in, want := range map[[2]string]string{
		{"windows2019srv_64Guest", "Microsoft Windows Server 2019"}: "windows_server",
		{"windows9_64Guest", "Microsoft Windows 10"}:                "windows",
		{"ubuntu64Guest", "Ubuntu Linux"}:                           "linux",
		{"", ""}:                                                    "unknown",
		{"otherGuest", "Other"}:                                     "unknown",
	} {
		if got := guestFamily(in[0], in[1]); got != want {
			t.Errorf("%v: got %s", in, got)
		}
	}
}

type fakeEC2 struct {
	calls []string
}

func (f *fakeEC2) DescribeVpcs(context.Context, *ec2.DescribeVpcsInput, ...func(*ec2.Options)) (*ec2.DescribeVpcsOutput, error) {
	f.calls = append(f.calls, "DescribeVpcs")
	return &ec2.DescribeVpcsOutput{Vpcs: []ec2types.Vpc{{VpcId: aws.String("vpc-1"),
		CidrBlockAssociationSet: []ec2types.VpcCidrBlockAssociation{{CidrBlock: aws.String("10.9.0.0/16")}}}}}, nil
}

func (f *fakeEC2) DescribeNetworkInterfaces(context.Context, *ec2.DescribeNetworkInterfacesInput, ...func(*ec2.Options)) (*ec2.DescribeNetworkInterfacesOutput, error) {
	f.calls = append(f.calls, "DescribeNetworkInterfaces")
	return &ec2.DescribeNetworkInterfacesOutput{NetworkInterfaces: []ec2types.NetworkInterface{
		{NetworkInterfaceId: aws.String("eni-1"), VpcId: aws.String("vpc-1"), Status: ec2types.NetworkInterfaceStatusInUse, MacAddress: aws.String("0a:00:00:00:00:01"),
			PrivateIpAddress: aws.String("10.9.0.10"), PrivateIpAddresses: []ec2types.NetworkInterfacePrivateIpAddress{
				{PrivateIpAddress: aws.String("10.9.0.10"), Association: &ec2types.NetworkInterfaceAssociation{PublicIp: aws.String("203.0.113.5")}}}},
		{NetworkInterfaceId: aws.String("eni-2"), VpcId: aws.String("vpc-1"), Status: ec2types.NetworkInterfaceStatusAvailable,
			PrivateIpAddresses: []ec2types.NetworkInterfacePrivateIpAddress{{PrivateIpAddress: aws.String("10.9.0.11")}}},
	}}, nil
}

func (f *fakeEC2) DescribeRouteTables(context.Context, *ec2.DescribeRouteTablesInput, ...func(*ec2.Options)) (*ec2.DescribeRouteTablesOutput, error) {
	return &ec2.DescribeRouteTablesOutput{RouteTables: []ec2types.RouteTable{{VpcId: aws.String("vpc-1"), Routes: []ec2types.Route{{DestinationCidrBlock: aws.String("0.0.0.0/0")}}}}}, nil
}

func (f *fakeEC2) DescribeVpcPeeringConnections(context.Context, *ec2.DescribeVpcPeeringConnectionsInput, ...func(*ec2.Options)) (*ec2.DescribeVpcPeeringConnectionsOutput, error) {
	return &ec2.DescribeVpcPeeringConnectionsOutput{VpcPeeringConnections: []ec2types.VpcPeeringConnection{
		{Status: &ec2types.VpcPeeringConnectionStateReason{Code: ec2types.VpcPeeringConnectionStateReasonCodeActive},
			RequesterVpcInfo: &ec2types.VpcPeeringConnectionVpcInfo{VpcId: aws.String("vpc-1")}, AccepterVpcInfo: &ec2types.VpcPeeringConnectionVpcInfo{VpcId: aws.String("vpc-2")}},
		{Status: &ec2types.VpcPeeringConnectionStateReason{Code: ec2types.VpcPeeringConnectionStateReasonCodeDeleted},
			RequesterVpcInfo: &ec2types.VpcPeeringConnectionVpcInfo{VpcId: aws.String("vpc-1")}, AccepterVpcInfo: &ec2types.VpcPeeringConnectionVpcInfo{VpcId: aws.String("vpc-3")}},
	}}, nil
}

type failingEC2 struct{ fakeEC2 }

func (failingEC2) DescribeVpcs(context.Context, *ec2.DescribeVpcsInput, ...func(*ec2.Options)) (*ec2.DescribeVpcsOutput, error) {
	return nil, errors.New("access denied")
}

func awsFixture(reader func(string) ec2Reader, regions ...string) *awsConnector {
	return &awsConnector{regions, func(context.Context) (string, error) { return "123456789012", nil }, reader}
}

func TestAWSCollect(t *testing.T) {
	r, err := awsFixture(func(string) ec2Reader { return &fakeEC2{} }, "ap-south-1").Collect(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Networks) != 1 || r.Networks[0].ExternalID != "vpc-1" || r.Networks[0].Region != "ap-south-1" {
		t.Fatalf("networks: %+v", r.Networks)
	}
	if len(r.Peerings) != 1 {
		t.Fatalf("only active peerings count: %+v", r.Peerings)
	}
	by := map[string]Observation{}
	for _, o := range r.Observations {
		by[o.Address] = o
	}
	if !by["10.9.0.10"].Alive || by["10.9.0.10"].Context != "vpc-1" {
		t.Errorf("in-use ENI: %+v", by["10.9.0.10"])
	}
	if by["10.9.0.11"].Alive {
		t.Error("an unattached ENI counted as alive")
	}
	if by["203.0.113.5"].Context != "" {
		t.Error("a public address was placed in the VPC's routing domain")
	}
}

func TestAWSOneBadRegionIsAWarning(t *testing.T) {
	c := awsFixture(func(region string) ec2Reader {
		if region == "bad" {
			return &failingEC2{}
		}
		return &fakeEC2{}
	}, "bad", "good")
	r, err := c.Collect(context.Background())
	if err != nil || len(r.Warnings) != 1 || r.Warnings[0].Context != "bad" || len(r.Networks) != 1 {
		t.Fatalf("%v %+v", err, r)
	}
	all := awsFixture(func(string) ec2Reader { return &failingEC2{} }, "bad")
	if _, err := all.Collect(context.Background()); err == nil {
		t.Fatal("every region failed but the sync succeeded")
	}
}

type fakeAzure struct {
	vnets []*armnetwork.VirtualNetwork
	nics  []*armnetwork.Interface
	nicEr error
}

func (f fakeAzure) virtualNetworks(context.Context) ([]*armnetwork.VirtualNetwork, error) {
	return f.vnets, nil
}
func (f fakeAzure) interfaces(context.Context) ([]*armnetwork.Interface, error) {
	return f.nics, f.nicEr
}

func TestAzureCollect(t *testing.T) {
	sp := func(s string) *string { return &s }
	vnet := "/subscriptions/s/resourceGroups/RG/providers/Microsoft.Network/virtualNetworks/Hub"
	connected, pending := armnetwork.VirtualNetworkPeeringStateConnected, armnetwork.VirtualNetworkPeeringStateInitiated
	f := fakeAzure{vnets: []*armnetwork.VirtualNetwork{{ID: sp(vnet), Name: sp("Hub"), Location: sp("southeastasia"), Properties: &armnetwork.VirtualNetworkPropertiesFormat{
		AddressSpace: &armnetwork.AddressSpace{AddressPrefixes: []*string{sp("10.8.0.0/16")}},
		VirtualNetworkPeerings: []*armnetwork.VirtualNetworkPeering{
			{Properties: &armnetwork.VirtualNetworkPeeringPropertiesFormat{PeeringState: &connected, RemoteVirtualNetwork: &armnetwork.SubResource{ID: sp(vnet + "2")}}},
			{Properties: &armnetwork.VirtualNetworkPeeringPropertiesFormat{PeeringState: &pending, RemoteVirtualNetwork: &armnetwork.SubResource{ID: sp(vnet + "3")}}}}}}},
		nics: []*armnetwork.Interface{
			{ID: sp(vnet + "/nic"), Name: sp("nic1"), Properties: &armnetwork.InterfacePropertiesFormat{MacAddress: sp("00-0D-3A-00-00-01"),
				VirtualMachine: &armnetwork.SubResource{ID: sp("vm1")}, IPConfigurations: []*armnetwork.InterfaceIPConfiguration{{Properties: &armnetwork.InterfaceIPConfigurationPropertiesFormat{
					PrivateIPAddress: sp("10.8.0.4"), Subnet: &armnetwork.Subnet{ID: sp(vnet + "/subnets/App")}}}}}},
			{ID: sp(vnet + "/nic2"), Name: sp("nic2"), Properties: &armnetwork.InterfacePropertiesFormat{IPConfigurations: []*armnetwork.InterfaceIPConfiguration{{Properties: &armnetwork.InterfaceIPConfigurationPropertiesFormat{
				PrivateIPAddress: sp("10.8.0.5"), Subnet: &armnetwork.Subnet{ID: sp(vnet + "/subnets/App")}}}}}},
		}}
	r, err := (&azureConnector{"s", "t", f}).Collect(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Networks) != 1 || r.Networks[0].ResourceGroup != "RG" || len(r.Peerings) != 1 {
		t.Fatalf("%+v %+v", r.Networks, r.Peerings)
	}
	// A NIC's context matches its VNet's, whatever case each API used.
	if r.Observations[0].Context != r.Networks[0].ExternalID {
		t.Fatalf("%q and %q", r.Observations[0].Context, r.Networks[0].ExternalID)
	}
	if r.Observations[0].MAC != "00:0D:3A:00:00:01" || !r.Observations[0].Alive || r.Observations[1].Alive {
		t.Fatalf("%+v", r.Observations)
	}
	f.nicEr = errors.New("forbidden\nrequest dump with headers")
	r, err = (&azureConnector{"s", "t", f}).Collect(context.Background())
	if err != nil || len(r.Warnings) != 1 || strings.Contains(r.Warnings[0].Message, "dump") {
		t.Fatalf("%v %+v", err, r)
	}
}

// Read-only is enforced three ways: the HTTP client has only GET, the cloud clients
// are reached through interfaces listing read calls, and this test fails if any
// connector source mentions a write operation.
func TestConnectorsContainNoWriteCalls(t *testing.T) {
	forbidden := regexp.MustCompile(`http\.Method(Post|Put|Patch|Delete)|\.(Create|Modify|Delete|Update|Authorize|Revoke|Terminate|Run|Start|Stop|Reboot|Attach|Detach|Associate|Allocate|Release|Replace|BeginCreate|BeginDelete|BeginUpdate|PowerOn|PowerOff|Reconfigure|Destroy)[A-Z]?\w*\(`)
	allowed := map[string]bool{"CreateContainerView(": true, "Destroy(": true, "ReplaceAll(": true} // session-local view, changes nothing in vCenter
	for _, file := range []string{"fortigate.go", "vcenter.go", "aws.go", "azure.go"} {
		raw, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		for _, m := range forbidden.FindAllString(string(raw), -1) {
			if !allowed[strings.TrimPrefix(m, ".")] {
				t.Errorf("%s: %s", file, m)
			}
		}
	}
}

// Sync output is evidence. If a statement in the only write path mentions one of
// these columns, this fails.
func TestStoreCannotReachOwnershipOrAllocation(t *testing.T) {
	raw, err := os.ReadFile("store.go")
	if err != nil {
		t.Fatal(err)
	}
	var sql strings.Builder
	for _, m := range regexp.MustCompile("(?s)`[^`]*`").FindAllString(string(raw), -1) {
		sql.WriteString(m)
	}
	for _, column := range []string{"owner_id", "business_service_id", "application_id", "allocation_state", "ip_allocations", "ip_reservations",
		"ip_requests", "site_id", "environment", "sensitivity", "INSERT INTO ip_addresses", "INSERT INTO subnets", "UPDATE subnets"} {
		if strings.Contains(sql.String(), column) {
			t.Errorf("store.go SQL mentions %s", column)
		}
	}
}

func TestContexts(t *testing.T) {
	r := &Result{Observations: []Observation{{Context: "b"}, {Context: "a"}, {Context: ""}}, Networks: []CloudNetwork{{ExternalID: "b"}},
		Devices: []Device{{VDOMs: []VDOM{{Name: "x", Context: "vdom:x"}}}}}
	if got := strings.Join(r.Contexts(), ","); got != "a,b,vdom:x" {
		t.Fatal(got)
	}
}
