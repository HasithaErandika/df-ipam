package allocation

import (
	"context"
	"fmt"
	"net/netip"

	"dfipam/api/internal/audit"
	"dfipam/api/internal/modules/notifications"
	"dfipam/api/internal/netcalc"
	"dfipam/api/internal/platform/db"

	"github.com/jackc/pgx/v5"
)

func rangeFor(a netip.Addr) netcalc.Range { return netcalc.Range{First: a, Last: a} }

// ReminderWindow is how far ahead of expiry a reservation's holder is told.
const ReminderWindow = "7 days"

const jobBatch = 200

// JobResult counts one pass of the workflow's background work.
type JobResult struct {
	ReservationsExpired, RemindersSent, RequestsExpired int
}

// RunJobs expires due reservations, sends expiry reminders and expires stale
// requests. Each step locks rows with SKIP LOCKED, so several workers can run
// at once. The expiry of a reservation is the end date a person approved, so
// it is not an automatic release in the sense of the safety rules.
func (s *Service) RunJobs(ctx context.Context) (JobResult, error) {
	var res JobResult
	var err error
	if res.ReservationsExpired, err = s.expireReservations(ctx); err != nil {
		return res, fmt.Errorf("expire reservations: %w", err)
	}
	if res.RemindersSent, err = s.remindReservations(ctx); err != nil {
		return res, fmt.Errorf("reservation reminders: %w", err)
	}
	if res.RequestsExpired, err = s.expireRequests(ctx); err != nil {
		return res, fmt.Errorf("expire requests: %w", err)
	}
	return res, nil
}

// eachDueReservation runs fn on reservations selected by cond, read in one query.
func (s *Service) eachDueReservation(ctx context.Context, tx pgx.Tx, cond string, fn func(Reservation) error) (int, error) {
	ids, err := s.repo.due(ctx, tx, cond, jobBatch)
	if err != nil {
		return 0, err
	}
	list, err := s.repo.ReservationsByID(ctx, tx, ids)
	if err != nil {
		return 0, err
	}
	for _, rv := range list {
		if err := fn(rv); err != nil {
			return 0, err
		}
	}
	return len(list), nil
}

func (s *Service) expireReservations(ctx context.Context) (int, error) {
	n := 0
	err := db.InTx(ctx, s.conn, func(tx pgx.Tx) (err error) {
		n, err = s.eachDueReservation(ctx, tx, "expires_at <= now()", func(rv Reservation) error { return s.expireReservation(ctx, tx, rv) })
		return err
	})
	return n, err
}

func (s *Service) expireReservation(ctx context.Context, tx pgx.Tx, rv Reservation) error {
	if err := s.endReservation(ctx, tx, rv, "expired", ""); err != nil {
		return err
	}
	if err := audit.Write(ctx, tx, audit.Entry{ActorType: "system", Action: "ip_reservation.expire", ObjectType: "ip_reservation",
		ObjectID: rv.ID, Old: rv, New: map[string]string{"status": "expired"}}); err != nil {
		return err
	}
	return notifications.Enqueue(ctx, tx, "", notifications.Event{Type: notifications.ReservationExpired,
		ObjectType: "ip_reservation", ObjectID: rv.ID, DedupeKey: "reservation_expired:" + rv.ID,
		Title: "Reservation expired: " + rv.Address,
		Body:  fmt.Sprintf("Your reservation of %s in %s (%s) has expired, and the address is an available candidate again.", rv.Address, rv.SubnetName, rv.SubnetCIDR),
		Link:  reservationLink(rv.ID), Recipients: []string{rv.ReservedBy}})
}

func (s *Service) remindReservations(ctx context.Context) (int, error) {
	n := 0
	err := db.InTx(ctx, s.conn, func(tx pgx.Tx) (err error) {
		n, err = s.eachDueReservation(ctx, tx, "reminded_at IS NULL AND expires_at > now() AND expires_at <= now() + interval '"+ReminderWindow+"'",
			func(rv Reservation) error { return remindReservation(ctx, tx, rv) })
		return err
	})
	return n, err
}

func remindReservation(ctx context.Context, tx pgx.Tx, rv Reservation) error {
	if _, err := tx.Exec(ctx, `UPDATE ip_reservations SET reminded_at = now() WHERE id = $1::uuid`, rv.ID); err != nil {
		return err
	}
	return notifications.Enqueue(ctx, tx, "", notifications.Event{Type: notifications.ReservationExpiring,
		ObjectType: "ip_reservation", ObjectID: rv.ID, DedupeKey: fmt.Sprintf("reservation_expiring:%s:%s", rv.ID, rv.ExpiresAt.UTC().Format(dateLayout)),
		Title: "Reservation ending soon: " + rv.Address,
		Body: fmt.Sprintf("Your reservation of %s in %s ends on %s UTC. Extend it or request an allocation if you still need the address.",
			rv.Address, rv.SubnetName, rv.ExpiresAt.UTC().Format("2 Jan 2006 15:04")),
		Link: reservationLink(rv.ID), Recipients: []string{rv.ReservedBy}})
}

// expireRequests ends approvals nobody acted on, and reservation requests whose
// end date passed before they were fulfilled.
func (s *Service) expireRequests(ctx context.Context) (int, error) {
	n := 0
	err := db.InTx(ctx, s.conn, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `SELECT id::text FROM ip_requests
			WHERE (status = 'approved' AND decided_at < now() - make_interval(secs => $1))
			   OR (request_type = 'reservation' AND status IN ('pending_approval','approved') AND expiry_date <= current_date)
			ORDER BY created_at LIMIT $2 FOR UPDATE SKIP LOCKED`, ApprovalTTL.Seconds(), jobBatch)
		if err != nil {
			return err
		}
		ids, err := pgx.CollectRows(rows, pgx.RowTo[string])
		if err != nil {
			return err
		}
		list, err := s.repo.RequestsByID(ctx, tx, ids)
		if err != nil {
			return err
		}
		for _, r := range list {
			if err := s.expireRequest(ctx, tx, r); err != nil {
				return err
			}
			n++
		}
		return nil
	})
	return n, err
}

func (s *Service) expireRequest(ctx context.Context, tx pgx.Tx, r Request) error {
	if err := s.repo.SetStatus(ctx, tx, r.ID, StatusExpired, ""); err != nil {
		return err
	}
	if err := audit.Write(ctx, tx, audit.Entry{ActorType: "system", Action: "ip_request.expire", ObjectType: "ip_request", ObjectID: r.ID,
		Old: map[string]string{"status": r.Status}, New: map[string]string{"status": StatusExpired}}); err != nil {
		return err
	}
	return notifications.Enqueue(ctx, tx, "", notifications.Event{Type: notifications.RequestExpired,
		ObjectType: "ip_request", ObjectID: r.ID, DedupeKey: "request_expired:" + r.ID,
		Title: "Expired: " + describe(r), Body: fmt.Sprintf("Your %s expired before it was fulfilled. Make a new request if you still need it.", describe(r)),
		Link: requestLink(r.ID), Recipients: []string{r.RequestedBy}})
}
