package integrations

import (
	"context"
	"strings"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/network/armnetwork/v6"
)

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

func (a *azureConnector) Collect(ctx context.Context) (*Result, error) {
	vnets, err := a.reader.virtualNetworks(ctx)
	if err != nil {
		return nil, azureError(err)
	}
	r := &Result{Accounts: []CloudAccount{{Provider: "azure", ExternalID: a.subscription, TenantID: a.tenant, Name: a.subscription}}}
	now := time.Now()
	for _, v := range vnets {
		collectVNet(r, v, a.subscription)
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

func collectVNet(r *Result, v *armnetwork.VirtualNetwork, subscription string) {
	id := azureID(str(v.ID))
	n := CloudNetwork{AccountExternalID: subscription, ExternalID: id, Name: str(v.Name), Region: str(v.Location), ResourceGroup: resourceGroupOf(str(v.ID))}
	if v.Properties != nil {
		if v.Properties.AddressSpace != nil {
			for _, p := range v.Properties.AddressSpace.AddressPrefixes {
				n.AddressSpaces = append(n.AddressSpaces, str(p))
			}
		}
		addVNetPeerings(r, id, v.Properties.VirtualNetworkPeerings)
	}
	r.Networks = append(r.Networks, n)
}

func addVNetPeerings(r *Result, id string, peerings []*armnetwork.VirtualNetworkPeering) {
	for _, p := range peerings {
		if p.Properties == nil || p.Properties.RemoteVirtualNetwork == nil || p.Properties.PeeringState == nil ||
			*p.Properties.PeeringState != armnetwork.VirtualNetworkPeeringStateConnected {
			continue
		}
		r.Peerings = append(r.Peerings, Peering{id, azureID(str(p.Properties.RemoteVirtualNetwork.ID)), "peering"})
	}
}
