package subnets

import (
	"context"
	"net/netip"

	"dfipam/api/internal/netcalc"
)

const (
	maxListed   = 200
	maxGridPage = 1024
)

// Utilisation is the full capacity breakdown of a subnet.
type Utilisation struct {
	Total              string `json:"totalAddresses"`
	Usable             string `json:"usableAddresses"`
	Allocated          int64  `json:"allocated"`
	Reserved           int64  `json:"reserved"`
	Quarantined        int64  `json:"quarantined"`
	ReservedRanges     string `json:"reservedRangeAddresses"`
	DHCPPool           string `json:"dhcpPoolAddresses"`
	AvailableCandidate string `json:"availableCandidates"`
	UtilisationPct     string `json:"utilisationPct"`
	Level              string `json:"level"`
}

type FreeRange struct {
	First string   `json:"first"`
	Last  string   `json:"last"`
	Size  string   `json:"size"`
	CIDRs []string `json:"cidrs"`
}

// blocked returns every range that can't be offered as a candidate.
func (s *Service) blocked(ctx context.Context, sub Subnet) ([]netcalc.Range, []netcalc.Range, error) {
	held, err := s.repo.HeldAddresses(ctx, s.pool, sub.ID)
	if err != nil {
		return nil, nil, err
	}
	dhcp, err := s.repo.DHCPRanges(ctx, s.pool, sub.ID)
	if err != nil {
		return nil, nil, err
	}
	used := append(append([]netcalc.Range(nil), sub.ReservedRanges...), dhcp...)
	for _, a := range held {
		used = append(used, netcalc.Range{First: a, Last: a})
	}
	if sub.Gateway != nil {
		if a, err := netip.ParseAddr(*sub.Gateway); err == nil {
			used = append(used, netcalc.Range{First: a, Last: a})
		}
	}
	return used, dhcp, nil
}

// Analyse returns the capacity breakdown. Available candidates exclude allocated,
// reserved, quarantined and retired records, reserved ranges, DHCP pools and the gateway.
func (s *Service) Analyse(ctx context.Context, id string) (Utilisation, error) {
	sub, err := s.load(ctx, s.pool, id, false)
	if err != nil {
		return Utilisation{}, err
	}
	used, dhcp, err := s.blocked(ctx, sub)
	if err != nil {
		return Utilisation{}, err
	}
	p := netip.MustParsePrefix(sub.CIDR)
	usable := netcalc.UsableRange(p)
	r := toResponse(sub)
	return Utilisation{
		Total: r.Calculated.Total, Usable: r.Calculated.Usable, Allocated: sub.Allocated, Reserved: sub.Reserved,
		Quarantined: sub.Quarantined, ReservedRanges: r.ReservedRangeAddresses,
		DHCPPool:           netcalc.Total(netcalc.Merge(netcalc.Clip(usable, dhcp))).String(),
		AvailableCandidate: netcalc.Total(netcalc.Gaps(usable, used)).String(),
		UtilisationPct:     r.UtilisationPct, Level: r.UtilisationLevel,
	}, nil
}

func toFree(r netcalc.Range) FreeRange {
	cidrs := []string{}
	for _, p := range netcalc.Prefixes(r, 16) {
		cidrs = append(cidrs, p.String())
	}
	return FreeRange{r.First.String(), r.Last.String(), r.Size().String(), cidrs}
}

func (s *Service) FreeRanges(ctx context.Context, id string) ([]FreeRange, string, int, error) {
	sub, err := s.load(ctx, s.pool, id, false)
	if err != nil {
		return nil, "", 0, err
	}
	used, _, err := s.blocked(ctx, sub)
	if err != nil {
		return nil, "", 0, err
	}
	gaps := netcalc.Gaps(netcalc.UsableRange(netip.MustParsePrefix(sub.CIDR)), used)
	out := []FreeRange{}
	for i, g := range gaps {
		if i == maxListed {
			break
		}
		out = append(out, toFree(g))
	}
	return out, netcalc.Total(gaps).String(), len(gaps), nil
}
