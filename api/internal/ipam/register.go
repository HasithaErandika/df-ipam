package ipam

import (
	"context"
	"net/netip"

	"dfipam/api/internal/platform/db"
	"dfipam/api/internal/platform/httpx"
)

// RegisterIssue explains why an address can't go in the register, or returns "".
// The network and broadcast addresses never can, nor can a DHCP pool address
// (it changes: the device is approved instead). The gateway may only be
// registered as network equipment.
func RegisterIssue(s Status, ipType string) string {
	switch {
	case !s.InSegment:
		return "The IP must be inside the selected segment."
	case !s.Usable:
		return "This is the segment's network or broadcast address."
	case s.InDHCPPool:
		return "This IP is inside the DHCP pool, so it changes. Approve the device instead of registering the address."
	case s.IsGateway && ipType != "network":
		return "This is the segment's gateway. Register it with the type Network."
	case s.Reserved:
		return "This IP is held by a reservation. Convert or release the reservation first."
	}
	return ""
}

// CheckRegister returns a validation error when the address can't be registered.
// Callers lock the segment first (refuseReserved does), so a reservation can't cross it.
func CheckRegister(ctx context.Context, conn db.Conn, segmentID, ip, ipType string) error {
	addr, err := netip.ParseAddr(ip)
	if err != nil {
		return httpx.Validation("ip", "Enter a valid IPv4 address.")
	}
	st, err := Check(ctx, conn, segmentID, addr)
	if err != nil {
		return err
	}
	if issue := RegisterIssue(st, ipType); issue != "" {
		return httpx.Validation("ip", issue)
	}
	return nil
}

// RangeIssueSQL finds why a range ($1 first, $2 last) can't be held in segment
// $3: outside it, the network or broadcast address, the gateway, or the DHCP
// pool. It returns NULL when none applies.
const RangeIssueSQL = `SELECT CASE
	WHEN NOT ($1::inet << s.network AND $2::inet << s.network) THEN 'The range must be inside the selected segment.'
	WHEN masklen(s.network) <= 30 AND ($1::inet <= set_masklen(s.network::inet, 32) OR $2::inet >= set_masklen(broadcast(s.network), 32))
		THEN 'The range includes the segment''s network or broadcast address.'
	WHEN s.gateway IS NOT NULL AND s.gateway BETWEEN $1::inet AND $2::inet THEN 'The range includes the segment''s gateway.'
	WHEN s.dhcp_start IS NOT NULL AND s.dhcp_start <= $2::inet AND s.dhcp_end >= $1::inet THEN 'The range overlaps the DHCP pool. Reserve static addresses outside it.'
	END FROM segments s WHERE s.id = $3::uuid`
