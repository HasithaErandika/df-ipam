package integrations

import (
	"context"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
)

func (a *awsConnector) region(ctx context.Context, r *Result, c ec2Reader, account, region string, now time.Time) error {
	if err := collectVPCs(ctx, r, c, account, region); err != nil {
		return err
	}
	if err := collectENIs(ctx, r, c, account, region, now); err != nil {
		return err
	}
	if err := collectRouteTables(ctx, r, c); err != nil {
		r.warn("ROUTES_UNAVAILABLE", region, err)
	}
	if err := collectVPCPeerings(ctx, r, c); err != nil {
		r.warn("PEERINGS_UNAVAILABLE", region, err)
	}
	return nil
}

func collectVPCs(ctx context.Context, r *Result, c ec2Reader, account, region string) error {
	vpcs := ec2.NewDescribeVpcsPaginator(c, &ec2.DescribeVpcsInput{})
	for vpcs.HasMorePages() {
		page, err := vpcs.NextPage(ctx)
		if err != nil {
			return err
		}
		for _, v := range page.Vpcs {
			r.Networks = append(r.Networks, vpcNetwork(v, account, region))
		}
	}
	return nil
}

func vpcNetwork(v ec2types.Vpc, account, region string) CloudNetwork {
	n := CloudNetwork{AccountExternalID: account, ExternalID: aws.ToString(v.VpcId), Name: tagName(v.Tags), Region: region}
	for _, b := range v.CidrBlockAssociationSet {
		n.AddressSpaces = append(n.AddressSpaces, aws.ToString(b.CidrBlock))
	}
	for _, b := range v.Ipv6CidrBlockAssociationSet {
		n.AddressSpaces = append(n.AddressSpaces, aws.ToString(b.Ipv6CidrBlock))
	}
	return n
}

func collectENIs(ctx context.Context, r *Result, c ec2Reader, account, region string, now time.Time) error {
	enis := ec2.NewDescribeNetworkInterfacesPaginator(c, &ec2.DescribeNetworkInterfacesInput{})
	for enis.HasMorePages() {
		page, err := enis.NextPage(ctx)
		if err != nil {
			return err
		}
		for _, e := range page.NetworkInterfaces {
			collectENI(r, e, account, region, now)
		}
	}
	return nil
}

func collectRouteTables(ctx context.Context, r *Result, c ec2Reader) error {
	routes := ec2.NewDescribeRouteTablesPaginator(c, &ec2.DescribeRouteTablesInput{})
	for routes.HasMorePages() {
		page, err := routes.NextPage(ctx)
		if err != nil {
			return err
		}
		for _, t := range page.RouteTables {
			addRouteTable(r, t)
		}
	}
	return nil
}

func addRouteTable(r *Result, t ec2types.RouteTable) {
	vpc := aws.ToString(t.VpcId)
	device := "routes:" + vpc
	if !hasDevice(r, device) {
		r.Devices = append(r.Devices, Device{ExternalID: device, Name: "VPC router " + vpc, DeviceType: "router", Vendor: "AWS"})
	}
	for _, route := range t.Routes {
		dest := aws.ToString(route.DestinationCidrBlock)
		if dest == "" {
			dest = aws.ToString(route.DestinationIpv6CidrBlock)
		}
		if dest != "" {
			r.Routes = append(r.Routes, Route{Context: vpc, DeviceExternalID: device, Destination: dest, Type: "cloud"})
		}
	}
}

func collectVPCPeerings(ctx context.Context, r *Result, c ec2Reader) error {
	peerings := ec2.NewDescribeVpcPeeringConnectionsPaginator(c, &ec2.DescribeVpcPeeringConnectionsInput{})
	for peerings.HasMorePages() {
		page, err := peerings.NextPage(ctx)
		if err != nil {
			return err
		}
		for _, p := range page.VpcPeeringConnections {
			if p.Status != nil && p.Status.Code == ec2types.VpcPeeringConnectionStateReasonCodeActive && p.RequesterVpcInfo != nil && p.AccepterVpcInfo != nil {
				r.Peerings = append(r.Peerings, Peering{aws.ToString(p.RequesterVpcInfo.VpcId), aws.ToString(p.AccepterVpcInfo.VpcId), "peering"})
			}
		}
	}
	return nil
}
