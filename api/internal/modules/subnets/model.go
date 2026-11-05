// Package subnets manages subnets within routing domains.
package subnets

import (
	"math/big"
	"net/netip"
	"time"

	"dfipam/api/internal/netcalc"
)

// Ref is a mapped record: its ID and a readable name.
type Ref struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// Subnet is a stored subnet with its mappings and address usage.
type Subnet struct {
	ID                string
	RoutingDomainID   string
	RoutingDomainName string
	CIDR              string
	Name              string
	Gateway           *string
	Environment       string
	SecurityZone      string
	Description       string
	DiscoveryEnabled  bool
	WarnThreshold     int16
	CriticalThreshold int16
	Sensitivity       string
	Version           int32
	ArchivedAt        *time.Time
	CreatedAt         time.Time
	UpdatedAt         time.Time
	Allocated         int64
	Reserved          int64
	Quarantined       int64

	NetworkBlockID, NetworkBlockName       *string
	SiteID, SiteName                       *string
	NetworkZoneID, NetworkZoneName         *string
	VDOMID, VDOMName                       *string
	VLANID, VLANName                       *string
	BusinessUnitID, BusinessUnitName       *string
	BusinessServiceID, BusinessServiceName *string
	TechnicalOwnerID, TechnicalOwnerName   *string
	BusinessOwnerID, BusinessOwnerName     *string
	OverlapExceptionID                     *string

	// ReservedRanges are provider and manual reservations, loaded separately.
	ReservedRanges []netcalc.Range
}

func ref(id, name *string) *Ref {
	if id == nil {
		return nil
	}
	r := &Ref{ID: *id}
	if name != nil {
		r.Name = *name
	}
	return r
}

// assigned counts addresses held by a record or a reserved range.
func assigned(s Subnet, usable netcalc.Range) *big.Int {
	n := big.NewInt(s.Allocated + s.Reserved + s.Quarantined)
	return n.Add(n, netcalc.Total(netcalc.Merge(netcalc.Clip(usable, s.ReservedRanges))))
}

// Conflict is an existing subnet that overlaps a proposed one.
type Conflict struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	CIDR string `json:"cidr"`
}

// Filter narrows a subnet list.
type Filter struct {
	Query, RoutingDomainID, Environment, Sort                 string
	SiteID, VLANID, VDOMID, BusinessServiceID, NetworkBlockID string
	NetworkZoneID, BusinessUnitID                             string
	Family                                                    int
	IncludeArchived                                           bool
}

// ProviderReserved returns the addresses a cloud provider keeps in every subnet:
// the first four and the last, in both AWS VPCs and Azure VNets. Prefixes too
// small for a cloud subnet get none.
func ProviderReserved(kind string, p netip.Prefix) (string, []netcalc.Range) {
	provider := map[string]string{"vpc": "aws", "vnet": "azure"}[kind]
	if provider == "" || p.Addr().BitLen()-p.Bits() < 3 {
		return "", nil
	}
	all := netcalc.PrefixRange(p)
	return provider, []netcalc.Range{
		{First: all.First, Last: netcalc.Offset(all.First, big.NewInt(3))},
		{First: all.Last, Last: all.Last},
	}
}
