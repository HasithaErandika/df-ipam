package integrations

// Kinds of connection.
const (
	KindFortiGate = "fortigate"
	KindVCenter   = "vcenter"
	KindAWS       = "aws"
	KindAzure     = "azure"
	KindDNS       = "dns"
	KindDHCP      = "dhcp"
)

// Source names, matching observation_sources.
const (
	SourceFortiGate = "FortiGate"
	SourceVMware    = "VMware"
	SourceAWS       = "AWS"
	SourceAzure     = "Azure"
	SourceDHCP      = "DHCP"
)

// Kind describes a connector for the Integrations screen.
type Kind struct {
	Kind        string   `json:"kind"`
	Label       string   `json:"label"`
	Available   bool     `json:"available"`
	Note        string   `json:"note"`
	NeedsURL    bool     `json:"needsUrl"`
	Config      []Option `json:"config"`
	Credentials []Option `json:"credentials"`
}

// Option is one setting a connector takes.
type Option struct {
	Key      string `json:"key"`
	Label    string `json:"label"`
	Required bool   `json:"required"`
	Hint     string `json:"hint,omitempty"`
}

// Kinds lists every connector, including those still waiting on information.
var Kinds = []Kind{
	{Kind: KindFortiGate, Label: "FortiGate", Available: true, NeedsURL: true,
		Note:        "FortiOS REST API with a read-only API profile. DHCP scopes and leases come through this connector.",
		Credentials: []Option{{Key: "apiToken", Label: "API token", Required: true}}},
	{Kind: KindVCenter, Label: "VMware vCenter", Available: true, NeedsURL: true,
		Note:        "Uses a read-only role. Guest IPs on powered-off VMs are kept as low-confidence evidence.",
		Credentials: []Option{{Key: "username", Label: "Username", Required: true}, {Key: "password", Label: "Password", Required: true}}},
	{Kind: KindAWS, Label: "AWS", Available: true,
		Note: "Assumes a read-only role with short-lived credentials. Leave the access key empty to use the server's own identity.",
		Config: []Option{{Key: "roleArn", Label: "Role ARN", Required: true}, {Key: "externalId", Label: "External ID", Required: true},
			{Key: "regions", Label: "Regions", Required: true, Hint: "Comma separated, for example ap-south-1,ap-southeast-1"}},
		Credentials: []Option{{Key: "accessKeyId", Label: "Access key ID"}, {Key: "secretAccessKey", Label: "Secret access key"}}},
	{Kind: KindAzure, Label: "Microsoft Azure", Available: true,
		Note: "Uses a service principal with the Reader role, one subscription per connection.",
		Config: []Option{{Key: "tenantId", Label: "Tenant ID", Required: true}, {Key: "clientId", Label: "Client ID", Required: true},
			{Key: "subscriptionId", Label: "Subscription ID", Required: true},
			{Key: "resourceGroup", Label: "Resource group", Hint: "Optional: limits the sync to one resource group"}},
		Credentials: []Option{{Key: "clientSecret", Label: "Client secret", Required: true}}},
	{Kind: KindDNS, Label: "DNS servers", Note: "Waiting for the network team to confirm the server types."},
	{Kind: KindDHCP, Label: "DHCP servers", Note: "Waiting for the network team to confirm the server types. FortiGate DHCP already works."},
}

func kindByName(name string) (Kind, bool) {
	for _, k := range Kinds {
		if k.Kind == name {
			return k, true
		}
	}
	return Kind{}, false
}
