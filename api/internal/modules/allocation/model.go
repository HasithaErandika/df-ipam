// Package allocation runs the controlled allocation workflow: people request an
// allocation, reservation or release; approvers decide it step by step; and an
// authorised person fulfils it. Every write happens in one transaction with its
// audit row and notifications, and the ip_addresses row for an address is the
// single point where concurrent allocations and reservations meet.
package allocation

import "time"

// Request types.
const (
	TypeAllocation  = "allocation"
	TypeReservation = "reservation"
	TypeRelease     = "release"
)

// Request statuses. "submitted" exists in the schema for compatibility; a
// submitted request goes straight to pending_approval.
const (
	StatusDraft           = "draft"
	StatusPendingApproval = "pending_approval"
	StatusApproved        = "approved"
	StatusRejected        = "rejected"
	StatusAllocated       = "allocated"
	StatusReserved        = "reserved"
	StatusReleased        = "released"
	StatusCancelled       = "cancelled"
	StatusExpired         = "expired"
)

// FulfilledStatus is the final status for each request type.
var FulfilledStatus = map[string]string{
	TypeAllocation: StatusAllocated, TypeReservation: StatusReserved, TypeRelease: StatusReleased,
}

var cancellable = map[string]bool{StatusDraft: true, StatusPendingApproval: true, StatusApproved: true}

// CanCancel reports whether a request in this status may still be cancelled.
func CanCancel(status string) bool { return cancellable[status] }

var environments = map[string]bool{"production": true, "disaster_recovery": true, "uat": true, "development": true, "testing": true}

// Request is a request with its names resolved, its approval steps and what
// fulfilling it produced.
type Request struct {
	ID                  string     `json:"id"`
	RequestType         string     `json:"requestType"`
	Status              string     `json:"status"`
	SubnetID            string     `json:"subnetId"`
	SubnetName          string     `json:"subnetName"`
	SubnetCIDR          string     `json:"subnetCidr"`
	RoutingDomainID     string     `json:"routingDomainId"`
	RoutingDomainName   string     `json:"routingDomainName"`
	SelectionMode       string     `json:"selectionMode"`
	RequestedAddress    *string    `json:"requestedAddress"`
	AllocationID        *string    `json:"allocationId"`
	ReservationID       *string    `json:"reservationId"`
	ApplicationID       *string    `json:"applicationId"`
	ApplicationName     *string    `json:"applicationName"`
	BusinessServiceID   *string    `json:"businessServiceId"`
	BusinessServiceName *string    `json:"businessServiceName"`
	TechnicalOwnerID    *string    `json:"technicalOwnerId"`
	TechnicalOwnerEmail *string    `json:"technicalOwnerEmail"`
	BusinessOwnerID     *string    `json:"businessOwnerId"`
	BusinessOwnerEmail  *string    `json:"businessOwnerEmail"`
	Justification       string     `json:"justification"`
	Department          string     `json:"department"`
	Environment         *string    `json:"environment"`
	RequiredDate        *string    `json:"requiredDate"`
	ExpiryDate          *string    `json:"expiryDate"`
	Comments            string     `json:"comments"`
	RequestedBy         string     `json:"requestedBy"`
	RequestedByEmail    string     `json:"requestedByEmail"`
	WorkflowID          *string    `json:"workflowId"`
	WorkflowName        *string    `json:"workflowName"`
	Version             int32      `json:"version"`
	CreatedAt           time.Time  `json:"createdAt"`
	UpdatedAt           time.Time  `json:"updatedAt"`
	SubmittedAt         *time.Time `json:"submittedAt"`
	DecidedAt           *time.Time `json:"decidedAt"`
	FulfilledAt         *time.Time `json:"fulfilledAt"`
	FulfilledByEmail    *string    `json:"fulfilledByEmail"`
	Approvals           []Approval `json:"approvals"`
	Result              *Result    `json:"result"`
	// MyApprovalID is the step the caller may decide now, if any.
	MyApprovalID *string `json:"myApprovalId"`
}

// Approval is one approval step of a request.
type Approval struct {
	ID               string     `json:"id"`
	RequestID        string     `json:"requestId"`
	StepOrder        int16      `json:"stepOrder"`
	ApproverRoleID   *string    `json:"approverRoleId"`
	ApproverRoleName *string    `json:"approverRoleName"`
	ApproverID       *string    `json:"approverId"`
	ApproverEmail    *string    `json:"approverEmail"`
	Decision         string     `json:"decision"`
	Comment          string     `json:"comment"`
	DecidedAt        *time.Time `json:"decidedAt"`
}

// Result is the allocation or reservation a request produced or acted on.
type Result struct {
	Kind      string     `json:"kind"` // allocation or reservation
	ID        string     `json:"id"`
	Address   string     `json:"address"`
	Status    string     `json:"status"`
	ExpiresAt *time.Time `json:"expiresAt,omitempty"`
}

// PendingApproval is an approval step waiting on the caller, with its request.
type PendingApproval struct {
	Approval
	Request Request `json:"request"`
}

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

// RequestInput is the editable part of a request.
type RequestInput struct {
	RequestType       string  `json:"requestType"`
	SubnetID          string  `json:"subnetId"`
	SelectionMode     string  `json:"selectionMode"`
	RequestedAddress  *string `json:"requestedAddress"`
	AllocationID      *string `json:"allocationId"`
	ReservationID     *string `json:"reservationId"`
	ApplicationID     *string `json:"applicationId"`
	BusinessServiceID *string `json:"businessServiceId"`
	TechnicalOwnerID  *string `json:"technicalOwnerId"`
	BusinessOwnerID   *string `json:"businessOwnerId"`
	Justification     string  `json:"justification"`
	Department        string  `json:"department"`
	Environment       *string `json:"environment"`
	RequiredDate      *string `json:"requiredDate"`
	ExpiryDate        *string `json:"expiryDate"`
	Comments          string  `json:"comments"`
	// Submit sends the request for approval straight away.
	Submit bool `json:"submit"`
}

// RequestFilter narrows the request list.
type RequestFilter struct {
	Status, Type, SubnetID, Query string
	Mine                          bool
	OnlyUser                      string // set when the caller may only see their own requests
}

// Workflow is an approval workflow: rules choose it, steps run in order.
type Workflow struct {
	ID         string     `json:"id"`
	Name       string     `json:"name"`
	MatchRules MatchRules `json:"matchRules"`
	IsActive   bool       `json:"isActive"`
	Steps      []Step     `json:"steps"`
	InUse      int64      `json:"inUse"`
}

// Step names the role whose holders may approve it.
type Step struct {
	StepOrder        int16  `json:"stepOrder"`
	ApproverRoleID   string `json:"approverRoleId"`
	ApproverRoleName string `json:"approverRoleName"`
}

// MatchRules chooses which requests a workflow governs. An empty list matches
// anything.
type MatchRules struct {
	RequestTypes  []string `json:"requestTypes,omitempty"`
	Environments  []string `json:"environments,omitempty"`
	Sensitivities []string `json:"sensitivities,omitempty"`
}
