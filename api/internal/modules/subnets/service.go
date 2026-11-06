package subnets

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/netip"
	"strconv"
	"strings"

	"dfipam/api/internal/audit"
	"dfipam/api/internal/netcalc"
	"dfipam/api/internal/platform/db"
	"dfipam/api/internal/platform/httpx"

	"github.com/jackc/pgx/v5"
)

var environments = map[string]bool{"production": true, "disaster_recovery": true, "uat": true, "development": true, "testing": true}

type Service struct {
	pool db.Conn
	repo Repository
}

func NewService(pool db.Conn) *Service { return &Service{pool: pool} }

// ParseCIDR accepts only canonical prefixes, so 10.0.0.5/24 can't pass as a network.
func ParseCIDR(value string) (netip.Prefix, error) {
	p, err := netip.ParsePrefix(strings.TrimSpace(value))
	if err != nil {
		return p, httpx.Validation("cidr", "Enter a valid CIDR, such as 10.0.0.0/24 or 2001:db8::/64.")
	}
	if p.Addr().Is4In6() {
		return p, httpx.Validation("cidr", "Enter IPv4 networks in IPv4 form.")
	}
	if p.Masked() != p {
		return p, httpx.Validation("cidr", fmt.Sprintf("Host bits are set. Did you mean %s?", p.Masked()))
	}
	return p, nil
}

func overlapError(conflicts []Conflict) error {
	msg := "This subnet overlaps another subnet in the same routing domain."
	if len(conflicts) > 0 {
		msg = fmt.Sprintf("This subnet overlaps %s (%s) in the same routing domain.", conflicts[0].CIDR, conflicts[0].Name)
	}
	return httpx.NewError(http.StatusConflict, "SUBNET_OVERLAP", msg)
}

func level(pct string, warn, critical int16) string {
	v, _ := strconv.ParseFloat(pct, 64)
	switch {
	case v >= float64(critical):
		return "critical"
	case v >= float64(warn):
		return "warning"
	}
	return "normal"
}

func toResponse(s Subnet) Response {
	p := netip.MustParsePrefix(s.CIDR)
	info := netcalc.Calculate(p)
	usable := netcalc.UsableRange(p)
	pct := netcalc.UtilisationPercent(assigned(s, usable), info.Usable)
	return Response{
		ID: s.ID, RoutingDomainID: s.RoutingDomainID, RoutingDomainName: s.RoutingDomainName, Name: s.Name,
		Gateway: s.Gateway, Environment: s.Environment, SecurityZone: s.SecurityZone, Description: s.Description,
		DiscoveryEnabled: s.DiscoveryEnabled, WarnThreshold: s.WarnThreshold, CriticalThreshold: s.CriticalThreshold,
		Sensitivity: s.Sensitivity, Version: s.Version, Archived: s.ArchivedAt != nil,
		CreatedAt: s.CreatedAt, UpdatedAt: s.UpdatedAt, Allocated: s.Allocated, Reserved: s.Reserved, Quarantined: s.Quarantined,
		ReservedRangeAddresses: netcalc.Total(netcalc.Merge(netcalc.Clip(usable, s.ReservedRanges))).String(),
		UtilisationPct:         pct, UtilisationLevel: level(pct, s.WarnThreshold, s.CriticalThreshold), Calculated: info,
		NetworkBlock: ref(s.NetworkBlockID, s.NetworkBlockName), Site: ref(s.SiteID, s.SiteName),
		NetworkZone: ref(s.NetworkZoneID, s.NetworkZoneName), VDOM: ref(s.VDOMID, s.VDOMName), VLAN: ref(s.VLANID, s.VLANName),
		BusinessUnit: ref(s.BusinessUnitID, s.BusinessUnitName), BusinessService: ref(s.BusinessServiceID, s.BusinessServiceName),
		TechnicalOwner: ref(s.TechnicalOwnerID, s.TechnicalOwnerName), BusinessOwner: ref(s.BusinessOwnerID, s.BusinessOwnerName),
		HasOverlapException: s.OverlapExceptionID != nil,
	}
}

func (s *Service) withRanges(ctx context.Context, q querier, rows []Subnet) error {
	ids := make([]string, len(rows))
	for i, r := range rows {
		ids[i] = r.ID
	}
	ranges, err := s.repo.ReservedRanges(ctx, q, ids)
	if err != nil {
		return err
	}
	for i := range rows {
		rows[i].ReservedRanges = ranges[rows[i].ID]
	}
	return nil
}

func (s *Service) load(ctx context.Context, q querier, id string, lock bool) (Subnet, error) {
	row, err := s.repo.Get(ctx, q, id, lock)
	if errors.Is(err, pgx.ErrNoRows) {
		return row, httpx.ErrNotFound
	}
	if err != nil {
		return row, err
	}
	one := []Subnet{row}
	err = s.withRanges(ctx, q, one)
	return one[0], err
}

func (s *Service) List(ctx context.Context, f Filter, p httpx.Page) ([]Response, int64, error) {
	rows, total, err := s.repo.List(ctx, s.pool, f, p)
	if err != nil {
		return nil, 0, err
	}
	if err := s.withRanges(ctx, s.pool, rows); err != nil {
		return nil, 0, err
	}
	out := make([]Response, len(rows))
	for i, r := range rows {
		out[i] = toResponse(r)
	}
	return out, total, nil
}

func (s *Service) Get(ctx context.Context, id string) (Response, error) {
	row, err := s.load(ctx, s.pool, id, false)
	if err != nil {
		return Response{}, err
	}
	return toResponse(row), nil
}

func (s *Service) CheckOverlap(ctx context.Context, req OverlapRequest) ([]Conflict, error) {
	p, err := ParseCIDR(req.CIDR)
	if err != nil {
		return nil, err
	}
	return s.repo.Overlaps(ctx, s.pool, req.RoutingDomainID, p.String())
}

func (s *Service) Archive(ctx context.Context, actor, id string) error {
	return db.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		old, err := s.load(ctx, tx, id, true)
		if err != nil {
			return err
		}
		if old.ArchivedAt != nil {
			return nil
		}
		if err := s.repo.Archive(ctx, tx, actor, id); err != nil {
			return err
		}
		return audit.Write(ctx, tx, audit.Entry{ActorID: actor, Action: "subnet.archive", ObjectType: "subnet",
			ObjectID: id, Old: toResponse(old)})
	})
}

// Delete removes a subnet nothing refers to. Anything else must be archived.
func (s *Service) Delete(ctx context.Context, actor, id string) error {
	err := db.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		old, err := s.load(ctx, tx, id, true)
		if err != nil {
			return err
		}
		if err := s.repo.Delete(ctx, tx, id); err != nil {
			return err
		}
		return audit.Write(ctx, tx, audit.Entry{ActorID: actor, Action: "subnet.delete", ObjectType: "subnet",
			ObjectID: id, Old: toResponse(old)})
	})
	if db.ErrorCode(err) == db.CodeForeignKey {
		return httpx.NewError(http.StatusConflict, "SUBNET_IN_USE", "This subnet has linked records. Archive it instead.")
	}
	return err
}

// ChangeFields lists what a change request may set on a subnet.
var ChangeFields = []db.Field{
	{Name: "technicalOwnerId", Column: "technical_owner_id", Cast: "uuid", Nullable: true},
	{Name: "businessOwnerId", Column: "business_owner_id", Cast: "uuid", Nullable: true},
}

// ApplyChange applies an approved change request, moving the version on so anyone
// editing the subnet at the same time gets a conflict.
func ApplyChange(ctx context.Context, tx pgx.Tx, id, actor string, changes map[string]string) (map[string]string, map[string]string, error) {
	return db.ApplyFields(ctx, tx, "subnets", id, actor, ", version = version + 1", ChangeFields, changes)
}
