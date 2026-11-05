package integrations

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/credentials/stscreds"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/aws/aws-sdk-go-v2/service/sts"
)

// ec2Reader is every EC2 call the connector can make. Only Describe calls are listed,
// so write calls aren't reachable from here.
type ec2Reader interface {
	DescribeVpcs(context.Context, *ec2.DescribeVpcsInput, ...func(*ec2.Options)) (*ec2.DescribeVpcsOutput, error)
	DescribeNetworkInterfaces(context.Context, *ec2.DescribeNetworkInterfacesInput, ...func(*ec2.Options)) (*ec2.DescribeNetworkInterfacesOutput, error)
	DescribeRouteTables(context.Context, *ec2.DescribeRouteTablesInput, ...func(*ec2.Options)) (*ec2.DescribeRouteTablesOutput, error)
	DescribeVpcPeeringConnections(context.Context, *ec2.DescribeVpcPeeringConnectionsInput, ...func(*ec2.Options)) (*ec2.DescribeVpcPeeringConnectionsOutput, error)
}

type awsConnector struct {
	regions []string
	account func(context.Context) (string, error)
	reader  func(region string) ec2Reader
}

func splitRegions(v string) []string {
	var out []string
	for _, r := range strings.Split(v, ",") {
		if r = strings.TrimSpace(r); r != "" {
			out = append(out, r)
		}
	}
	return out
}

func newAWS(c Connection, creds map[string]string) (Connector, error) {
	regions := splitRegions(c.Config["regions"])
	if len(regions) == 0 {
		return nil, errors.New("no regions are configured")
	}
	ctx := context.Background()
	opts := []func(*config.LoadOptions) error{config.WithRegion(regions[0]), config.WithRetryMaxAttempts(3)}
	if creds["accessKeyId"] != "" {
		opts = append(opts, config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(creds["accessKeyId"], creds["secretAccessKey"], "")))
	}
	cfg, err := config.LoadDefaultConfig(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("AWS settings: %w", err)
	}
	// Everything below runs as the assumed read-only role, with short-lived credentials.
	external := c.Config["externalId"]
	cfg.Credentials = aws.NewCredentialsCache(stscreds.NewAssumeRoleProvider(sts.NewFromConfig(cfg), c.Config["roleArn"], func(o *stscreds.AssumeRoleOptions) {
		o.ExternalID = &external
		o.RoleSessionName = "df-ipam-read-only"
		o.Duration = 15 * time.Minute
	}))
	identity := sts.NewFromConfig(cfg)
	return &awsConnector{regions,
		func(ctx context.Context) (string, error) {
			out, err := identity.GetCallerIdentity(ctx, &sts.GetCallerIdentityInput{})
			if err != nil {
				return "", err
			}
			return aws.ToString(out.Account), nil
		},
		func(region string) ec2Reader {
			return ec2.NewFromConfig(cfg, func(o *ec2.Options) { o.Region = region })
		},
	}, nil
}

func (a *awsConnector) Test(ctx context.Context) error {
	_, err := a.account(ctx)
	return err
}

func tagName(tags []ec2types.Tag) string {
	for _, t := range tags {
		if aws.ToString(t.Key) == "Name" {
			return aws.ToString(t.Value)
		}
	}
	return ""
}

func (a *awsConnector) Collect(ctx context.Context) (*Result, error) {
	account, err := a.account(ctx)
	if err != nil {
		return nil, err
	}
	r := &Result{Accounts: []CloudAccount{{Provider: "aws", ExternalID: account, Name: account}}}
	now := time.Now()
	failed := 0
	for _, region := range a.regions {
		if err := a.region(ctx, r, a.reader(region), account, region, now); err != nil {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			r.warn("REGION_UNAVAILABLE", region, err)
			failed++
		}
	}
	if failed == len(a.regions) {
		return nil, errors.New("no region could be read: " + r.Warnings[0].Message)
	}
	return r, nil
}

func (a *awsConnector) region(ctx context.Context, r *Result, c ec2Reader, account, region string, now time.Time) error {
	vpcs := ec2.NewDescribeVpcsPaginator(c, &ec2.DescribeVpcsInput{})
	for vpcs.HasMorePages() {
		page, err := vpcs.NextPage(ctx)
		if err != nil {
			return err
		}
		for _, v := range page.Vpcs {
			n := CloudNetwork{AccountExternalID: account, ExternalID: aws.ToString(v.VpcId), Name: tagName(v.Tags), Region: region}
			for _, b := range v.CidrBlockAssociationSet {
				n.AddressSpaces = append(n.AddressSpaces, aws.ToString(b.CidrBlock))
			}
			for _, b := range v.Ipv6CidrBlockAssociationSet {
				n.AddressSpaces = append(n.AddressSpaces, aws.ToString(b.Ipv6CidrBlock))
			}
			r.Networks = append(r.Networks, n)
		}
	}
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
	routes := ec2.NewDescribeRouteTablesPaginator(c, &ec2.DescribeRouteTablesInput{})
	for routes.HasMorePages() {
		page, err := routes.NextPage(ctx)
		if err != nil {
			r.warn("ROUTES_UNAVAILABLE", region, err)
			break
		}
		for _, t := range page.RouteTables {
			device := "routes:" + aws.ToString(t.VpcId)
			if !hasDevice(r, device) {
				r.Devices = append(r.Devices, Device{ExternalID: device, Name: "VPC router " + aws.ToString(t.VpcId), DeviceType: "router", Vendor: "AWS"})
			}
			for _, route := range t.Routes {
				dest := aws.ToString(route.DestinationCidrBlock)
				if dest == "" {
					dest = aws.ToString(route.DestinationIpv6CidrBlock)
				}
				if dest != "" {
					r.Routes = append(r.Routes, Route{Context: aws.ToString(t.VpcId), DeviceExternalID: device, Destination: dest, Type: "cloud"})
				}
			}
		}
	}
	peerings := ec2.NewDescribeVpcPeeringConnectionsPaginator(c, &ec2.DescribeVpcPeeringConnectionsInput{})
	for peerings.HasMorePages() {
		page, err := peerings.NextPage(ctx)
		if err != nil {
			r.warn("PEERINGS_UNAVAILABLE", region, err)
			break
		}
		for _, p := range page.VpcPeeringConnections {
			if p.Status != nil && p.Status.Code == ec2types.VpcPeeringConnectionStateReasonCodeActive && p.RequesterVpcInfo != nil && p.AccepterVpcInfo != nil {
				r.Peerings = append(r.Peerings, Peering{aws.ToString(p.RequesterVpcInfo.VpcId), aws.ToString(p.AccepterVpcInfo.VpcId), "peering"})
			}
		}
	}
	return nil
}

func hasDevice(r *Result, id string) bool {
	for i := len(r.Devices) - 1; i >= 0; i-- {
		if r.Devices[i].ExternalID == id {
			return true
		}
	}
	return false
}

// collectENI covers EC2, load balancers and EKS alike, since each appears as an ENI.
func collectENI(r *Result, e ec2types.NetworkInterface, account, region string, now time.Time) {
	vpc, id, mac := aws.ToString(e.VpcId), aws.ToString(e.NetworkInterfaceId), aws.ToString(e.MacAddress)
	alive := e.Status == ec2types.NetworkInterfaceStatusInUse
	payload := func(public bool) map[string]any {
		p := map[string]any{"account": account, "region": region, "vpc": vpc, "subnet": aws.ToString(e.SubnetId), "eni": id,
			"status": string(e.Status), "type": string(e.InterfaceType), "description": aws.ToString(e.Description), "public": public}
		if e.Attachment != nil {
			p["instance"] = aws.ToString(e.Attachment.InstanceId)
		}
		return p
	}
	d := Device{ExternalID: id, Name: id, DeviceType: "cloud_interface", Vendor: "AWS",
		Interfaces: []Interface{{Name: id, MAC: mac, Type: string(e.InterfaceType), Context: vpc, OperStatus: string(e.Status), Description: aws.ToString(e.Description)}}}
	if name := tagName(e.TagSet); name != "" {
		d.Name = name
	}
	d.ManagementIP = aws.ToString(e.PrivateIpAddress)
	r.Devices = append(r.Devices, d)
	for _, ip := range e.PrivateIpAddresses {
		r.Observations = append(r.Observations, Observation{Source: SourceAWS, Context: vpc, Address: aws.ToString(ip.PrivateIpAddress), MAC: mac,
			Hostname: aws.ToString(ip.PrivateDnsName), Alive: alive, ObservedAt: now, Payload: payload(false)})
		if ip.Association != nil && aws.ToString(ip.Association.PublicIp) != "" {
			// A public address sits outside the VPC's routing domain, so it has no context.
			r.Observations = append(r.Observations, Observation{Source: SourceAWS, Address: aws.ToString(ip.Association.PublicIp), Alive: alive, ObservedAt: now, Payload: payload(true)})
		}
	}
	for _, ip := range e.Ipv6Addresses {
		r.Observations = append(r.Observations, Observation{Source: SourceAWS, Context: vpc, Address: aws.ToString(ip.Ipv6Address), MAC: mac, Alive: alive, ObservedAt: now, Payload: payload(false)})
	}
}
