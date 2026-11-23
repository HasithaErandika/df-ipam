package fortigate

// What Network Links reads from the firewall: live counters (monitor) and the
// configured WAN interfaces, routes and IPsec tunnels (cmdb). GET only. The
// API token's admin profile needs read access to System, Network (router) and
// VPN. Field names are FortiOS's.

// IfaceStat is one interface's live state and byte counters.
type IfaceStat struct {
	Name     string `json:"name"`
	Alias    string `json:"alias"`
	IP       string `json:"ip"`
	Mask     int    `json:"mask"`
	Link     bool   `json:"link"`
	Speed    int    `json:"speed"` // Mbps
	TxBytes  uint64 `json:"tx_bytes"`
	RxBytes  uint64 `json:"rx_bytes"`
	TxErrors uint64 `json:"tx_errors"`
	RxErrors uint64 `json:"rx_errors"`
}

// TunnelStat is one IPsec tunnel's live state.
type TunnelStat struct {
	Name     string `json:"name"`
	Gateway  string `json:"rgwy"`
	InBytes  uint64 `json:"incoming_bytes"`
	OutBytes uint64 `json:"outgoing_bytes"`
	Comments string `json:"comments"`
	ProxyIDs []struct {
		Status string `json:"status"`
	} `json:"proxyid"`
}

// HealthMember is one SD-WAN member's measurements in one health check.
type HealthMember struct {
	Check, Member string
	Status        string  `json:"status"`
	Latency       float64 `json:"latency"`
	Jitter        float64 `json:"jitter"`
	Loss          float64 `json:"packet_loss"`
}

// IfaceConfig is a configured interface.
type IfaceConfig struct {
	Name     string `json:"name"`
	Alias    string `json:"alias"`
	Role     string `json:"role"`
	Type     string `json:"type"`
	Mode     string `json:"mode"` // static, dhcp, pppoe
	IP       string `json:"ip"`   // "203.0.113.2 255.255.255.252"
	VLAN     int    `json:"vlanid"`
	Parent   string `json:"interface"`
	Status   string `json:"status"`
	UpKbps   int    `json:"estimated-upstream-bandwidth"`
	DownKbps int    `json:"estimated-downstream-bandwidth"`
	VDOM     string `json:"vdom"`
	Desc     string `json:"description"`
}

// Phase1 is an IPsec tunnel's phase 1.
type Phase1 struct {
	Name      string `json:"name"`
	Interface string `json:"interface"`
	RemoteGW  string `json:"remote-gw"`
	Type      string `json:"type"` // static, dynamic (dial-up), ddns
	Comments  string `json:"comments"`
	ModeCfg   string `json:"mode-cfg"` // enable: dial-up users get addresses from StartIP to EndIP
	StartIP   string `json:"ipv4-start-ip"`
	EndIP     string `json:"ipv4-end-ip"`
}

// Phase2 is one phase 2 selector of a tunnel.
type Phase2 struct {
	Phase1  string `json:"phase1name"`
	SrcType string `json:"src-addr-type"`
	Src     string `json:"src-subnet"`
	DstType string `json:"dst-addr-type"`
	Dst     string `json:"dst-subnet"`
}

// Route is a static route.
type Route struct {
	Dst     string `json:"dst"`
	Gateway string `json:"gateway"`
	Device  string `json:"device"`
	Status  string `json:"status"`
}

// Resources are the firewall's current CPU and memory use (%) and sessions.
type Resources struct {
	CPU, Memory, Sessions *int
}

// Interfaces reads every interface's live counters, VLANs included.
