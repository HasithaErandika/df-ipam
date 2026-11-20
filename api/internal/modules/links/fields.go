package links

import (
	"net/mail"
	"net/netip"
	"net/url"

	"dfipam/api/internal/validate"
)

// email checks an optional address.
func email(v *validate.V, field string, value *string, label string) {
	v.Text(field, value, 254, false, label)
	if *value != "" {
		if a, err := mail.ParseAddress(*value); err != nil || a.Address != *value {
			v.Add(field, label+" must be an email address such as noc@provider.lk.")
		}
	}
}

func positive(v *validate.V, field string, n *float64, label string) {
	if n != nil && (*n <= 0 || *n > 1e9) {
		v.Add(field, label+" must be more than 0.")
	}
}

func money(v *validate.V, field string, n *float64, label string) {
	if n != nil && (*n < 0 || *n > 1e12) {
		v.Add(field, label+" can't be negative.")
	}
}

// peer accepts an IPv4 address or a host name.
func peer(v *validate.V, field string, value *string, label string) {
	v.Text(field, value, 253, false, label)
	if *value == "" {
		return
	}
	if a, err := netip.ParseAddr(*value); err == nil && a.Is4() {
		return
	}
	if !hostName.MatchString(*value) {
		v.Add(field, label+" must be an IPv4 address or a name such as vpn.partner.lk.")
	}
}

func phone(v *validate.V, field string, value *string, label string) {
	v.Text(field, value, 40, false, label)
	if !phoneChars.MatchString(*value) {
		v.Add(field, label+" may hold digits, spaces and + ( ) - only.")
	}
}

func website(v *validate.V, field string, value *string) {
	v.Text(field, value, 300, false, "Website")
	if *value == "" {
		return
	}
	u, err := url.Parse(*value)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
		v.Add(field, "Website must start with https:// or http://.")
	}
}
