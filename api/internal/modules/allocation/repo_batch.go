package allocation

import (
	"context"

	"github.com/jackc/pgx/v5"
)

// These reads serve a whole page in a fixed number of queries.

// RequestsByID returns the requests in the order of ids.
func (Repository) RequestsByID(ctx context.Context, q querier, ids []string) ([]Request, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	rows, err := q.Query(ctx, "SELECT "+requestCols+requestFrom+" WHERE r.id = ANY($1::uuid[])", ids)
	if err != nil {
		return nil, err
	}
	found, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (Request, error) { return scanRequest(r) })
	if err != nil {
		return nil, err
	}
	return inOrder(ids, found, func(r Request) string { return r.ID }), nil
}

// ApprovalsByID returns the approval steps in the order of ids.
func (Repository) ApprovalsByID(ctx context.Context, q querier, ids []string) ([]Approval, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	rows, err := q.Query(ctx, "SELECT "+approvalCols+approvalFrom+" WHERE ap.id = ANY($1::uuid[])", ids)
	if err != nil {
		return nil, err
	}
	found, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (Approval, error) { return scanApproval(r) })
	if err != nil {
		return nil, err
	}
	return inOrder(ids, found, func(a Approval) string { return a.ID }), nil
}

// inOrder sorts found into the order of ids. An ID with no row is left out.
func inOrder[T any](ids []string, found []T, key func(T) string) []T {
	byID := make(map[string]T, len(found))
	for _, v := range found {
		byID[key(v)] = v
	}
	out := make([]T, 0, len(ids))
	for _, id := range ids {
		if v, ok := byID[id]; ok {
			out = append(out, v)
		}
	}
	return out
}

// ApprovalsOf returns each request's steps in order, by request ID.
func (Repository) ApprovalsOf(ctx context.Context, q querier, requestIDs []string) (map[string][]Approval, error) {
	out := make(map[string][]Approval, len(requestIDs))
	if len(requestIDs) == 0 {
		return out, nil
	}
	rows, err := q.Query(ctx, "SELECT "+approvalCols+approvalFrom+" WHERE ap.request_id = ANY($1::uuid[]) ORDER BY ap.request_id, ap.step_order", requestIDs)
	if err != nil {
		return nil, err
	}
	list, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (Approval, error) { return scanApproval(r) })
	if err != nil {
		return nil, err
	}
	for _, a := range list {
		out[a.RequestID] = append(out[a.RequestID], a)
	}
	return out, nil
}

// MyApprovals returns, by request ID, the step of each request the user may decide now.
func (Repository) MyApprovals(ctx context.Context, q querier, requestIDs []string, user string) (map[string]string, error) {
	out := make(map[string]string, len(requestIDs))
	if len(requestIDs) == 0 {
		return out, nil
	}
	rows, err := q.Query(ctx, `SELECT r.id::text, ap.id::text FROM ip_request_approvals ap JOIN ip_requests r ON r.id = ap.request_id
		WHERE r.id = ANY($2::uuid[]) AND `+eligible, user, requestIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var request, approval string
		if err := rows.Scan(&request, &approval); err != nil {
			return nil, err
		}
		out[request] = approval
	}
	return out, rows.Err()
}

// Results finds what each request produced, or for a release, what it acts on.
func (Repository) Results(ctx context.Context, q querier, list []Request) (map[string]*Result, error) {
	var produced, reserved, released []string
	releaseOf := map[string][]string{} // allocation ID to the requests releasing it
	for _, r := range list {
		switch {
		case r.RequestType == TypeRelease && r.AllocationID != nil:
			released = append(released, *r.AllocationID)
			releaseOf[*r.AllocationID] = append(releaseOf[*r.AllocationID], r.ID)
		case r.RequestType == TypeReservation:
			reserved = append(reserved, r.ID)
		default:
			produced = append(produced, r.ID)
		}
	}
	out := make(map[string]*Result, len(list))
	if err := readResults(ctx, q, out, `SELECT request_id::text, id::text, host(address), status, NULL::timestamptz FROM ip_allocations
		WHERE request_id = ANY($1::uuid[])`, "allocation", produced, nil); err != nil {
		return nil, err
	}
	if err := readResults(ctx, q, out, `SELECT request_id::text, id::text, host(address), status, expires_at FROM ip_reservations
		WHERE request_id = ANY($1::uuid[])`, "reservation", reserved, nil); err != nil {
		return nil, err
	}
	if err := readResults(ctx, q, out, `SELECT id::text, id::text, host(address), status, NULL::timestamptz FROM ip_allocations
		WHERE id = ANY($1::uuid[])`, "allocation", released, releaseOf); err != nil {
		return nil, err
	}
	return out, nil
}

// readResults stores one query's results under their request IDs. The query's
// first column is the request ID, or with fanOut a key listing the requests.
func readResults(ctx context.Context, q querier, out map[string]*Result, sql, kind string, ids []string, fanOut map[string][]string) error {
	if len(ids) == 0 {
		return nil
	}
	rows, err := q.Query(ctx, sql, ids)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var key string
		res := Result{Kind: kind}
		if err := rows.Scan(&key, &res.ID, &res.Address, &res.Status, &res.ExpiresAt); err != nil {
			return err
		}
		if fanOut == nil {
			out[key] = &res
			continue
		}
		for _, request := range fanOut[key] {
			out[request] = &res
		}
	}
	return rows.Err()
}

// ReservationsByID returns the reservations in the order of ids.
func (Repository) ReservationsByID(ctx context.Context, q querier, ids []string) ([]Reservation, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	rows, err := q.Query(ctx, "SELECT "+reservationCols+reservationFrom+" WHERE rv.id = ANY($1::uuid[])", ids)
	if err != nil {
		return nil, err
	}
	found, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (Reservation, error) { return scanReservation(r) })
	if err != nil {
		return nil, err
	}
	return inOrder(ids, found, func(r Reservation) string { return r.ID }), nil
}
