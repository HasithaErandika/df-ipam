package inventory

import (
	"net/netip"

	"dfipam/api/internal/validate"
)

type segmentInput struct {
	Name             string `json:"name"`
	Kind             string `json:"kind"`
	Floor            *int   `json:"floor"`
	VLAN             *int   `json:"vlan"`
	Network          string `json:"network"`
	Gateway          string `json:"gateway"`
	DHCPStart        string `json:"dhcpStart"`
	DHCPEnd          string `json:"dhcpEnd"`
	EnvironmentID    string `json:"defaultEnvironmentId"`
	Description      string `json:"description"`
	AlertPrivateMACs bool   `json:"alertPrivateMacs"`
	CheckPorts       bool   `json:"checkPorts"`
}

func (in *segmentInput) validate() error {
	var v validate.V
	v.Text("name", &in.Name, 80, true, "Name")
	v.OneOf("kind", in.Kind, []string{"dhcp", "server", "cloud"}, "segment type")
	v.IntRange("floor", in.Floor, -5, 200, "Floor")
	v.IntRange("vlan", in.VLAN, 1, 4094, "VLAN")
	v.Text("description", &in.Description, 500, false, "Description")
	p := v.CIDR("network", &in.Network)
	gw := v.IPv4("gateway", &in.Gateway, false)
	start := v.IPv4("dhcpStart", &in.DHCPStart, false)
	end := v.IPv4("dhcpEnd", &in.DHCPEnd, false)
	inside := func(field string, a netip.Addr) {
		if p.IsValid() && a.IsValid() && !p.Contains(a) {
			v.Add(field, "Must be inside "+p.String()+".")
		}
	}
	inside("gateway", gw)
	inside("dhcpStart", start)
	inside("dhcpEnd", end)
	switch {
	case in.Kind != "dhcp" && (in.DHCPStart != "" || in.DHCPEnd != ""):
		v.Add("dhcpStart", "Only floor segments have a DHCP pool.")
	case (in.DHCPStart == "") != (in.DHCPEnd == ""):
		v.Add("dhcpEnd", "Enter both the first and last pool address.")
	case start.IsValid() && end.IsValid() && end.Less(start):
		v.Add("dhcpEnd", "The last pool address must not be before the first.")
	}
	v.UUID("defaultEnvironmentId", in.EnvironmentID, false, "an environment")
	if in.Kind == "dhcp" {
		in.EnvironmentID = ""
	}
	if in.Kind == "cloud" {
		in.CheckPorts = false // never swept from the building
	}
	return v.Err()
}
