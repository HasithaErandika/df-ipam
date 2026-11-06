package integrations

import (
	"encoding/json"
	"net"
	"net/netip"
	"slices"
)

func nullText(v string) *string {
	if v == "" {
		return nil
	}
	return &v
}

func validMAC(v string) *string {
	hw, err := net.ParseMAC(v)
	if err != nil {
		return nil
	}
	out := hw.String()
	return &out
}

func validAddr(v string) (string, bool) {
	a, err := netip.ParseAddr(v)
	if err != nil {
		return "", false
	}
	return a.Unmap().WithZone("").String(), true
}

func oneOf(v string, allowed []string, fallback string) string {
	if slices.Contains(allowed, v) {
		return v
	}
	return fallback
}

var (
	deviceTypes = []string{"firewall", "router", "switch", "physical_server", "virtual_machine", "load_balancer",
		"network_appliance", "cloud_interface", "other"}
	osFamilies = []string{"unknown", "windows", "windows_server", "linux", "macos", "network_os", "hypervisor", "other"}
)

func status(v string) string {
	if v == "" {
		return "unknown"
	}
	return truncate(v, 50)
}

func payloadJSON(p map[string]any) []byte {
	if len(p) == 0 {
		return []byte(`{}`)
	}
	raw, err := json.Marshal(p)
	if err != nil || len(raw) > maxPayload {
		return []byte(`{}`)
	}
	return raw
}
