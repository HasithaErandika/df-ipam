package subnets

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/netip"
	"slices"
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

func validateGateway(gateway *string, p netip.Prefix) error {
	if gateway == nil || *gateway == "" {
		return nil
	}
	a, err := netip.ParseAddr(strings.TrimSpace(*gateway))
	if err != nil {
		return httpx.Validation("gateway", "Enter a valid IP address.")
	}
	if !p.Contains(a) {
		return httpx.Validation("gateway", "The gateway must be inside the subnet.")
	}
	return nil
}

func validateThresholds(warn, critical int16) error {
	if warn < 1 || critical > 100 || warn >= critical {
		return httpx.Validation("warnThreshold", "Thresholds must be 1 to 100, with warning below critical.")
	}
	return nil
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
		HasOverlapException: s.OverlapExceptionID != nil, OverlapExceptionID: s.OverlapExceptionID,
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

// cleanMappings validates IDs, and turns empty strings into NULL.
func cleanMappings(m *Mappings) error {
	for name, field := range map[string]**string{
		"networkBlockId": &m.NetworkBlockID, "siteId": &m.SiteID, "networkZoneId": &m.NetworkZoneID, "vdomId": &m.VDOMID,
		"vlanId": &m.VLANID, "businessUnitId": &m.BusinessUnitID, "businessServiceId": &m.BusinessServiceID,
		"technicalOwnerId": &m.TechnicalOwnerID, "businessOwnerId": &m.BusinessOwnerID, "overlapExceptionId": &m.OverlapExceptionID,
	} {
		v := *field
		if v == nil {
			continue
		}
		if *v == "" {
			*field = nil
		} else if !httpx.IsUUID(*v) {
			return httpx.Validation(name, "Choose a valid record.")
		}
	}
	return nil
}

func (s *Service) checkMappings(ctx context.Context, q querier, routingDomainID, cidr string, m Mappings) error {
	field, issue, err := s.repo.MappingProblem(ctx, q, routingDomainID, cidr, m)
	if err != nil {
		return err
	}
	if field != "" {
		return httpx.Validation(field, issue)
	}
	return nil
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

func (s *Service) Create(ctx context.Context, actor string, req CreateRequest) (Response, error) {
	p, err := ParseCIDR(req.CIDR)
	if err != nil {
		return Response{}, err
	}
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		return Response{}, httpx.Validation("name", "Enter a subnet name.")
	}
	if req.RoutingDomainID == "" {
		return Response{}, httpx.Validation("routingDomainId", "Choose a routing domain.")
	}
	if !environments[req.Environment] {
		return Response{}, httpx.Validation("environment", "Choose an environment.")
	}
	if err := validateGateway(req.Gateway, p); err != nil {
		return Response{}, err
	}
	warn, critical := int16(80), int16(95)
	if req.WarnThreshold != nil {
		warn = *req.WarnThreshold
	}
	if req.CriticalThreshold != nil {
		critical = *req.CriticalThreshold
	}
	if err := validateThresholds(warn, critical); err != nil {
		return Response{}, err
	}
	if req.Gateway != nil && *req.Gateway == "" {
		req.Gateway = nil
	}

	if err := cleanMappings(&req.Mappings); err != nil {
		return Response{}, err
	}

	var created Subnet
	err = db.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		if err := s.checkMappings(ctx, tx, req.RoutingDomainID, p.String(), req.Mappings); err != nil {
			return err
		}
		id, err := s.repo.Insert(ctx, tx, actor, req, p.String(), warn, critical)
		if err != nil {
			return err
		}
		if err := s.repo.AddProviderRanges(ctx, tx, id, req.RoutingDomainID, p); err != nil {
			return err
		}
		if created, err = s.load(ctx, tx, id, false); err != nil {
			return err
		}
		return audit.Write(ctx, tx, audit.Entry{ActorID: actor, Action: "subnet.create", ObjectType: "subnet",
			ObjectID: id, New: toResponse(created)})
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

func (s *Service) Update(ctx context.Context, actor, id string, version int32, req UpdateRequest) (Response, error) {
	var updated Subnet
	err := db.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		old, err := s.load(ctx, tx, id, true)
		if err != nil {
			return err
		}
		updated, err = s.update(ctx, tx, actor, old, &version, req)
		return err
	})
	if err != nil {
		return Response{}, s.writeError(ctx, err, id)
	}
	return toResponse(updated), nil
}

// writeError turns database failures from a subnet change into API errors.
func (s *Service) writeError(ctx context.Context, err error, id string) error {
	switch db.ErrorCode(err) {
	case db.CodeExclusion:
		var conflicts []Conflict
		if sub, loadErr := s.load(ctx, s.pool, id, false); loadErr == nil {
			conflicts, _ = s.repo.Overlaps(ctx, s.pool, sub.RoutingDomainID, sub.CIDR)
			conflicts = without(conflicts, id)
		}
		return overlapError(conflicts)
	case db.CodeForeignKey:
		return httpx.Validation("body", "A chosen record doesn't exist.")
	case db.CodeBadValue, db.CodeCheck:
		return httpx.Validation("body", "A value is not valid.")
	}
	return err
}

func without(conflicts []Conflict, id string) []Conflict {
	out := conflicts[:0]
	for _, c := range conflicts {
		if c.ID != id {
			out = append(out, c)
		}
	}
	return out
}

// update applies a change to a locked subnet. A nil version skips the version
// check, which only bulk changes do: they never carry a form the user loaded.
func (s *Service) update(ctx context.Context, tx pgx.Tx, actor string, old Subnet, version *int32, req UpdateRequest) (Subnet, error) {
	if old.ArchivedAt != nil {
		return Subnet{}, httpx.NewError(http.StatusConflict, "SUBNET_ARCHIVED", fmt.Sprintf("Archived subnets can't be edited (%s).", old.CIDR))
	}
	if version != nil && old.Version != *version {
		return Subnet{}, httpx.NewError(http.StatusPreconditionFailed, "STALE_VERSION", "This subnet changed since you opened it. Reload and try again.")
	}
	next := old
	if req.Name != nil {
		if next.Name = strings.TrimSpace(*req.Name); next.Name == "" {
			return Subnet{}, httpx.Validation("name", "Enter a subnet name.")
		}
	}
	if req.Gateway != nil {
		if err := validateGateway(req.Gateway, netip.MustParsePrefix(old.CIDR)); err != nil {
			return Subnet{}, err
		}
		next.Gateway = req.Gateway
		if *req.Gateway == "" {
			next.Gateway = nil
		}
	}
	if req.Environment != nil {
		if !environments[*req.Environment] {
			return Subnet{}, httpx.Validation("environment", "Choose an environment.")
		}
		next.Environment = *req.Environment
	}
	if req.SecurityZone != nil {
		next.SecurityZone = strings.TrimSpace(*req.SecurityZone)
	}
	if req.Description != nil {
		next.Description = strings.TrimSpace(*req.Description)
	}
	if req.DiscoveryEnabled != nil {
		next.DiscoveryEnabled = *req.DiscoveryEnabled
	}
	if req.WarnThreshold != nil {
		next.WarnThreshold = *req.WarnThreshold
	}
	if req.CriticalThreshold != nil {
		next.CriticalThreshold = *req.CriticalThreshold
	}
	if err := validateThresholds(next.WarnThreshold, next.CriticalThreshold); err != nil {
		return Subnet{}, err
	}
	if req.Sensitivity != nil {
		if !sensitivities[*req.Sensitivity] {
			return Subnet{}, httpx.Validation("sensitivity", "Choose standard, sensitive or restricted.")
		}
		next.Sensitivity = *req.Sensitivity
	}
	if err := cleanMappingUpdate(&next, req.Mappings); err != nil {
		return Subnet{}, err
	}
	check := Mappings{NetworkBlockID: next.NetworkBlockID, SiteID: next.SiteID, NetworkZoneID: next.NetworkZoneID, VDOMID: next.VDOMID, VLANID: next.VLANID}
	if req.OverlapExceptionID != nil {
		check.OverlapExceptionID = next.OverlapExceptionID // only a newly applied exception must be current
	}
	if err := s.checkMappings(ctx, tx, next.RoutingDomainID, next.CIDR, check); err != nil {
		return Subnet{}, err
	}
	if err := s.repo.Update(ctx, tx, actor, next); err != nil {
		return Subnet{}, err
	}
	if !equal(old.OverlapExceptionID, next.OverlapExceptionID) {
		if err := s.repo.SetOverlapException(ctx, tx, old.ID, next.OverlapExceptionID); err != nil {
			return Subnet{}, err
		}
	}
	updated, err := s.load(ctx, tx, old.ID, false)
	if err != nil {
		return Subnet{}, err
	}
	return updated, audit.Write(ctx, tx, audit.Entry{ActorID: actor, Action: "subnet.update", ObjectType: "subnet",
		ObjectID: old.ID, Old: toResponse(old), New: toResponse(updated)})
}

func equal(a, b *string) bool { return (a == nil) == (b == nil) && (a == nil || *a == *b) }

var sensitivities = map[string]bool{"standard": true, "sensitive": true, "restricted": true}

// cleanMappingUpdate applies only the mappings present in the request.
func cleanMappingUpdate(next *Subnet, m Mappings) error {
	for name, pair := range map[string][2]**string{
		"networkBlockId": {&m.NetworkBlockID, &next.NetworkBlockID}, "siteId": {&m.SiteID, &next.SiteID},
		"networkZoneId": {&m.NetworkZoneID, &next.NetworkZoneID}, "vdomId": {&m.VDOMID, &next.VDOMID}, "vlanId": {&m.VLANID, &next.VLANID},
		"businessUnitId": {&m.BusinessUnitID, &next.BusinessUnitID}, "businessServiceId": {&m.BusinessServiceID, &next.BusinessServiceID},
		"technicalOwnerId": {&m.TechnicalOwnerID, &next.TechnicalOwnerID}, "businessOwnerId": {&m.BusinessOwnerID, &next.BusinessOwnerID},
		"overlapExceptionId": {&m.OverlapExceptionID, &next.OverlapExceptionID},
	} {
		v := *pair[0]
		if v == nil {
			continue
		}
		switch {
		case *v == "":
			*pair[1] = nil
		case httpx.IsUUID(*v):
			*pair[1] = v
		default:
			return httpx.Validation(name, "Choose a valid record.")
		}
	}
	return nil
}

func (s *Service) Archive(ctx context.Context, actor, id string) error {
	return db.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		old, err := s.load(ctx, tx, id, true)
		if err != nil {
			return err
		}
		return s.archive(ctx, tx, actor, old)
	})
}

func (s *Service) archive(ctx context.Context, tx pgx.Tx, actor string, old Subnet) error {
	if old.ArchivedAt != nil {
		return nil
	}
	if err := s.repo.Archive(ctx, tx, actor, old.ID); err != nil {
		return err
	}
	return audit.Write(ctx, tx, audit.Entry{ActorID: actor, Action: "subnet.archive", ObjectType: "subnet",
		ObjectID: old.ID, Old: toResponse(old)})
}

var errInUse = httpx.NewError(http.StatusConflict, "SUBNET_IN_USE", "This subnet has linked records. Archive it instead.")

// Delete removes a subnet nothing refers to. Anything else must be archived.
func (s *Service) Delete(ctx context.Context, actor, id string) error {
	err := db.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		old, err := s.load(ctx, tx, id, true)
		if err != nil {
			return err
		}
		return s.remove(ctx, tx, actor, old)
	})
	if db.ErrorCode(err) == db.CodeForeignKey {
		return errInUse
	}
	return err
}

func (s *Service) remove(ctx context.Context, tx pgx.Tx, actor string, old Subnet) error {
	if err := s.repo.Delete(ctx, tx, old.ID); err != nil {
		return err
	}
	return audit.Write(ctx, tx, audit.Entry{ActorID: actor, Action: "subnet.delete", ObjectType: "subnet",
		ObjectID: old.ID, Old: toResponse(old)})
}

// MaxBulk caps one bulk request, so a single transaction stays short.
const MaxBulk = 500

// Bulk applies one action to every listed subnet in a single transaction: if any
// subnet fails, nothing changes, and the error names that subnet.
func (s *Service) Bulk(ctx context.Context, actor string, req BulkRequest) (int, error) {
	if len(req.IDs) == 0 || len(req.IDs) > MaxBulk {
		return 0, httpx.Validation("ids", fmt.Sprintf("Choose between 1 and %d subnets.", MaxBulk))
	}
	for _, id := range req.IDs {
		if !httpx.IsUUID(id) {
			return 0, httpx.Validation("ids", "Every ID must be a valid subnet ID.")
		}
	}
	// Sorted and unique, so concurrent bulk requests lock rows in the same order.
	ids := slices.Compact(slices.Sorted(slices.Values(req.IDs)))
	switch req.Action {
	case "update":
		c := req.Changes
		if c.Name != nil || c.Gateway != nil {
			return 0, httpx.Validation("changes", "Name and gateway are set one subnet at a time.")
		}
		if c == (UpdateRequest{}) {
			return 0, httpx.Validation("changes", "Choose at least one change.")
		}
	case "archive", "delete":
	default:
		return 0, httpx.Validation("action", "Choose update, archive or delete.")
	}
	var current Subnet
	err := db.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		for _, id := range ids {
			old, err := s.load(ctx, tx, id, true)
			if err != nil {
				return err
			}
			current = old
			switch req.Action {
			case "update":
				_, err = s.update(ctx, tx, actor, old, nil, req.Changes)
			case "archive":
				err = s.archive(ctx, tx, actor, old)
			case "delete":
				err = s.remove(ctx, tx, actor, old)
			}
			if err != nil {
				return err
			}
		}
		return nil
	})
	if err == nil {
		return len(ids), nil
	}
	if db.ErrorCode(err) == db.CodeForeignKey && req.Action == "delete" {
		err = errInUse
	} else {
		err = s.writeError(ctx, err, current.ID)
	}
	var e *httpx.Error
	if errors.As(err, &e) && current.CIDR != "" {
		named := *e
		named.Message = fmt.Sprintf("%s (%s): %s", current.CIDR, current.Name, e.Message)
		if len(e.Details) > 0 && e.Code == "VALIDATION_FAILED" {
			named.Message = fmt.Sprintf("%s (%s): %s", current.CIDR, current.Name, e.Details[0].Issue)
		}
		return 0, &named
	}
	return 0, err
}
