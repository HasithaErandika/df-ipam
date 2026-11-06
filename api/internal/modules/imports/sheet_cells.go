package imports

import (
	"net"
	"net/netip"
	"slices"
	"strconv"
	"strings"
)

var yes = map[string]string{"yes": "true", "y": "true", "true": "true", "1": "true", "no": "false", "n": "false", "false": "false", "0": "false"}

func checkPrefix(v string) (string, string) {
	p, err := netip.ParsePrefix(v)
	if err != nil {
		return v, "isn't a valid CIDR"
	}
	if p.Masked() != p {
		return v, "has host bits set. Did you mean " + p.Masked().String() + "?"
	}
	return p.String(), ""
}

// cellIssue cleans a value and says what is wrong with it, if anything.
func cellIssue(c *column, v string) (string, string) {
	switch c.Kind {
	case intCell:
		if _, err := strconv.Atoi(v); err != nil {
			return v, "must be a whole number"
		}
	case boolCell:
		b, ok := yes[strings.ToLower(v)]
		if !ok {
			return v, "must be yes or no"
		}
		return b, ""
	case cidrCell:
		return checkPrefix(v)
	case ipCell:
		a, err := netip.ParseAddr(v)
		if err != nil {
			return v, "isn't a valid IP address"
		}
		return a.String(), ""
	case macCell:
		if _, err := net.ParseMAC(v); err != nil {
			return v, "isn't a valid MAC address"
		}
	}
	if c.Enum != nil {
		v = strings.ToLower(v)
		if !slices.Contains(c.Enum, v) {
			return v, "must be one of: " + strings.Join(c.Enum, ", ")
		}
	}
	return v, ""
}

func checkCells(r *rowCheck) {
	s := r.row.sheet
	for i := range s.Columns {
		c := &s.Columns[i]
		v, present := r.row.fields[c.Name]
		if !present {
			if c.Required {
				r.fail("invalid", "%s is required.", c.Name)
			}
			continue
		}
		clean, issue := cellIssue(c, v)
		r.row.fields[c.Name] = clean
		if issue != "" {
			r.fail("invalid", "%s %q %s.", c.Name, v, issue)
		}
	}
}
