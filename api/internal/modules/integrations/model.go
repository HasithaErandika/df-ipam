// Package integrations reads infrastructure platforms and stores what they report
// as evidence. Connectors only read, and only store.go writes: it can reach
// observation fields and platform records, never ownership or allocation data.
package integrations

import (
	"context"
	"encoding/json"
	"strings"
	"time"
)

// Connector reads one platform. There is deliberately no write method.
type Connector interface {
	// Test makes one cheap read to prove the URL and credentials work.
	Test(ctx context.Context) error
	// Collect reads everything the connector covers.
	Collect(ctx context.Context) (*Result, error)
}

// Result is everything one sync collected. Context strings name a place on the
// platform, such as a VDOM or VPC. People map contexts to routing domains.
type Result struct {
	Devices        []Device
	Observations   []Observation
	Routes         []Route
	AddressObjects []AddressObject
	AddressGroups  []AddressGroup
	Accounts       []CloudAccount
	Networks       []CloudNetwork
	Peerings       []Peering
	DHCPScopes     []DHCPScope
	Warnings       []Warning
}

// Device is a platform's record of a device. ExternalID is stable across syncs.
type Device struct {
	ExternalID, Name, Hostname, DeviceType, Vendor, Model, Serial, OSVersion, OSFamily, ManagementIP string
	VDOMs                                                                                            []VDOM
	Interfaces                                                                                       []Interface
}

// VDOM is a virtual domain on a device.
type VDOM struct{ Name, Context string }

// Interface belongs to a device. Context, when set, places it in a routing domain.
type Interface struct {
	Name, MAC, Type, AdminStatus, OperStatus, VDOM, Context, Description string
}

// Observation is one piece of raw evidence. Alive says the source saw the address
// in use now. Stale data, such as a powered-off VM's guest IP, is kept with Alive
// false, so it is never treated as active.
type Observation struct {
	Source, Context, Address, MAC, Hostname string
	Alive                                   bool
	Payload                                 map[string]any
	ObservedAt                              time.Time
}

// Route is a route a platform reports.
type Route struct {
	Context, DeviceExternalID, VDOM, Destination, NextHop, Interface, Type string
}

// AddressObject is a firewall address object. Network is set when it is one CIDR.
type AddressObject struct {
	DeviceExternalID, VDOM, Name, Type, Value, Network, Comment string
}

// AddressGroup is a firewall address group. Members name address objects.
type AddressGroup struct {
	DeviceExternalID, VDOM, Name, Comment string
	Members                               []string
}

// CloudAccount is an AWS account or Azure subscription.
type CloudAccount struct{ Provider, ExternalID, TenantID, Name string }

// CloudNetwork is a VPC or VNet. Its Context is its external ID.
type CloudNetwork struct {
	AccountExternalID, ExternalID, Name, Region, ResourceGroup string
	AddressSpaces                                              []string
}

// Peering records that two networks peer. It never merges their routing domains.
type Peering struct{ ContextA, ContextB, Kind string }

// DHCPScope is a scope a server reports. It is stored only against a subnet that
// already exists in the mapped routing domain.
type DHCPScope struct {
	Context, DeviceExternalID, Name, RangeStart, RangeEnd string
}

// Warning is a partial failure: the sync carried on without this part.
type Warning struct {
	Code, Message, Context string
}

func (r *Result) warn(code, context string, err error) {
	r.Warnings = append(r.Warnings, Warning{code, truncate(err.Error(), 500), context})
}

// Contexts lists every context the result mentions, sorted and without repeats.
func (r *Result) Contexts() []string {
	seen := map[string]bool{}
	add := func(c string) {
		if c != "" {
			seen[c] = true
		}
	}
	for _, d := range r.Devices {
		for _, v := range d.VDOMs {
			add(v.Context)
		}
		for _, i := range d.Interfaces {
			add(i.Context)
		}
	}
	for _, o := range r.Observations {
		add(o.Context)
	}
	for _, n := range r.Networks {
		add(n.ExternalID)
	}
	out := make([]string, 0, len(seen))
	for c := range seen {
		out = append(out, c)
	}
	sortStrings(out)
	return out
}

// Connection is the API's view of a connection. Credentials are write-only: this
// type has no field that could carry them.
type Connection struct {
	ID                  string            `json:"id"`
	Kind                string            `json:"kind"`
	Name                string            `json:"name"`
	BaseURL             string            `json:"baseUrl"`
	Config              map[string]string `json:"config"`
	RoutingDomains      map[string]string `json:"routingDomains"`
	HasCredentials      bool              `json:"hasCredentials"`
	IsEnabled           bool              `json:"isEnabled"`
	SyncIntervalMinutes int               `json:"syncIntervalMinutes"`
	Health              string            `json:"health"`
	LastSuccessAt       *time.Time        `json:"lastSuccessAt"`
	LastSyncAt          *time.Time        `json:"lastSyncAt"`
	LastError           string            `json:"lastError"`
	Contexts            []string          `json:"contexts"`
	CreatedAt           time.Time         `json:"createdAt"`
	UpdatedAt           time.Time         `json:"updatedAt"`
}

// storedConfig is the shape of integration_connections.config.
type storedConfig struct {
	Settings       map[string]string `json:"settings"`
	RoutingDomains map[string]string `json:"routingDomains"`
}

// SyncJob is one sync's history.
type SyncJob struct {
	ID           string          `json:"id"`
	ConnectionID string          `json:"connectionId"`
	Status       string          `json:"status"`
	Counts       json.RawMessage `json:"counts"`
	Contexts     []string        `json:"contexts"`
	Error        string          `json:"error"`
	ErrorCount   int64           `json:"errorCount"`
	RequestedBy  *string         `json:"requestedBy"`
	CreatedAt    time.Time       `json:"createdAt"`
	StartedAt    *time.Time      `json:"startedAt"`
	FinishedAt   *time.Time      `json:"finishedAt"`
}

// SyncError is a problem recorded during a sync.
type SyncError struct {
	Code       string          `json:"code"`
	Message    string          `json:"message"`
	Context    json.RawMessage `json:"context"`
	OccurredAt time.Time       `json:"occurredAt"`
}

func truncate(s string, n int) string {
	s = strings.ToValidUTF8(s, "")
	if len(s) <= n {
		return s
	}
	return strings.ToValidUTF8(s[:n], "")
}
