// Package fortigate reads ARP entries, DHCP leases and VPN sessions from a
// FortiGate through its REST API. It only ever sends GET requests: DF-Atlas never
// changes the firewall. Use an API token with a read-only admin profile.
package fortigate

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const maxBody = 20 << 20

// Client talks to one FortiGate.
type Client struct {
	base, token, vdom string
	http              *http.Client
}

// NewClient checks the address and builds a client. Redirects are refused, so a
// token is only ever sent to the address an Admin entered.
func NewClient(base, token, vdom string, verifyTLS bool) (*Client, error) {
	u, err := url.Parse(strings.TrimRight(strings.TrimSpace(base), "/"))
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || (u.Path != "" && u.Path != "/") {
		return nil, errors.New("the address must look like https://firewall.example.local or https://10.0.0.1:8443")
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.TLSClientConfig = &tls.Config{MinVersion: tls.VersionTLS12, InsecureSkipVerify: !verifyTLS} //nolint:gosec // an Admin's explicit choice for self-signed firewalls
	return &Client{base: u.Scheme + "://" + u.Host, token: token, vdom: vdom, http: &http.Client{
		Timeout: 20 * time.Second, Transport: transport,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}}, nil
}

// get fetches one monitor endpoint and decodes its "results".
func (c *Client) get(ctx context.Context, path string, out any) error {
	address := c.base + path
	if c.vdom != "" {
		sep := "?"
		if strings.Contains(path, "?") {
			sep = "&"
		}
		address += sep + url.Values{"vdom": {c.vdom}}.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, address, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept", "application/json")
	res, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("can't reach the FortiGate: %w", unwrap(err))
	}
	defer res.Body.Close()
	switch {
	case res.StatusCode == http.StatusUnauthorized || res.StatusCode == http.StatusForbidden:
		return errors.New("the FortiGate refused the API token: check it and its trusted hosts")
	case res.StatusCode == http.StatusNotFound:
		return fmt.Errorf("this FortiOS version has no %s endpoint", path)
	case res.StatusCode != http.StatusOK:
		return fmt.Errorf("the FortiGate answered %d for %s", res.StatusCode, path)
	}
	var envelope struct {
		Results json.RawMessage `json:"results"`
	}
	if err := json.NewDecoder(io.LimitReader(res.Body, maxBody)).Decode(&envelope); err != nil {
		return fmt.Errorf("unexpected answer from %s", path)
	}
	if len(envelope.Results) == 0 || string(envelope.Results) == "null" {
		return nil
	}
	return json.Unmarshal(envelope.Results, out)
}

// unwrap keeps network errors short and free of the request URL.
func unwrap(err error) error {
	var ue *url.Error
	if errors.As(err, &ue) {
		return ue.Err
	}
	return err
}

// Status checks the connection and returns the firewall's name and version.
func (c *Client) Status(ctx context.Context) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+"/api/v2/monitor/system/status", nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	res, err := c.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("can't reach the FortiGate: %w", unwrap(err))
	}
	defer res.Body.Close()
	if res.StatusCode == http.StatusUnauthorized || res.StatusCode == http.StatusForbidden {
		return "", errors.New("the FortiGate refused the API token: check it and its trusted hosts")
	}
	if res.StatusCode != http.StatusOK {
		return "", fmt.Errorf("the FortiGate answered %d", res.StatusCode)
	}
	var body struct {
		Version string `json:"version"`
		Serial  string `json:"serial"`
		Results struct {
			Hostname string `json:"hostname"`
		} `json:"results"`
	}
	_ = json.NewDecoder(io.LimitReader(res.Body, 1<<20)).Decode(&body)
	name := body.Results.Hostname
	if name == "" {
		name = body.Serial
	}
	return strings.TrimSpace(name + " " + body.Version), nil
}
