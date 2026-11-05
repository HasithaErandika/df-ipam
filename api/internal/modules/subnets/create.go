package subnets

import (
	"context"
	"dfipam/api/internal/audit"
	"dfipam/api/internal/netcalc"
	"dfipam/api/internal/platform/db"
	"dfipam/api/internal/platform/httpx"
	"math/big"
	"net/netip"
	"strings"

	"github.com/jackc/pgx/v5"
)

// providers maps a routing domain type to the cloud that reserves addresses in it.
var providers = map[string]string{"vpc": "aws", "vnet": "azure"}

// ProviderReserved returns the addresses AWS and Azure keep in every subnet:
// the first four and the last. Prefixes too small to hold them get none.
func ProviderReserved(p netip.Prefix) []netcalc.Range {
	all := netcalc.PrefixRange(p)
	if all.Size().Cmp(big.NewInt(8)) < 0 {
		return nil
	}
	return []netcalc.Range{
		{First: all.First, Last: netcalc.Offset(all.First, big.NewInt(3))},
		{First: all.Last, Last: all.Last},
	}
}

func (s *Service) Create(ctx context.Context, actor string, req CreateRequest) (Response, error) {
	p, warn, critical, err := checkCreate(&req)
	if err != nil {
		return Response{}, err
	}
	var created Subnet
	err = db.InTx(ctx, s.pool, func(tx pgx.Tx) (err error) {
		created, err = s.insert(ctx, tx, actor, req, p, warn, critical)
		return err
	})
	switch db.ErrorCode(err) {
	case db.CodeBadValue:
		return Response{}, httpx.Validation("body", "A value is not valid.")
	case db.CodeExclusion:
		conflicts, _ := s.repo.Overlaps(ctx, s.pool, req.RoutingDomainID, p.String())
		return Response{}, overlapError(conflicts)
	case db.CodeForeignKey:
		return Response{}, httpx.Validation("routingDomainId", "A chosen record doesn't exist.")
	}
	if err != nil {
		return Response{}, err
	}
	return toResponse(created), nil
}

// checkCreate validates and cleans a new subnet, and returns its network and thresholds.
func checkCreate(req *CreateRequest) (p netip.Prefix, warn, critical int16, err error) {
	if p, err = ParseCIDR(req.CIDR); err != nil {
		return p, 0, 0, err
	}
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		return p, 0, 0, httpx.Validation("name", "Enter a subnet name.")
	}
	if req.RoutingDomainID == "" {
		return p, 0, 0, httpx.Validation("routingDomainId", "Choose a routing domain.")
	}
	if !environments[req.Environment] {
		return p, 0, 0, httpx.Validation("environment", "Choose an environment.")
	}
	if err = validateGateway(req.Gateway, p); err != nil {
		return p, 0, 0, err
	}
	warn, critical = thresholdOr(req.WarnThreshold, 80), thresholdOr(req.CriticalThreshold, 95)
	if err = validateThresholds(warn, critical); err != nil {
		return p, 0, 0, err
	}
	if req.Gateway != nil && *req.Gateway == "" {
		req.Gateway = nil
	}
	if err = cleanMappings(&req.Mappings); err != nil {
		return p, 0, 0, err
	}
	if req.OverlapExceptionID != nil && (*req.OverlapExceptionID == "" || !httpx.IsUUID(*req.OverlapExceptionID)) {
		req.OverlapExceptionID = nil
	}
	return p, warn, critical, nil
}

func thresholdOr(v *int16, fallback int16) int16 {
	if v != nil {
		return *v
	}
	return fallback
}

func (s *Service) insert(ctx context.Context, tx pgx.Tx, actor string, req CreateRequest, p netip.Prefix, warn, critical int16) (Subnet, error) {
	if err := s.checkMappings(ctx, tx, req.RoutingDomainID, p.String(), req.Mappings); err != nil {
		return Subnet{}, err
	}
	id, err := s.repo.Insert(ctx, tx, actor, req, p.String(), warn, critical)
	if err != nil {
		return Subnet{}, err
	}
	if err := reserveForProvider(ctx, tx, id, req.RoutingDomainID, p); err != nil {
		return Subnet{}, err
	}
	created, err := s.load(ctx, tx, id, false)
	if err != nil {
		return created, err
	}
	return created, audit.Write(ctx, tx, audit.Entry{ActorID: actor, Action: "subnet.create", ObjectType: "subnet",
		ObjectID: id, New: toResponse(created)})
}

// reserveForProvider records the addresses a cloud provider keeps for itself.
func reserveForProvider(ctx context.Context, tx pgx.Tx, id, routingDomainID string, p netip.Prefix) error {
	var kind string
	if err := tx.QueryRow(ctx, `SELECT kind FROM routing_domains WHERE id = $1::uuid`, routingDomainID).Scan(&kind); err != nil {
		return err
	}
	provider, ok := providers[kind]
	if !ok {
		return nil
	}
	var starts, ends []string
	for _, r := range ProviderReserved(p) {
		starts, ends = append(starts, r.First.String()), append(ends, r.Last.String())
	}
	if len(starts) == 0 {
		return nil
	}
	_, err := tx.Exec(ctx, `INSERT INTO subnet_reserved_ranges (subnet_id, range_start, range_end, reason, provider)
		SELECT $1::uuid, first::inet, last::inet, $4, $5 FROM unnest($2::text[], $3::text[]) AS t(first, last)`,
		id, starts, ends, "Reserved by "+map[string]string{"aws": "AWS", "azure": "Azure"}[provider], provider)
	return err
}
