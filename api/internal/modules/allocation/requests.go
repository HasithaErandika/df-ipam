package allocation

import (
	"context"
	"net/http"

	"dfipam/api/internal/audit"
	"dfipam/api/internal/auth"
	"dfipam/api/internal/platform/db"
	"dfipam/api/internal/platform/httpx"

	"github.com/jackc/pgx/v5"
)

func (s *Service) CreateRequest(ctx context.Context, p *auth.Principal, in RequestInput) (Request, error) {
	in, err := Normalise(in, s.today())
	if err != nil {
		return Request{}, err
	}
	var out Request
	err = db.InTx(ctx, s.conn, func(tx pgx.Tx) error {
		out, err = s.createRequest(ctx, tx, p, in)
		return err
	})
	return out, translate(err)
}

func (s *Service) createRequest(ctx context.Context, tx pgx.Tx, p *auth.Principal, in RequestInput) (Request, error) {
	in, err := s.resolve(ctx, tx, in, "")
	if err != nil {
		return Request{}, err
	}
	id, err := s.repo.InsertRequest(ctx, tx, p.UserID, in)
	if err != nil {
		return Request{}, err
	}
	r, err := s.repo.GetRequest(ctx, tx, id, true)
	if err != nil {
		return r, err
	}
	if err := audit.Write(ctx, tx, audit.Entry{ActorID: p.UserID, Action: "ip_request.create", ObjectType: "ip_request", ObjectID: id, New: r}); err != nil {
		return r, err
	}
	return s.submitIfAsked(ctx, tx, p, r, in.Submit)
}

// submitIfAsked submits the saved request when asked to, and returns it as it is now.
func (s *Service) submitIfAsked(ctx context.Context, tx pgx.Tx, p *auth.Principal, r Request, submit bool) (Request, error) {
	if submit {
		if err := s.submit(ctx, tx, p, r); err != nil {
			return r, err
		}
	}
	return s.reload(ctx, tx, r.ID, p)
}

func (s *Service) UpdateRequest(ctx context.Context, p *auth.Principal, id string, version int32, in RequestInput) (Request, error) {
	in, err := Normalise(in, s.today())
	if err != nil {
		return Request{}, err
	}
	var out Request
	err = db.InTx(ctx, s.conn, func(tx pgx.Tx) error {
		out, err = s.updateRequest(ctx, tx, p, id, version, in)
		return err
	})
	return out, translate(err)
}

func (s *Service) updateRequest(ctx context.Context, tx pgx.Tx, p *auth.Principal, id string, version int32, in RequestInput) (Request, error) {
	old, err := s.own(ctx, tx, p, id, StatusDraft)
	if err != nil {
		return old, err
	}
	if old.Version != version {
		return old, httpx.NewError(http.StatusPreconditionFailed, "STALE_VERSION", "This request changed since you opened it. Reload and try again.")
	}
	if in, err = s.resolve(ctx, tx, in, id); err != nil {
		return old, err
	}
	if err := s.repo.UpdateRequest(ctx, tx, id, in); err != nil {
		return old, err
	}
	updated, err := s.repo.GetRequest(ctx, tx, id, false)
	if err != nil {
		return updated, err
	}
	if err := audit.Write(ctx, tx, audit.Entry{ActorID: p.UserID, Action: "ip_request.update", ObjectType: "ip_request", ObjectID: id, Old: old, New: updated}); err != nil {
		return updated, err
	}
	return s.submitIfAsked(ctx, tx, p, updated, in.Submit)
}

func (s *Service) Submit(ctx context.Context, p *auth.Principal, id string) (Request, error) {
	var out Request
	err := db.InTx(ctx, s.conn, func(tx pgx.Tx) error {
		r, err := s.own(ctx, tx, p, id, StatusDraft)
		if err != nil {
			return err
		}
		out, err = s.submitIfAsked(ctx, tx, p, r, true)
		return err
	})
	return out, translate(err)
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
		out, err = s.reload(ctx, tx, id, p)
		return err
	})
	return out, err
}
