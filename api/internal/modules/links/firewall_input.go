package links

import (
	"dfipam/api/internal/platform/db"

	"github.com/jackc/pgx/v5"
)

// newInput is a new link: the business details people gave on review, and
// the technical details the firewall has.
func (r *applyRun) newInput(item applyItem, p proposal) *linkInput {
	s := p.Suggestion
	in := &linkInput{
		Name: firstOf(item.Name, s.Name), Type: firstOf(item.LinkType, s.LinkType), ProviderID: item.ProviderID, SourceSite: item.SourceSite,
		Destination: item.Destination, Purpose: item.Purpose, Role: item.LinkRole, Criticality: item.Criticality,
		OfficerID: item.OfficerID, Status: firstOf(item.Status, "active"),
		technical: technical{DownMbps: s.DownMbps, UpMbps: s.UpMbps, WANIP: s.WANIP, GatewayIP: s.GatewayIP, VLAN: s.VLAN,
			DeviceText: s.DeviceText, Interface: s.Interface, VRF: s.VRF, Routing: s.Routing},
		vpn:        vpn{VPNTunnel: s.VPNTunnel, VPNType: s.VPNType, LocalPeer: s.LocalPeer, RemotePeer: s.RemotePeer, RemoteSubnets: s.RemoteSubnets},
		commercial: commercial{Currency: "LKR"},
	}
	for _, sg := range s.LocalSegments {
		in.LocalSegmentIDs = append(in.LocalSegmentIDs, sg.ID)
	}
	if under := r.made[s.Underlying]; under != "" {
		in.Relations = []relationInput{{RelatedID: under, Kind: "underlying"}}
	}
	return in
}

// current is the link as recorded, as a form, for an update of some fields.
func (r *applyRun) current(tx pgx.Tx, id string) (*linkInput, error) {
	var w db.Where
	w.Add("l.id = ?::uuid", id)
	rows, err := ioRows(r.c, tx, w, r.s)
	if err != nil || len(rows) == 0 {
		return nil, rowError("The recorded link couldn't be read.")
	}
	if r.names == nil {
		x, err := newResolver(r.c, tx, r.s)
		if err != nil {
			return nil, err
		}
		r.names = x
	}
	return inputFrom(rows[0], r.names), nil
}

// setFromFirewall takes one field's value from the firewall.
func setFromFirewall(in *linkInput, field string, s suggestion) {
	switch field {
	case "interface":
		in.Interface = s.Interface
	case "wanIp":
		in.WANIP = s.WANIP
	case "gatewayIp":
		in.GatewayIP = s.GatewayIP
	case "vlan":
		in.VLAN = s.VLAN
	case "downMbps":
		in.DownMbps = s.DownMbps
	case "upMbps":
		in.UpMbps = s.UpMbps
	case "vpnTunnel":
		in.VPNTunnel = s.VPNTunnel
	case "localPeer":
		in.LocalPeer = s.LocalPeer
	case "remotePeer":
		in.RemotePeer = s.RemotePeer
	case "remoteSubnets":
		in.RemoteSubnets = s.RemoteSubnets
	}
}
