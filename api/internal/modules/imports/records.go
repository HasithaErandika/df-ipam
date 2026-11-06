package imports

import (
	"context"
	"dfipam/api/internal/audit"
	"dfipam/api/internal/auth"
	"dfipam/api/internal/platform/db"
	"dfipam/api/internal/platform/httpx"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
)

const recordCols = `r.id::text, r.batch_id::text, r.line_number, r.raw_text, coalesce(r.parsed, '{}'), r.status, r.errors, r.created_object_id::text`

func scanRecord(r pgx.CollectableRow) (Record, error) {
	var rec Record
	return rec, r.Scan(&rec.ID, &rec.BatchID, &rec.LineNumber, &rec.RawText, &rec.Parsed, &rec.Status, &rec.Errors, &rec.CreatedObjectID)
}

var recordStatuses = map[string]bool{"valid": true, "invalid": true, "duplicate": true, "overlap": true, "needs_enrichment": true, "committed": true}

func (h *Handler) preview(c *gin.Context) {
	id, ok := httpx.PathID(c)
	if !ok {
		return
	}
	p := httpx.ParsePage(c)
	status := c.Query("status")
	if !recordStatuses[status] {
		status = ""
	}
	ctx := c.Request.Context()
	var total int64
	if err := h.conn.QueryRow(ctx, `SELECT count(*) FROM import_records r WHERE r.batch_id = $1::uuid AND ($2 = '' OR r.status = $2)`, id, status).Scan(&total); err != nil {
		httpx.Fail(c, err)
		return
	}
	rows, err := h.conn.Query(ctx, "SELECT "+recordCols+` FROM import_records r WHERE r.batch_id = $1::uuid AND ($2 = '' OR r.status = $2)
		ORDER BY r.line_number LIMIT $3 OFFSET $4`, id, status, p.Size, p.Offset())
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	out, err := pgx.CollectRows(rows, scanRecord)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.List(c, out, p, total)
}

func (h *Handler) queue(c *gin.Context) {
	p := httpx.ParsePage(c)
	ctx := c.Request.Context()
	const where = ` FROM import_records r JOIN import_batches b ON b.id = r.batch_id WHERE b.status = 'validated' AND r.status IN ('needs_enrichment','invalid')
		AND NOT coalesce((r.parsed->>'comment')::boolean, false) AND b.format <> 'template'`
	var total int64
	if err := h.conn.QueryRow(ctx, "SELECT count(*)"+where).Scan(&total); err != nil {
		httpx.Fail(c, err)
		return
	}
	rows, err := h.conn.Query(ctx, "SELECT "+recordCols+", b.file_name"+where+" ORDER BY b.created_at, r.line_number LIMIT $1 OFFSET $2", p.Size, p.Offset())
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	out, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (Record, error) {
		var rec Record
		return rec, r.Scan(&rec.ID, &rec.BatchID, &rec.LineNumber, &rec.RawText, &rec.Parsed, &rec.Status, &rec.Errors, &rec.CreatedObjectID, &rec.FileName)
	})
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.List(c, out, p, total)
}

// enrich corrects one record before approval. The raw line never changes.
func (h *Handler) enrich(c *gin.Context) {
	id, ok := httpx.PathID(c)
	if !ok {
		return
	}
	var req Parsed
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Fail(c, httpx.ErrBadRequest)
		return
	}
	ctx, actor := c.Request.Context(), auth.Current(c).UserID
	var out Record
	err := db.InTx(ctx, h.conn, func(tx pgx.Tx) (err error) {
		out, err = enrichRecord(ctx, tx, actor, id, req)
		return err
	})
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, http.StatusOK, out)
}

// lockedRecord is a record and its batch, locked for correction.
type lockedRecord struct {
	batchID, batchStatus, routingDomain, parsed, status string
	line                                                int
}

func lockRecord(ctx context.Context, tx pgx.Tx, id string) (lockedRecord, error) {
	var r lockedRecord
	err := tx.QueryRow(ctx, `SELECT r.batch_id::text, b.status, coalesce(b.routing_domain_id::text, ''), coalesce(r.parsed::text, '{}'), r.status, r.line_number
		FROM import_records r JOIN import_batches b ON b.id = r.batch_id WHERE r.id = $1::uuid FOR UPDATE OF r, b`, id).
		Scan(&r.batchID, &r.batchStatus, &r.routingDomain, &r.parsed, &r.status, &r.line)
	if errors.Is(err, pgx.ErrNoRows) {
		return r, httpx.ErrNotFound
	}
	if err == nil && r.batchStatus != "validated" {
		return r, httpx.NewError(http.StatusConflict, "IMPORT_LOCKED", "Only batches waiting for approval can be corrected.")
	}
	if err == nil && r.routingDomain == "" {
		return r, httpx.NewError(http.StatusConflict, "IMPORT_FIX_IN_FILE", "Template rows are corrected in the file: discard this import, fix the row and upload again.")
	}
	return r, err
}

// rebuildLine makes a line from the corrected fields and checks it the same way.
func rebuildLine(req Parsed, number int) Line {
	text := strings.Join([]string{req.CIDR, "gw=" + req.Gateway, "vdom=" + req.VDOM, "vlan=" + req.VLAN, "name=" + req.Name}, ",")
	text = strings.NewReplacer(",gw=,", ",", ",vdom=,", ",", ",vlan=,", ",").Replace(text)
	parsed, errs := ParseLine("csv", text)
	parsed.Name, parsed.VDOM, parsed.VLAN = strings.TrimSpace(req.Name), strings.TrimSpace(req.VDOM), strings.TrimSpace(req.VLAN)
	l := Line{Number: number, Parsed: parsed, Errors: errs, Status: "valid"}
	if len(errs) > 0 {
		l.Status = "invalid"
	} else if parsed.Name == "" {
		l.Status, l.Errors = "needs_enrichment", []string{"No name: add one."}
	}
	return l
}

// otherNetworks returns the networks of the batch's other records, for the
// duplicate check within the file.
func otherNetworks(ctx context.Context, tx pgx.Tx, batchID, id string) (map[string]int, error) {
	rows, err := tx.Query(ctx, `SELECT parsed->>'cidr', line_number FROM import_records WHERE batch_id = $1::uuid AND id <> $2::uuid
		AND status IN ('valid','needs_enrichment') AND parsed->>'cidr' IS NOT NULL`, batchID, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	seen := map[string]int{}
	for rows.Next() {
		var cidr string
		var n int
		if err := rows.Scan(&cidr, &n); err != nil {
			return nil, err
		}
		seen[cidr] = n
	}
	return seen, rows.Err()
}

func enrichRecord(ctx context.Context, tx pgx.Tx, actor, id string, req Parsed) (Record, error) {
	old, err := lockRecord(ctx, tx, id)
	if err != nil {
		return Record{}, err
	}
	seen, err := otherNetworks(ctx, tx, old.batchID, id)
	if err != nil {
		return Record{}, err
	}
	lines := []Line{rebuildLine(req, old.line)}
	if err := classify(ctx, tx, old.routingDomain, lines, seen); err != nil {
		return Record{}, err
	}
	l := lines[0]
	pj, _ := json.Marshal(l.Parsed)
	ej, _ := json.Marshal(append([]string{}, l.Errors...))
	if _, err := tx.Exec(ctx, `UPDATE import_records SET parsed = $2::jsonb, status = $3, errors = $4::jsonb WHERE id = $1::uuid`, id, pj, l.Status, ej); err != nil {
		return Record{}, err
	}
	if err := recount(ctx, tx, old.batchID); err != nil {
		return Record{}, err
	}
	rows, err := tx.Query(ctx, "SELECT "+recordCols+" FROM import_records r WHERE r.id = $1::uuid", id)
	if err != nil {
		return Record{}, err
	}
	out, err := pgx.CollectExactlyOneRow(rows, scanRecord)
	if err != nil {
		return out, err
	}
	return out, audit.Write(ctx, tx, audit.Entry{ActorID: actor, Action: "import_record.enrich", ObjectType: "import_record", ObjectID: id,
		Old: map[string]any{"parsed": json.RawMessage(old.parsed), "status": old.status}, New: map[string]any{"parsed": l.Parsed, "status": l.Status}})
}
