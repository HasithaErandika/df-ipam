// Package intune reads managed devices from Microsoft Intune, and devices
// joined to or registered in Microsoft Entra ID, through Microsoft Graph, to
// name the person who uses each device. It only sends GET requests, with the
// app's own token (client credentials): the Entra app registration needs the
// DeviceManagementManagedDevices.Read.All application permission for Intune,
// and Device.Read.All and User.Read.All for Entra ID, granted once by a tenant
// administrator.
package intune

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	graphBase = "https://graph.microsoft.com"
	devices   = graphBase + "/v1.0/deviceManagement/managedDevices?$top=500&$select=deviceName,userPrincipalName,userDisplayName,wiFiMacAddress,ethernetMacAddress,lastSyncDateTime,serialNumber,manufacturer,model,operatingSystem,osVersion,complianceState,isEncrypted"
	maxPages  = 200
	maxBody   = 20 << 20
)

// ErrNotConfigured means api/.env has no Entra app to call Graph with.
var ErrNotConfigured = errors.New("set ENTRA_TENANT_ID, ENTRA_CLIENT_ID and ENTRA_CLIENT_SECRET in api/.env first")

// Device is one Intune-managed device and its primary user.
type Device struct {
	Name     string    `json:"deviceName"`
	User     string    `json:"userPrincipalName"`
	UserName string    `json:"userDisplayName"`
	WiFiMAC  string    `json:"wiFiMacAddress"`
	EtherMAC string    `json:"ethernetMacAddress"`
	LastSync time.Time `json:"lastSyncDateTime"`
	Serial   string    `json:"serialNumber"`
	Maker    string    `json:"manufacturer"`
	Model    string    `json:"model"`
	OS       string    `json:"operatingSystem"`
	Version  string    `json:"osVersion"`
	Comply   string    `json:"complianceState"` // compliant, noncompliant, inGracePeriod, unknown…
	Encrypt  bool      `json:"isEncrypted"`
}

// Client calls Graph as the DF-IPAM app registration.
type Client struct {
	authority, tenant, id, secret string
	http                          *http.Client
}

// NewClient returns nil when the Entra app isn't configured.
func NewClient(authority, tenant, id, secret string) *Client {
	if tenant == "" || id == "" || secret == "" {
		return nil
	}
	if authority == "" {
		authority = "https://login.microsoftonline.com"
	}
	return &Client{authority: strings.TrimRight(authority, "/"), tenant: tenant, id: id, secret: secret, http: &http.Client{
		Timeout:       30 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}}
}

// token gets an app-only Graph token. It is used for one read and never stored.
func (c *Client) token(ctx context.Context) (string, error) {
	form := url.Values{"grant_type": {"client_credentials"}, "client_id": {c.id}, "client_secret": {c.secret}, "scope": {graphBase + "/.default"}}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.authority+"/"+url.PathEscape(c.tenant)+"/oauth2/v2.0/token", strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	res, err := c.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("can't reach Microsoft sign-in: %w", unwrap(err))
	}
	defer res.Body.Close()
	var body struct {
		AccessToken string `json:"access_token"`
	}
	if res.StatusCode != http.StatusOK || json.NewDecoder(io.LimitReader(res.Body, 1<<20)).Decode(&body) != nil || body.AccessToken == "" {
		return "", fmt.Errorf("microsoft refused the app's credentials (%d): check ENTRA_CLIENT_SECRET", res.StatusCode)
	}
	return body.AccessToken, nil
}

// Devices lists every managed device, following Graph's paging.
func (c *Client) Devices(ctx context.Context) ([]Device, error) {
	tok, err := c.token(ctx)
	if err != nil {
		return nil, err
	}
	return pages[Device](ctx, c, tok, devices, "DeviceManagementManagedDevices.Read.All")
}

// pages reads a Graph list from its first address to its last page. perm is
// the permission named when Graph refuses.
func pages[T any](ctx context.Context, c *Client, tok, first, perm string) ([]T, error) {
	var out []T
	next := first
	for page := 0; next != "" && page < maxPages; page++ {
		var body struct {
			Value []T    `json:"value"`
			Next  string `json:"@odata.nextLink"`
		}
		if err := c.get(ctx, tok, next, perm, &body); err != nil {
			return nil, err
		}
		out = append(out, body.Value...)
		next = body.Next
		if next != "" && !strings.HasPrefix(next, graphBase+"/") {
			return nil, errors.New("microsoft Graph returned an unexpected next page address")
		}
	}
	return out, nil
}

func (c *Client) get(ctx context.Context, tok, address, perm string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, address, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+tok)
	req.Header.Set("Accept", "application/json")
	res, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("can't reach Microsoft Graph: %w", unwrap(err))
	}
	defer res.Body.Close()
	switch {
	case res.StatusCode == http.StatusUnauthorized || res.StatusCode == http.StatusForbidden:
		return fmt.Errorf("microsoft Graph refused: give the app the %s application permission and grant admin consent", perm)
	case res.StatusCode != http.StatusOK:
		return fmt.Errorf("microsoft Graph answered %d", res.StatusCode)
	}
	if err := json.NewDecoder(io.LimitReader(res.Body, maxBody)).Decode(out); err != nil {
		return errors.New("unexpected answer from Microsoft Graph")
	}
	return nil
}

// unwrap keeps network errors short and free of the request URL.
func unwrap(err error) error {
	var ue *url.Error
	if errors.As(err, &ue) {
		return ue.Err
	}
	return err
}
