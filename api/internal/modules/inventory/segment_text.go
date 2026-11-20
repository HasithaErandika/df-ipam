package inventory

import (
	"bufio"
	"net/netip"
	"regexp"
	"strconv"
	"strings"
)

// textSegment is one network read from pasted text. Skip says why it can't be
// a segment, such as a tunnel with no address.
type textSegment struct {
	Line     int
	Name     string
	Network  netip.Prefix
	Gateway  netip.Addr
	VLAN     *int
	Firewall string
	Down     bool
	Kind     string // the interface type, such as vlan or loopback
	Skip     string
}

var (
	// "POT-IDC-00-FW-01 $ show system interface" names the firewall.
	promptLine = regexp.MustCompile(`^\s*(\S+)\s*[#$>]\s*(?:show|get)\b`)
	// FortiGate "show system interface" in its short form:
	// name  mode  ip mask  ip mask  status  ...  type  ...
	fortiLine = regexp.MustCompile(`^\s*(.+?)\s+(static|dhcp|pppoe)\s+\S+\s+\S+\s+(\d+\.\d+\.\d+\.\d+)\s+(\d+\.\d+\.\d+\.\d+)\s+(up|down)\s+\S+\s+(\S+)`)
	// "name 10.1.2.0/24" or "10.1.2.1/24 name" or just "10.1.2.0/24".
	cidrLine   = regexp.MustCompile(`(\d+\.\d+\.\d+\.\d+/\d{1,2})`)
	vlanInName = regexp.MustCompile(`(?i)vlan[ _-]?(\d{1,4})\b`)
)

// parseSegmentText reads networks from FortiGate interface output, or from
// lines that hold a CIDR and an optional name. Unrecognised lines are ignored.
func parseSegmentText(text string) []textSegment {
	var out []textSegment
	firewall := ""
	sc := bufio.NewScanner(strings.NewReader(text))
	sc.Buffer(make([]byte, 0, 64<<10), 1<<20)
	for n := 1; sc.Scan(); n++ {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "name ") {
			continue
		}
		if m := promptLine.FindStringSubmatch(line); m != nil {
			firewall = m[1]
			continue
		}
		if s, ok := parseFortiLine(line); ok {
			s.Line, s.Firewall = n, firewall
			out = append(out, s)
		} else if s, ok := parseCIDRLine(line); ok {
			s.Line = n
			out = append(out, s)
		}
	}
	return out
}

func parseFortiLine(line string) (textSegment, bool) {
	m := fortiLine.FindStringSubmatch(line)
	if m == nil {
		return textSegment{}, false
	}
	s := textSegment{Name: strings.TrimSpace(m[1]), Down: m[5] == "down", Kind: m[6]}
	ip, err1 := netip.ParseAddr(m[3])
	bits, err2 := maskBits(m[4])
	switch {
	case err1 != nil || err2 != nil:
		s.Skip = "The address or mask can't be read"
	case ip.IsUnspecified():
		s.Skip = "No address (a tunnel or unused interface)"
	case s.Kind == "loopback":
		s.Skip = "A loopback, not a network"
	default:
		setNetwork(&s, ip, bits)
	}
	s.VLAN = vlanOf(s.Name)
	return s, true
}

func parseCIDRLine(line string) (textSegment, bool) {
	loc := cidrLine.FindStringIndex(line)
	if loc == nil {
		return textSegment{}, false
	}
	p, err := netip.ParsePrefix(line[loc[0]:loc[1]])
	if err != nil || !p.Addr().Is4() {
		return textSegment{Name: strings.TrimSpace(line), Skip: "Not a valid IPv4 network"}, true
	}
	name := strings.TrimSpace(strings.Trim(line[:loc[0]]+" "+line[loc[1]:], " \t,;|"))
	s := textSegment{Name: name, VLAN: vlanOf(name)}
	setNetwork(&s, p.Addr(), p.Bits())
	if s.Name == "" {
		s.Name = s.Network.String()
	}
	return s, true
}

// setNetwork takes an interface address and prefix length. The address becomes
// the gateway when it is a host address inside the network. A /32 is the
// address itself, with no gateway.
func setNetwork(s *textSegment, ip netip.Addr, bits int) {
	s.Network = netip.PrefixFrom(ip, bits).Masked()
	if bits < 32 && ip != s.Network.Addr() {
		s.Gateway = ip
	}
}

func maskBits(mask string) (int, error) {
	m, err := netip.ParseAddr(mask)
	if err != nil || !m.Is4() {
		return 0, strconv.ErrSyntax
	}
	b := m.As4()
	v := uint32(b[0])<<24 | uint32(b[1])<<16 | uint32(b[2])<<8 | uint32(b[3])
	bits := 0
	for v&0x80000000 != 0 {
		bits, v = bits+1, v<<1
	}
	if v != 0 {
		return 0, strconv.ErrSyntax // not a contiguous mask
	}
	return bits, nil
}

func vlanOf(name string) *int {
	m := vlanInName.FindStringSubmatch(name)
	if m == nil {
		return nil
	}
	n, err := strconv.Atoi(m[1])
	if err != nil || n < 1 || n > 4094 {
		return nil
	}
	return &n
}
