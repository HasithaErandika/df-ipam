package imports

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"

	"dfipam/api/internal/modules/catalog"
	"dfipam/api/internal/modules/ipaddresses"
	"dfipam/api/internal/modules/organisations"
	"dfipam/api/internal/modules/routingdomains"
	"dfipam/api/internal/modules/subnets"
	"dfipam/api/internal/platform/db"
	"dfipam/api/internal/platform/httpx"

	"github.com/jackc/pgx/v5"
)

func removeSheet(ctx context.Context, tx pgx.Tx, actor string, s *sheet, b Batch, ids []string) error {
	if e, ok := catalog.ByPath(s.Entity); ok {
		return catalog.DeleteMany(ctx, tx, actor, e, ids)
	}
	if s.Key == "ip_addresses" {
		return ipaddresses.RemoveImported(ctx, tx, actor, ids, *b.CommittedAt)
	}
	for _, id := range ids {
		if err := removeRow(ctx, tx, actor, s, id); err != nil && !errors.Is(err, httpx.ErrNotFound) {
			return err
		}
	}
	return nil
}

func removeRow(ctx context.Context, tx pgx.Tx, actor string, s *sheet, id string) error {
	old := map[string]string{"id": id, "reason": "import reversed"}
	switch s.Key {
	case "organisations":
		return organisations.Remove(ctx, tx, actor, id, old)
	case "routing_domains":
		return routingdomains.Remove(ctx, tx, actor, id, old)
	}
	return subnets.NewService(tx).Delete(ctx, actor, id)
}

func blockedReversal(s *sheet, err error) error {
	var e *httpx.Error
	message := ""
	switch {
	case errors.As(err, &e):
		message = e.Message + " "
	case db.ErrorCode(err) != db.CodeForeignKey:
		return err
	}
	return httpx.NewError(http.StatusConflict, "IMPORT_REVERSE_BLOCKED", fmt.Sprintf("%s from this import are now in use. %sNothing was reversed.", s.Title, message))
}

// reverseSheets removes what a template import created, children first. Anything
// still in use stops the whole reversal.
func reverseSheets(ctx context.Context, tx pgx.Tx, actor string, b Batch) error {
	rows, err := tx.Query(ctx, `SELECT created_object_type, created_object_id::text FROM import_records WHERE batch_id = $1::uuid AND status = 'committed'`, b.ID)
	if err != nil {
		return err
	}
	made := map[string][]string{}
	var kind, id string
	if _, err := pgx.ForEachRow(rows, []any{&kind, &id}, func() error {
		made[kind] = append(made[kind], id)
		return nil
	}); err != nil {
		return err
	}
	for _, s := range slices.Backward(sheets) {
		if ids := made[s.Object]; len(ids) > 0 {
			if err := removeSheet(ctx, tx, actor, &s, b, ids); err != nil {
				return blockedReversal(&s, err)
			}
		}
	}
	_, err = tx.Exec(ctx, `UPDATE import_records SET status = 'valid', created_object_id = NULL, created_object_type = NULL WHERE batch_id = $1::uuid AND status = 'committed'`, b.ID)
	return err
}
