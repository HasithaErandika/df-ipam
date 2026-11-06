package imports

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"dfipam/api/internal/auth"
	"dfipam/api/internal/platform/db"
	"dfipam/api/internal/platform/httpx"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
)

const maxBytes = 5 << 20 // larger exports are split before upload
const maxBody = 7 << 20  // a 5 MB workbook arrives as base64

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

var environments = map[string]bool{"production": true, "disaster_recovery": true, "uat": true, "development": true, "testing": true}

type Handler struct{ conn db.Conn }

func NewHandler(conn db.Conn) *Handler { return &Handler{conn} }

func (h *Handler) Register(g *auth.Guard) {
	g.Handle(http.MethodGet, "/imports", h.list, auth.ImportsRun, auth.AuditView)
	g.Handle(http.MethodPost, "/imports", h.upload, auth.ImportsRun)
	g.Handle(http.MethodGet, "/imports/templates", h.templates, auth.ImportsRun)
	g.Handle(http.MethodGet, "/imports/templates/:key", h.template, auth.ImportsRun)
	g.Handle(http.MethodGet, "/imports/enrichment-queue", h.queue, auth.ImportsRun)
	g.Handle(http.MethodPatch, "/imports/records/:id", h.enrich, auth.ImportsRun)
	g.Handle(http.MethodGet, "/imports/:id", h.get, auth.ImportsRun, auth.AuditView)
	g.Handle(http.MethodGet, "/imports/:id/preview", h.preview, auth.ImportsRun, auth.AuditView)
	g.Handle(http.MethodPost, "/imports/:id/approve", h.approve, auth.ImportsRun)
	g.Handle(http.MethodPost, "/imports/:id/reverse", h.reverse, auth.ImportsRun)
	g.Handle(http.MethodPost, "/imports/:id/discard", h.discard, auth.ImportsRun)
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
		  'committed', count(*) FILTER (WHERE status = 'committed'),
		  'sheets', coalesce((SELECT jsonb_object_agg(k.sheet, k.n) FROM (SELECT parsed->>'sheet' AS sheet, count(*) AS n FROM import_records
		    WHERE batch_id = $1::uuid AND status IN ('valid','committed') AND parsed ? 'sheet' GROUP BY 1) k), '{}'::jsonb))
		FROM import_records WHERE batch_id = $1::uuid) WHERE id = $1::uuid`, batchID)
	return err
}
