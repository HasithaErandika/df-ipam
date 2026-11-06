// Package imports loads production network exports. Every source line is kept
// as an import record, so nothing is dropped silently, and nothing reaches the
// inventory until a second person approves the batch. A committed batch can be
// reversed while its subnets are still untouched.
package imports

import (
	"net/netip"
	"regexp"
	"strings"
)

// Formats the parser understands. The real Prod_Subnets.txt and
// Prod_VDOMs_Subnets.txt layouts haven't been received, so the parser is
// tolerant: it finds the network on each line however it is written.
var Formats = map[string]string{
	"prod_subnets":       "Production subnets (one network per line)",
	"prod_vdoms_subnets": "Production VDOM subnets (VDOM, then network)",
	"csv":                "CSV: cidr, name, gateway, vlan",
}

// Parsed is what a line yields.
type Parsed struct {
	CIDR    string `json:"cidr,omitempty"`
	Name    string `json:"name,omitempty"`
	Gateway string `json:"gateway,omitempty"`
	VDOM    string `json:"vdom,omitempty"`
	VLAN    string `json:"vlan,omitempty"`
	Comment bool   `json:"comment,omitempty"`
	// Template rows say what they are and where they came from.
	Type   string            `json:"type,omitempty"`
	Sheet  string            `json:"sheet,omitempty"`
	Row    int               `json:"row,omitempty"`
	Fields map[string]string `json:"fields,omitempty"`
}

// Line is one parsed source line.
type Line struct {
	Number int
	Raw    string
	Parsed Parsed
	Status string // valid, invalid, needs_enrichment
	Errors []string
}

var (
	splitter = regexp.MustCompile(`[,\t;|]+|\s{2,}`)
	vlanRe   = regexp.MustCompile(`(?i)^(?:vlan[-_ ]?)(\d{1,4})$`)
	keyValue = regexp.MustCompile(`(?i)^(vdom|vlan|name|gw|gateway)\s*[=:]\s*(.+)$`)
)

// ParseFile parses every line. Blank and comment lines are kept too, marked
// as such, so line numbers match the source file exactly.
func ParseFile(format, content string) []Line {
	content = strings.ReplaceAll(content, "\r\n", "\n")
	raw := strings.Split(content, "\n")
	if len(raw) > 0 && raw[len(raw)-1] == "" {
		raw = raw[:len(raw)-1]
	}
	out := make([]Line, 0, len(raw))
	for i, r := range raw {
		p, errs := ParseLine(format, r)
		l := Line{Number: i + 1, Raw: r, Parsed: p, Errors: errs}
		switch {
		case p.Comment:
			l.Status = "invalid"
			l.Errors = []string{"Blank or comment line: kept for the record, nothing to import."}
		case len(errs) > 0:
			l.Status = "invalid"
		case p.Name == "":
			l.Status = "needs_enrichment"
			l.Errors = []string{"No name: add one in the enrichment queue."}
		default:
			l.Status = "valid"
		}
		out = append(out, l)
	}
	return out
}

// ParseLine reads one line. Host bits set on a network are reported, not
// silently masked, because they usually mean the export holds an interface
// address rather than a network.
func ParseLine(format, raw string) (Parsed, []string) {
	var p Parsed
	line := strings.TrimSpace(raw)
	if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "//") {
		p.Comment = true
		return p, nil
	}
	fields := splitter.Split(line, -1)
	if len(fields) == 1 {
		fields = strings.Fields(line)
	}
	rest := readFields(&p, fields)
	if format == "prod_vdoms_subnets" && p.VDOM == "" && len(rest) > 0 {
		p.VDOM, rest = rest[0], rest[1:]
	}
	if p.Name == "" && len(rest) > 0 {
		p.Name = strings.Join(rest, " ")
	}
	return p, checkParsed(p)
}

// readFields fills p from the fields, and returns those it couldn't place.
func readFields(p *Parsed, fields []string) []string {
	var rest []string
	for i := 0; i < len(fields); i++ {
		f := strings.Trim(strings.TrimSpace(fields[i]), `"'`)
		if f == "" || readKeyValue(p, f) {
			continue
		}
		if used, extra := readAddress(p, fields, i, f); used {
			i += extra
			continue
		}
		if m := vlanRe.FindStringSubmatch(f); m != nil && p.VLAN == "" {
			p.VLAN = m[1]
			continue
		}
		rest = append(rest, f)
	}
	return rest
}

func readKeyValue(p *Parsed, f string) bool {
	m := keyValue.FindStringSubmatch(f)
	if m == nil {
		return false
	}
	value := strings.TrimSpace(m[2])
	switch strings.ToLower(m[1]) {
	case "vdom":
		p.VDOM = value
	case "vlan":
		p.VLAN = value
	case "name":
		p.Name = value
	default:
		p.Gateway = value
	}
	return true
}

// readAddress takes the network, then the gateway. It reports whether field i was
// used, and how many fields after it were used too.
func readAddress(p *Parsed, fields []string, i int, f string) (bool, int) {
	if p.CIDR != "" {
		if a, err := netip.ParseAddr(f); err == nil && p.Gateway == "" {
			p.Gateway = a.String()
			return true, 0
		}
		return false, 0
	}
	if pr, err := netip.ParsePrefix(f); err == nil {
		p.CIDR = pr.String()
		return true, 0
	}
	if a, err := netip.ParseAddr(f); err == nil && a.Is4() && i+1 < len(fields) {
		if bits, ok := maskBits(strings.TrimSpace(fields[i+1])); ok {
			p.CIDR = netip.PrefixFrom(a, bits).String()
			return true, 1
		}
	}
	return false, 0
}

func checkParsed(p Parsed) []string {
	if p.CIDR == "" {
		return []string{"No network (CIDR or address and mask) on this line."}
	}
	var errs []string
	pr := netip.MustParsePrefix(p.CIDR)
	if pr.Masked() != pr {
		errs = append(errs, "Host bits are set: "+p.CIDR+" is not a network address. Did you mean "+pr.Masked().String()+"?")
	}
	if p.Gateway != "" {
		if g, err := netip.ParseAddr(p.Gateway); err != nil || !pr.Masked().Contains(g) {
			errs = append(errs, "The gateway "+p.Gateway+" is outside "+p.CIDR+".")
		}
	}
	return errs
}
