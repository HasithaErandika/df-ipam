package intune

import (
	"context"
	"strings"
	"time"
)

// entraDevices asks for every device object with its registered owner. Entra
// keeps no MAC addresses, so the name is all a sighting can be matched by.
const entraDevices = graphBase + "/v1.0/devices?$top=100&$select=displayName,accountEnabled,trustType,approximateLastSignInDateTime,operatingSystem" +
	"&$expand=registeredOwners($select=userPrincipalName,displayName)"

// EntraDevice is one device in Entra ID and the people registered as its owners.
type EntraDevice struct {
	Name       string     `json:"displayName"`
	Enabled    bool       `json:"accountEnabled"`
	Join       string     `json:"trustType"` // AzureAd (joined), ServerAd (hybrid joined), Workplace (registered)
	LastSignIn *time.Time `json:"approximateLastSignInDateTime"`
	OS         string     `json:"operatingSystem"`
	Owners     []struct {
		UPN  string `json:"userPrincipalName"`
		Name string `json:"displayName"`
	} `json:"registeredOwners"`
}

// EntraDevices lists Entra ID's devices, following Graph's paging.
func (c *Client) EntraDevices(ctx context.Context) ([]EntraDevice, error) {
	tok, err := c.token(ctx)
	if err != nil {
		return nil, err
	}
	return pages[EntraDevice](ctx, c, tok, entraDevices, "Device.Read.All (and User.Read.All)")
}

// owner is the device's first registered owner that is a person with a
// sign-in name. Hybrid-joined PCs often have none.
func (d EntraDevice) owner() (upn, name string) {
	for _, o := range d.Owners {
		if u := strings.ToLower(strings.TrimSpace(o.UPN)); u != "" {
			return u, strings.TrimSpace(o.Name)
		}
	}
	return "", ""
}

func (d EntraDevice) signedIn() time.Time {
	if d.LastSignIn == nil {
		return time.Time{}
	}
	return *d.LastSignIn
}

// entraIndex keeps, per computer name, the enabled device with an owner that
// signed in most recently: Entra often keeps stale copies of a re-joined PC.
func entraIndex(devs []EntraDevice) map[string]EntraDevice {
	byName := map[string]EntraDevice{}
	for _, d := range devs {
		n := nameKey(d.Name)
		if u, _ := d.owner(); n == "" || u == "" || !d.Enabled {
			continue
		}
		if old, ok := byName[n]; !ok || d.signedIn().After(old.signedIn()) {
			byName[n] = d
		}
	}
	return byName
}
