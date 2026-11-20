package links

import (
	"net/mail"
	"net/netip"
	"net/url"
	"strings"

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

// ipOrNetwork accepts an IPv4 address or network and returns it in canonical form.
func ipOrNetwork(v *validate.V, field, value, label string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if a, err := netip.ParseAddr(value); err == nil && a.Is4() {
		return a.String() + "/32"
	}
	p, err := netip.ParsePrefix(value)
	if err != nil || !p.Addr().Is4() {
		v.Add(field, label+" must be an IPv4 address or network such as 203.0.113.0/29.")
		return value
	}
	if p.Masked() != p {
		v.Add(field, label+": use the network address "+p.Masked().String()+".")
	}
	return p.String()
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
