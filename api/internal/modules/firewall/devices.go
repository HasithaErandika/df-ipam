package firewall

import (
	"context"
	"net"
	"sort"
	"strings"

	"dfipam/api/internal/integrations/fortigate"
	"dfipam/api/internal/platform/db"
)

// devProposal is one MAC the firewall knows, with what it says about it and
// whether DF-IPAM already knows the device.
type devProposal struct {
	MAC        string   `json:"mac"`
	IP         string   `json:"ip"`
	Hostname   string   `json:"hostname"`
	OS         string   `json:"os"`
	Vendor     string   `json:"vendor"`
	Kind       string   `json:"kind"`
	Interface  string   `json:"interface"`
	Online     bool     `json:"online"`
	Sources    []string `json:"sources"` // arp, dhcp, detected
	PrivateMAC bool     `json:"privateMac"`
	// Suggested, from the firewall's identification.
	DeviceType string `json:"deviceType"`
	Platform   string `json:"platform"`
	// DF-IPAM's record: none, pending, approved or rejected.
	DeviceID     *string `json:"deviceId"`
	DeviceStatus string  `json:"deviceStatus"`
	DeviceName   string  `json:"deviceName"`
}

type deviceRow struct {
	ID     string `db:"id"`
	MAC    string `db:"mac"`
	Status string `db:"status"`
	Name   string `db:"name"`
}

// macs gathers what the firewall says about each MAC.
type macs map[string]*devProposal

func (m macs) get(mac string) *devProposal {
	hw, err := net.ParseMAC(mac)
	if err != nil || len(hw) != 6 {
		return nil
	}
	key := hw.String()
	if m[key] == nil {
		m[key] = &devProposal{MAC: key, Sources: []string{}, PrivateMAC: hw[0]&2 != 0, DeviceStatus: "none"}
	}
	return m[key]
}

func (m macs) fromKnown(known []fortigate.Entry) {
	for _, e := range known {
		if d := m.get(e.MAC); d != nil && e.Source != "vpn" {
			d.IP, d.Hostname = firstOf(d.IP, e.IP), firstOf(d.Hostname, e.Hostname)
			d.Sources = appendOnce(d.Sources, e.Source)
		}
	}
}

func (m macs) fromDetected(detected []fortigate.Detected) {
	for _, x := range detected {
		if d := m.get(x.MAC); d != nil {
			d.IP, d.Hostname = firstOf(x.IP, d.IP), firstOf(x.Hostname, d.Hostname)
			d.OS, d.Vendor, d.Kind, d.Interface, d.Online = strings.TrimSpace(x.OS+" "+x.OSVersion), x.Vendor, x.Type, x.Interface, x.Online
			d.Sources = appendOnce(d.Sources, "detected")
		}
	}
}

// devices joins the ARP table, DHCP leases and device identification by MAC,
// and marks each with DF-IPAM's decision on it.
func (h *Handler) devices(ctx context.Context, client *fortigate.Client, out *inventory) error {
	found := macs{}
	if known, err := client.Known(ctx); err == nil {
		found.fromKnown(known)
	} else {
		out.Problems = append(out.Problems, "ARP table and DHCP leases: "+err.Error())
	}
	if detected, err := client.Devices(ctx); err == nil {
		found.fromDetected(detected)
	} else {
		out.Problems = append(out.Problems, "Device identification: "+err.Error())
	}
	keys := make([]string, 0, len(found))
	for k := range found {
		keys = append(keys, k)
	}
	rows, err := db.All[deviceRow](ctx, h.conn, `SELECT id::text AS id, mac::text AS mac, status, name FROM devices WHERE mac = ANY($1::macaddr[])`, keys)
	if err != nil {
		return err
	}
	for _, r := range rows {
		if d := found[r.MAC]; d != nil {
			id := r.ID
			d.DeviceID, d.DeviceStatus, d.DeviceName = &id, r.Status, r.Name
		}
	}
	for _, d := range found {
		d.DeviceType, d.Platform = guessType(d.Kind, d.OS), guessPlatform(d.OS)
		out.Devices = append(out.Devices, *d)
	}
	sort.Slice(out.Devices, func(i, j int) bool { return out.Devices[i].MAC < out.Devices[j].MAC })
	return nil
}

func appendOnce(list []string, v string) []string {
	for _, x := range list {
		if x == v {
			return list
		}
	}
	return append(list, v)
}

// typeWords map the firewall's hardware type to DF-IPAM's device types, in order.
var typeWords = []struct{ word, kind string }{
	{"server", "server"}, {"printer", "printer"}, {"camera", "cctv"}, {"access point", "access_point"}, {"phone", "phone"}, {"mobile", "phone"},
	{"tablet", "phone"}, {"router", "network"}, {"switch", "network"}, {"firewall", "network"}, {"laptop", "laptop"},
}

func guessType(kind, os string) string {
	k := strings.ToLower(kind + " " + os)
	for _, t := range typeWords {
		if strings.Contains(k, t.word) {
			return t.kind
		}
	}
	if guessPlatform(os) != "" {
		return "pc"
	}
	return "other"
}

func guessPlatform(os string) string {
	o := strings.ToLower(os)
	switch {
	case strings.Contains(o, "windows") && strings.Contains(o, "server"):
		return "windows_server"
	case strings.Contains(o, "windows"):
		return "windows"
	case strings.Contains(o, "ios") || strings.Contains(o, "ipados"):
		return "ios"
	case strings.Contains(o, "mac"):
		return "macos"
	case strings.Contains(o, "android"):
		return "android"
	case strings.Contains(o, "chrome"):
		return "chromeos"
	case strings.Contains(o, "linux"):
		return "linux"
	}
	return ""
}
