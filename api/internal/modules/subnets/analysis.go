package subnets

import (
	"context"
	"errors"
	"math/big"
	"net/http"
	"net/netip"
	"strconv"
	"strings"

	"dfipam/api/internal/netcalc"
	"dfipam/api/internal/platform/httpx"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
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

func (h *Handler) utilisation(c *gin.Context) {
	id, ok := httpx.PathID(c)
	if !ok {
		return
	}
	out, err := h.svc.Analyse(c.Request.Context(), id)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, http.StatusOK, out)
}

func (h *Handler) freeRanges(c *gin.Context) {
	id, ok := httpx.PathID(c)
	if !ok {
		return
	}
	ranges, free, count, err := h.svc.FreeRanges(c.Request.Context(), id)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, http.StatusOK, gin.H{"ranges": ranges, "totalFree": free, "rangeCount": count, "truncated": count > len(ranges)})
}

func (h *Handler) addresses(c *gin.Context) {
	id, ok := httpx.PathID(c)
	if !ok {
		return
	}
	offset, valid := new(big.Int).SetString(c.DefaultQuery("offset", "0"), 10)
	limit, err := strconv.Atoi(c.DefaultQuery("limit", "256"))
	if !valid || err != nil || limit < 1 || limit > maxGridPage {
		httpx.Fail(c, httpx.Validation("limit", "Use a whole-number offset and a limit from 1 to 1024."))
		return
	}
	cells, total, err := h.svc.Grid(c.Request.Context(), id, offset, limit)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": cells, "meta": gin.H{"offset": offset.String(), "limit": limit, "total": total}})
}

func (h *Handler) blockFreeSpace(c *gin.Context) {
	id, ok := httpx.PathID(c)
	if !ok {
		return
	}
	out, err := h.svc.BlockFreeSpace(c.Request.Context(), id)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, http.StatusOK, out)
}

func (h *Handler) split(c *gin.Context) {
	var req struct {
		CIDR      string `json:"cidr"`
		NewPrefix int    `json:"newPrefix"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Fail(c, httpx.ErrBadRequest)
		return
	}
	p, err := ParseCIDR(req.CIDR)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	parts, total, err := netcalc.Split(p, req.NewPrefix, maxListed)
	if err != nil {
		httpx.Fail(c, httpx.Validation("newPrefix", err.Error()))
		return
	}
	out := make([]string, len(parts))
	for i, part := range parts {
		out[i] = part.String()
	}
	httpx.OK(c, http.StatusOK, gin.H{"subnets": out, "total": total, "truncated": total != strconv.Itoa(len(out))})
}

func (h *Handler) summarise(c *gin.Context) {
	var req struct {
		CIDRs []string `json:"cidrs"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Fail(c, httpx.ErrBadRequest)
		return
	}
	if len(req.CIDRs) == 0 || len(req.CIDRs) > 5000 {
		httpx.Fail(c, httpx.Validation("cidrs", "Enter between 1 and 5,000 CIDRs."))
		return
	}
	prefixes := make([]netip.Prefix, len(req.CIDRs))
	for i, value := range req.CIDRs {
		p, err := ParseCIDR(value)
		if err != nil {
			httpx.Fail(c, err)
			return
		}
		prefixes[i] = p
	}
	out := []string{}
	for _, p := range netcalc.Summarise(prefixes) {
		out = append(out, p.String())
	}
	httpx.OK(c, http.StatusOK, gin.H{"summary": out})
}

// locate finds the most specific subnet for an address in each routing domain,
// because the same private address can exist in several.
func (h *Handler) locate(c *gin.Context) {
	a, err := netip.ParseAddr(strings.TrimSpace(c.Query("address")))
	if err != nil {
		httpx.Fail(c, httpx.Validation("address", "Enter a valid IP address."))
		return
	}
	rows, err := h.svc.pool.Query(c.Request.Context(), `
		SELECT DISTINCT ON (s.routing_domain_id) s.id::text, s.name, s.cidr::text, s.routing_domain_id::text, rd.name
		FROM subnets s JOIN routing_domains rd ON rd.id = s.routing_domain_id
		WHERE s.archived_at IS NULL AND s.cidr >>= $1::text::inet
		ORDER BY s.routing_domain_id, masklen(s.cidr) DESC`, a.String())
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	type match struct {
		SubnetID          string `json:"subnetId"`
		SubnetName        string `json:"subnetName"`
		CIDR              string `json:"cidr"`
		RoutingDomainID   string `json:"routingDomainId"`
		RoutingDomainName string `json:"routingDomainName"`
	}
	out, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (match, error) {
		var m match
		return m, r.Scan(&m.SubnetID, &m.SubnetName, &m.CIDR, &m.RoutingDomainID, &m.RoutingDomainName)
	})
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	if out == nil {
		out = []match{}
	}
	httpx.OK(c, http.StatusOK, out)
}
