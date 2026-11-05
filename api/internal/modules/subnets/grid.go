package subnets

import (
	"context"
	"dfipam/api/internal/netcalc"
	"dfipam/api/internal/platform/httpx"
	"errors"
	"math/big"
	"net/netip"

	"github.com/jackc/pgx/v5"
)

// GridCell is one address in the subnet grid. Addresses without a record are
// calculated, so a large subnet needs no rows at all.
type GridCell struct {
	Address          string  `json:"address"`
	Kind             string  `json:"kind"` // record, candidate, network, broadcast, gateway, reserved_range, dhcp
	ID               *string `json:"id,omitempty"`
	AllocationState  string  `json:"allocationState,omitempty"`
	ObservationState string  `json:"observationState,omitempty"`
	Hostname         string  `json:"hostname,omitempty"`
}

func inRanges(a netip.Addr, ranges []netcalc.Range) bool {
	for _, r := range ranges {
		if !a.Less(r.First) && !r.Last.Less(a) {
			return true
		}
	}
	return false
}

// Grid returns one page of addresses, starting offset places into the subnet.
func (s *Service) Grid(ctx context.Context, id string, offset *big.Int, limit int) ([]GridCell, string, error) {
	sub, err := s.load(ctx, s.pool, id, false)
	if err != nil {
		return nil, "", err
	}
	p := netip.MustParsePrefix(sub.CIDR)
	all := netcalc.PrefixRange(p)
	total := all.Size()
	if offset.Sign() < 0 || offset.Cmp(total) >= 0 {
		return []GridCell{}, total.String(), nil
	}
	if remaining := new(big.Int).Sub(total, offset); remaining.Cmp(big.NewInt(int64(limit))) < 0 {
		limit = int(remaining.Int64())
	}
	first := netcalc.Offset(all.First, offset)
	last := netcalc.Offset(first, big.NewInt(int64(limit-1)))
	cells, err := s.repo.Cells(ctx, s.pool, id, first.String(), last.String())
	if err != nil {
		return nil, "", err
	}
	byAddress := make(map[string]Cell, len(cells))
	for _, c := range cells {
		byAddress[c.Address] = c
	}
	dhcp, err := s.repo.DHCPRanges(ctx, s.pool, id)
	if err != nil {
		return nil, "", err
	}
	usable := netcalc.UsableRange(p)
	out := make([]GridCell, 0, limit)
	for a, i := first, 0; i < limit; a, i = a.Next(), i+1 {
		cell := GridCell{Address: a.String(), Kind: "candidate"}
		if c, ok := byAddress[a.String()]; ok {
			id := c.ID
			cell.Kind, cell.ID, cell.AllocationState, cell.ObservationState, cell.Hostname = "record", &id, c.AllocationState, c.ObservationState, c.Hostname
		}
		switch {
		case a.Less(usable.First):
			cell.Kind = "network"
		case usable.Last.Less(a):
			cell.Kind = "broadcast"
		case cell.Kind == "record":
		case sub.Gateway != nil && *sub.Gateway == a.String():
			cell.Kind = "gateway"
		case inRanges(a, sub.ReservedRanges):
			cell.Kind = "reserved_range"
		case inRanges(a, dhcp):
			cell.Kind = "dhcp"
		}
		out = append(out, cell)
	}
	return out, total.String(), nil
}

// BlockSpace is what a network block has left for new subnets.
type BlockSpace struct {
	CIDR           string      `json:"cidr"`
	Total          string      `json:"totalAddresses"`
	Free           string      `json:"freeAddresses"`
	UtilisationPct string      `json:"utilisationPct"`
	Largest        string      `json:"largestFreeCidr,omitempty"`
	FreeRanges     []FreeRange `json:"freeRanges"`
}

func (s *Service) BlockFreeSpace(ctx context.Context, id string) (BlockSpace, error) {
	var cidr, routingDomainID string
	err := s.pool.QueryRow(ctx, `SELECT cidr::text, routing_domain_id::text FROM network_blocks WHERE id = $1::uuid`, id).Scan(&cidr, &routingDomainID)
	if errors.Is(err, pgx.ErrNoRows) {
		return BlockSpace{}, httpx.ErrNotFound
	}
	if err != nil {
		return BlockSpace{}, err
	}
	rows, err := s.pool.Query(ctx, `SELECT cidr::text FROM subnets WHERE routing_domain_id = $1::uuid AND archived_at IS NULL AND cidr <<= $2::text::cidr`, routingDomainID, cidr)
	if err != nil {
		return BlockSpace{}, err
	}
	taken, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return BlockSpace{}, err
	}
	used := make([]netcalc.Range, 0, len(taken))
	for _, t := range taken {
		used = append(used, netcalc.PrefixRange(netip.MustParsePrefix(t)))
	}
	whole := netcalc.PrefixRange(netip.MustParsePrefix(cidr))
	gaps := netcalc.Gaps(whole, used)
	out := BlockSpace{CIDR: cidr, Total: whole.Size().String(), Free: netcalc.Total(gaps).String(), FreeRanges: []FreeRange{}}
	out.UtilisationPct = netcalc.UtilisationPercent(netcalc.Total(netcalc.Merge(netcalc.Clip(whole, used))), out.Total)
	best := -1
	for i, g := range gaps {
		if i < maxListed {
			out.FreeRanges = append(out.FreeRanges, toFree(g))
		}
		for _, p := range netcalc.Prefixes(g, 64) {
			if best == -1 || p.Bits() < best {
				best, out.Largest = p.Bits(), p.String()
			}
		}
	}
	return out, nil
}
