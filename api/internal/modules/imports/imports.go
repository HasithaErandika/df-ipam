package imports

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"dfipam/api/internal/audit"
	"dfipam/api/internal/auth"
	"dfipam/api/internal/modules/subnets"
	"dfipam/api/internal/platform/db"
	"dfipam/api/internal/platform/httpx"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
)

const maxBytes = 5 << 20 // larger exports are split before upload

type Batch struct {
	ID                string          `json:"id"`
	FileName          string          `json:"fileName"`
	FileHash          string          `json:"fileHash"`
	Format            string          `json:"format"`
	Status            string          `json:"status"`
	Summary           json.RawMessage `json:"summary"`
	RoutingDomainID   *string         `json:"routingDomainId"`
	RoutingDomainName *string         `json:"routingDomainName"`
	Environment       *string         `json:"environment"`
	UploadedBy        string          `json:"uploadedBy"`
	UploadedByEmail   string          `json:"uploadedByEmail"`
	ApprovedByEmail   *string         `json:"approvedByEmail"`
	CreatedAt         time.Time       `json:"createdAt"`
	CommittedAt       *time.Time      `json:"committedAt"`
	ReversedAt        *time.Time      `json:"reversedAt"`
}

type Record struct {
	ID              string          `json:"id"`
	BatchID         string          `json:"batchId"`
	LineNumber      int             `json:"lineNumber"`
	RawText         string          `json:"rawText"`
	Parsed          json.RawMessage `json:"parsed"`
	Status          string          `json:"status"`
	Errors          json.RawMessage `json:"errors"`
	CreatedObjectID *string         `json:"createdObjectId"`
	FileName        string          `json:"fileName,omitempty"`
}

type Summary struct {
	Lines           int `json:"lines"`
	Valid           int `json:"valid"`
	Invalid         int `json:"invalid"`
	Ignored         int `json:"ignored"`
	Duplicate       int `json:"duplicate"`
	Overlap         int `json:"overlap"`
	NeedsEnrichment int `json:"needsEnrichment"`
	Committed       int `json:"committed"`
}

var environments = map[string]bool{"production": true, "disaster_recovery": true, "uat": true, "development": true, "testing": true}

type Handler struct{ conn db.Conn }

func NewHandler(conn db.Conn) *Handler { return &Handler{conn} }

func (h *Handler) Register(g *auth.Guard) {
	g.Handle(http.MethodGet, "/imports", h.list, auth.ImportsRun, auth.AuditView)
	g.Handle(http.MethodPost, "/imports", h.upload, auth.ImportsRun)
	g.Handle(http.MethodGet, "/imports/enrichment-queue", h.queue, auth.ImportsRun)
	g.Handle(http.MethodPatch, "/imports/records/:id", h.enrich, auth.ImportsRun)
	g.Handle(http.MethodGet, "/imports/:id", h.get, auth.ImportsRun, auth.AuditView)
	g.Handle(http.MethodGet, "/imports/:id/preview", h.preview, auth.ImportsRun, auth.AuditView)
	g.Handle(http.MethodPost, "/imports/:id/approve", h.approve, auth.ImportsRun)
	g.Handle(http.MethodPost, "/imports/:id/reverse", h.reverse, auth.ImportsRun)
}

const batchCols = `b.id::text, b.file_name, b.file_hash, b.format, b.status, b.summary, b.routing_domain_id::text, rd.name, b.environment,
	b.uploaded_by::text, u.email, a.email, b.created_at, b.committed_at, b.reversed_at`
const batchFrom = ` FROM import_batches b JOIN users u ON u.id = b.uploaded_by LEFT JOIN users a ON a.id = b.approved_by
	LEFT JOIN routing_domains rd ON rd.id = b.routing_domain_id`

func scanBatch(r pgx.Row) (Batch, error) {
	var b Batch
	err := r.Scan(&b.ID, &b.FileName, &b.FileHash, &b.Format, &b.Status, &b.Summary, &b.RoutingDomainID, &b.RoutingDomainName, &b.Environment,
		&b.UploadedBy, &b.UploadedByEmail, &b.ApprovedByEmail, &b.CreatedAt, &b.CommittedAt, &b.ReversedAt)
	return b, err
}

func loadBatch(ctx context.Context, q interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}, id string, lock bool) (Batch, error) {
	sql := "SELECT " + batchCols + batchFrom + " WHERE b.id = $1::uuid"
	if lock {
		sql += " FOR UPDATE OF b"
	}
	b, err := scanBatch(q.QueryRow(ctx, sql, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return b, httpx.ErrNotFound
	}
	return b, err
}

func (h *Handler) list(c *gin.Context) {
	p := httpx.ParsePage(c)
	ctx := c.Request.Context()
	var total int64
	if err := h.conn.QueryRow(ctx, `SELECT count(*) FROM import_batches`).Scan(&total); err != nil {
		httpx.Fail(c, err)
		return
	}
	rows, err := h.conn.Query(ctx, "SELECT "+batchCols+batchFrom+" ORDER BY b.created_at DESC, b.id LIMIT $1 OFFSET $2", p.Size, p.Offset())
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	out, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (Batch, error) { return scanBatch(r) })
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.List(c, out, p, total)
}

func (h *Handler) get(c *gin.Context) {
	id, ok := httpx.PathID(c)
	if !ok {
		return
	}
	b, err := loadBatch(c.Request.Context(), h.conn, id, false)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, http.StatusOK, b)
}

// recount rebuilds a batch's summary from its records.
func recount(ctx context.Context, tx pgx.Tx, batchID string) error {
	_, err := tx.Exec(ctx, `UPDATE import_batches SET summary = (
		SELECT jsonb_build_object(
		  'lines', count(*),
		  'valid', count(*) FILTER (WHERE status = 'valid'),
		  'invalid', count(*) FILTER (WHERE status = 'invalid' AND NOT coalesce((parsed->>'comment')::boolean, false)),
		  'ignored', count(*) FILTER (WHERE coalesce((parsed->>'comment')::boolean, false)),
		  'duplicate', count(*) FILTER (WHERE status = 'duplicate'),
		  'overlap', count(*) FILTER (WHERE status = 'overlap'),
		  'needsEnrichment', count(*) FILTER (WHERE status = 'needs_enrichment'),
		  'committed', count(*) FILTER (WHERE status = 'committed'))
		FROM import_records WHERE batch_id = $1::uuid) WHERE id = $1::uuid`, batchID)
	return err
}

// classify checks a valid line against the file and the inventory: an exact
// match is a duplicate, any other overlap in the routing domain is an overlap.
func classify(ctx context.Context, tx pgx.Tx, routingDomainID string, l *Line, seen map[string]int) error {
	if l.Status != "valid" && l.Status != "needs_enrichment" {
		return nil
	}
	if first, ok := seen[l.Parsed.CIDR]; ok {
		l.Status, l.Errors = "duplicate", []string{fmt.Sprintf("Same network as line %d.", first)}
		return nil
	}
	seen[l.Parsed.CIDR] = l.Number
	var exact, overlap *string
	err := tx.QueryRow(ctx, `SELECT
		(SELECT name || ' (' || cidr::text || ')' FROM subnets WHERE routing_domain_id = $1::uuid AND archived_at IS NULL AND cidr = $2::cidr LIMIT 1),
		(SELECT name || ' (' || cidr::text || ')' FROM subnets WHERE routing_domain_id = $1::uuid AND archived_at IS NULL AND cidr && $2::cidr LIMIT 1)`,
		routingDomainID, l.Parsed.CIDR).Scan(&exact, &overlap)
	if err != nil {
		return err
	}
	switch {
	case exact != nil:
		l.Status, l.Errors = "duplicate", []string{"Already in the inventory: " + *exact + "."}
	case overlap != nil:
		l.Status, l.Errors = "overlap", []string{"Overlaps " + *overlap + " in this routing domain."}
	}
	return nil
}

func (h *Handler) upload(c *gin.Context) {
	var req struct {
		FileName        string `json:"fileName"`
		Format          string `json:"format"`
		Content         string `json:"content"`
		RoutingDomainID string `json:"routingDomainId"`
		Environment     string `json:"environment"`
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxBytes+1<<16)
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Fail(c, httpx.ErrBadRequest)
		return
	}
	req.FileName = strings.TrimSpace(req.FileName)
	switch {
	case req.FileName == "" || len(req.FileName) > 255:
		httpx.Fail(c, httpx.Validation("fileName", "Name the file."))
		return
	case Formats[req.Format] == "":
		httpx.Fail(c, httpx.Validation("format", "Choose a file format."))
		return
	case !httpx.IsUUID(req.RoutingDomainID):
		httpx.Fail(c, httpx.Validation("routingDomainId", "Choose the routing domain these networks belong to."))
		return
	case !environments[req.Environment]:
		httpx.Fail(c, httpx.Validation("environment", "Choose an environment."))
		return
	case strings.TrimSpace(req.Content) == "" || len(req.Content) > maxBytes:
		httpx.Fail(c, httpx.Validation("content", "Upload a non-empty file of at most 5 MB."))
		return
	}
	sum := sha256.Sum256([]byte(req.Content))
	hash := hex.EncodeToString(sum[:])
	lines := ParseFile(req.Format, req.Content)
	ctx, actor := c.Request.Context(), auth.Current(c).UserID
	var out Batch
	err := db.InTx(ctx, h.conn, func(tx pgx.Tx) error {
		var id string
		if err := tx.QueryRow(ctx, `INSERT INTO import_batches (file_name, file_hash, format, status, uploaded_by, routing_domain_id, environment)
			VALUES ($1, $2, $3, 'validated', $4::uuid, $5::uuid, $6) RETURNING id::text`,
			req.FileName, hash, req.Format, actor, req.RoutingDomainID, req.Environment).Scan(&id); err != nil {
			return err
		}
		seen := map[string]int{}
		for i := range lines {
			l := &lines[i]
			if err := classify(ctx, tx, req.RoutingDomainID, l, seen); err != nil {
				return err
			}
			parsed, _ := json.Marshal(l.Parsed)
			errs, _ := json.Marshal(append([]string{}, l.Errors...))
			if _, err := tx.Exec(ctx, `INSERT INTO import_records (batch_id, line_number, raw_text, parsed, status, errors)
				VALUES ($1::uuid, $2, $3, $4::jsonb, $5, $6::jsonb)`, id, l.Number, l.Raw, parsed, l.Status, errs); err != nil {
				return err
			}
		}
		if err := recount(ctx, tx, id); err != nil {
			return err
		}
		var err error
		if out, err = loadBatch(ctx, tx, id, false); err != nil {
			return err
		}
		return audit.Write(ctx, tx, audit.Entry{ActorID: actor, Action: "import.upload", ObjectType: "import_batch", ObjectID: id,
			New: map[string]any{"fileName": req.FileName, "fileHash": hash, "format": req.Format, "summary": out.Summary}})
	})
	switch db.ErrorCode(err) {
	case db.CodeUnique:
		err = httpx.NewError(http.StatusConflict, "IMPORT_DUPLICATE_FILE", "This exact file was uploaded before.")
	case db.CodeForeignKey:
		err = httpx.Validation("routingDomainId", "That routing domain doesn't exist.")
	}
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, http.StatusCreated, out)
}

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
		AND NOT coalesce((r.parsed->>'comment')::boolean, false)`
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
	err := db.InTx(ctx, h.conn, func(tx pgx.Tx) error {
		var batchID, batchStatus, rd, oldParsed, oldStatus string
		var line int
		err := tx.QueryRow(ctx, `SELECT r.batch_id::text, b.status, coalesce(b.routing_domain_id::text, ''), coalesce(r.parsed::text, '{}'), r.status, r.line_number
			FROM import_records r JOIN import_batches b ON b.id = r.batch_id WHERE r.id = $1::uuid FOR UPDATE OF r, b`, id).
			Scan(&batchID, &batchStatus, &rd, &oldParsed, &oldStatus, &line)
		if errors.Is(err, pgx.ErrNoRows) {
			return httpx.ErrNotFound
		}
		if err != nil {
			return err
		}
		if batchStatus != "validated" {
			return httpx.NewError(http.StatusConflict, "IMPORT_LOCKED", "Only batches waiting for approval can be corrected.")
		}
		// Rebuild a line from the corrected fields and check it the same way.
		text := strings.Join([]string{req.CIDR, "gw=" + req.Gateway, "vdom=" + req.VDOM, "vlan=" + req.VLAN, "name=" + req.Name}, ",")
		text = strings.NewReplacer(",gw=,", ",", ",vdom=,", ",", ",vlan=,", ",").Replace(text)
		parsed, errs := ParseLine("csv", text)
		parsed.Name, parsed.VDOM, parsed.VLAN = strings.TrimSpace(req.Name), strings.TrimSpace(req.VDOM), strings.TrimSpace(req.VLAN)
		l := Line{Number: line, Parsed: parsed, Errors: errs, Status: "valid"}
		if len(errs) > 0 {
			l.Status = "invalid"
		} else if parsed.Name == "" {
			l.Status, l.Errors = "needs_enrichment", []string{"No name: add one."}
		}
		// Duplicates within the file are checked against the other records.
		seen := map[string]int{}
		rows, err := tx.Query(ctx, `SELECT parsed->>'cidr', line_number FROM import_records WHERE batch_id = $1::uuid AND id <> $2::uuid
			AND status IN ('valid','needs_enrichment') AND parsed->>'cidr' IS NOT NULL`, batchID, id)
		if err != nil {
			return err
		}
		for rows.Next() {
			var cidr string
			var n int
			if err := rows.Scan(&cidr, &n); err != nil {
				rows.Close()
				return err
			}
			seen[cidr] = n
		}
		rows.Close()
		if err := classify(ctx, tx, rd, &l, seen); err != nil {
			return err
		}
		pj, _ := json.Marshal(l.Parsed)
		ej, _ := json.Marshal(append([]string{}, l.Errors...))
		if _, err := tx.Exec(ctx, `UPDATE import_records SET parsed = $2::jsonb, status = $3, errors = $4::jsonb WHERE id = $1::uuid`, id, pj, l.Status, ej); err != nil {
			return err
		}
		if err := recount(ctx, tx, batchID); err != nil {
			return err
		}
		rows2, err := tx.Query(ctx, "SELECT "+recordCols+" FROM import_records r WHERE r.id = $1::uuid", id)
		if err != nil {
			return err
		}
		if out, err = pgx.CollectExactlyOneRow(rows2, scanRecord); err != nil {
			return err
		}
		return audit.Write(ctx, tx, audit.Entry{ActorID: actor, Action: "import_record.enrich", ObjectType: "import_record", ObjectID: id,
			Old: map[string]any{"parsed": json.RawMessage(oldParsed), "status": oldStatus}, New: map[string]any{"parsed": l.Parsed, "status": l.Status}})
	})
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, http.StatusOK, out)
}

// approve commits every valid record as a subnet, in one transaction. Someone
// other than the uploader must approve. Invalid, duplicate and overlapping
// lines stay recorded and are not imported.
func (h *Handler) approve(c *gin.Context) {
	id, ok := httpx.PathID(c)
	if !ok {
		return
	}
	ctx, actor := c.Request.Context(), auth.Current(c).UserID
	var out Batch
	err := db.InTx(ctx, h.conn, func(tx pgx.Tx) error {
		b, err := loadBatch(ctx, tx, id, true)
		if err != nil {
			return err
		}
		if b.Status != "validated" {
			return httpx.NewError(http.StatusConflict, "IMPORT_LOCKED", "Only batches waiting for approval can be approved.")
		}
		if b.UploadedBy == actor {
			return httpx.NewError(http.StatusForbidden, "SELF_APPROVAL_DENIED", "Someone other than the uploader must approve an import.")
		}
		if b.RoutingDomainID == nil || b.Environment == nil {
			return httpx.NewError(http.StatusConflict, "IMPORT_INCOMPLETE", "This batch has no routing domain or environment.")
		}
		rows, err := tx.Query(ctx, `SELECT id::text, line_number, parsed FROM import_records WHERE batch_id = $1::uuid AND status = 'valid' ORDER BY line_number`, id)
		if err != nil {
			return err
		}
		type pending struct {
			id     string
			line   int
			parsed Parsed
		}
		var todo []pending
		for rows.Next() {
			var p pending
			var raw []byte
			if err := rows.Scan(&p.id, &p.line, &raw); err != nil {
				rows.Close()
				return err
			}
			_ = json.Unmarshal(raw, &p.parsed)
			todo = append(todo, p)
		}
		rows.Close()
		if len(todo) == 0 {
			return httpx.NewError(http.StatusConflict, "IMPORT_EMPTY", "No valid lines to import.")
		}
		svc := subnets.NewService(tx)
		for _, p := range todo {
			req := subnets.CreateRequest{RoutingDomainID: *b.RoutingDomainID, CIDR: p.parsed.CIDR, Name: p.parsed.Name, Environment: *b.Environment,
				Description: fmt.Sprintf("Imported from %s line %d", b.FileName, p.line)}
			if p.parsed.Gateway != "" {
				g := p.parsed.Gateway
				req.Gateway = &g
			}
			if v, err := lookup(ctx, tx, `SELECT id::text FROM vdoms WHERE name = $1 AND (routing_domain_id = $2::uuid OR routing_domain_id IS NULL) LIMIT 2`, p.parsed.VDOM, *b.RoutingDomainID); err == nil && v != "" {
				req.VDOMID = &v
			}
			if v, err := lookup(ctx, tx, `SELECT id::text FROM vlans WHERE vlan_id::text = $1 LIMIT 2`, p.parsed.VLAN, ""); err == nil && v != "" {
				req.VLANID = &v
			}
			created, err := svc.Create(ctx, actor, req)
			if err != nil {
				var e *httpx.Error
				if errors.As(err, &e) {
					return httpx.NewError(http.StatusConflict, "IMPORT_LINE_FAILED", fmt.Sprintf("Line %d: %s Nothing was imported.", p.line, e.Message))
				}
				return err
			}
			if _, err := tx.Exec(ctx, `UPDATE import_records SET status = 'committed', created_object_type = 'subnet', created_object_id = $2::uuid WHERE id = $1::uuid`,
				p.id, created.ID); err != nil {
				return err
			}
		}
		if _, err := tx.Exec(ctx, `UPDATE import_batches SET status = 'committed', approved_by = $2::uuid, committed_at = now() WHERE id = $1::uuid`, id, actor); err != nil {
			return err
		}
		if err := recount(ctx, tx, id); err != nil {
			return err
		}
		if out, err = loadBatch(ctx, tx, id, false); err != nil {
			return err
		}
		return audit.Write(ctx, tx, audit.Entry{ActorID: actor, Action: "import.approve", ObjectType: "import_batch", ObjectID: id, ApprovalRef: id,
			New: map[string]any{"committed": len(todo)}})
	})
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, http.StatusOK, out)
}

// lookup returns the single match, or "" when there is none or more than one.
func lookup(ctx context.Context, tx pgx.Tx, sql, value, scope string) (string, error) {
	if strings.TrimSpace(value) == "" {
		return "", nil
	}
	args := []any{strings.TrimSpace(value)}
	if scope != "" {
		args = append(args, scope)
	}
	rows, err := tx.Query(ctx, sql, args...)
	if err != nil {
		return "", err
	}
	ids, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil || len(ids) != 1 {
		return "", err
	}
	return ids[0], nil
}

// reverse deletes the subnets a batch created. It only works while they are
// untouched: a subnet that has gained IP records or other links stops the
// whole reversal, and nothing changes.
func (h *Handler) reverse(c *gin.Context) {
	id, ok := httpx.PathID(c)
	if !ok {
		return
	}
	ctx, actor := c.Request.Context(), auth.Current(c).UserID
	var out Batch
	err := db.InTx(ctx, h.conn, func(tx pgx.Tx) error {
		b, err := loadBatch(ctx, tx, id, true)
		if err != nil {
			return err
		}
		if b.Status != "committed" {
			return httpx.NewError(http.StatusConflict, "IMPORT_LOCKED", "Only committed batches can be reversed.")
		}
		rows, err := tx.Query(ctx, `SELECT id::text, line_number, created_object_id::text FROM import_records
			WHERE batch_id = $1::uuid AND status = 'committed' AND created_object_type = 'subnet' ORDER BY line_number DESC`, id)
		if err != nil {
			return err
		}
		type made struct {
			record string
			line   int
			subnet string
		}
		var list []made
		for rows.Next() {
			var m made
			if err := rows.Scan(&m.record, &m.line, &m.subnet); err != nil {
				rows.Close()
				return err
			}
			list = append(list, m)
		}
		rows.Close()
		svc := subnets.NewService(tx)
		for _, m := range list {
			if err := svc.Delete(ctx, actor, m.subnet); err != nil && !errors.Is(err, httpx.ErrNotFound) {
				var e *httpx.Error
				if errors.As(err, &e) {
					return httpx.NewError(http.StatusConflict, "IMPORT_REVERSE_BLOCKED",
						fmt.Sprintf("Line %d's subnet is now in use (%s) Nothing was reversed.", m.line, e.Message))
				}
				return err
			}
			if _, err := tx.Exec(ctx, `UPDATE import_records SET status = 'valid', created_object_id = NULL, created_object_type = NULL WHERE id = $1::uuid`, m.record); err != nil {
				return err
			}
		}
		if _, err := tx.Exec(ctx, `UPDATE import_batches SET status = 'reversed', reversed_at = now(), reversed_by = $2::uuid WHERE id = $1::uuid`, id, actor); err != nil {
			return err
		}
		if err := recount(ctx, tx, id); err != nil {
			return err
		}
		if out, err = loadBatch(ctx, tx, id, false); err != nil {
			return err
		}
		return audit.Write(ctx, tx, audit.Entry{ActorID: actor, Action: "import.reverse", ObjectType: "import_batch", ObjectID: id,
			Old: map[string]any{"subnets": len(list)}})
	})
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, http.StatusOK, out)
}

// Summary is decoded by the web app; this keeps the type in one place.
var _ = Summary{}
