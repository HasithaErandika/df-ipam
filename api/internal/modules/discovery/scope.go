// Package discovery runs controlled, approved discovery. Every job's scope is
// frozen when it is created: the policy's allowlist bounds it, exclusions and
// blackouts always win, sensitive networks get passive methods only, and the
// emergency stop halts everything. Results feed evidence and observation
// state only: discovery never changes allocation state or ownership.
package discovery

import (
	"fmt"
	"net/netip"
	"strconv"
	"strings"
	"time"

	"dfipam/api/internal/netcalc"
)

// Methods an agent can run. Passive methods send nothing to the targets.
var Methods = map[string]bool{"icmp": true, "arp": true, "ndp": true, "dns": true, "snmp": true, "nmap": true}

var passive = map[string]bool{"dns": true}

// SourceName maps a method to its observation source.
var SourceName = map[string]string{"icmp": "ICMP", "arp": "ARP", "ndp": "ARP", "dns": "DNS", "snmp": "SNMP", "nmap": "Nmap"}

// Sweep limits: an active sweep never covers more than this many addresses
// per target. Larger IPv4 subnets must be split; IPv6 subnets are probed only
// at their recorded addresses, never enumerated.
const (
	MaxIPv4Bits     = 20 // a /20 holds 4,096 addresses
	MaxIPv6Bits     = 120
	MaxKnownTargets = 1024
	MaxTargets      = 512
)

// Target is one frozen piece of a job's scope.
type Target struct {
	RoutingDomainID string `json:"routingDomainId"`
	CIDR            string `json:"cidr"`
}

// Exclusion is a range discovery must never touch.
type Exclusion struct {
	RoutingDomainID string // empty means every routing domain
	CIDR            netip.Prefix
}

// Allowed reports whether the whole prefix sits inside the allowlist.
func Allowed(p netip.Prefix, allowlist []netip.Prefix) bool {
	for _, a := range allowlist {
		if a.Bits() <= p.Bits() && a.Contains(p.Addr()) {
			return true
		}
	}
	return false
}

// Subtract removes excluded ranges from a prefix and returns what is left as
// prefixes. Exclusions always win over allowlists.
func Subtract(p netip.Prefix, routingDomainID string, exclusions []Exclusion) []netip.Prefix {
	var used []netcalc.Range
	for _, e := range exclusions {
		if (e.RoutingDomainID == "" || e.RoutingDomainID == routingDomainID) && e.CIDR.Addr().BitLen() == p.Addr().BitLen() && e.CIDR.Overlaps(p) {
			used = append(used, netcalc.PrefixRange(e.CIDR))
		}
	}
	if len(used) == 0 {
		return []netip.Prefix{p}
	}
	var out []netip.Prefix
	for _, g := range netcalc.Gaps(netcalc.PrefixRange(p), used) {
		out = append(out, netcalc.Prefixes(g, MaxTargets)...)
	}
	return out
}

// TooLarge reports a prefix an active sweep may not cover whole.
func TooLarge(p netip.Prefix) bool {
	if p.Addr().Is4() {
		return p.Bits() < MaxIPv4Bits
	}
	return p.Bits() < MaxIPv6Bits
}

// FilterMethods keeps the policy's methods a target may receive. Sensitive
// targets get passive methods only, unless the policy itself is an approved
// sensitive policy.
func FilterMethods(methods []string, sensitiveTarget, sensitivePolicy bool) []string {
	var out []string
	for _, m := range methods {
		if !Methods[m] {
			continue
		}
		if sensitiveTarget && !sensitivePolicy && !passive[m] {
			continue
		}
		out = append(out, m)
	}
	return out
}

// Window is a time of week when active discovery may run.
type Window struct {
	Days  []int  `json:"days"` // 1 Monday ... 7 Sunday; empty means every day
	Start string `json:"start"`
	End   string `json:"end"` // may be earlier than Start, crossing midnight
}

func minutes(hhmm string) (int, error) {
	h, m, ok := strings.Cut(hhmm, ":")
	if !ok {
		return 0, fmt.Errorf("time %q must be HH:MM", hhmm)
	}
	hi, err1 := strconv.Atoi(h)
	mi, err2 := strconv.Atoi(m)
	if err1 != nil || err2 != nil || hi < 0 || hi > 23 || mi < 0 || mi > 59 {
		return 0, fmt.Errorf("time %q must be HH:MM", hhmm)
	}
	return hi*60 + mi, nil
}

// ValidateWindows checks window definitions.
func ValidateWindows(ws []Window) error {
	for _, w := range ws {
		if _, err := minutes(w.Start); err != nil {
			return err
		}
		if _, err := minutes(w.End); err != nil {
			return err
		}
		for _, d := range w.Days {
			if d < 1 || d > 7 {
				return fmt.Errorf("days run from 1 (Monday) to 7 (Sunday)")
			}
		}
	}
	return nil
}

// InWindow reports whether t falls in any window. No windows means any time.
func InWindow(ws []Window, t time.Time) bool {
	if len(ws) == 0 {
		return true
	}
	day := int(t.Weekday())
	if day == 0 {
		day = 7
	}
	now := t.Hour()*60 + t.Minute()
	for _, w := range ws {
		start, err1 := minutes(w.Start)
		end, err2 := minutes(w.End)
		if err1 != nil || err2 != nil {
			continue
		}
		dayOK := len(w.Days) == 0
		for _, d := range w.Days {
			// A window crossing midnight belongs to the day it started.
			if d == day || (end < start && now < end && d == day-1) || (end < start && now < end && day == 1 && d == 7) {
				dayOK = true
			}
		}
		if !dayOK {
			continue
		}
		if start <= end && now >= start && now < end {
			return true
		}
		if start > end && (now >= start || now < end) {
			return true
		}
	}
	return false
}

// Cron is a parsed five-field schedule: minute, hour, day of month, month, day of week.
type Cron struct{ fields [5]map[int]bool }

var cronRanges = [5][2]int{{0, 59}, {0, 23}, {1, 31}, {1, 12}, {0, 6}}

// ParseCron reads "*", "n", "a-b", "*/n", "a-b/n" and comma lists.
func ParseCron(spec string) (Cron, error) {
	var c Cron
	parts := strings.Fields(spec)
	if len(parts) != 5 {
		return c, fmt.Errorf("a schedule needs five fields: minute hour day month weekday")
	}
	for i, part := range parts {
		set := map[int]bool{}
		lo, hi := cronRanges[i][0], cronRanges[i][1]
		for _, item := range strings.Split(part, ",") {
			step := 1
			if base, s, ok := strings.Cut(item, "/"); ok {
				n, err := strconv.Atoi(s)
				if err != nil || n < 1 {
					return c, fmt.Errorf("bad step in %q", item)
				}
				step, item = n, base
			}
			from, to := lo, hi
			switch {
			case item == "*":
			case strings.Contains(item, "-"):
				a, b, _ := strings.Cut(item, "-")
				x, err1 := strconv.Atoi(a)
				y, err2 := strconv.Atoi(b)
				if err1 != nil || err2 != nil {
					return c, fmt.Errorf("bad range %q", item)
				}
				from, to = x, y
			default:
				x, err := strconv.Atoi(item)
				if err != nil {
					return c, fmt.Errorf("bad value %q", item)
				}
				from, to = x, x
			}
			if i == 4 && to == 7 { // Sunday as 7
				to = 6
				set[0] = true
			}
			if from < lo || to > hi || from > to {
				return c, fmt.Errorf("%q is out of range %d-%d", item, lo, hi)
			}
			for v := from; v <= to; v += step {
				set[v] = true
			}
		}
		c.fields[i] = set
	}
	return c, nil
}

// Matches reports whether the schedule fires in the minute of t.
func (c Cron) Matches(t time.Time) bool {
	return c.fields[0][t.Minute()] && c.fields[1][t.Hour()] && c.fields[2][t.Day()] && c.fields[3][int(t.Month())] && c.fields[4][int(t.Weekday())]
}
