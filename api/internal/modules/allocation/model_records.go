package allocation

import "time"

// Allocation is an address given to a use.
type Allocation struct {
	ID                string     `json:"id"`
	Address           string     `json:"address"`
	IPAddressID       string     `json:"ipAddressId"`
	SubnetID          string     `json:"subnetId"`
	SubnetName        string     `json:"subnetName"`
	SubnetCIDR        string     `json:"subnetCidr"`
	RoutingDomainID   string     `json:"routingDomainId"`
	RoutingDomainName string     `json:"routingDomainName"`
	Status            string     `json:"status"`
	Hostname          string     `json:"hostname"`
	OwnerEmail        *string    `json:"ownerEmail"`
	RequestID         *string    `json:"requestId"`
	ReleaseRequestID  *string    `json:"releaseRequestId"`
	OpenReleaseID     *string    `json:"openReleaseRequestId"`
	AllocatedByEmail  string     `json:"allocatedByEmail"`
	AllocatedAt       time.Time  `json:"allocatedAt"`
	ReleasedByEmail   *string    `json:"releasedByEmail"`
	ReleasedAt        *time.Time `json:"releasedAt"`
}

// Reservation is an address held for planned work.
type Reservation struct {
	ID                string     `json:"id"`
	Address           string     `json:"address"`
	IPAddressID       string     `json:"ipAddressId"`
	SubnetID          string     `json:"subnetId"`
	SubnetName        string     `json:"subnetName"`
	SubnetCIDR        string     `json:"subnetCidr"`
	RoutingDomainID   string     `json:"routingDomainId"`
	RoutingDomainName string     `json:"routingDomainName"`
	Status            string     `json:"status"`
	Reason            string     `json:"reason"`
	ExpiresAt         *time.Time `json:"expiresAt"`
	RequestID         *string    `json:"requestId"`
	ReservedBy        string     `json:"reservedBy"`
	ReservedByEmail   string     `json:"reservedByEmail"`
	ReservedAt        time.Time  `json:"reservedAt"`
	ReleasedByEmail   *string    `json:"releasedByEmail"`
	ReleasedAt        *time.Time `json:"releasedAt"`
}
