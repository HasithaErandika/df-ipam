package discovery

import "time"

// findingDTO is one IP and MAC the scanner saw. Register and device state come
// alongside, as separate fields: evidence never stands in for the register.
type findingDTO struct {
	ID             string    `db:"id" json:"id"`
	IP             string    `db:"ip" json:"ip"`
	MAC            *string   `db:"mac" json:"mac"`
	Hostname       string    `db:"hostname" json:"hostname"`
	Vendor         string    `db:"vendor" json:"vendor"`
	Online         bool      `db:"online" json:"online"`
	Source         string    `db:"source" json:"source"`
	IsStatic       bool      `db:"is_static" json:"isStatic"`
	IsPrivateMAC   bool      `db:"is_private_mac" json:"isPrivateMac"`
	IsGateway      bool      `db:"is_gateway" json:"isGateway"`
	FirstSeen      time.Time `db:"first_seen" json:"firstSeen"`
	LastSeen       time.Time `db:"last_seen" json:"lastSeen"`
	SegmentID      *string   `db:"segment_id" json:"segmentId"`
	SegmentName    *string   `db:"segment_name" json:"segmentName"`
	SegmentKind    *string   `db:"segment_kind" json:"segmentKind"`
	RegisterID     *string   `db:"register_id" json:"registerId"`
	RegisterStatus *string   `db:"register_status" json:"registerStatus"`
	AssignedName   *string   `db:"assigned_name" json:"assignedName"`
	DeviceID       *string   `db:"device_id" json:"deviceId"`
	DeviceStatus   *string   `db:"device_status" json:"deviceStatus"`
	DeviceName     *string   `db:"device_name" json:"deviceName"`
	Owner          *string   `db:"owner" json:"owner"`
	NICVendor      string    `db:"nic_vendor" json:"nicVendor"`
	Maker          string    `db:"maker" json:"maker"`
	Model          string    `db:"model" json:"model"`
	DeviceKind     string    `db:"device_kind" json:"deviceKind"`
	OSHint         string    `db:"os_hint" json:"osHint"`
	OpenPorts      []int32   `db:"open_ports" json:"openPorts"`
	Known          bool      `db:"known" json:"known"`
	HomeIP         *string   `db:"home_ip" json:"homeIp"`
	HomeSegment    *string   `db:"home_segment" json:"homeSegment"`
	PortsChecked   bool      `db:"ports_checked" json:"portsChecked"`
	Platform       string    `db:"platform" json:"platform"`
	OpenAlerts     int64     `db:"open_alerts" json:"openAlerts"`
	// From Microsoft Intune: the primary user of the device, and when Intune last named them.
	IntuneUser     string     `db:"intune_user" json:"intuneUser"`
	IntuneUserName string     `db:"intune_user_name" json:"intuneUserName"`
	IntuneDevice   string     `db:"intune_device" json:"intuneDevice"`
	IntuneSeenAt   *time.Time `db:"intune_seen_at" json:"intuneSeenAt"`
	// Intune's record of the device, when it matched one.
	IntuneSerial     string     `db:"intune_serial" json:"intuneSerial"`
	IntuneMaker      string     `db:"intune_maker" json:"intuneMaker"`
	IntuneModel      string     `db:"intune_model" json:"intuneModel"`
	IntuneOS         string     `db:"intune_os" json:"intuneOs"`
	IntuneCompliance string     `db:"intune_compliance" json:"intuneCompliance"`
	IntuneEncrypted  *bool      `db:"intune_encrypted" json:"intuneEncrypted"`
	IntuneLastSync   *time.Time `db:"intune_last_sync" json:"intuneLastSync"`
	// From Microsoft Entra ID, matched by computer name: the device's registered
	// owner, its Entra name, join type and last sign-in.
	EntraUser       string     `db:"entra_user" json:"entraUser"`
	EntraUserName   string     `db:"entra_user_name" json:"entraUserName"`
	EntraDevice     string     `db:"entra_device" json:"entraDevice"`
	EntraJoin       string     `db:"entra_join" json:"entraJoin"`
	EntraLastSignIn *time.Time `db:"entra_last_sign_in" json:"entraLastSignIn"`
	EntraSeenAt     *time.Time `db:"entra_seen_at" json:"entraSeenAt"`
	// What the device says on the network: its NetBIOS workgroup or domain, and
	// from the Windows file-sharing handshake, its domain, full name and version.
	Workgroup     string `db:"workgroup" json:"workgroup"`
	WindowsDomain string `db:"windows_domain" json:"windowsDomain"`
	WindowsFQDN   string `db:"windows_fqdn" json:"windowsFqdn"`
	OSVersion     string `db:"os_version" json:"osVersion"`
	// A VPN client: the user the FortiGate signed in, "ssl" or "ipsec", the
	// IPsec tunnel's name and the public address the session came from.
	VPNUser   string `db:"vpn_user" json:"vpnUser"`
	VPNKind   string `db:"vpn_kind" json:"vpnKind"`
	VPNTunnel string `db:"vpn_tunnel" json:"vpnTunnel"`
	VPNRemote string `db:"vpn_remote" json:"vpnRemote"`
	Rank      int64  `db:"rn" json:"-"`
}
