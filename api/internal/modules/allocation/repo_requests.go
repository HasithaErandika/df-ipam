package allocation

import (
	"context"
	"dfipam/api/internal/platform/httpx"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
)

const requestCols = `r.id::text, r.request_type, r.status, r.subnet_id::text, s.name, s.cidr::text, s.routing_domain_id::text, rd.name,
	r.selection_mode, host(r.requested_address), r.allocation_id::text, r.reservation_id::text,
	r.application_id::text, a.name, r.business_service_id::text, bs.name,
	r.technical_owner_id::text, tou.email, r.business_owner_id::text, bou.email,
	r.justification, r.department, r.environment, to_char(r.required_date, 'YYYY-MM-DD'), to_char(r.expiry_date, 'YYYY-MM-DD'),
	r.comments, r.requested_by::text, ru.email, r.workflow_id::text, w.name, r.version, r.created_at, r.updated_at,
	r.submitted_at, r.decided_at, r.fulfilled_at, fu.email`

const requestFrom = ` FROM ip_requests r
	JOIN subnets s ON s.id = r.subnet_id
	JOIN routing_domains rd ON rd.id = s.routing_domain_id
	JOIN users ru ON ru.id = r.requested_by
	LEFT JOIN applications a ON a.id = r.application_id
	LEFT JOIN business_services bs ON bs.id = r.business_service_id
	LEFT JOIN users tou ON tou.id = r.technical_owner_id
	LEFT JOIN users bou ON bou.id = r.business_owner_id
	LEFT JOIN users fu ON fu.id = r.fulfilled_by
	LEFT JOIN approval_workflows w ON w.id = r.workflow_id`

func scanRequest(row pgx.Row) (Request, error) {
	var r Request
	err := row.Scan(&r.ID, &r.RequestType, &r.Status, &r.SubnetID, &r.SubnetName, &r.SubnetCIDR, &r.RoutingDomainID, &r.RoutingDomainName,
		&r.SelectionMode, &r.RequestedAddress, &r.AllocationID, &r.ReservationID,
		&r.ApplicationID, &r.ApplicationName, &r.BusinessServiceID, &r.BusinessServiceName,
		&r.TechnicalOwnerID, &r.TechnicalOwnerEmail, &r.BusinessOwnerID, &r.BusinessOwnerEmail,
		&r.Justification, &r.Department, &r.Environment, &r.RequiredDate, &r.ExpiryDate,
		&r.Comments, &r.RequestedBy, &r.RequestedByEmail, &r.WorkflowID, &r.WorkflowName, &r.Version, &r.CreatedAt, &r.UpdatedAt,
		&r.SubmittedAt, &r.DecidedAt, &r.FulfilledAt, &r.FulfilledByEmail)
	return r, err
}

var requestStatuses = map[string]bool{StatusDraft: true, StatusPendingApproval: true, StatusApproved: true, StatusRejected: true,
	StatusAllocated: true, StatusReserved: true, StatusReleased: true, StatusCancelled: true, StatusExpired: true}

// open is shorthand for the statuses still in progress.
const openStatuses = "'draft','pending_approval','approved'"

func (Repository) ListRequests(ctx context.Context, q querier, f RequestFilter, p httpx.Page) ([]Request, int64, error) {
	var w where
	switch {
	case f.Status == "open":
		w.clauses = append(w.clauses, "r.status IN ("+openStatuses+")")
	case requestStatuses[f.Status]:
		w.add("r.status = $%d", f.Status)
	}
	if _, ok := FulfilledStatus[f.Type]; ok {
		w.add("r.request_type = $%d", f.Type)
	}
	if httpx.IsUUID(f.SubnetID) {
		w.add("r.subnet_id = $%d::uuid", f.SubnetID)
	}
	if f.OnlyUser != "" {
		w.add("r.requested_by = $%d::uuid", f.OnlyUser)
	}
	if f.Query != "" {
		w.add("(r.justification ILIKE '%%' || $%[1]d || '%%' OR host(r.requested_address) LIKE $%[1]d || '%%' OR s.name ILIKE '%%' || $%[1]d || '%%' OR ru.email ILIKE '%%' || $%[1]d || '%%')", f.Query)
	}
	var total int64
	if err := q.QueryRow(ctx, "SELECT count(*)"+requestFrom+w.sql(), w.args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	args := append(w.args, p.Size, p.Offset())
	rows, err := q.Query(ctx, fmt.Sprintf("SELECT %s%s%s ORDER BY r.created_at DESC, r.id LIMIT $%d OFFSET $%d",
		requestCols, requestFrom, w.sql(), len(args)-1, len(args)), args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var out []Request
	for rows.Next() {
		r, err := scanRequest(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, r)
	}
	return out, total, rows.Err()
}

func (Repository) GetRequest(ctx context.Context, q querier, id string, lock bool) (Request, error) {
	sql := "SELECT " + requestCols + requestFrom + " WHERE r.id = $1::uuid"
	if lock {
		sql += " FOR UPDATE OF r"
	}
	r, err := scanRequest(q.QueryRow(ctx, sql, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return r, httpx.ErrNotFound
	}
	return r, err
}

func (Repository) InsertRequest(ctx context.Context, q querier, actor string, in RequestInput) (string, error) {
	var id string
	err := q.QueryRow(ctx, `INSERT INTO ip_requests (request_type, subnet_id, requested_address, selection_mode, allocation_id, reservation_id,
		application_id, business_service_id, technical_owner_id, business_owner_id, justification, department, environment,
		required_date, expiry_date, comments, requested_by)
		VALUES ($1, $2::uuid, $3::text::inet, $4, $5::uuid, $6::uuid, $7::uuid, $8::uuid, $9::uuid, $10::uuid, $11, $12, $13,
		        $14::date, $15::date, $16, $17::uuid) RETURNING id::text`,
		in.RequestType, in.SubnetID, in.RequestedAddress, in.SelectionMode, in.AllocationID, in.ReservationID,
		in.ApplicationID, in.BusinessServiceID, in.TechnicalOwnerID, in.BusinessOwnerID, in.Justification, in.Department, in.Environment,
		in.RequiredDate, in.ExpiryDate, in.Comments, actor).Scan(&id)
	return id, err
}

func (Repository) UpdateRequest(ctx context.Context, q querier, id string, in RequestInput) error {
	_, err := q.Exec(ctx, `UPDATE ip_requests SET request_type = $2, subnet_id = $3::uuid, requested_address = $4::text::inet,
		selection_mode = $5, allocation_id = $6::uuid, reservation_id = $7::uuid, application_id = $8::uuid, business_service_id = $9::uuid,
		technical_owner_id = $10::uuid, business_owner_id = $11::uuid, justification = $12, department = $13, environment = $14,
		required_date = $15::date, expiry_date = $16::date, comments = $17, version = version + 1
		WHERE id = $1::uuid`,
		id, in.RequestType, in.SubnetID, in.RequestedAddress, in.SelectionMode, in.AllocationID, in.ReservationID,
		in.ApplicationID, in.BusinessServiceID, in.TechnicalOwnerID, in.BusinessOwnerID, in.Justification, in.Department, in.Environment,
		in.RequiredDate, in.ExpiryDate, in.Comments)
	return err
}

// SetStatus moves a request on. The timestamp column, when named, is set to now.
func (Repository) SetStatus(ctx context.Context, q querier, id, status, stamp string) error {
	set := ""
	switch stamp {
	case "submitted_at", "decided_at":
		set = ", " + stamp + " = now()"
	case "":
	default:
		return fmt.Errorf("unknown timestamp %q", stamp)
	}
	_, err := q.Exec(ctx, "UPDATE ip_requests SET status = $2, version = version + 1"+set+" WHERE id = $1::uuid", id, status)
	return err
}

func (Repository) MarkFulfilled(ctx context.Context, q querier, id, status, actor string) error {
	_, err := q.Exec(ctx, `UPDATE ip_requests SET status = $2, fulfilled_at = now(), fulfilled_by = $3::uuid, version = version + 1
		WHERE id = $1::uuid`, id, status, actor)
	return err
}
