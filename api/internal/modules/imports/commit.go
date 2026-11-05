package imports

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"dfipam/api/internal/audit"
	"dfipam/api/internal/auth"
	"dfipam/api/internal/modules/subnets"
	"dfipam/api/internal/platform/db"
	"dfipam/api/internal/platform/httpx"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
)

// approve commits every valid record as a subnet, in one transaction. Someone
// other than the uploader must approve. Invalid, duplicate and overlapping
// lines stay recorded and are not imported.
func (h *Handler) approve(c *gin.Context) {
	h.changeBatch(c, approveBatch)
}

func (h *Handler) changeBatch(c *gin.Context, change func(context.Context, pgx.Tx, string, string) (Batch, error)) {
	id, ok := httpx.PathID(c)
	if !ok {
		return
	}
	ctx, actor := c.Request.Context(), auth.Current(c).UserID
	var out Batch
	err := db.InTx(ctx, h.conn, func(tx pgx.Tx) (err error) {
		out, err = change(ctx, tx, actor, id)
		return err
	})
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, http.StatusOK, out)
}

func checkApprovable(b Batch, actor string) error {
	if b.Status != "validated" {
		return httpx.NewError(http.StatusConflict, "IMPORT_LOCKED", "Only batches waiting for approval can be approved.")
	}
	if b.UploadedBy == actor {
		return httpx.NewError(http.StatusForbidden, "SELF_APPROVAL_DENIED", "Someone other than the uploader must approve an import.")
	}
	if b.RoutingDomainID == nil || b.Environment == nil {
		return httpx.NewError(http.StatusConflict, "IMPORT_INCOMPLETE", "This batch has no routing domain or environment.")
	}
	return nil
}

func approveBatch(ctx context.Context, tx pgx.Tx, actor, id string) (Batch, error) {
	b, err := loadBatch(ctx, tx, id, true)
	if err != nil {
		return b, err
	}
	if err := checkApprovable(b, actor); err != nil {
		return b, err
	}
	todo, err := validRecords(ctx, tx, id)
	if err != nil {
		return b, err
	}
	if len(todo) == 0 {
		return b, httpx.NewError(http.StatusConflict, "IMPORT_EMPTY", "No valid lines to import.")
	}
	if err := commitRecords(ctx, tx, actor, b, todo); err != nil {
		return b, err
	}
	if _, err := tx.Exec(ctx, `UPDATE import_batches SET status = 'committed', approved_by = $2::uuid, committed_at = now() WHERE id = $1::uuid`, id, actor); err != nil {
		return b, err
	}
	if err := recount(ctx, tx, id); err != nil {
		return b, err
	}
	out, err := loadBatch(ctx, tx, id, false)
	if err != nil {
		return out, err
	}
	return out, audit.Write(ctx, tx, audit.Entry{ActorID: actor, Action: "import.approve", ObjectType: "import_batch", ObjectID: id, ApprovalRef: id,
		New: map[string]any{"committed": len(todo)}})
}

type pendingRecord struct {
	id     string
	line   int
	parsed Parsed
}

func validRecords(ctx context.Context, tx pgx.Tx, batchID string) ([]pendingRecord, error) {
	rows, err := tx.Query(ctx, `SELECT id::text, line_number, parsed FROM import_records WHERE batch_id = $1::uuid AND status = 'valid' ORDER BY line_number`, batchID)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (pendingRecord, error) {
		var p pendingRecord
		var raw []byte
		if err := r.Scan(&p.id, &p.line, &raw); err != nil {
			return p, err
		}
		_ = json.Unmarshal(raw, &p.parsed)
		return p, nil
	})
}

// commitRecords creates the subnets. Each goes through the subnet service, so
// its overlap rules and audit row apply, then all records are marked at once.
func commitRecords(ctx context.Context, tx pgx.Tx, actor string, b Batch, todo []pendingRecord) error {
	links, err := loadLinks(ctx, tx, todo, *b.RoutingDomainID)
	if err != nil {
		return err
	}
	svc := subnets.NewService(tx)
	ids, created := make([]string, len(todo)), make([]string, len(todo))
	for i, p := range todo {
		s, err := svc.Create(ctx, actor, links.request(b, p))
		if err != nil {
			var e *httpx.Error
			if errors.As(err, &e) {
				return httpx.NewError(http.StatusConflict, "IMPORT_LINE_FAILED", fmt.Sprintf("Line %d: %s Nothing was imported.", p.line, e.Message))
			}
			return err
		}
		ids[i], created[i] = p.id, s.ID
	}
	_, err = tx.Exec(ctx, `UPDATE import_records r SET status = 'committed', created_object_type = 'subnet', created_object_id = t.subnet
		FROM unnest($1::uuid[], $2::uuid[]) AS t(id, subnet) WHERE r.id = t.id`, ids, created)
	return err
}
