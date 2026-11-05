package subnets

import (
	"time"

	"dfipam/api/internal/netcalc"
)

type CreateRequest struct {
	RoutingDomainID   string  `json:"routingDomainId"`
	CIDR              string  `json:"cidr"`
	Name              string  `json:"name"`
	Gateway           *string `json:"gateway"`
	Environment       string  `json:"environment"`
	SecurityZone      string  `json:"securityZone"`
	Description       string  `json:"description"`
	DiscoveryEnabled  bool    `json:"discoveryEnabled"`
	WarnThreshold     *int16  `json:"warnThreshold"`
	CriticalThreshold *int16  `json:"criticalThreshold"`
	Mappings
}

// Mappings link a subnet to the rest of the hierarchy. An empty string clears one.
type Mappings struct {
	NetworkBlockID    *string `json:"networkBlockId"`
	SiteID            *string `json:"siteId"`
	NetworkZoneID     *string `json:"networkZoneId"`
	VDOMID            *string `json:"vdomId"`
	VLANID            *string `json:"vlanId"`
	BusinessUnitID    *string `json:"businessUnitId"`
	BusinessServiceID *string `json:"businessServiceId"`
	TechnicalOwnerID  *string `json:"technicalOwnerId"`
	BusinessOwnerID   *string `json:"businessOwnerId"`
	// OverlapExceptionID applies an approved exception, so this subnet may overlap others.
	OverlapExceptionID *string `json:"overlapExceptionId"`
}

// UpdateRequest changes only the fields that are present. CIDR and routing
// domain can't change, because IP records depend on them.
type UpdateRequest struct {
	Name              *string `json:"name"`
	Gateway           *string `json:"gateway"`
	Environment       *string `json:"environment"`
	SecurityZone      *string `json:"securityZone"`
	Description       *string `json:"description"`
	DiscoveryEnabled  *bool   `json:"discoveryEnabled"`
	WarnThreshold     *int16  `json:"warnThreshold"`
	CriticalThreshold *int16  `json:"criticalThreshold"`
	Sensitivity       *string `json:"sensitivity"`
	Mappings
}

type OverlapRequest struct {
	RoutingDomainID string `json:"routingDomainId"`
	CIDR            string `json:"cidr"`
}

type Response struct {
	ID                     string       `json:"id"`
	RoutingDomainID        string       `json:"routingDomainId"`
	RoutingDomainName      string       `json:"routingDomainName"`
	Name                   string       `json:"name"`
	Gateway                *string      `json:"gateway"`
	Environment            string       `json:"environment"`
	SecurityZone           string       `json:"securityZone"`
	Description            string       `json:"description"`
	DiscoveryEnabled       bool         `json:"discoveryEnabled"`
	WarnThreshold          int16        `json:"warnThreshold"`
	CriticalThreshold      int16        `json:"criticalThreshold"`
	Sensitivity            string       `json:"sensitivity"`
	Version                int32        `json:"version"`
	Archived               bool         `json:"archived"`
	CreatedAt              time.Time    `json:"createdAt"`
	UpdatedAt              time.Time    `json:"updatedAt"`
	Allocated              int64        `json:"allocated"`
	Reserved               int64        `json:"reserved"`
	Quarantined            int64        `json:"quarantined"`
	ReservedRangeAddresses string       `json:"reservedRangeAddresses"`
	UtilisationPct         string       `json:"utilisationPct"`
	UtilisationLevel       string       `json:"utilisationLevel"`
	NetworkBlock           *Ref         `json:"networkBlock"`
	Site                   *Ref         `json:"site"`
	NetworkZone            *Ref         `json:"networkZone"`
	VDOM                   *Ref         `json:"vdom"`
	VLAN                   *Ref         `json:"vlan"`
	BusinessUnit           *Ref         `json:"businessUnit"`
	BusinessService        *Ref         `json:"businessService"`
	TechnicalOwner         *Ref         `json:"technicalOwner"`
	BusinessOwner          *Ref         `json:"businessOwner"`
	HasOverlapException    bool         `json:"hasOverlapException"`
	OverlapExceptionID     *string      `json:"overlapExceptionId"`
	Calculated             netcalc.Info `json:"calculated"`
}

// BulkRequest applies one action to many subnets, all or nothing.
type BulkRequest struct {
	IDs     []string      `json:"ids"`
	Action  string        `json:"action"` // update, archive or delete
	Changes UpdateRequest `json:"changes"`
}
