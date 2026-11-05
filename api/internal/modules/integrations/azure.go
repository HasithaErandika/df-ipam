package integrations

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/network/armnetwork/v6"
)

// azureReader is everything the connector reads. Both calls are List operations.
type azureReader interface {
	virtualNetworks(ctx context.Context) ([]*armnetwork.VirtualNetwork, error)
	interfaces(ctx context.Context) ([]*armnetwork.Interface, error)
}

type armReader struct {
	vnets         *armnetwork.VirtualNetworksClient
	nics          *armnetwork.InterfacesClient
	resourceGroup string
}

func (a armReader) virtualNetworks(ctx context.Context) ([]*armnetwork.VirtualNetwork, error) {
	var out []*armnetwork.VirtualNetwork
	if a.resourceGroup != "" {
		pager := a.vnets.NewListPager(a.resourceGroup, nil)
		for pager.More() {
			page, err := pager.NextPage(ctx)
			if err != nil {
				return nil, err
			}
			out = append(out, page.Value...)
		}
		return out, nil
	}
	pager := a.vnets.NewListAllPager(nil)
	for pager.More() {
		page, err := pager.NextPage(ctx)
		if err != nil {
			return nil, err
		}
		out = append(out, page.Value...)
	}
	return out, nil
}

func (a armReader) interfaces(ctx context.Context) ([]*armnetwork.Interface, error) {
	var out []*armnetwork.Interface
	if a.resourceGroup != "" {
		pager := a.nics.NewListPager(a.resourceGroup, nil)
		for pager.More() {
			page, err := pager.NextPage(ctx)
			if err != nil {
				return nil, err
			}
			out = append(out, page.Value...)
		}
		return out, nil
	}
	pager := a.nics.NewListAllPager(nil)
	for pager.More() {
		page, err := pager.NextPage(ctx)
		if err != nil {
			return nil, err
		}
		out = append(out, page.Value...)
	}
	return out, nil
}

type azureConnector struct {
	subscription, tenant string
	reader               azureReader
}

func newAzure(c Connection, creds map[string]string) (Connector, error) {
	cred, err := azidentity.NewClientSecretCredential(c.Config["tenantId"], c.Config["clientId"], creds["clientSecret"], nil)
	if err != nil {
		return nil, errors.New("the Azure tenant or client ID is not valid")
	}
	factory, err := armnetwork.NewClientFactory(c.Config["subscriptionId"], cred, nil)
	if err != nil {
		return nil, err
	}
	return &azureConnector{c.Config["subscriptionId"], c.Config["tenantId"],
		armReader{factory.NewVirtualNetworksClient(), factory.NewInterfacesClient(), c.Config["resourceGroup"]}}, nil
}

func (a *azureConnector) Test(ctx context.Context) error {
	_, err := a.reader.virtualNetworks(ctx)
	return azureError(err)
}

// azureError keeps the status and code but drops the response dump, which repeats
// request details.
func azureError(err error) error {
	if err == nil {
		return nil
	}
	first, _, _ := strings.Cut(err.Error(), "\n")
	return errors.New("Azure: " + first)
}

func str(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

// azureID normalises a resource ID. Azure IDs are case-insensitive, and the same
// resource can come back in different cases from different APIs.
func azureID(id string) string { return strings.ToLower(id) }

// vnetOfSubnet turns a subnet's resource ID into its VNet's.
func vnetOfSubnet(id string) string {
	id = azureID(id)
	if i := strings.Index(id, "/subnets/"); i > 0 {
		return id[:i]
	}
	return id
}

func resourceGroupOf(id string) string {
	parts := strings.Split(id, "/")
	for i, p := range parts {
		if strings.EqualFold(p, "resourceGroups") && i+1 < len(parts) {
			return parts[i+1]
		}
	}
	return ""
}

func (a *azureConnector) Collect(ctx context.Context) (*Result, error) {
	vnets, err := a.reader.virtualNetworks(ctx)
	if err != nil {
		return nil, azureError(err)
	}
	r := &Result{Accounts: []CloudAccount{{Provider: "azure", ExternalID: a.subscription, TenantID: a.tenant, Name: a.subscription}}}
	now := time.Now()
	for _, v := range vnets {
		id := azureID(str(v.ID))
		n := CloudNetwork{AccountExternalID: a.subscription, ExternalID: id, Name: str(v.Name), Region: str(v.Location), ResourceGroup: resourceGroupOf(str(v.ID))}
		if v.Properties != nil {
			if v.Properties.AddressSpace != nil {
				for _, p := range v.Properties.AddressSpace.AddressPrefixes {
					n.AddressSpaces = append(n.AddressSpaces, str(p))
				}
			}
			for _, p := range v.Properties.VirtualNetworkPeerings {
				if p.Properties == nil || p.Properties.RemoteVirtualNetwork == nil || p.Properties.PeeringState == nil ||
					*p.Properties.PeeringState != armnetwork.VirtualNetworkPeeringStateConnected {
					continue
				}
				r.Peerings = append(r.Peerings, Peering{id, azureID(str(p.Properties.RemoteVirtualNetwork.ID)), "peering"})
			}
		}
		r.Networks = append(r.Networks, n)
	}
	nics, err := a.reader.interfaces(ctx)
	if err != nil {
		r.warn("INTERFACES_UNAVAILABLE", a.subscription, azureError(err))
		return r, nil
	}
	for _, nic := range nics {
		collectNIC(r, nic, a.subscription, now)
	}
	return r, nil
}

func collectNIC(r *Result, nic *armnetwork.Interface, subscription string, now time.Time) {
	if nic.Properties == nil {
		return
	}
	id := azureID(str(nic.ID))
	mac := strings.ReplaceAll(str(nic.Properties.MacAddress), "-", ":")
	attached := nic.Properties.VirtualMachine != nil
	d := Device{ExternalID: id, Name: str(nic.Name), DeviceType: "cloud_interface", Vendor: "Microsoft Azure"}
	context_ := ""
	for _, cfg := range nic.Properties.IPConfigurations {
		if cfg.Properties == nil {
			continue
		}
		if cfg.Properties.Subnet != nil {
			context_ = vnetOfSubnet(str(cfg.Properties.Subnet.ID))
		}
		address := str(cfg.Properties.PrivateIPAddress)
		if d.ManagementIP == "" {
			d.ManagementIP = address
		}
		payload := map[string]any{"subscription": subscription, "nic": str(nic.Name), "resourceGroup": resourceGroupOf(str(nic.ID)), "attached": attached}
		if cfg.Properties.Subnet != nil {
			payload["subnet"] = str(cfg.Properties.Subnet.ID)
		}
		if nic.Properties.VirtualMachine != nil {
			payload["vm"] = str(nic.Properties.VirtualMachine.ID)
		}
		r.Observations = append(r.Observations, Observation{Source: SourceAzure, Context: context_, Address: address, MAC: mac, Alive: attached, ObservedAt: now, Payload: payload})
	}
	d.Interfaces = []Interface{{Name: str(nic.Name), MAC: mac, Type: "nic", Context: context_}}
	r.Devices = append(r.Devices, d)
}
