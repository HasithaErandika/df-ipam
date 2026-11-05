package allocation

import (
	"net/http"
	"net/netip"
	"sort"
	"strings"
	"time"

	"dfipam/api/internal/netcalc"
	"dfipam/api/internal/platform/httpx"
)

// MaxReservationDays caps how far ahead a reservation may run.
const MaxReservationDays = 365

// ApprovalTTL is how long an approved request may wait to be fulfilled.
const ApprovalTTL = 30 * 24 * time.Hour

const dateLayout = "2006-01-02"

var (
	errNotApproved = httpx.NewError(http.StatusConflict, "REQUEST_NOT_APPROVED", "Only approved requests can be fulfilled.")
	errUnavailable = httpx.NewError(http.StatusConflict, "ADDRESS_UNAVAILABLE", "This address is no longer available. Choose another or use automatic selection.")
	errExhausted   = httpx.NewError(http.StatusConflict, "SUBNET_EXHAUSTED", "This subnet has no available candidate addresses.")
	errSelf        = httpx.NewError(http.StatusForbidden, "SELF_APPROVAL_DENIED", "You can't approve or reject your own request.")
	errSeparation  = httpx.NewError(http.StatusForbidden, "SEPARATION_OF_DUTIES", "You've already decided a step of this request. Another approver must decide this one.")
	errNotEligible = httpx.NewError(http.StatusForbidden, "NOT_ELIGIBLE_APPROVER", "This step needs an approver with a different role.")
	errNotCurrent  = httpx.NewError(http.StatusConflict, "APPROVAL_NOT_CURRENT", "This approval step isn't open. It may have been decided already.")
	errArchived    = httpx.NewError(http.StatusConflict, "SUBNET_ARCHIVED", "This subnet is archived.")
)

func badState(msg string) error { return httpx.NewError(http.StatusConflict, "INVALID_STATE", msg) }

func optionalUUID(field string, v *string) (*string, error) {
	if v == nil || strings.TrimSpace(*v) == "" {
		return nil, nil
	}
	s := strings.TrimSpace(*v)
	if !httpx.IsUUID(s) {
		return nil, httpx.Validation(field, "Choose a valid record.")
	}
	return &s, nil
}

func optionalDate(field string, v *string) (*time.Time, error) {
	if v == nil || strings.TrimSpace(*v) == "" {
		return nil, nil
	}
	t, err := time.Parse(dateLayout, strings.TrimSpace(*v))
	if err != nil {
		return nil, httpx.Validation(field, "Enter a date as YYYY-MM-DD.")
	}
	return &t, nil
}

func dateString(t *time.Time) *string {
	if t == nil {
		return nil
	}
	s := t.Format(dateLayout)
	return &s
}

// Normalise checks a request's fields on their own, before any database
// lookup, and returns a cleaned copy. today is the current UTC date.
func Normalise(in RequestInput, today time.Time) (RequestInput, error) {
	out := in
	out.Justification = strings.TrimSpace(in.Justification)
	out.Department = strings.TrimSpace(in.Department)
	out.Comments = strings.TrimSpace(in.Comments)
	switch in.RequestType {
	case TypeAllocation, TypeReservation, TypeRelease:
	default:
		return out, httpx.Validation("requestType", "Choose allocation, reservation or release.")
	}
	if out.Justification == "" {
		return out, httpx.Validation("justification", "Say why this is needed.")
	}
	if len(out.Justification) > 2000 || len(out.Comments) > 2000 || len(out.Department) > 200 {
		return out, httpx.Validation("justification", "Keep text fields under 2,000 characters.")
	}
	var err error
	for field, ptr := range map[string]**string{"applicationId": &out.ApplicationID, "businessServiceId": &out.BusinessServiceID,
		"technicalOwnerId": &out.TechnicalOwnerID, "businessOwnerId": &out.BusinessOwnerID,
		"allocationId": &out.AllocationID, "reservationId": &out.ReservationID} {
		if *ptr, err = optionalUUID(field, *ptr); err != nil {
			return out, err
		}
	}
	if out.Environment != nil && *out.Environment == "" {
		out.Environment = nil
	}
	if out.Environment != nil && !environments[*out.Environment] {
		return out, httpx.Validation("environment", "Choose an environment.")
	}
	required, err := optionalDate("requiredDate", in.RequiredDate)
	if err != nil {
		return out, err
	}
	expiry, err := optionalDate("expiryDate", in.ExpiryDate)
	if err != nil {
		return out, err
	}
	out.RequiredDate, out.ExpiryDate = dateString(required), dateString(expiry)
	if expiry != nil && !expiry.After(today) {
		return out, httpx.Validation("expiryDate", "The expiry date must be in the future.")
	}
	if required != nil && expiry != nil && expiry.Before(*required) {
		return out, httpx.Validation("expiryDate", "The expiry date can't be before the required date.")
	}

	switch in.RequestType {
	case TypeRelease:
		if out.AllocationID == nil {
			return out, httpx.Validation("allocationId", "Choose the allocation to release.")
		}
		out.ReservationID, out.SelectionMode, out.RequestedAddress = nil, "specific", nil
		return out, nil
	case TypeReservation:
		out.ReservationID, out.AllocationID = nil, nil
		if expiry == nil {
			return out, httpx.Validation("expiryDate", "Reservations need an expiry date.")
		}
		if expiry.After(today.AddDate(0, 0, MaxReservationDays)) {
			return out, httpx.Validation("expiryDate", "Reservations can run for at most a year.")
		}
	case TypeAllocation:
		out.AllocationID = nil
		if out.ReservationID != nil {
			// Converting a reservation: the address comes from it.
			out.SelectionMode, out.RequestedAddress = "specific", nil
			return out, nil
		}
	}
	if !httpx.IsUUID(strings.TrimSpace(in.SubnetID)) {
		return out, httpx.Validation("subnetId", "Choose a subnet.")
	}
	out.SubnetID = strings.TrimSpace(in.SubnetID)
	switch in.SelectionMode {
	case "", "automatic":
		out.SelectionMode, out.RequestedAddress = "automatic", nil
	case "specific":
		if in.RequestedAddress == nil {
			return out, httpx.Validation("requestedAddress", "Enter the address you need.")
		}
		a, err := netip.ParseAddr(strings.TrimSpace(*in.RequestedAddress))
		if err != nil || a.Zone() != "" || a.Is4In6() {
			return out, httpx.Validation("requestedAddress", "Enter a valid IP address.")
		}
		s := a.String()
		out.RequestedAddress = &s
	default:
		return out, httpx.Validation("selectionMode", "Choose automatic or specific.")
	}
	return out, nil
}

// ReservationEnd turns an expiry date into the moment a reservation ends: the
// end of that day in UTC.
func ReservationEnd(date string) (time.Time, error) {
	t, err := time.Parse(dateLayout, date)
	if err != nil {
		return t, err
	}
	return t.AddDate(0, 0, 1), nil
}

// Target describes the request a workflow is being chosen for.
type Target struct {
	RequestType, Environment, Sensitivity string
}

func listMatches(list []string, v string) bool {
	if len(list) == 0 {
		return true
	}
	for _, item := range list {
		if item == v {
			return true
		}
	}
	return false
}

func (m MatchRules) specificity() int {
	n := 0
	for _, l := range [][]string{m.RequestTypes, m.Environments, m.Sensitivities} {
		if len(l) > 0 {
			n++
		}
	}
	return n
}

// Matches reports whether the rules cover the target.
func (m MatchRules) Matches(t Target) bool {
	return listMatches(m.RequestTypes, t.RequestType) && listMatches(m.Environments, t.Environment) &&
		listMatches(m.Sensitivities, t.Sensitivity)
}

// ChooseWorkflow picks the active workflow with steps whose rules match and say
// the most; ties go to the name that sorts first. Nil means the default: one
// step that anyone with the approve permission may decide.
func ChooseWorkflow(workflows []Workflow, t Target) *Workflow {
	var best *Workflow
	candidates := append([]Workflow(nil), workflows...)
	sort.Slice(candidates, func(i, j int) bool { return candidates[i].Name < candidates[j].Name })
	for i := range candidates {
		w := &candidates[i]
		if !w.IsActive || len(w.Steps) == 0 || !w.MatchRules.Matches(t) {
			continue
		}
		if best == nil || w.MatchRules.specificity() > best.MatchRules.specificity() {
			best = w
		}
	}
	return best
}

// ValidateRules checks workflow rules against the known values.
func ValidateRules(m MatchRules) error {
	for _, v := range m.RequestTypes {
		if _, ok := FulfilledStatus[v]; !ok {
			return httpx.Validation("matchRules.requestTypes", "Unknown request type "+v+".")
		}
	}
	for _, v := range m.Environments {
		if !environments[v] {
			return httpx.Validation("matchRules.environments", "Unknown environment "+v+".")
		}
	}
	for _, v := range m.Sensitivities {
		if v != "standard" && v != "sensitive" && v != "restricted" {
			return httpx.Validation("matchRules.sensitivities", "Unknown sensitivity "+v+".")
		}
	}
	return nil
}

// Space is what the picker needs to know about a subnet.
type Space struct {
	Prefix  netip.Prefix
	Blocked []netcalc.Range
}

// NextFree returns the lowest usable address that nothing blocks. It works on
// ranges, never single addresses, so it is safe for any IPv6 prefix.
func NextFree(s Space) (netip.Addr, bool) {
	gaps := netcalc.Gaps(netcalc.UsableRange(s.Prefix), s.Blocked)
	if len(gaps) == 0 {
		return netip.Addr{}, false
	}
	return gaps[0].First, true
}

// CheckSpecific says why a chosen address can't be used, or nil if it can,
// as far as the subnet's own layout goes. The ip_addresses row is checked
// separately, under its lock.
func CheckSpecific(s Space, a netip.Addr) error {
	if !s.Prefix.Contains(a) {
		return httpx.NewError(http.StatusUnprocessableEntity, "ADDRESS_NOT_IN_SUBNET", "This address is outside the chosen subnet.")
	}
	u := netcalc.UsableRange(s.Prefix)
	if a.Less(u.First) || u.Last.Less(a) {
		return httpx.Validation("requestedAddress", "This is the subnet's network or broadcast address.")
	}
	for _, r := range s.Blocked {
		if !a.Less(r.First) && !r.Last.Less(a) {
			return errUnavailable
		}
	}
	return nil
}
