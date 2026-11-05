package integrations

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
	"time"

	"dfipam/api/internal/platform/safehttp"

	"golang.org/x/time/rate"
)

const (
	fortiPageSize = 1000
	fortiMaxBody  = 64 << 20
	fortiMaxPages = 500
	fortiRetries  = 3
)

// fortiGate reads the FortiOS REST API. Its only request method is get.
type fortiGate struct {
	base    *url.URL
	token   string
	client  *http.Client
	limiter *rate.Limiter
	backoff time.Duration
}

func newFortiGate(c Connection, credentials map[string]string, policy safehttp.Policy) (Connector, error) {
	base, err := policy.CheckURL(c.BaseURL)
	if err != nil {
		return nil, err
	}
	return &fortiGate{base, credentials["apiToken"], policy.Client(60 * time.Second), rate.NewLimiter(10, 10), time.Second}, nil
}

type fortiError struct {
	Path   string
	Status int
}

func (e *fortiError) Error() string {
	return fmt.Sprintf("FortiGate answered %d for %s", e.Status, e.Path)
}

// get performs one read. The token travels in a header, so it can't appear in a
// logged URL, and it is never part of an error.
func (f *fortiGate) get(ctx context.Context, path string, query url.Values) ([]byte, error) {
	target := f.base.JoinPath(path)
	target.RawQuery = query.Encode()
	var last error
	for attempt := range fortiRetries {
		if err := f.limiter.Wait(ctx); err != nil {
			return nil, err
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, target.String(), nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Authorization", "Bearer "+f.token)
		req.Header.Set("Accept", "application/json")
		res, err := f.client.Do(req)
		if err != nil {
			var urlErr *url.Error
			if errors.As(err, &urlErr) {
				err = urlErr.Err // the wrapper repeats the URL
			}
			if errors.Is(err, safehttp.ErrBlocked) {
				return nil, err
			}
			last = err
		} else {
			body, rerr := io.ReadAll(io.LimitReader(res.Body, fortiMaxBody))
			res.Body.Close()
			switch {
			case rerr != nil:
				last = rerr
			case res.StatusCode == http.StatusOK:
				return body, nil
			case res.StatusCode == http.StatusTooManyRequests || res.StatusCode >= 500:
				last = &fortiError{path, res.StatusCode}
			default:
				return nil, &fortiError{path, res.StatusCode}
			}
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(f.backoff << attempt):
		}
	}
	return nil, last
}

type fortiResponse[T any] struct {
	Results []T    `json:"results"`
	Serial  string `json:"serial"`
	Version string `json:"version"`
}

// list reads every page of a table. Monitor endpoints ignore start and count, and
// a short page ends the loop.
func fortiList[T any](ctx context.Context, f *fortiGate, path, vdom string) ([]T, error) {
	var out []T
	for page := range fortiMaxPages {
		q := url.Values{"start": {strconv.Itoa(page * fortiPageSize)}, "count": {strconv.Itoa(fortiPageSize)}}
		if vdom != "" {
			q.Set("vdom", vdom)
		}
		body, err := f.get(ctx, path, q)
		if err != nil {
			return nil, err
		}
		var r fortiResponse[T]
		if err := json.Unmarshal(body, &r); err != nil {
			return nil, fmt.Errorf("%s: the response can't be read", path)
		}
		out = append(out, r.Results...)
		if len(r.Results) < fortiPageSize {
			return out, nil
		}
	}
	return out, fmt.Errorf("%s: more than %d pages", path, fortiMaxPages)
}

func (f *fortiGate) Test(ctx context.Context) error {
	_, err := f.get(ctx, "/api/v2/monitor/system/status", nil)
	return err
}

type fortiStatus struct {
	Serial  string `json:"serial"`
	Version string `json:"version"`
	Results struct {
		Hostname  string `json:"hostname"`
		ModelName string `json:"model_name"`
		Model     string `json:"model_number"`
	} `json:"results"`
}

type fortiHA struct {
	Results struct {
		Mode      string `json:"mode"`
		GroupName string `json:"group-name"`
	} `json:"results"`
}

type fortiInterface struct {
	Name        string `json:"name"`
	VDOM        string `json:"vdom"`
	IP          string `json:"ip"`
	Type        string `json:"type"`
	Status      string `json:"status"`
	Alias       string `json:"alias"`
	Description string `json:"description"`
	MAC         string `json:"macaddr"`
	VLAN        int    `json:"vlanid"`
	Secondary   []struct {
		IP string `json:"ip"`
	} `json:"secondaryip"`
}

type fortiAddress struct {
	Name    string `json:"name"`
	Type    string `json:"type"`
	Subnet  string `json:"subnet"`
	StartIP string `json:"start-ip"`
	EndIP   string `json:"end-ip"`
	FQDN    string `json:"fqdn"`
	Comment string `json:"comment"`
}

type fortiGroup struct {
	Name    string `json:"name"`
	Comment string `json:"comment"`
	Member  []struct {
		Name string `json:"name"`
	} `json:"member"`
}

type fortiRoute struct {
	Dst     string `json:"dst"`
	Gateway string `json:"gateway"`
	Device  string `json:"device"`
}

type fortiARP struct {
	IP        string `json:"ip"`
	MAC       string `json:"mac"`
	Interface string `json:"interface"`
}

type fortiDHCPServer struct {
	ID        int    `json:"id"`
	Interface string `json:"interface"`
	IPRange   []struct {
		Start string `json:"start-ip"`
		End   string `json:"end-ip"`
	} `json:"ip-range"`
}

type fortiLease struct {
	IP        string `json:"ip"`
	MAC       string `json:"mac"`
	Hostname  string `json:"hostname"`
	Interface string `json:"interface"`
	Status    string `json:"status"`
}

// splitIPMask reads FortiOS's "10.0.0.1 255.255.255.0" form. Zero means unset.
func splitIPMask(v string) (netip.Addr, netip.Prefix, bool) {
	ip, mask, ok := strings.Cut(strings.TrimSpace(v), " ")
	if !ok {
		return netip.Addr{}, netip.Prefix{}, false
	}
	a, err := netip.ParseAddr(ip)
	if err != nil {
		return netip.Addr{}, netip.Prefix{}, false
	}
	m := net.ParseIP(strings.TrimSpace(mask)).To4()
	if m == nil {
		return netip.Addr{}, netip.Prefix{}, false
	}
	ones, bits := net.IPMask(m).Size()
	if bits == 0 {
		return netip.Addr{}, netip.Prefix{}, false // not a contiguous mask
	}
	p, err := a.Prefix(ones)
	if err != nil {
		return netip.Addr{}, netip.Prefix{}, false
	}
	return a, p, true
}

func vdomContext(name string) string { return "vdom:" + name }

func fortiMAC(v string) string {
	if v == "00:00:00:00:00:00" {
		return ""
	}
	return v
}

func (f *fortiGate) Collect(ctx context.Context) (*Result, error) {
	raw, err := f.get(ctx, "/api/v2/monitor/system/status", nil)
	if err != nil {
		return nil, err
	}
	var st fortiStatus
	if err := json.Unmarshal(raw, &st); err != nil {
		return nil, errors.New("system status: the response can't be read")
	}
	r := &Result{}
	now := time.Now()

	// HA members share one logical device, so a failover doesn't duplicate interfaces.
	external := st.Serial
	if raw, err := f.get(ctx, "/api/v2/cmdb/system/ha", nil); err != nil {
		r.warn("HA_UNAVAILABLE", "", err)
	} else {
		var ha fortiHA
		if json.Unmarshal(raw, &ha) == nil && ha.Results.Mode != "" && ha.Results.Mode != "standalone" && ha.Results.GroupName != "" {
			external = "ha:" + ha.Results.GroupName
		}
	}
	if external == "" {
		return nil, errors.New("the device reported no serial number")
	}
	dev := Device{ExternalID: external, Name: st.Results.Hostname, Hostname: st.Results.Hostname, DeviceType: "firewall", Vendor: "Fortinet",
		Model: st.Results.ModelName + st.Results.Model, Serial: st.Serial, OSVersion: st.Version, OSFamily: "network_os"}
	if dev.Name == "" {
		dev.Name = external
	}

	vdoms, err := fortiList[struct {
		Name string `json:"name"`
	}](ctx, f, "/api/v2/cmdb/system/vdom", "")
	if err != nil {
		return nil, err
	}
	for _, v := range vdoms {
		dev.VDOMs = append(dev.VDOMs, VDOM{v.Name, vdomContext(v.Name)})
	}
	if len(vdoms) == 0 {
		return nil, errors.New("the token can see no VDOMs")
	}

	interfaces, err := fortiList[fortiInterface](ctx, f, "/api/v2/cmdb/system/interface", "")
	if err != nil {
		return nil, err
	}
	for _, i := range interfaces {
		status := "up"
		if i.Status == "down" {
			status = "down"
		}
		description := i.Description
		if description == "" {
			description = i.Alias
		}
		dev.Interfaces = append(dev.Interfaces, Interface{Name: i.Name, MAC: fortiMAC(i.MAC), Type: i.Type, AdminStatus: status, VDOM: i.VDOM,
			Context: vdomContext(i.VDOM), Description: description})
		ips := []string{i.IP}
		for _, s := range i.Secondary {
			ips = append(ips, s.IP)
		}
		for n, ip := range ips {
			a, p, ok := splitIPMask(ip)
			if !ok || a.IsUnspecified() {
				continue // tunnels and unaddressed interfaces stay interfaces only
			}
			r.Observations = append(r.Observations, Observation{Source: SourceFortiGate, Context: vdomContext(i.VDOM), Address: a.String(), MAC: fortiMAC(i.MAC),
				Hostname: dev.Name, Alive: status == "up", ObservedAt: now,
				Payload: map[string]any{"interface": i.Name, "vdom": i.VDOM, "prefix": p.Masked().String(), "secondary": n > 0, "vlan": i.VLAN, "alias": i.Alias}})
		}
	}

	for _, v := range vdoms {
		f.collectVDOM(ctx, r, &dev, v.Name, now)
		if err := ctx.Err(); err != nil {
			return nil, err
		}
	}
	r.Devices = []Device{dev}
	return r, nil
}

// collectVDOM treats each table as optional: a token limited to some tables still
// produces the rest, and the gap shows as a warning.
func (f *fortiGate) collectVDOM(ctx context.Context, r *Result, dev *Device, vdom string, now time.Time) {
	context_ := vdomContext(vdom)
	if items, err := fortiList[fortiAddress](ctx, f, "/api/v2/cmdb/firewall/address", vdom); err != nil {
		r.warn("ADDRESSES_UNAVAILABLE", context_, err)
	} else {
		for _, a := range items {
			o := AddressObject{DeviceExternalID: dev.ExternalID, VDOM: vdom, Name: a.Name, Type: a.Type, Comment: a.Comment}
			switch a.Type {
			case "iprange":
				o.Value = a.StartIP + "-" + a.EndIP
			case "fqdn":
				o.Value = a.FQDN
			default:
				o.Value = a.Subnet
				if _, p, ok := splitIPMask(a.Subnet); ok {
					o.Network, o.Value = p.Masked().String(), p.Masked().String()
				}
			}
			r.AddressObjects = append(r.AddressObjects, o)
		}
	}
	if items, err := fortiList[fortiGroup](ctx, f, "/api/v2/cmdb/firewall/addrgrp", vdom); err != nil {
		r.warn("GROUPS_UNAVAILABLE", context_, err)
	} else {
		for _, g := range items {
			grp := AddressGroup{DeviceExternalID: dev.ExternalID, VDOM: vdom, Name: g.Name, Comment: g.Comment}
			for _, m := range g.Member {
				grp.Members = append(grp.Members, m.Name)
			}
			r.AddressGroups = append(r.AddressGroups, grp)
		}
	}
	if items, err := fortiList[fortiRoute](ctx, f, "/api/v2/cmdb/router/static", vdom); err != nil {
		r.warn("ROUTES_UNAVAILABLE", context_, err)
	} else {
		for _, s := range items {
			if _, p, ok := splitIPMask(s.Dst); ok {
				r.Routes = append(r.Routes, Route{Context: context_, DeviceExternalID: dev.ExternalID, VDOM: vdom, Destination: p.Masked().String(),
					NextHop: s.Gateway, Interface: s.Device, Type: "static"})
			}
		}
	}
	if items, err := fortiList[fortiARP](ctx, f, "/api/v2/monitor/network/arp", vdom); err != nil {
		r.warn("ARP_UNAVAILABLE", context_, err)
	} else {
		for _, a := range items {
			r.Observations = append(r.Observations, Observation{Source: SourceFortiGate, Context: context_, Address: a.IP, MAC: a.MAC, Alive: true, ObservedAt: now,
				Payload: map[string]any{"table": "arp", "interface": a.Interface, "vdom": vdom}})
		}
	}
	if items, err := fortiList[fortiDHCPServer](ctx, f, "/api/v2/cmdb/system.dhcp/server", vdom); err != nil {
		r.warn("DHCP_SCOPES_UNAVAILABLE", context_, err)
	} else {
		for _, d := range items {
			for _, ip := range d.IPRange {
				r.DHCPScopes = append(r.DHCPScopes, DHCPScope{Context: context_, DeviceExternalID: dev.ExternalID,
					Name: fmt.Sprintf("%s (%d)", d.Interface, d.ID), RangeStart: ip.Start, RangeEnd: ip.End})
			}
		}
	}
	if items, err := fortiList[fortiLease](ctx, f, "/api/v2/monitor/system/dhcp", vdom); err != nil {
		r.warn("DHCP_LEASES_UNAVAILABLE", context_, err)
	} else {
		for _, l := range items {
			r.Observations = append(r.Observations, Observation{Source: SourceDHCP, Context: context_, Address: l.IP, MAC: l.MAC, Hostname: l.Hostname,
				Alive: l.Status != "expired", ObservedAt: now, Payload: map[string]any{"interface": l.Interface, "status": l.Status, "vdom": vdom}})
		}
	}
}
