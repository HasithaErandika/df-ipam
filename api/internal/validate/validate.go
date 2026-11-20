// Package validate collects field problems in a request, so a form can show every
// issue at once. The database checks again; this gives clear messages first.
package validate

import (
	"net"
	"net/http"
	"net/netip"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"dfipam/api/internal/platform/httpx"
)

// V gathers issues. The zero value is ready to use.
type V struct{ issues []httpx.FieldIssue }

func (v *V) Add(field, issue string) {
	for _, i := range v.issues {
		if i.Field == field {
			return // the first problem with a field is the one to fix
		}
	}
	v.issues = append(v.issues, httpx.FieldIssue{Field: field, Issue: issue})
}

// OK reports whether nothing has failed so far.
func (v *V) OK() bool { return len(v.issues) == 0 }

// Err is nil when every check passed.
func (v *V) Err() error {
	if v.OK() {
		return nil
	}
	return &httpx.Error{Status: http.StatusUnprocessableEntity, Code: "VALIDATION_FAILED",
		Message: "Some fields need attention.", Details: v.issues}
}

// Text trims the value in place and checks its length.
func (v *V) Text(field string, value *string, max int, required bool, label string) {
	*value = strings.TrimSpace(*value)
	switch {
	case required && *value == "":
		v.Add(field, label+" is required.")
	case utf8.RuneCountInString(*value) > max:
		v.Add(field, label+" can be at most "+itoa(max)+" characters.")
	case strings.ContainsAny(*value, "\x00\r"):
		v.Add(field, label+" contains characters that are not allowed.")
	}
}

// IPv4 trims and checks a single IPv4 address. Empty is allowed unless required.
func (v *V) IPv4(field string, value *string, required bool) netip.Addr {
	*value = strings.TrimSpace(*value)
	if *value == "" {
		if required {
			v.Add(field, "Enter an IPv4 address.")
		}
		return netip.Addr{}
	}
	a, err := netip.ParseAddr(*value)
	if err != nil || !a.Is4() {
		v.Add(field, "Enter a valid IPv4 address such as 192.168.1.10.")
		return netip.Addr{}
	}
	*value = a.String()
	return a
}

// CIDR checks an IPv4 network written as its network address, such as 192.168.4.0/24.
func (v *V) CIDR(field string, value *string) netip.Prefix {
	*value = strings.TrimSpace(*value)
	p, err := netip.ParsePrefix(*value)
	switch {
	case *value == "":
		v.Add(field, "Enter a network such as 192.168.4.0/24.")
	case err != nil || !p.Addr().Is4():
		v.Add(field, "Enter an IPv4 network in CIDR form, such as 192.168.4.0/24.")
	case p.Masked() != p:
		v.Add(field, "Use the network address: "+p.Masked().String()+".")
	case p.Bits() < 16 || p.Bits() > 30:
		v.Add(field, "Use a prefix from /16 to /30.")
	default:
		return p
	}
	return netip.Prefix{}
}

// MAC normalises a MAC address to lower-case colon form. Empty is allowed unless required.
func (v *V) MAC(field string, value *string, required bool) {
	*value = strings.TrimSpace(*value)
	if *value == "" {
		if required {
			v.Add(field, "Enter a MAC address.")
		}
		return
	}
	hw, err := net.ParseMAC(*value)
	if err != nil || len(hw) != 6 {
		v.Add(field, "Enter a MAC address such as 00:11:22:33:44:55.")
		return
	}
	*value = hw.String()
}

// OneOf checks a value against a fixed list.
func (v *V) OneOf(field, value string, allowed []string, label string) {
	if !slices.Contains(allowed, value) {
		v.Add(field, "Choose a valid "+label+".")
	}
}

// UUID checks an identifier. Empty is allowed unless required.
func (v *V) UUID(field, value string, required bool, label string) {
	if value == "" && !required {
		return
	}
	if !httpx.IsUUID(value) {
		v.Add(field, "Choose "+label+".")
	}
}

// IntRange checks an optional whole number.
func (v *V) IntRange(field string, value *int, min, max int, label string) {
	if value != nil && (*value < min || *value > max) {
		v.Add(field, label+" must be from "+itoa(min)+" to "+itoa(max)+".")
	}
}

// sriLanka is Sri Lanka's time zone, which has no daylight saving. Dates are
// checked against its calendar day, as people there see it.
var sriLanka = time.FixedZone("Asia/Colombo", 5*3600+1800)

// Date checks an optional YYYY-MM-DD date. future requires today or later.
func (v *V) Date(field string, value *string, future bool, label string) {
	*value = strings.TrimSpace(*value)
	if *value == "" {
		return
	}
	d, err := time.Parse(time.DateOnly, *value)
	if err != nil {
		v.Add(field, label+" must be a date.")
		return
	}
	now := time.Now().In(sriLanka)
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	if future && d.Before(today) {
		v.Add(field, label+" can't be in the past.")
	}
	if d.After(today.AddDate(5, 0, 0)) {
		v.Add(field, label+" must be within five years.")
	}
}

var hexColour = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)

// Colour checks a #rrggbb colour.
func (v *V) Colour(field, value string) {
	if !hexColour.MatchString(value) {
		v.Add(field, "Choose a colour such as #2563eb.")
	}
}

func itoa(n int) string { return strconv.Itoa(n) }
