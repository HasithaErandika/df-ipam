package allocation

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/netip"
	"strings"

	"dfipam/api/internal/netcalc"
	"dfipam/api/internal/platform/httpx"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type querier interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
	QueryRow(context.Context, string, ...any) pgx.Row
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
}

// Repository holds the workflow's SQL. It reads subnets and addresses directly,
// because allocation must lock them in its own transaction.
type Repository struct{}

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

type where struct {
	clauses []string
	args    []any
}

func (w *where) add(clause string, v any) {
	w.args = append(w.args, v)
	w.clauses = append(w.clauses, fmt.Sprintf(clause, len(w.args)))
}

func (w *where) sql() string {
	if len(w.clauses) == 0 {
		return ""
	}
	return " WHERE " + strings.Join(w.clauses, " AND ")
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

const approvalCols = `ap.id::text, ap.request_id::text, ap.step_order, ap.approver_role_id::text, ro.name, ap.approver_id::text, au.email,
	ap.decision, ap.comment, ap.decided_at`
const approvalFrom = ` FROM ip_request_approvals ap LEFT JOIN roles ro ON ro.id = ap.approver_role_id LEFT JOIN users au ON au.id = ap.approver_id`

func scanApproval(row pgx.Row) (Approval, error) {
	var a Approval
	err := row.Scan(&a.ID, &a.RequestID, &a.StepOrder, &a.ApproverRoleID, &a.ApproverRoleName, &a.ApproverID, &a.ApproverEmail,
		&a.Decision, &a.Comment, &a.DecidedAt)
	return a, err
}

func (Repository) Approvals(ctx context.Context, q querier, requestID string) ([]Approval, error) {
	rows, err := q.Query(ctx, "SELECT "+approvalCols+approvalFrom+" WHERE ap.request_id = $1::uuid ORDER BY ap.step_order", requestID)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (Approval, error) { return scanApproval(r) })
}

func (Repository) GetApproval(ctx context.Context, q querier, id string) (Approval, error) {
	a, err := scanApproval(q.QueryRow(ctx, "SELECT "+approvalCols+approvalFrom+" WHERE ap.id = $1::uuid", id))
	if errors.Is(err, pgx.ErrNoRows) {
		return a, httpx.ErrNotFound
	}
	return a, err
}

// eligible is the condition for "user $1 may decide approval ap now".
const eligible = `r.status = 'pending_approval' AND ap.decision = 'pending' AND r.requested_by <> $1::uuid
	AND ap.step_order = (SELECT min(x.step_order) FROM ip_request_approvals x WHERE x.request_id = r.id AND x.decision = 'pending')
	AND (ap.approver_role_id IS NULL OR EXISTS (SELECT 1 FROM user_roles ur WHERE ur.user_id = $1::uuid AND ur.role_id = ap.approver_role_id))
	AND NOT EXISTS (SELECT 1 FROM ip_request_approvals y WHERE y.request_id = r.id AND y.approver_id = $1::uuid)`

// MyApproval returns the step of the request the user may decide now.
func (Repository) MyApproval(ctx context.Context, q querier, requestID, user string) (*string, error) {
	var id string
	err := q.QueryRow(ctx, `SELECT ap.id::text FROM ip_request_approvals ap JOIN ip_requests r ON r.id = ap.request_id
		WHERE r.id = $2::uuid AND `+eligible, user, requestID).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &id, nil
}

// PendingFor lists approval steps the user may decide now, oldest first.
func (Repository) PendingFor(ctx context.Context, q querier, user string, p httpx.Page) ([]string, int64, error) {
	base := ` FROM ip_request_approvals ap JOIN ip_requests r ON r.id = ap.request_id WHERE ` + eligible
	var total int64
	if err := q.QueryRow(ctx, "SELECT count(*)"+base, user).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := q.Query(ctx, "SELECT ap.id::text"+base+" ORDER BY r.submitted_at, ap.id LIMIT $2 OFFSET $3", user, p.Size, p.Offset())
	if err != nil {
		return nil, 0, err
	}
	ids, err := pgx.CollectRows(rows, pgx.RowTo[string])
	return ids, total, err
}

// Result finds what a request produced, or for a release, what it acts on.
func (Repository) Result(ctx context.Context, q querier, r Request) (*Result, error) {
	var res Result
	var err error
	switch {
	case r.RequestType == TypeRelease && r.AllocationID != nil:
		res.Kind = "allocation"
		err = q.QueryRow(ctx, `SELECT id::text, host(address), status FROM ip_allocations WHERE id = $1::uuid`, *r.AllocationID).Scan(&res.ID, &res.Address, &res.Status)
	case r.RequestType == TypeReservation:
		res.Kind = "reservation"
		err = q.QueryRow(ctx, `SELECT id::text, host(address), status, expires_at FROM ip_reservations WHERE request_id = $1::uuid`, r.ID).
			Scan(&res.ID, &res.Address, &res.Status, &res.ExpiresAt)
	default:
		res.Kind = "allocation"
		err = q.QueryRow(ctx, `SELECT id::text, host(address), status FROM ip_allocations WHERE request_id = $1::uuid`, r.ID).Scan(&res.ID, &res.Address, &res.Status)
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &res, nil
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

// Workflows loads workflows with their steps, in name order.
func (Repository) Workflows(ctx context.Context, q querier, id string) ([]Workflow, error) {
	var w where
	if id != "" {
		w.add("w.id = $%d::uuid", id)
	}
	rows, err := q.Query(ctx, `SELECT w.id::text, w.name, w.match_rules, w.is_active,
		COALESCE((SELECT jsonb_agg(jsonb_build_object('stepOrder', st.step_order, 'approverRoleId', st.approver_role_id, 'approverRoleName', ro.name)
		          ORDER BY st.step_order) FROM approval_workflow_steps st JOIN roles ro ON ro.id = st.approver_role_id
		          WHERE st.workflow_id = w.id), '[]'),
		(SELECT count(*) FROM ip_requests r WHERE r.workflow_id = w.id)
		FROM approval_workflows w`+w.sql()+` ORDER BY w.name`, w.args...)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (Workflow, error) {
		var wf Workflow
		var rules, steps []byte
		if err := r.Scan(&wf.ID, &wf.Name, &rules, &wf.IsActive, &steps, &wf.InUse); err != nil {
			return wf, err
		}
		if err := json.Unmarshal(rules, &wf.MatchRules); err != nil {
			return wf, err
		}
		return wf, json.Unmarshal(steps, &wf.Steps)
	})
}

// subnet is what fulfilment needs to know about a subnet.
type subnet struct {
	ID, RoutingDomainID, CIDR, Environment, Sensitivity string
	Gateway                                             *string
	Archived                                            bool
}

func (Repository) Subnet(ctx context.Context, q querier, id string, lock bool) (subnet, error) {
	sql := `SELECT id::text, routing_domain_id::text, cidr::text, environment, sensitivity, host(gateway), archived_at IS NOT NULL
		FROM subnets WHERE id = $1::uuid`
	if lock {
		sql += " FOR UPDATE" // serialises automatic selection per subnet
	}
	var s subnet
	err := q.QueryRow(ctx, sql, id).Scan(&s.ID, &s.RoutingDomainID, &s.CIDR, &s.Environment, &s.Sensitivity, &s.Gateway, &s.Archived)
	if errors.Is(err, pgx.ErrNoRows) {
		return s, httpx.Validation("subnetId", "That subnet doesn't exist.")
	}
	return s, err
}

func rangeOf(first, last string) (netcalc.Range, bool) {
	a, errA := netip.ParseAddr(first)
	b, errB := netip.ParseAddr(last)
	return netcalc.Range{First: a, Last: b}, errA == nil && errB == nil && a.BitLen() == b.BitLen()
}

// Space loads every range in the subnet that can't be offered: held records,
// addresses seen in use or in conflict, reserved ranges, DHCP pools, the
// gateway, and more specific subnets that own their own addresses.
func (Repository) Space(ctx context.Context, q querier, s subnet) (Space, error) {
	sp := Space{Prefix: netip.MustParsePrefix(s.CIDR)}
	rows, err := q.Query(ctx, `
		SELECT host(address), host(address) FROM ip_addresses
		 WHERE subnet_id = $1::uuid AND (allocation_state <> 'available_candidate' OR observation_state IN ('observed_active','conflict_suspected'))
		UNION ALL SELECT host(range_start), host(range_end) FROM subnet_reserved_ranges WHERE subnet_id = $1::uuid
		UNION ALL SELECT host(range_start), host(range_end) FROM dhcp_scopes WHERE subnet_id = $1::uuid
		UNION ALL SELECT host(network(c.cidr)), host(broadcast(c.cidr)) FROM subnets c
		 WHERE c.routing_domain_id = $2::uuid AND c.id <> $1::uuid AND c.archived_at IS NULL AND c.cidr << $3::cidr`,
		s.ID, s.RoutingDomainID, s.CIDR)
	if err != nil {
		return sp, err
	}
	defer rows.Close()
	for rows.Next() {
		var first, last string
		if err := rows.Scan(&first, &last); err != nil {
			return sp, err
		}
		if r, ok := rangeOf(first, last); ok {
			sp.Blocked = append(sp.Blocked, r)
		}
	}
	if s.Gateway != nil {
		if r, ok := rangeOf(*s.Gateway, *s.Gateway); ok {
			sp.Blocked = append(sp.Blocked, r)
		}
	}
	return sp, rows.Err()
}

// Claim moves one address to state (allocated or reserved), creating its record
// if it has none. It succeeds only if the record is an available candidate in
// this subnet and isn't seen in use; otherwise it changes nothing and returns
// ok=false. The row lock this takes is what settles concurrent claims.
func (Repository) Claim(ctx context.Context, q querier, s subnet, addr, state, actor string, r Request) (string, bool, error) {
	var id string
	owner := r.TechnicalOwnerID
	if owner == nil {
		owner = &r.RequestedBy
	}
	err := q.QueryRow(ctx, `
		INSERT INTO ip_addresses (routing_domain_id, subnet_id, address, allocation_state, application_id, business_service_id,
		                          owner_id, created_by, updated_by)
		VALUES ($1::uuid, $2::uuid, $3::text::inet, $4, $5::uuid, $6::uuid, $7::uuid, $8::uuid, $8::uuid)
		ON CONFLICT (routing_domain_id, address) DO UPDATE SET allocation_state = EXCLUDED.allocation_state,
		  application_id = COALESCE(EXCLUDED.application_id, ip_addresses.application_id),
		  business_service_id = COALESCE(EXCLUDED.business_service_id, ip_addresses.business_service_id),
		  owner_id = EXCLUDED.owner_id, updated_by = EXCLUDED.updated_by
		WHERE ip_addresses.allocation_state = 'available_candidate' AND ip_addresses.subnet_id = EXCLUDED.subnet_id
		  AND ip_addresses.observation_state NOT IN ('observed_active','conflict_suspected')
		RETURNING id::text`,
		s.RoutingDomainID, s.ID, addr, state, r.ApplicationID, r.BusinessServiceID, owner, actor).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", false, nil
	}
	return id, err == nil, err
}

// SetState changes a record's state only if it is currently in from.
func (Repository) SetState(ctx context.Context, q querier, ipID, from, to, actor string, clearOwner bool) (bool, error) {
	extra := ""
	if clearOwner {
		extra = ", owner_id = NULL, application_id = NULL, business_service_id = NULL"
	}
	tag, err := q.Exec(ctx, `UPDATE ip_addresses SET allocation_state = $3, updated_by = NULLIF($4, '')::uuid`+extra+`
		WHERE id = $1::uuid AND allocation_state = $2`, ipID, from, to, actor)
	return tag.RowsAffected() == 1, err
}

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
