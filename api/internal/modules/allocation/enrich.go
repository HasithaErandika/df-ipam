package allocation

import (
	"context"
	"errors"
	"net/netip"

	"dfipam/api/internal/auth"
	"dfipam/api/internal/platform/httpx"

	"github.com/jackc/pgx/v5"
)

// complete adds the approval steps, the result and the caller's open step.
func (s *Service) complete(ctx context.Context, q querier, r *Request, viewer *auth.Principal) error {
	list := []Request{*r}
	if err := s.completeAll(ctx, q, list, viewer); err != nil {
		return err
	}
	*r = list[0]
	return nil
}

// completeAll does the work of complete for a whole page: its query count doesn't
// grow with the number of requests.
func (s *Service) completeAll(ctx context.Context, q querier, list []Request, viewer *auth.Principal) error {
	ids := make([]string, len(list))
	for i := range list {
		ids[i] = list[i].ID
	}
	steps, err := s.repo.ApprovalsOf(ctx, q, ids)
	if err != nil {
		return err
	}
	results, err := s.repo.Results(ctx, q, list)
	if err != nil {
		return err
	}
	mine := map[string]string{}
	if viewer != nil && viewer.Has(auth.RequestsApprove) {
		if mine, err = s.repo.MyApprovals(ctx, q, ids, viewer.UserID); err != nil {
			return err
		}
	}
	for i := range list {
		r := &list[i]
		r.Approvals = steps[r.ID]
		if r.Approvals == nil {
			r.Approvals = []Approval{}
		}
		r.Result = results[r.ID]
		if id, ok := mine[r.ID]; ok {
			r.MyApprovalID = &id
		}
	}
	return nil
}

// reload returns the request as the caller should now see it.
func (s *Service) reload(ctx context.Context, tx pgx.Tx, id string, p *auth.Principal) (Request, error) {
	r, err := s.repo.GetRequest(ctx, tx, id, false)
	if err != nil {
		return r, err
	}
	return r, s.complete(ctx, tx, &r, p)
}

// Pending lists the approval steps the caller may decide now.
func (s *Service) Pending(ctx context.Context, p *auth.Principal, page httpx.Page) ([]PendingApproval, int64, error) {
	ids, total, err := s.repo.PendingFor(ctx, s.conn, p.UserID, page)
	if err != nil {
		return nil, 0, err
	}
	approvals, err := s.repo.ApprovalsByID(ctx, s.conn, ids)
	if err != nil {
		return nil, 0, err
	}
	requestIDs := make([]string, len(approvals))
	for i, ap := range approvals {
		requestIDs[i] = ap.RequestID
	}
	requests, err := s.repo.RequestsByID(ctx, s.conn, requestIDs)
	if err != nil {
		return nil, 0, err
	}
	if err := s.completeAll(ctx, s.conn, requests, p); err != nil {
		return nil, 0, err
	}
	byID := make(map[string]Request, len(requests))
	for _, r := range requests {
		byID[r.ID] = r
	}
	out := make([]PendingApproval, len(approvals))
	for i, ap := range approvals {
		out[i] = PendingApproval{Approval: ap, Request: byID[ap.RequestID]}
	}
	return out, total, nil
}

// resolve checks a request against the database: the subnet, the address and
// the allocation or reservation it refers to. It fills in the subnet and
// address that a release or conversion takes from its target. self is the
// request's own ID when it already exists.
func (s *Service) resolve(ctx context.Context, q querier, in RequestInput, self string) (RequestInput, error) {
	switch {
	case in.RequestType == TypeRelease:
		return s.resolveRelease(ctx, q, in, self)
	case in.ReservationID != nil:
		return s.resolveConversion(ctx, q, in)
	}
	return in, s.checkSubnetChoice(ctx, q, in)
}

func (s *Service) resolveRelease(ctx context.Context, q querier, in RequestInput, self string) (RequestInput, error) {
	al, err := s.repo.GetAllocation(ctx, q, *in.AllocationID, false)
	if errors.Is(err, httpx.ErrNotFound) {
		return in, httpx.Validation("allocationId", "That allocation doesn't exist.")
	}
	if err != nil {
		return in, err
	}
	if al.Status != "active" {
		return in, errAllocationReleased
	}
	if al.OpenReleaseID != nil && *al.OpenReleaseID != self {
		return in, errReleaseRequested
	}
	in.SubnetID, in.RequestedAddress = al.SubnetID, &al.Address
	return in, nil
}

func (s *Service) resolveConversion(ctx context.Context, q querier, in RequestInput) (RequestInput, error) {
	rv, err := s.repo.GetReservation(ctx, q, *in.ReservationID, false)
	if errors.Is(err, httpx.ErrNotFound) {
		return in, httpx.Validation("reservationId", "That reservation doesn't exist.")
	}
	if err != nil {
		return in, err
	}
	if rv.Status != "active" {
		return in, errReservationEnded
	}
	in.SubnetID, in.RequestedAddress = rv.SubnetID, &rv.Address
	return in, nil
}

func (s *Service) checkSubnetChoice(ctx context.Context, q querier, in RequestInput) error {
	sub, err := s.repo.Subnet(ctx, q, in.SubnetID, false)
	if err != nil {
		return err
	}
	if sub.Archived {
		return errArchived
	}
	if in.SelectionMode != "specific" {
		return nil
	}
	space, err := s.repo.Space(ctx, q, sub)
	if err != nil {
		return err
	}
	return CheckSpecific(space, netip.MustParseAddr(*in.RequestedAddress))
}
