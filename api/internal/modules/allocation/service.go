package allocation

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/netip"
	"strings"
	"time"

	"dfipam/api/internal/audit"
	"dfipam/api/internal/auth"
	"dfipam/api/internal/modules/notifications"
	"dfipam/api/internal/platform/db"
	"dfipam/api/internal/platform/httpx"

	"github.com/jackc/pgx/v5"
)

// maxPickAttempts bounds retries when an address picked automatically is taken
// between reading the subnet and claiming the address.
const maxPickAttempts = 8

type Service struct {
	conn db.Conn
	repo Repository
	now  func() time.Time
}

func NewService(conn db.Conn) *Service { return &Service{conn: conn, now: time.Now} }

func (s *Service) today() time.Time {
	t := s.now().UTC()
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
}

// CanSeeAll reports whether the caller may see everyone's requests. Others see
// only their own.
func CanSeeAll(p *auth.Principal) bool {
	return p.Has(auth.RequestsApprove) || p.Has(auth.IPAllocate) || p.Has(auth.IPReserve) || p.Has(auth.AuditView)
}

func requestLink(id string) string     { return "/requests?id=" + id }
func reservationLink(id string) string { return "/reservations?id=" + id }

func describe(r Request) string {
	what := map[string]string{TypeAllocation: "IP allocation", TypeReservation: "IP reservation", TypeRelease: "IP release"}[r.RequestType]
	target := r.SubnetCIDR
	if r.RequestedAddress != nil {
		target = *r.RequestedAddress
	}
	return fmt.Sprintf("%s request for %s (%s)", what, target, r.SubnetName)
}

// complete adds the approval steps, the result and the caller's open step.
func (s *Service) complete(ctx context.Context, q querier, r *Request, viewer *auth.Principal) error {
	var err error
	if r.Approvals, err = s.repo.Approvals(ctx, q, r.ID); err != nil {
		return err
	}
	if r.Approvals == nil {
		r.Approvals = []Approval{}
	}
	if r.Result, err = s.repo.Result(ctx, q, *r); err != nil {
		return err
	}
	if viewer != nil && viewer.Has(auth.RequestsApprove) {
		r.MyApprovalID, err = s.repo.MyApproval(ctx, q, r.ID, viewer.UserID)
	}
	return err
}

func (s *Service) ListRequests(ctx context.Context, p *auth.Principal, f RequestFilter, page httpx.Page) ([]Request, int64, error) {
	if f.Mine || !CanSeeAll(p) {
		f.OnlyUser = p.UserID
	}
	rows, total, err := s.repo.ListRequests(ctx, s.conn, f, page)
	if err != nil {
		return nil, 0, err
	}
	for i := range rows {
		rows[i].Approvals = []Approval{}
	}
	return rows, total, nil
}

func (s *Service) GetRequest(ctx context.Context, p *auth.Principal, id string) (Request, error) {
	r, err := s.repo.GetRequest(ctx, s.conn, id, false)
	if err != nil {
		return r, err
	}
	if !CanSeeAll(p) && r.RequestedBy != p.UserID {
		return Request{}, httpx.ErrNotFound
	}
	return r, s.complete(ctx, s.conn, &r, p)
}

func inputOf(r Request) RequestInput {
	return RequestInput{RequestType: r.RequestType, SubnetID: r.SubnetID, SelectionMode: r.SelectionMode,
		RequestedAddress: r.RequestedAddress, AllocationID: r.AllocationID, ReservationID: r.ReservationID,
		ApplicationID: r.ApplicationID, BusinessServiceID: r.BusinessServiceID, TechnicalOwnerID: r.TechnicalOwnerID,
		BusinessOwnerID: r.BusinessOwnerID, Justification: r.Justification, Department: r.Department, Environment: r.Environment,
		RequiredDate: r.RequiredDate, ExpiryDate: r.ExpiryDate, Comments: r.Comments}
}

// resolve checks a request against the database: the subnet, the address and
// the allocation or reservation it refers to. It fills in the subnet and
// address that a release or conversion takes from its target. self is the
// request's own ID when it already exists.
func (s *Service) resolve(ctx context.Context, q querier, in RequestInput, self string) (RequestInput, error) {
	switch {
	case in.RequestType == TypeRelease:
		al, err := s.repo.GetAllocation(ctx, q, *in.AllocationID, false)
		if errors.Is(err, httpx.ErrNotFound) {
			return in, httpx.Validation("allocationId", "That allocation doesn't exist.")
		}
		if err != nil {
			return in, err
		}
		if al.Status != "active" {
			return in, httpx.NewError(http.StatusConflict, "ALLOCATION_RELEASED", "This allocation has already been released.")
		}
		if al.OpenReleaseID != nil && *al.OpenReleaseID != self {
			return in, httpx.NewError(http.StatusConflict, "RELEASE_ALREADY_REQUESTED", "A release of this allocation is already in progress.")
		}
		in.SubnetID, in.RequestedAddress = al.SubnetID, &al.Address
		return in, nil
	case in.ReservationID != nil:
		rv, err := s.repo.GetReservation(ctx, q, *in.ReservationID, false)
		if errors.Is(err, httpx.ErrNotFound) {
			return in, httpx.Validation("reservationId", "That reservation doesn't exist.")
		}
		if err != nil {
			return in, err
		}
		if rv.Status != "active" {
			return in, httpx.NewError(http.StatusConflict, "RESERVATION_NOT_ACTIVE", "This reservation is no longer active.")
		}
		in.SubnetID, in.RequestedAddress = rv.SubnetID, &rv.Address
		return in, nil
	}
	sub, err := s.repo.Subnet(ctx, q, in.SubnetID, false)
	if err != nil {
		return in, err
	}
	if sub.Archived {
		return in, errArchived
	}
	if in.SelectionMode == "specific" {
		space, err := s.repo.Space(ctx, q, sub)
		if err != nil {
			return in, err
		}
		if err := CheckSpecific(space, netip.MustParseAddr(*in.RequestedAddress)); err != nil {
			return in, err
		}
	}
	return in, nil
}

func (s *Service) CreateRequest(ctx context.Context, p *auth.Principal, in RequestInput) (Request, error) {
	in, err := Normalise(in, s.today())
	if err != nil {
		return Request{}, err
	}
	var out Request
	err = db.InTx(ctx, s.conn, func(tx pgx.Tx) error {
		if in, err = s.resolve(ctx, tx, in, ""); err != nil {
			return err
		}
		id, err := s.repo.InsertRequest(ctx, tx, p.UserID, in)
		if err != nil {
			return err
		}
		r, err := s.repo.GetRequest(ctx, tx, id, true)
		if err != nil {
			return err
		}
		if err := audit.Write(ctx, tx, audit.Entry{ActorID: p.UserID, Action: "ip_request.create", ObjectType: "ip_request", ObjectID: id, New: r}); err != nil {
			return err
		}
		if in.Submit {
			if err := s.submit(ctx, tx, p, r); err != nil {
				return err
			}
		}
		out, err = s.repo.GetRequest(ctx, tx, id, false)
		if err != nil {
			return err
		}
		return s.complete(ctx, tx, &out, p)
	})
	return out, translate(err)
}

// own loads and locks a request the caller made, in one of the given statuses.
func (s *Service) own(ctx context.Context, q querier, p *auth.Principal, id string, statuses ...string) (Request, error) {
	r, err := s.repo.GetRequest(ctx, q, id, true)
	if err != nil {
		return r, err
	}
	if r.RequestedBy != p.UserID {
		if CanSeeAll(p) {
			return r, httpx.NewError(http.StatusForbidden, "NOT_REQUESTER", "Only the person who made this request can change it.")
		}
		return r, httpx.ErrNotFound
	}
	for _, st := range statuses {
		if r.Status == st {
			return r, nil
		}
	}
	return r, badState("This request can't be changed in its current status (" + strings.ReplaceAll(r.Status, "_", " ") + ").")
}

func (s *Service) UpdateRequest(ctx context.Context, p *auth.Principal, id string, version int32, in RequestInput) (Request, error) {
	in, err := Normalise(in, s.today())
	if err != nil {
		return Request{}, err
	}
	var out Request
	err = db.InTx(ctx, s.conn, func(tx pgx.Tx) error {
		old, err := s.own(ctx, tx, p, id, StatusDraft)
		if err != nil {
			return err
		}
		if old.Version != version {
			return httpx.NewError(http.StatusPreconditionFailed, "STALE_VERSION", "This request changed since you opened it. Reload and try again.")
		}
		if in, err = s.resolve(ctx, tx, in, id); err != nil {
			return err
		}
		if err := s.repo.UpdateRequest(ctx, tx, id, in); err != nil {
			return err
		}
		updated, err := s.repo.GetRequest(ctx, tx, id, false)
		if err != nil {
			return err
		}
		if err := audit.Write(ctx, tx, audit.Entry{ActorID: p.UserID, Action: "ip_request.update", ObjectType: "ip_request", ObjectID: id, Old: old, New: updated}); err != nil {
			return err
		}
		if in.Submit {
			if err := s.submit(ctx, tx, p, updated); err != nil {
				return err
			}
		}
		if out, err = s.repo.GetRequest(ctx, tx, id, false); err != nil {
			return err
		}
		return s.complete(ctx, tx, &out, p)
	})
	return out, translate(err)
}

func (s *Service) DeleteRequest(ctx context.Context, p *auth.Principal, id string) error {
	return db.InTx(ctx, s.conn, func(tx pgx.Tx) error {
		old, err := s.own(ctx, tx, p, id, StatusDraft)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `DELETE FROM ip_requests WHERE id = $1::uuid`, id); err != nil {
			return err
		}
		return audit.Write(ctx, tx, audit.Entry{ActorID: p.UserID, Action: "ip_request.delete", ObjectType: "ip_request", ObjectID: id, Old: old})
	})
}

func (s *Service) Submit(ctx context.Context, p *auth.Principal, id string) (Request, error) {
	var out Request
	err := db.InTx(ctx, s.conn, func(tx pgx.Tx) error {
		r, err := s.own(ctx, tx, p, id, StatusDraft)
		if err != nil {
			return err
		}
		if err := s.submit(ctx, tx, p, r); err != nil {
			return err
		}
		if out, err = s.repo.GetRequest(ctx, tx, id, false); err != nil {
			return err
		}
		return s.complete(ctx, tx, &out, p)
	})
	return out, translate(err)
}

// submit freezes the approval steps from the matching workflow and tells the
// first step's approvers.
func (s *Service) submit(ctx context.Context, tx pgx.Tx, p *auth.Principal, r Request) error {
	if _, err := s.resolve(ctx, tx, inputOf(r), r.ID); err != nil {
		return err
	}
	sub, err := s.repo.Subnet(ctx, tx, r.SubnetID, false)
	if err != nil {
		return err
	}
	env := sub.Environment
	if r.Environment != nil {
		env = *r.Environment
	}
	workflows, err := s.repo.Workflows(ctx, tx, "")
	if err != nil {
		return err
	}
	wf := ChooseWorkflow(workflows, Target{RequestType: r.RequestType, Environment: env, Sensitivity: sub.Sensitivity})
	var workflowID *string
	steps := []*string{nil} // default: one step for anyone with the approve permission
	if wf != nil {
		workflowID = &wf.ID
		steps = steps[:0]
		for _, st := range wf.Steps {
			role := st.ApproverRoleID
			steps = append(steps, &role)
		}
	}
	for i, role := range steps {
		if _, err := tx.Exec(ctx, `INSERT INTO ip_request_approvals (request_id, step_order, approver_role_id) VALUES ($1::uuid, $2, $3::uuid)`,
			r.ID, i+1, role); err != nil {
			return err
		}
	}
	if _, err := tx.Exec(ctx, `UPDATE ip_requests SET workflow_id = $2::uuid WHERE id = $1::uuid`, r.ID, workflowID); err != nil {
		return err
	}
	if err := s.repo.SetStatus(ctx, tx, r.ID, StatusPendingApproval, "submitted_at"); err != nil {
		return err
	}
	if err := audit.Write(ctx, tx, audit.Entry{ActorID: p.UserID, Action: "ip_request.submit", ObjectType: "ip_request", ObjectID: r.ID,
		New: map[string]any{"workflowId": workflowID, "steps": len(steps)}}); err != nil {
		return err
	}
	return s.notifyStep(ctx, tx, p.UserID, r, 1, steps[0])
}

func (s *Service) notifyStep(ctx context.Context, tx pgx.Tx, actor string, r Request, step int, role *string) error {
	approvers, err := notifications.Approvers(ctx, tx, role)
	if err != nil {
		return err
	}
	var recipients []string
	for _, a := range approvers {
		if a != r.RequestedBy {
			recipients = append(recipients, a)
		}
	}
	return notifications.Enqueue(ctx, tx, actor, notifications.Event{
		Type: notifications.ApprovalRequested, ObjectType: "ip_request", ObjectID: r.ID,
		DedupeKey: fmt.Sprintf("approval_requested:%s:%d", r.ID, step),
		Title:     "Approval needed: " + describe(r),
		Body:      fmt.Sprintf("%s asked for an %s.\n\nJustification: %s", r.RequestedByEmail, describe(r), r.Justification),
		Link:      requestLink(r.ID), Recipients: recipients,
	})
}

func (s *Service) Cancel(ctx context.Context, p *auth.Principal, id string) (Request, error) {
	var out Request
	err := db.InTx(ctx, s.conn, func(tx pgx.Tx) error {
		r, err := s.own(ctx, tx, p, id, StatusDraft, StatusPendingApproval, StatusApproved)
		if err != nil {
			return err
		}
		if err := s.repo.SetStatus(ctx, tx, id, StatusCancelled, ""); err != nil {
			return err
		}
		if err := audit.Write(ctx, tx, audit.Entry{ActorID: p.UserID, Action: "ip_request.cancel", ObjectType: "ip_request", ObjectID: id,
			Old: map[string]string{"status": r.Status}, New: map[string]string{"status": StatusCancelled}}); err != nil {
			return err
		}
		if out, err = s.repo.GetRequest(ctx, tx, id, false); err != nil {
			return err
		}
		return s.complete(ctx, tx, &out, p)
	})
	return out, err
}

// Pending lists the approval steps the caller may decide now.
func (s *Service) Pending(ctx context.Context, p *auth.Principal, page httpx.Page) ([]PendingApproval, int64, error) {
	ids, total, err := s.repo.PendingFor(ctx, s.conn, p.UserID, page)
	if err != nil {
		return nil, 0, err
	}
	out := make([]PendingApproval, 0, len(ids))
	for _, id := range ids {
		ap, err := s.repo.GetApproval(ctx, s.conn, id)
		if err != nil {
			return nil, 0, err
		}
		r, err := s.repo.GetRequest(ctx, s.conn, ap.RequestID, false)
		if err != nil {
			return nil, 0, err
		}
		if err := s.complete(ctx, s.conn, &r, p); err != nil {
			return nil, 0, err
		}
		out = append(out, PendingApproval{Approval: ap, Request: r})
	}
	return out, total, nil
}

// Decide approves or rejects one step. Self-approval, deciding two steps of one
// request, deciding out of order and lacking the step's role are all refused;
// the database refuses the first two as well.
func (s *Service) Decide(ctx context.Context, p *auth.Principal, approvalID string, approve bool, comment string) (Request, error) {
	comment = strings.TrimSpace(comment)
	if !approve && comment == "" {
		return Request{}, httpx.Validation("comment", "Say why you are rejecting this request.")
	}
	if len(comment) > 2000 {
		return Request{}, httpx.Validation("comment", "Keep the comment under 2,000 characters.")
	}
	var out Request
	err := db.InTx(ctx, s.conn, func(tx pgx.Tx) error {
		ap, err := s.repo.GetApproval(ctx, tx, approvalID)
		if err != nil {
			return err
		}
		r, err := s.repo.GetRequest(ctx, tx, ap.RequestID, true) // serialises decisions on one request
		if err != nil {
			return err
		}
		if r.RequestedBy == p.UserID {
			return errSelf
		}
		if r.Status != StatusPendingApproval {
			return errNotCurrent
		}
		steps, err := s.repo.Approvals(ctx, tx, r.ID)
		if err != nil {
			return err
		}
		var current *Approval
		var next *Approval
		for i := range steps {
			if steps[i].ApproverID != nil && *steps[i].ApproverID == p.UserID {
				return errSeparation
			}
			if steps[i].Decision == "pending" {
				if current == nil {
					current = &steps[i]
				} else if next == nil {
					next = &steps[i]
				}
			}
		}
		if current == nil || current.ID != approvalID {
			return errNotCurrent
		}
		if current.ApproverRoleID != nil {
			var held bool
			if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM user_roles WHERE user_id = $1::uuid AND role_id = $2::uuid)`,
				p.UserID, *current.ApproverRoleID).Scan(&held); err != nil {
				return err
			}
			if !held {
				return errNotEligible
			}
		}
		decision := "rejected"
		if approve {
			decision = "approved"
		}
		if _, err := tx.Exec(ctx, `UPDATE ip_request_approvals SET approver_id = $2::uuid, decision = $3, comment = $4, decided_at = now()
			WHERE id = $1::uuid AND decision = 'pending'`, approvalID, p.UserID, decision, comment); err != nil {
			switch db.ErrorCode(err) {
			case db.CodeUnique:
				return errSeparation
			case db.CodeCheck:
				return errSelf
			}
			return err
		}
		action := "ip_request.approve"
		switch {
		case !approve:
			action = "ip_request.reject"
			if err := s.repo.SetStatus(ctx, tx, r.ID, StatusRejected, "decided_at"); err != nil {
				return err
			}
			if err := notifications.Enqueue(ctx, tx, p.UserID, notifications.Event{Type: notifications.RequestRejected,
				ObjectType: "ip_request", ObjectID: r.ID, DedupeKey: "request_rejected:" + r.ID,
				Title: "Rejected: " + describe(r), Body: fmt.Sprintf("%s rejected your %s.\n\nReason: %s", p.Email, describe(r), comment),
				Link: requestLink(r.ID), Recipients: []string{r.RequestedBy}}); err != nil {
				return err
			}
		case next == nil:
			if err := s.repo.SetStatus(ctx, tx, r.ID, StatusApproved, "decided_at"); err != nil {
				return err
			}
			if err := s.notifyApproved(ctx, tx, p, r); err != nil {
				return err
			}
		default:
			if err := s.notifyStep(ctx, tx, p.UserID, r, int(next.StepOrder), next.ApproverRoleID); err != nil {
				return err
			}
		}
		if err := audit.Write(ctx, tx, audit.Entry{ActorID: p.UserID, Action: action, ObjectType: "ip_request", ObjectID: r.ID,
			ApprovalRef: approvalID, New: map[string]any{"step": current.StepOrder, "decision": decision, "comment": comment}}); err != nil {
			return err
		}
		if out, err = s.repo.GetRequest(ctx, tx, r.ID, false); err != nil {
			return err
		}
		return s.complete(ctx, tx, &out, p)
	})
	return out, err
}

func (s *Service) notifyApproved(ctx context.Context, tx pgx.Tx, p *auth.Principal, r Request) error {
	if err := notifications.Enqueue(ctx, tx, p.UserID, notifications.Event{Type: notifications.RequestApproved,
		ObjectType: "ip_request", ObjectID: r.ID, DedupeKey: "request_approved:" + r.ID,
		Title: "Approved: " + describe(r), Body: fmt.Sprintf("Your %s is approved and waiting to be fulfilled.", describe(r)),
		Link: requestLink(r.ID), Recipients: []string{r.RequestedBy}}); err != nil {
		return err
	}
	perm := auth.IPAllocate
	if r.RequestType == TypeReservation {
		perm = auth.IPReserve
	}
	fulfillers, err := notifications.UsersWithPermission(ctx, tx, perm, nil)
	if err != nil {
		return err
	}
	return notifications.Enqueue(ctx, tx, p.UserID, notifications.Event{Type: notifications.RequestReady,
		ObjectType: "ip_request", ObjectID: r.ID, DedupeKey: "request_ready:" + r.ID,
		Title: "Ready to fulfil: " + describe(r), Body: fmt.Sprintf("The %s from %s is approved and can be fulfilled.", describe(r), r.RequestedByEmail),
		Link: requestLink(r.ID), Recipients: fulfillers})
}

func (s *Service) notifyFulfilled(ctx context.Context, tx pgx.Tx, actor string, r Request, address string) error {
	verb := map[string]string{TypeAllocation: "allocated", TypeReservation: "reserved", TypeRelease: "released"}[r.RequestType]
	return notifications.Enqueue(ctx, tx, actor, notifications.Event{Type: notifications.RequestFulfilled,
		ObjectType: "ip_request", ObjectID: r.ID, DedupeKey: "request_fulfilled:" + r.ID,
		Title: fmt.Sprintf("%s %s", strings.ToUpper(verb[:1])+verb[1:], address),
		Body:  fmt.Sprintf("%s is now %s for your %s.", address, verb, describe(r)),
		Link:  requestLink(r.ID), Recipients: []string{r.RequestedBy}})
}

// approvedRequest locks a request for fulfilment. done is true when it was
// fulfilled already, so repeating a call returns the same result.
func (s *Service) approvedRequest(ctx context.Context, tx pgx.Tx, id, requestType string) (Request, bool, error) {
	r, err := s.repo.GetRequest(ctx, tx, id, true)
	if errors.Is(err, httpx.ErrNotFound) {
		return r, false, httpx.Validation("requestId", "That request doesn't exist.")
	}
	if err != nil {
		return r, false, err
	}
	if r.RequestType != requestType {
		return r, false, httpx.Validation("requestId", "This request is not a "+requestType+" request.")
	}
	if r.Status == FulfilledStatus[requestType] {
		return r, true, nil
	}
	if r.Status != StatusApproved {
		return r, false, errNotApproved
	}
	return r, false, nil
}

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
			var id string
			if err := tx.QueryRow(ctx, `SELECT id::text FROM ip_allocations WHERE request_id = $1::uuid`, r.ID).Scan(&id); err != nil {
				return err
			}
			out, err = s.repo.GetAllocation(ctx, tx, id, false)
			return err
		}
		var ipID, addr string
		if r.ReservationID != nil {
			rv, err := s.repo.GetReservation(ctx, tx, *r.ReservationID, true)
			if err != nil {
				return err
			}
			if rv.Status != "active" {
				return httpx.NewError(http.StatusConflict, "RESERVATION_NOT_ACTIVE", "The reservation being converted is no longer active.")
			}
			if _, err := tx.Exec(ctx, `UPDATE ip_reservations SET status = 'converted', released_by = $2::uuid, released_at = now()
				WHERE id = $1::uuid`, rv.ID, p.UserID); err != nil {
				return err
			}
			ok, err := s.repo.SetState(ctx, tx, rv.IPAddressID, "reserved", "allocated", p.UserID, false)
			if err != nil {
				return err
			}
			if !ok {
				return errUnavailable
			}
			owner := r.TechnicalOwnerID
			if owner == nil {
				owner = &r.RequestedBy
			}
			if _, err := tx.Exec(ctx, `UPDATE ip_addresses SET owner_id = $2::uuid,
				application_id = COALESCE($3::uuid, application_id), business_service_id = COALESCE($4::uuid, business_service_id)
				WHERE id = $1::uuid`, rv.IPAddressID, owner, r.ApplicationID, r.BusinessServiceID); err != nil {
				return err
			}
			ipID, addr = rv.IPAddressID, rv.Address
		} else if ipID, addr, err = s.claim(ctx, tx, r, "allocated", p.UserID); err != nil {
			return err
		}
		var id string
		if err := tx.QueryRow(ctx, `INSERT INTO ip_allocations (ip_address_id, subnet_id, routing_domain_id, address, request_id, allocated_by)
			SELECT id, subnet_id, routing_domain_id, address, $2::uuid, $3::uuid FROM ip_addresses WHERE id = $1::uuid
			RETURNING id::text`, ipID, r.ID, p.UserID).Scan(&id); err != nil {
			return err
		}
		if err := s.repo.MarkFulfilled(ctx, tx, r.ID, StatusAllocated, p.UserID); err != nil {
			return err
		}
		if out, err = s.repo.GetAllocation(ctx, tx, id, false); err != nil {
			return err
		}
		created = true
		if err := audit.Write(ctx, tx, audit.Entry{ActorID: p.UserID, Action: "ip_allocation.create", ObjectType: "ip_allocation",
			ObjectID: id, ApprovalRef: r.ID, New: out}); err != nil {
			return err
		}
		return s.notifyFulfilled(ctx, tx, p.UserID, r, addr)
	})
	return out, created, translate(err)
}

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
			var id string
			if err := tx.QueryRow(ctx, `SELECT id::text FROM ip_reservations WHERE request_id = $1::uuid`, r.ID).Scan(&id); err != nil {
				return err
			}
			out, err = s.repo.GetReservation(ctx, tx, id, false)
			return err
		}
		if r.ExpiryDate == nil {
			return httpx.Validation("expiryDate", "Reservations need an expiry date.")
		}
		end, err := ReservationEnd(*r.ExpiryDate)
		if err != nil {
			return err
		}
		if !end.After(s.now()) {
			return httpx.NewError(http.StatusConflict, "REQUEST_EXPIRED", "This reservation's expiry date has passed. Make a new request.")
		}
		ipID, addr, err := s.claim(ctx, tx, r, "reserved", p.UserID)
		if err != nil {
			return err
		}
		var id string
		if err := tx.QueryRow(ctx, `INSERT INTO ip_reservations (ip_address_id, subnet_id, routing_domain_id, address, reason, expires_at,
			request_id, reserved_by)
			SELECT id, subnet_id, routing_domain_id, address, $2, $3, $4::uuid, $5::uuid FROM ip_addresses WHERE id = $1::uuid
			RETURNING id::text`, ipID, r.Justification, end, r.ID, r.RequestedBy).Scan(&id); err != nil {
			return err
		}
		if err := s.repo.MarkFulfilled(ctx, tx, r.ID, StatusReserved, p.UserID); err != nil {
			return err
		}
		if out, err = s.repo.GetReservation(ctx, tx, id, false); err != nil {
			return err
		}
		created = true
		if err := audit.Write(ctx, tx, audit.Entry{ActorID: p.UserID, Action: "ip_reservation.create", ObjectType: "ip_reservation",
			ObjectID: id, ApprovalRef: r.ID, New: out}); err != nil {
			return err
		}
		return s.notifyFulfilled(ctx, tx, p.UserID, r, addr)
	})
	return out, created, translate(err)
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
		old, err := s.repo.GetAllocation(ctx, tx, allocationID, true)
		if err != nil {
			return err
		}
		if old.Status != "active" {
			return httpx.NewError(http.StatusConflict, "ALLOCATION_RELEASED", "This allocation has already been released.")
		}
		if _, err := tx.Exec(ctx, `UPDATE ip_allocations SET status = 'released', release_request_id = $2::uuid,
			released_by = $3::uuid, released_at = now() WHERE id = $1::uuid`, allocationID, r.ID, p.UserID); err != nil {
			return err
		}
		// A record someone moved on by hand (quarantined, say) is left as it is.
		if _, err := s.repo.SetState(ctx, tx, old.IPAddressID, "allocated", "available_candidate", p.UserID, true); err != nil {
			return err
		}
		if err := s.repo.MarkFulfilled(ctx, tx, r.ID, StatusReleased, p.UserID); err != nil {
			return err
		}
		if out, err = s.repo.GetAllocation(ctx, tx, allocationID, false); err != nil {
			return err
		}
		if err := audit.Write(ctx, tx, audit.Entry{ActorID: p.UserID, Action: "ip_allocation.release", ObjectType: "ip_allocation",
			ObjectID: allocationID, ApprovalRef: r.ID, Old: old, New: out}); err != nil {
			return err
		}
		return s.notifyFulfilled(ctx, tx, p.UserID, r, old.Address)
	})
	return out, translate(err)
}

func (s *Service) ListAllocations(ctx context.Context, f ListFilter, page httpx.Page) ([]Allocation, int64, error) {
	return s.repo.ListAllocations(ctx, s.conn, f, page)
}

func (s *Service) GetAllocation(ctx context.Context, id string) (Allocation, error) {
	return s.repo.GetAllocation(ctx, s.conn, id, false)
}

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
		return rv, httpx.NewError(http.StatusConflict, "RESERVATION_NOT_ACTIVE", "This reservation is no longer active.")
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

// translate maps database refusals that mean "someone else got there first".
func translate(err error) error {
	switch db.ErrorCode(err) {
	case db.CodeUnique:
		return errUnavailable
	case db.CodeForeignKey, db.CodeBadValue:
		return httpx.Validation("body", "A chosen record doesn't exist.")
	case db.CodeCheck:
		return httpx.NewError(http.StatusUnprocessableEntity, "VALIDATION_FAILED", "A value is outside what this record allows.")
	}
	return err
}
