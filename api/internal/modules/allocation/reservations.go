package allocation

import (
	"context"
	"dfipam/api/internal/audit"
	"dfipam/api/internal/auth"
	"dfipam/api/internal/modules/notifications"
	"dfipam/api/internal/platform/db"
	"dfipam/api/internal/platform/httpx"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
)

func (s *Service) ListReservations(ctx context.Context, f ListFilter, page httpx.Page) ([]Reservation, int64, error) {
	return s.repo.ListReservations(ctx, s.conn, f, page)
}

func (s *Service) GetReservation(ctx context.Context, id string) (Reservation, error) {
	return s.repo.GetReservation(ctx, s.conn, id, false)
}

func activeReservation(ctx context.Context, s *Service, tx pgx.Tx, id string) (Reservation, error) {
	rv, err := s.repo.GetReservation(ctx, tx, id, true)
	if err != nil {
		return rv, err
	}
	if rv.Status != "active" {
		return rv, errReservationEnded
	}
	return rv, nil
}

// Extend moves a reservation's expiry later. It may run at most a year ahead.
func (s *Service) Extend(ctx context.Context, p *auth.Principal, id, date, reason string) (Reservation, error) {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return Reservation{}, httpx.Validation("reason", "Say why the reservation needs longer.")
	}
	end, err := ReservationEnd(strings.TrimSpace(date))
	if err != nil {
		return Reservation{}, httpx.Validation("expiryDate", "Enter a date as YYYY-MM-DD.")
	}
	if !end.After(s.now()) {
		return Reservation{}, httpx.Validation("expiryDate", "The new expiry date must be in the future.")
	}
	if end.After(s.today().AddDate(0, 0, MaxReservationDays+1)) {
		return Reservation{}, httpx.Validation("expiryDate", "Reservations can run for at most a year from today.")
	}
	var out Reservation
	err = db.InTx(ctx, s.conn, func(tx pgx.Tx) error {
		old, err := activeReservation(ctx, s, tx, id)
		if err != nil {
			return err
		}
		if old.ExpiresAt != nil && !end.After(*old.ExpiresAt) {
			return httpx.Validation("expiryDate", "Choose a date after the current expiry.")
		}
		if _, err := tx.Exec(ctx, `UPDATE ip_reservations SET expires_at = $2, reminded_at = NULL WHERE id = $1::uuid`, id, end); err != nil {
			return err
		}
		if out, err = s.repo.GetReservation(ctx, tx, id, false); err != nil {
			return err
		}
		return audit.Write(ctx, tx, audit.Entry{ActorID: p.UserID, Action: "ip_reservation.extend", ObjectType: "ip_reservation", ObjectID: id,
			Old: map[string]any{"expiresAt": old.ExpiresAt}, New: map[string]any{"expiresAt": out.ExpiresAt, "reason": reason}})
	})
	return out, err
}

// ReleaseReservation ends a reservation early and returns the address to the
// candidate pool.
func (s *Service) ReleaseReservation(ctx context.Context, p *auth.Principal, id, reason string) (Reservation, error) {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return Reservation{}, httpx.Validation("reason", "Say why the reservation is no longer needed.")
	}
	var out Reservation
	err := db.InTx(ctx, s.conn, func(tx pgx.Tx) error {
		old, err := activeReservation(ctx, s, tx, id)
		if err != nil {
			return err
		}
		if err := s.endReservation(ctx, tx, old, "released", p.UserID); err != nil {
			return err
		}
		if out, err = s.repo.GetReservation(ctx, tx, id, false); err != nil {
			return err
		}
		if err := audit.Write(ctx, tx, audit.Entry{ActorID: p.UserID, Action: "ip_reservation.release", ObjectType: "ip_reservation", ObjectID: id,
			Old: old, New: map[string]any{"status": out.Status, "reason": reason}}); err != nil {
			return err
		}
		return notifications.Enqueue(ctx, tx, p.UserID, notifications.Event{Type: notifications.ReservationReleased,
			ObjectType: "ip_reservation", ObjectID: id, DedupeKey: "reservation_released:" + id,
			Title: "Reservation released: " + old.Address, Body: fmt.Sprintf("%s released your reservation of %s.\n\nReason: %s", p.Email, old.Address, reason),
			Link: reservationLink(id), Recipients: []string{old.ReservedBy}})
	})
	return out, err
}

func (s *Service) endReservation(ctx context.Context, tx pgx.Tx, rv Reservation, status, actor string) error {
	if _, err := tx.Exec(ctx, `UPDATE ip_reservations SET status = $2, released_by = NULLIF($3, '')::uuid, released_at = now()
		WHERE id = $1::uuid`, rv.ID, status, actor); err != nil {
		return err
	}
	_, err := s.repo.SetState(ctx, tx, rv.IPAddressID, "reserved", "available_candidate", actor, true)
	return err
}
