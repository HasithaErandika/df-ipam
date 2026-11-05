package subnets

import (
	"context"
	"dfipam/api/internal/audit"
	"dfipam/api/internal/platform/db"
	"dfipam/api/internal/platform/httpx"
	"net/http"
	"net/netip"
	"strings"

	"github.com/jackc/pgx/v5"
)

func (s *Service) Update(ctx context.Context, actor, id string, version int32, req UpdateRequest) (Response, error) {
	var updated Subnet
	err := db.InTx(ctx, s.pool, func(tx pgx.Tx) (err error) {
		updated, err = s.update(ctx, tx, actor, id, version, req)
		return err
	})
	if err != nil {
		return Response{}, err
	}
	return toResponse(updated), nil
}

func (s *Service) update(ctx context.Context, tx pgx.Tx, actor, id string, version int32, req UpdateRequest) (Subnet, error) {
	old, err := s.load(ctx, tx, id, true)
	if err != nil {
		return old, err
	}
	if old.ArchivedAt != nil {
		return old, httpx.NewError(http.StatusConflict, "SUBNET_ARCHIVED", "Archived subnets can't be edited.")
	}
	if old.Version != version {
		return old, httpx.NewError(http.StatusPreconditionFailed, "STALE_VERSION", "This subnet changed since you opened it. Reload and try again.")
	}
	next := old
	if err := applyUpdate(&next, req); err != nil {
		return old, err
	}
	if err := s.checkMappings(ctx, tx, next.RoutingDomainID, next.CIDR, Mappings{NetworkBlockID: next.NetworkBlockID, SiteID: next.SiteID,
		NetworkZoneID: next.NetworkZoneID, VDOMID: next.VDOMID, VLANID: next.VLANID}); err != nil {
		return old, err
	}
	if err := s.repo.Update(ctx, tx, actor, next); err != nil {
		return old, err
	}
	updated, err := s.load(ctx, tx, id, false)
	if err != nil {
		return updated, err
	}
	return updated, audit.Write(ctx, tx, audit.Entry{ActorID: actor, Action: "subnet.update", ObjectType: "subnet",
		ObjectID: id, Old: toResponse(old), New: toResponse(updated)})
}

// setIfPresent copies a value the request carries over the current one.
func setIfPresent[T any](dst *T, v *T) {
	if v != nil {
		*dst = *v
	}
}

// applyUpdate copies the fields the request carries onto next, checking them.
func applyUpdate(next *Subnet, req UpdateRequest) error {
	if err := applyIdentity(next, req); err != nil {
		return err
	}
	setIfPresent(&next.SecurityZone, req.SecurityZone)
	setIfPresent(&next.Description, req.Description)
	setIfPresent(&next.DiscoveryEnabled, req.DiscoveryEnabled)
	setIfPresent(&next.WarnThreshold, req.WarnThreshold)
	setIfPresent(&next.CriticalThreshold, req.CriticalThreshold)
	if err := validateThresholds(next.WarnThreshold, next.CriticalThreshold); err != nil {
		return err
	}
	setIfPresent(&next.Sensitivity, req.Sensitivity)
	return cleanMappingUpdate(next, req.Mappings)
}

func applyIdentity(next *Subnet, req UpdateRequest) error {
	if req.Name != nil {
		if next.Name = strings.TrimSpace(*req.Name); next.Name == "" {
			return httpx.Validation("name", "Enter a subnet name.")
		}
	}
	if req.Gateway != nil {
		if err := validateGateway(req.Gateway, netip.MustParsePrefix(next.CIDR)); err != nil {
			return err
		}
		next.Gateway = req.Gateway
		if *req.Gateway == "" {
			next.Gateway = nil
		}
	}
	if req.Environment != nil && !environments[*req.Environment] {
		return httpx.Validation("environment", "Choose an environment.")
	}
	setIfPresent(&next.Environment, req.Environment)
	return nil
}

// cleanMappingUpdate applies only the mappings present in the request.
func cleanMappingUpdate(next *Subnet, m Mappings) error {
	for name, pair := range map[string][2]**string{
		"networkBlockId": {&m.NetworkBlockID, &next.NetworkBlockID}, "siteId": {&m.SiteID, &next.SiteID},
		"networkZoneId": {&m.NetworkZoneID, &next.NetworkZoneID}, "vdomId": {&m.VDOMID, &next.VDOMID}, "vlanId": {&m.VLANID, &next.VLANID},
		"businessUnitId": {&m.BusinessUnitID, &next.BusinessUnitID}, "businessServiceId": {&m.BusinessServiceID, &next.BusinessServiceID},
		"technicalOwnerId": {&m.TechnicalOwnerID, &next.TechnicalOwnerID}, "businessOwnerId": {&m.BusinessOwnerID, &next.BusinessOwnerID},
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
