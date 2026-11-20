package links

// The link form's sections, each checked on its own (input.go).

type technical struct {
	DownMbps     *float64 `json:"downMbps"`
	UpMbps       *float64 `json:"upMbps"`
	Medium       string   `json:"medium"`
	WANIP        string   `json:"wanIp"`
	WANSegmentID string   `json:"wanSegmentId"`
	ExtraWANIPs  []string `json:"extraWanIps"` // more WAN addresses, each with its prefix: 122.255.35.1/32
	PublicIPs    string   `json:"publicIps"`
	GatewayIP    string   `json:"gatewayIp"`
	DeviceIDs    []string `json:"deviceIds"`
	DeviceText   string   `json:"deviceText"`
	Interface    string   `json:"interface"`
	VLAN         *int     `json:"vlan"`
	VRF          string   `json:"vrf"`
	Routing      string   `json:"routing"`
}

type vpn struct {
	VPNTunnel     string   `json:"vpnTunnel"`
	VPNType       string   `json:"vpnType"`
	LocalPeer     string   `json:"localPeer"`
	RemotePeer    string   `json:"remotePeer"`
	RemoteSubnets []string `json:"remoteSubnets"`
	RemoteOrg     string   `json:"remoteOrg"`
	RemoteContact string   `json:"remoteContact"`
	LastTestDate  string   `json:"lastTestDate"`
	VaultRef      string   `json:"vaultRef"`
}

type commercial struct {
	NoSeparateCost    bool     `json:"noSeparateCost"`
	BillingAccount    string   `json:"billingAccount"`
	MonthlyRental     *float64 `json:"monthlyRental"`
	Currency          string   `json:"currency"`
	InstallCost       *float64 `json:"installCost"`
	ContractRef       string   `json:"contractRef"`
	PORef             string   `json:"poRef"`
	ActivationDate    string   `json:"activationDate"`
	ContractStart     string   `json:"contractStart"`
	ContractEnd       string   `json:"contractEnd"`
	RenewalNoticeDays *int     `json:"renewalNoticeDays"`
	CostCentre        string   `json:"costCentre"`
}

type support struct {
	SupportContact  string   `json:"supportContact"`
	SupportEmail    string   `json:"supportEmail"`
	SupportHotline  string   `json:"supportHotline"`
	AccountManager  string   `json:"accountManager"`
	SLAPct          *float64 `json:"slaPct"`
	ResolutionHours *float64 `json:"resolutionHours"`
}
