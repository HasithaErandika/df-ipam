package allocation

import (
	"context"
	"dfipam/api/internal/audit"
	"dfipam/api/internal/auth"
	"dfipam/api/internal/platform/db"
	"dfipam/api/internal/platform/httpx"
	"net/http"

	"github.com/jackc/pgx/v5"
)

// Reserve fulfils an approved reservation request.
func (s *Service) Reserve(ctx context.Context, p *auth.Principal, requestID string) (Reservation, bool, error) {
	var out Reservation
	created := false
	err := db.InTx(ctx, s.conn, func(tx pgx.Tx) error {
		r, done, err := s.approvedRequest(ctx, tx, requestID, TypeReservation)
		if err != nil {
			return err
		}
		if done {
			out, err = s.reservationOf(ctx, tx, r.ID)
			return err
		}
		created = true
		out, err = s.reserve(ctx, tx, p, r)
		return err
	})
	return out, created, translate(err)
}

func (s *Service) reservationOf(ctx context.Context, tx pgx.Tx, requestID string) (Reservation, error) {
	var id string
	if err := tx.QueryRow(ctx, `SELECT id::text FROM ip_reservations WHERE request_id = $1::uuid`, requestID).Scan(&id); err != nil {
		return Reservation{}, err
	}
	return s.repo.GetReservation(ctx, tx, id, false)
}

func (s *Service) reserve(ctx context.Context, tx pgx.Tx, p *auth.Principal, r Request) (Reservation, error) {
	if r.ExpiryDate == nil {
		return Reservation{}, httpx.Validation("expiryDate", "Reservations need an expiry date.")
	}
	end, err := ReservationEnd(*r.ExpiryDate)
	if err != nil {
		return Reservation{}, err
	}
	if !end.After(s.now()) {
		return Reservation{}, httpx.NewError(http.StatusConflict, "REQUEST_EXPIRED", "This reservation's expiry date has passed. Make a new request.")
	}
	ipID, addr, err := s.claim(ctx, tx, r, "reserved", p.UserID)
	if err != nil {
		return Reservation{}, err
	}
	var id string
	if err := tx.QueryRow(ctx, `INSERT INTO ip_reservations (ip_address_id, subnet_id, routing_domain_id, address, reason, expires_at,
		request_id, reserved_by)
		SELECT id, subnet_id, routing_domain_id, address, $2, $3, $4::uuid, $5::uuid FROM ip_addresses WHERE id = $1::uuid
		RETURNING id::text`, ipID, r.Justification, end, r.ID, r.RequestedBy).Scan(&id); err != nil {
		return Reservation{}, err
	}
	if err := s.repo.MarkFulfilled(ctx, tx, r.ID, StatusReserved, p.UserID); err != nil {
		return Reservation{}, err
	}
	out, err := s.repo.GetReservation(ctx, tx, id, false)
	if err != nil {
		return out, err
	}
	if err := audit.Write(ctx, tx, audit.Entry{ActorID: p.UserID, Action: "ip_reservation.create", ObjectType: "ip_reservation",
		ObjectID: id, ApprovalRef: r.ID, New: out}); err != nil {
		return out, err
	}
	return out, s.notifyFulfilled(ctx, tx, p.UserID, r, addr)
}

// Release fulfils an approved release request for the allocation.
func (s *Service) Release(ctx context.Context, p *auth.Principal, allocationID, requestID string) (Allocation, error) {
	var out Allocation
	err := db.InTx(ctx, s.conn, func(tx pgx.Tx) error {
		r, done, err := s.approvedRequest(ctx, tx, requestID, TypeRelease)
		if err != nil {
			return err
		}
		if r.AllocationID == nil || *r.AllocationID != allocationID {
			return httpx.Validation("requestId", "This release request is for a different allocation.")
		}
		if done {
			out, err = s.repo.GetAllocation(ctx, tx, allocationID, false)
			return err
		}
		out, err = s.release(ctx, tx, p, r, allocationID)
		return err
	})
	return out, translate(err)
}

func (s *Service) release(ctx context.Context, tx pgx.Tx, p *auth.Principal, r Request, allocationID string) (Allocation, error) {
	old, err := s.repo.GetAllocation(ctx, tx, allocationID, true)
	if err != nil {
		return old, err
	}
	if old.Status != "active" {
		return old, errAllocationReleased
	}
	if _, err := tx.Exec(ctx, `UPDATE ip_allocations SET status = 'released', release_request_id = $2::uuid,
		released_by = $3::uuid, released_at = now() WHERE id = $1::uuid`, allocationID, r.ID, p.UserID); err != nil {
		return old, err
	}
	// A record someone moved on by hand (quarantined, say) is left as it is.
	if _, err := s.repo.SetState(ctx, tx, old.IPAddressID, "allocated", "available_candidate", p.UserID, true); err != nil {
		return old, err
	}
	if err := s.repo.MarkFulfilled(ctx, tx, r.ID, StatusReleased, p.UserID); err != nil {
		return old, err
	}
	out, err := s.repo.GetAllocation(ctx, tx, allocationID, false)
	if err != nil {
		return out, err
	}
	if err := audit.Write(ctx, tx, audit.Entry{ActorID: p.UserID, Action: "ip_allocation.release", ObjectType: "ip_allocation",
		ObjectID: allocationID, ApprovalRef: r.ID, Old: old, New: out}); err != nil {
		return out, err
	}
	return out, s.notifyFulfilled(ctx, tx, p.UserID, r, old.Address)
}
