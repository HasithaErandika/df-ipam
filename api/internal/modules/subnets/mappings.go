package subnets

import (
	"context"
	"dfipam/api/internal/platform/httpx"
	"net/netip"
	"strings"
)

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

// cleanMappings validates IDs, and turns empty strings into NULL.
func cleanMappings(m *Mappings) error {
	for name, field := range map[string]**string{
		"networkBlockId": &m.NetworkBlockID, "siteId": &m.SiteID, "networkZoneId": &m.NetworkZoneID, "vdomId": &m.VDOMID,
		"vlanId": &m.VLANID, "businessUnitId": &m.BusinessUnitID, "businessServiceId": &m.BusinessServiceID,
		"technicalOwnerId": &m.TechnicalOwnerID, "businessOwnerId": &m.BusinessOwnerID,
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
