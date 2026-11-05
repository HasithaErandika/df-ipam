package imports

import (
	"context"
	"dfipam/api/internal/audit"
	"dfipam/api/internal/modules/subnets"
	"dfipam/api/internal/platform/httpx"
	"errors"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
)

// reverse deletes the subnets a batch created. It only works while they are
// untouched: a subnet that has gained IP records or other links stops the
// whole reversal, and nothing changes.
func (h *Handler) reverse(c *gin.Context) {
	h.changeBatch(c, reverseBatch)
}

type madeSubnet struct {
	record string
	line   int
	subnet string
}

func reverseBatch(ctx context.Context, tx pgx.Tx, actor, id string) (Batch, error) {
	b, err := loadBatch(ctx, tx, id, true)
	if err != nil {
		return b, err
	}
	if b.Status != "committed" {
		return b, httpx.NewError(http.StatusConflict, "IMPORT_LOCKED", "Only committed batches can be reversed.")
	}
	rows, err := tx.Query(ctx, `SELECT id::text, line_number, created_object_id::text FROM import_records
		WHERE batch_id = $1::uuid AND status = 'committed' AND created_object_type = 'subnet' ORDER BY line_number DESC`, id)
	if err != nil {
		return b, err
	}
	list, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (madeSubnet, error) {
		var m madeSubnet
		return m, r.Scan(&m.record, &m.line, &m.subnet)
	})
	if err != nil {
		return b, err
	}
	if err := deleteSubnets(ctx, tx, actor, list); err != nil {
		return b, err
	}
	if _, err := tx.Exec(ctx, `UPDATE import_batches SET status = 'reversed', reversed_at = now(), reversed_by = $2::uuid WHERE id = $1::uuid`, id, actor); err != nil {
		return b, err
	}
	if err := recount(ctx, tx, id); err != nil {
		return b, err
	}
	out, err := loadBatch(ctx, tx, id, false)
	if err != nil {
		return out, err
	}
	return out, audit.Write(ctx, tx, audit.Entry{ActorID: actor, Action: "import.reverse", ObjectType: "import_batch", ObjectID: id,
		Old: map[string]any{"subnets": len(list)}})
}

func deleteSubnets(ctx context.Context, tx pgx.Tx, actor string, list []madeSubnet) error {
	svc := subnets.NewService(tx)
	records := make([]string, len(list))
	for i, m := range list {
		records[i] = m.record
		if err := svc.Delete(ctx, actor, m.subnet); err != nil && !errors.Is(err, httpx.ErrNotFound) {
			var e *httpx.Error
			if errors.As(err, &e) {
				return httpx.NewError(http.StatusConflict, "IMPORT_REVERSE_BLOCKED",
					fmt.Sprintf("Line %d's subnet is now in use (%s) Nothing was reversed.", m.line, e.Message))
			}
			return err
		}
	}
	_, err := tx.Exec(ctx, `UPDATE import_records SET status = 'valid', created_object_id = NULL, created_object_type = NULL WHERE id = ANY($1::uuid[])`, records)
	return err
}
