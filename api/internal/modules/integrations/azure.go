package integrations

import (
	"context"
	"errors"
	"strings"

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
