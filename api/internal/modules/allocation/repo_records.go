package allocation

import (
	"context"
	"dfipam/api/internal/platform/httpx"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
)

const allocationCols = `al.id::text, host(al.address), al.ip_address_id::text, al.subnet_id::text, s.name, s.cidr::text,
	al.routing_domain_id::text, rd.name, al.status, i.hostname, ou.email, al.request_id::text, al.release_request_id::text,
	(SELECT x.id::text FROM ip_requests x WHERE x.allocation_id = al.id AND x.status IN (` + openStatuses + `) ORDER BY x.created_at LIMIT 1),
	abu.email, al.allocated_at, rbu.email, al.released_at`

const allocationFrom = ` FROM ip_allocations al
	JOIN subnets s ON s.id = al.subnet_id
	JOIN routing_domains rd ON rd.id = al.routing_domain_id
	JOIN ip_addresses i ON i.id = al.ip_address_id
	JOIN users abu ON abu.id = al.allocated_by
	LEFT JOIN users ou ON ou.id = i.owner_id
	LEFT JOIN users rbu ON rbu.id = al.released_by`

func scanAllocation(row pgx.Row) (Allocation, error) {
	var a Allocation
	err := row.Scan(&a.ID, &a.Address, &a.IPAddressID, &a.SubnetID, &a.SubnetName, &a.SubnetCIDR, &a.RoutingDomainID, &a.RoutingDomainName,
		&a.Status, &a.Hostname, &a.OwnerEmail, &a.RequestID, &a.ReleaseRequestID, &a.OpenReleaseID, &a.AllocatedByEmail, &a.AllocatedAt,
		&a.ReleasedByEmail, &a.ReleasedAt)
	return a, err
}

// ListFilter narrows allocation and reservation lists.
type ListFilter struct {
	Status, SubnetID, RoutingDomainID, Query string
	ExpiringWithinDays                       int
}

func (f ListFilter) where(alias string, statuses map[string]bool, defaultStatus string) where {
	var w where
	status := f.Status
	if status == "" {
		status = defaultStatus
	}
	if statuses[status] {
		w.add(alias+".status = $%d", status)
	}
	if httpx.IsUUID(f.SubnetID) {
		w.add(alias+".subnet_id = $%d::uuid", f.SubnetID)
	}
	if httpx.IsUUID(f.RoutingDomainID) {
		w.add(alias+".routing_domain_id = $%d::uuid", f.RoutingDomainID)
	}
	if f.Query != "" {
		w.add("(host("+alias+".address) LIKE $%[1]d || '%%' OR s.name ILIKE '%%' || $%[1]d || '%%')", f.Query)
	}
	return w
}

func (Repository) ListAllocations(ctx context.Context, q querier, f ListFilter, p httpx.Page) ([]Allocation, int64, error) {
	w := f.where("al", map[string]bool{"active": true, "released": true}, "active")
	var total int64
	if err := q.QueryRow(ctx, "SELECT count(*)"+allocationFrom+w.sql(), w.args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	args := append(w.args, p.Size, p.Offset())
	rows, err := q.Query(ctx, fmt.Sprintf("SELECT %s%s%s ORDER BY al.allocated_at DESC, al.id LIMIT $%d OFFSET $%d",
		allocationCols, allocationFrom, w.sql(), len(args)-1, len(args)), args...)
	if err != nil {
		return nil, 0, err
	}
	out, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (Allocation, error) { return scanAllocation(r) })
	return out, total, err
}

func (Repository) GetAllocation(ctx context.Context, q querier, id string, lock bool) (Allocation, error) {
	sql := "SELECT " + allocationCols + allocationFrom + " WHERE al.id = $1::uuid"
	if lock {
		sql += " FOR UPDATE OF al"
	}
	a, err := scanAllocation(q.QueryRow(ctx, sql, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return a, httpx.ErrNotFound
	}
	return a, err
}

const reservationCols = `rv.id::text, host(rv.address), rv.ip_address_id::text, rv.subnet_id::text, s.name, s.cidr::text,
	rv.routing_domain_id::text, rd.name, rv.status, rv.reason, rv.expires_at, rv.request_id::text,
	rv.reserved_by::text, ru.email, rv.reserved_at, rbu.email, rv.released_at`

const reservationFrom = ` FROM ip_reservations rv
	JOIN subnets s ON s.id = rv.subnet_id
	JOIN routing_domains rd ON rd.id = rv.routing_domain_id
	JOIN users ru ON ru.id = rv.reserved_by
	LEFT JOIN users rbu ON rbu.id = rv.released_by`

func scanReservation(row pgx.Row) (Reservation, error) {
	var r Reservation
	err := row.Scan(&r.ID, &r.Address, &r.IPAddressID, &r.SubnetID, &r.SubnetName, &r.SubnetCIDR, &r.RoutingDomainID, &r.RoutingDomainName,
		&r.Status, &r.Reason, &r.ExpiresAt, &r.RequestID, &r.ReservedBy, &r.ReservedByEmail, &r.ReservedAt, &r.ReleasedByEmail, &r.ReleasedAt)
	return r, err
}

var reservationStatuses = map[string]bool{"active": true, "released": true, "expired": true, "converted": true}

func (Repository) ListReservations(ctx context.Context, q querier, f ListFilter, p httpx.Page) ([]Reservation, int64, error) {
	w := f.where("rv", reservationStatuses, "active")
	if f.ExpiringWithinDays > 0 {
		w.add("rv.expires_at <= now() + make_interval(days => $%d)", f.ExpiringWithinDays)
	}
	var total int64
	if err := q.QueryRow(ctx, "SELECT count(*)"+reservationFrom+w.sql(), w.args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	args := append(w.args, p.Size, p.Offset())
	rows, err := q.Query(ctx, fmt.Sprintf("SELECT %s%s%s ORDER BY rv.expires_at NULLS LAST, rv.id LIMIT $%d OFFSET $%d",
		reservationCols, reservationFrom, w.sql(), len(args)-1, len(args)), args...)
	if err != nil {
		return nil, 0, err
	}
	out, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (Reservation, error) { return scanReservation(r) })
	return out, total, err
}

func (Repository) GetReservation(ctx context.Context, q querier, id string, lock bool) (Reservation, error) {
	sql := "SELECT " + reservationCols + reservationFrom + " WHERE rv.id = $1::uuid"
	if lock {
		sql += " FOR UPDATE OF rv"
	}
	r, err := scanReservation(q.QueryRow(ctx, sql, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return r, httpx.ErrNotFound
	}
	return r, err
}

// due returns ids of active reservations matching cond, locked for this
// transaction and skipped by any other worker.
func (Repository) due(ctx context.Context, q querier, cond string, limit int) ([]string, error) {
	rows, err := q.Query(ctx, `SELECT id::text FROM ip_reservations WHERE status = 'active' AND `+cond+`
		ORDER BY expires_at LIMIT $1 FOR UPDATE SKIP LOCKED`, limit)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowTo[string])
}
