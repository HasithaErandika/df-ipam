package catalog

import (
	"dfipam/api/internal/platform/httpx"
	"fmt"
	"net"
	"net/netip"
	"strings"
)

type Kind int

const (
	Text Kind = iota
	UUID
	Int
	Bool
	CIDR
	Inet
	MAC
)

var casts = map[Kind]string{Text: "text", UUID: "uuid", Int: "integer", Bool: "boolean", CIDR: "text::cidr", Inet: "text::inet", MAC: "text::macaddr"}

// Field maps a JSON property to a column. Names here are constants, never input.
type Field struct {
	JSON, Col string
	Kind      Kind
	Required  bool
}

// Label adds a read-only related value, such as a parent's name.
type Label struct{ JSON, SQL string }

func checkValue(f Field, v any) (any, error) {
	if v == nil || v == "" && f.Kind != Text {
		if f.Required {
			return nil, httpx.Validation(f.JSON, "This field is required.")
		}
		return nil, nil
	}
	switch f.Kind {
	case Int:
		n, ok := v.(float64)
		if !ok || n != float64(int64(n)) {
			return nil, httpx.Validation(f.JSON, "Enter a whole number.")
		}
		return int64(n), nil
	case Bool:
		b, ok := v.(bool)
		if !ok {
			return nil, httpx.Validation(f.JSON, "Enter true or false.")
		}
		return b, nil
	}
	s, ok := v.(string)
	if !ok {
		return nil, httpx.Validation(f.JSON, "Enter text.")
	}
	s = strings.TrimSpace(s)
	if issue := textIssue(f, s); issue != "" {
		return nil, httpx.Validation(f.JSON, issue)
	}
	return s, nil
}

// textIssue says what is wrong with a text-like value, or "" if nothing is.
func textIssue(f Field, s string) string {
	switch f.Kind {
	case Text:
		if f.Required && s == "" {
			return "This field is required."
		}
	case UUID:
		if !httpx.IsUUID(s) {
			return "Choose a valid record."
		}
	case CIDR:
		return cidrIssue(s)
	case Inet:
		if _, err := netip.ParseAddr(s); err != nil {
			return "Enter a valid IP address."
		}
	case MAC:
		if _, err := net.ParseMAC(s); err != nil {
			return "Enter a valid MAC address."
		}
	}
	return ""
}

func cidrIssue(s string) string {
	p, err := netip.ParsePrefix(s)
	if err != nil {
		return "Enter a valid CIDR."
	}
	if p.Masked() != p {
		return fmt.Sprintf("Host bits are set. Did you mean %s?", p.Masked())
	}
	return ""
}
