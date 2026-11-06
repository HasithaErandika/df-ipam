package integrations

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"

	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/network/armnetwork/v6"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/vmware/govmomi/simulator"
	"github.com/vmware/govmomi/vim25"
)

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
