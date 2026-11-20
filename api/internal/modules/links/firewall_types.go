package links

import (
	"time"

	"dfipam/api/internal/integrations/fortigate"
)

// What the live firewall view returns (firewall_live.go).

type linkRef struct {
	ID       string   `json:"id"`
	Code     string   `json:"code"`
	Name     string   `json:"name"`
	DownMbps *float64 `json:"downMbps"`
	UpMbps   *float64 `json:"upMbps"`
}

type liveIface struct {
	Name      string   `json:"name"`
	Alias     string   `json:"alias"`
	IP        string   `json:"ip"`
	Role      string   `json:"role"`
	Up        bool     `json:"up"`
	SpeedMbps int      `json:"speedMbps"`
	RxBps     *float64 `json:"rxBps"`
	TxBps     *float64 `json:"txBps"`
	RxBytes   uint64   `json:"rxBytes"`
	TxBytes   uint64   `json:"txBytes"`
	Errors    uint64   `json:"errors"`
	Link      *linkRef `json:"link"`
}

type liveTunnel struct {
	Name     string   `json:"name"`
	Gateway  string   `json:"gateway"`
	PhaseUp  int      `json:"phaseUp"`
	Phases   int      `json:"phases"`
	RxBps    *float64 `json:"rxBps"`
	TxBps    *float64 `json:"txBps"`
	Comments string   `json:"comments"`
	Link     *linkRef `json:"link"`
}

type liveHealth struct {
	Check     string  `json:"check"`
	Member    string  `json:"member"`
	Status    string  `json:"status"`
	LatencyMs float64 `json:"latencyMs"`
	JitterMs  float64 `json:"jitterMs"`
	LossPct   float64 `json:"lossPct"`
}

type liveView struct {
	Firewall   string       `json:"firewall"`
	At         time.Time    `json:"at"`
	Interfaces []liveIface  `json:"interfaces"`
	Tunnels    []liveTunnel `json:"tunnels"`
	Health     []liveHealth `json:"health"`
	// VPNUsers are the people connected over SSL or dial-up IPsec VPN now.
	VPNUsers []fortigate.VPNUser `json:"vpnUsers"`
	CPU      *int                `json:"cpu"`
	Memory   *int                `json:"memory"`
	Sessions *int                `json:"sessions"`
	// Problems are parts the firewall wouldn't give, such as SD-WAN on a
	// firewall without it, or a token whose profile can't read them.
	Problems []string `json:"problems"`
}
