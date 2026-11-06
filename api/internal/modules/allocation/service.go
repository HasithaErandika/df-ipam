package allocation

import (
	"context"
	"errors"
	"net/http"
	"time"

	"dfipam/api/internal/auth"
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

func (s *Service) ListAllocations(ctx context.Context, f ListFilter, page httpx.Page) ([]Allocation, int64, error) {
	return s.repo.ListAllocations(ctx, s.conn, f, page)
}

func (s *Service) GetAllocation(ctx context.Context, id string) (Allocation, error) {
	return s.repo.GetAllocation(ctx, s.conn, id, false)
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
