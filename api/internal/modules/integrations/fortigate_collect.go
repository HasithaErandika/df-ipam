package integrations

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"time"

	"dfipam/api/internal/platform/safehttp"
)

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
		body, retry, err := f.tryGet(ctx, target.String(), path)
		if !retry {
			return body, err
		}
		last = err
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(f.backoff << attempt):
		}
	}
	return nil, last
}

// tryGet makes one attempt, and says whether it is worth another.
func (f *fortiGate) tryGet(ctx context.Context, target, path string) (body []byte, retry bool, err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return nil, false, err
	}
	req.Header.Set("Authorization", "Bearer "+f.token)
	req.Header.Set("Accept", "application/json")
	res, err := f.client.Do(req)
	if err != nil {
		var urlErr *url.Error
		if errors.As(err, &urlErr) {
			err = urlErr.Err // the wrapper repeats the URL
		}
		return nil, !errors.Is(err, safehttp.ErrBlocked), err
	}
	defer res.Body.Close()
	body, err = io.ReadAll(io.LimitReader(res.Body, fortiMaxBody))
	switch {
	case err != nil:
		return nil, true, err
	case res.StatusCode == http.StatusOK:
		return body, false, nil
	case res.StatusCode == http.StatusTooManyRequests || res.StatusCode >= 500:
		return nil, true, &fortiError{path, res.StatusCode}
	}
	return nil, false, &fortiError{path, res.StatusCode}
}

func (f *fortiGate) Collect(ctx context.Context) (*Result, error) {
	r := &Result{}
	now := time.Now()
	dev, err := f.readDevice(ctx, r)
	if err != nil {
		return nil, err
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
		addInterface(r, &dev, i, now)
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

// readDevice identifies the firewall. HA members share one logical device, so a
// failover doesn't duplicate interfaces.
func (f *fortiGate) readDevice(ctx context.Context, r *Result) (Device, error) {
	raw, err := f.get(ctx, "/api/v2/monitor/system/status", nil)
	if err != nil {
		return Device{}, err
	}
	var st fortiStatus
	if err := json.Unmarshal(raw, &st); err != nil {
		return Device{}, errors.New("system status: the response can't be read")
	}
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
		return Device{}, errors.New("the device reported no serial number")
	}
	dev := Device{ExternalID: external, Name: st.Results.Hostname, Hostname: st.Results.Hostname, DeviceType: "firewall", Vendor: "Fortinet",
		Model: st.Results.ModelName + st.Results.Model, Serial: st.Serial, OSVersion: st.Version, OSFamily: "network_os"}
	if dev.Name == "" {
		dev.Name = external
	}
	return dev, nil
}

func addInterface(r *Result, dev *Device, i fortiInterface, now time.Time) {
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
