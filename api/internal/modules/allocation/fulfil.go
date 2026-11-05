package allocation

import (
	"context"
	"net/netip"

	"dfipam/api/internal/audit"
	"dfipam/api/internal/auth"
	"dfipam/api/internal/platform/db"
	"dfipam/api/internal/platform/httpx"

	"net/http"

	"github.com/jackc/pgx/v5"
)

// claim takes an address for the request, under the subnet lock. Automatic
// selection retries past addresses taken by someone else in the meantime.
func (s *Service) claim(ctx context.Context, tx pgx.Tx, r Request, state, actor string) (string, string, error) {
	sub, err := s.repo.Subnet(ctx, tx, r.SubnetID, true)
	if err != nil {
		return "", "", err
	}
	if sub.Archived {
		return "", "", errArchived
	}
	space, err := s.repo.Space(ctx, tx, sub)
	if err != nil {
		return "", "", err
	}
	if r.SelectionMode == "specific" && r.RequestedAddress != nil {
		return s.claimSpecific(ctx, tx, sub, space, r, state, actor)
	}
	for attempt := 0; attempt < maxPickAttempts; attempt++ {
		a, ok := NextFree(space)
		if !ok {
			return "", "", errExhausted
		}
		id, ok, err := s.repo.Claim(ctx, tx, sub, a.String(), state, actor, r)
		if err != nil {
			return "", "", err
		}
		if ok {
			return id, a.String(), nil
		}
		space.Blocked = append(space.Blocked, rangeFor(a))
	}
	return "", "", errUnavailable
}

func (s *Service) claimSpecific(ctx context.Context, tx pgx.Tx, sub subnet, space Space, r Request, state, actor string) (string, string, error) {
	a := netip.MustParseAddr(*r.RequestedAddress)
	if err := CheckSpecific(space, a); err != nil {
		return "", "", err
	}
	id, ok, err := s.repo.Claim(ctx, tx, sub, a.String(), state, actor, r)
	if err != nil {
		return "", "", err
	}
	if !ok {
		return "", "", errUnavailable
	}
	return id, a.String(), nil
}

// Allocate fulfils an approved allocation request.
func (s *Service) Allocate(ctx context.Context, p *auth.Principal, requestID string) (Allocation, bool, error) {
	var out Allocation
	created := false
	err := db.InTx(ctx, s.conn, func(tx pgx.Tx) error {
		r, done, err := s.approvedRequest(ctx, tx, requestID, TypeAllocation)
		if err != nil {
			return err
		}
		if done {
			out, err = s.allocationOf(ctx, tx, r.ID)
			return err
		}
		created = true
		out, err = s.allocate(ctx, tx, p, r)
		return err
	})
	return out, created, translate(err)
}

func (s *Service) allocationOf(ctx context.Context, tx pgx.Tx, requestID string) (Allocation, error) {
	var id string
	if err := tx.QueryRow(ctx, `SELECT id::text FROM ip_allocations WHERE request_id = $1::uuid`, requestID).Scan(&id); err != nil {
		return Allocation{}, err
	}
	return s.repo.GetAllocation(ctx, tx, id, false)
}

func (s *Service) allocate(ctx context.Context, tx pgx.Tx, p *auth.Principal, r Request) (Allocation, error) {
	ipID, addr, err := s.takeAddress(ctx, tx, p, r)
	if err != nil {
		return Allocation{}, err
	}
	var id string
	if err := tx.QueryRow(ctx, `INSERT INTO ip_allocations (ip_address_id, subnet_id, routing_domain_id, address, request_id, allocated_by)
		SELECT id, subnet_id, routing_domain_id, address, $2::uuid, $3::uuid FROM ip_addresses WHERE id = $1::uuid
		RETURNING id::text`, ipID, r.ID, p.UserID).Scan(&id); err != nil {
		return Allocation{}, err
	}
	if err := s.repo.MarkFulfilled(ctx, tx, r.ID, StatusAllocated, p.UserID); err != nil {
		return Allocation{}, err
	}
	out, err := s.repo.GetAllocation(ctx, tx, id, false)
	if err != nil {
		return out, err
	}
	if err := audit.Write(ctx, tx, audit.Entry{ActorID: p.UserID, Action: "ip_allocation.create", ObjectType: "ip_allocation",
		ObjectID: id, ApprovalRef: r.ID, New: out}); err != nil {
		return out, err
	}
	return out, s.notifyFulfilled(ctx, tx, p.UserID, r, addr)
}

// takeAddress claims an address, or converts the reservation the request names.
func (s *Service) takeAddress(ctx context.Context, tx pgx.Tx, p *auth.Principal, r Request) (string, string, error) {
	if r.ReservationID == nil {
		return s.claim(ctx, tx, r, "allocated", p.UserID)
	}
	rv, err := s.repo.GetReservation(ctx, tx, *r.ReservationID, true)
	if err != nil {
		return "", "", err
	}
	if rv.Status != "active" {
		return "", "", httpx.NewError(http.StatusConflict, "RESERVATION_NOT_ACTIVE", "The reservation being converted is no longer active.")
	}
	if _, err := tx.Exec(ctx, `UPDATE ip_reservations SET status = 'converted', released_by = $2::uuid, released_at = now()
		WHERE id = $1::uuid`, rv.ID, p.UserID); err != nil {
		return "", "", err
	}
	ok, err := s.repo.SetState(ctx, tx, rv.IPAddressID, "reserved", "allocated", p.UserID, false)
	if err != nil {
		return "", "", err
	}
	if !ok {
		return "", "", errUnavailable
	}
	owner := r.TechnicalOwnerID
	if owner == nil {
		owner = &r.RequestedBy
	}
	_, err = tx.Exec(ctx, `UPDATE ip_addresses SET owner_id = $2::uuid,
		application_id = COALESCE($3::uuid, application_id), business_service_id = COALESCE($4::uuid, business_service_id)
		WHERE id = $1::uuid`, rv.IPAddressID, owner, r.ApplicationID, r.BusinessServiceID)
	return rv.IPAddressID, rv.Address, err
}
