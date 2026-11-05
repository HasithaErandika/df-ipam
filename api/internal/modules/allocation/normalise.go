package allocation

import (
	"dfipam/api/internal/platform/httpx"
	"net/netip"
	"strings"
	"time"
)

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
	if err := checkRequestText(out); err != nil {
		return out, err
	}
	if err := normaliseReferences(&out); err != nil {
		return out, err
	}
	expiry, err := normaliseDates(&out, today)
	if err != nil {
		return out, err
	}
	addressFromTarget, err := normaliseByType(&out, expiry, today)
	if err != nil || addressFromTarget {
		return out, err
	}
	return out, normaliseSelection(&out)
}

func checkRequestText(in RequestInput) error {
	switch in.RequestType {
	case TypeAllocation, TypeReservation, TypeRelease:
	default:
		return httpx.Validation("requestType", "Choose allocation, reservation or release.")
	}
	if in.Justification == "" {
		return httpx.Validation("justification", "Say why this is needed.")
	}
	if len(in.Justification) > 2000 || len(in.Comments) > 2000 || len(in.Department) > 200 {
		return httpx.Validation("justification", "Keep text fields under 2,000 characters.")
	}
	return nil
}

func normaliseReferences(out *RequestInput) error {
	var err error
	for field, ptr := range map[string]**string{"applicationId": &out.ApplicationID, "businessServiceId": &out.BusinessServiceID,
		"technicalOwnerId": &out.TechnicalOwnerID, "businessOwnerId": &out.BusinessOwnerID,
		"allocationId": &out.AllocationID, "reservationId": &out.ReservationID} {
		if *ptr, err = optionalUUID(field, *ptr); err != nil {
			return err
		}
	}
	if out.Environment != nil && *out.Environment == "" {
		out.Environment = nil
	}
	if out.Environment != nil && !environments[*out.Environment] {
		return httpx.Validation("environment", "Choose an environment.")
	}
	return nil
}

// normaliseDates cleans both dates and returns the expiry.
func normaliseDates(out *RequestInput, today time.Time) (*time.Time, error) {
	required, err := optionalDate("requiredDate", out.RequiredDate)
	if err != nil {
		return nil, err
	}
	expiry, err := optionalDate("expiryDate", out.ExpiryDate)
	if err != nil {
		return nil, err
	}
	out.RequiredDate, out.ExpiryDate = dateString(required), dateString(expiry)
	if expiry != nil && !expiry.After(today) {
		return nil, httpx.Validation("expiryDate", "The expiry date must be in the future.")
	}
	if required != nil && expiry != nil && expiry.Before(*required) {
		return nil, httpx.Validation("expiryDate", "The expiry date can't be before the required date.")
	}
	return expiry, nil
}

// normaliseByType applies the rules of the request type. It returns true when the
// address comes from the release or conversion target, not from a subnet choice.
func normaliseByType(out *RequestInput, expiry *time.Time, today time.Time) (bool, error) {
	switch out.RequestType {
	case TypeRelease:
		if out.AllocationID == nil {
			return false, httpx.Validation("allocationId", "Choose the allocation to release.")
		}
		out.ReservationID, out.SelectionMode, out.RequestedAddress = nil, "specific", nil
		return true, nil
	case TypeReservation:
		out.ReservationID, out.AllocationID = nil, nil
		if expiry == nil {
			return false, httpx.Validation("expiryDate", "Reservations need an expiry date.")
		}
		if expiry.After(today.AddDate(0, 0, MaxReservationDays)) {
			return false, httpx.Validation("expiryDate", "Reservations can run for at most a year.")
		}
		return false, nil
	}
	out.AllocationID = nil
	if out.ReservationID != nil {
		// Converting a reservation: the address comes from it.
		out.SelectionMode, out.RequestedAddress = "specific", nil
		return true, nil
	}
	return false, nil
}

func normaliseSelection(out *RequestInput) error {
	subnet := strings.TrimSpace(out.SubnetID)
	if !httpx.IsUUID(subnet) {
		return httpx.Validation("subnetId", "Choose a subnet.")
	}
	out.SubnetID = subnet
	switch out.SelectionMode {
	case "", "automatic":
		out.SelectionMode, out.RequestedAddress = "automatic", nil
	case "specific":
		if out.RequestedAddress == nil {
			return httpx.Validation("requestedAddress", "Enter the address you need.")
		}
		a, err := netip.ParseAddr(strings.TrimSpace(*out.RequestedAddress))
		if err != nil || a.Zone() != "" || a.Is4In6() {
			return httpx.Validation("requestedAddress", "Enter a valid IP address.")
		}
		s := a.String()
		out.RequestedAddress = &s
	default:
		return httpx.Validation("selectionMode", "Choose automatic or specific.")
	}
	return nil
}
