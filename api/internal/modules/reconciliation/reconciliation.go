// Package reconciliation compares inventory with evidence and records findings.
// It never changes inventory: administrators review findings, then act.
package reconciliation

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"dfipam/api/internal/audit"
	"dfipam/api/internal/auth"
	"dfipam/api/internal/platform/db"
	"dfipam/api/internal/platform/httpx"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
)

const insert = `
INSERT INTO reconciliation_results AS r (fingerprint, classification, object_type, object_id, routing_domain_id, address, sources,
  evidence_from, evidence_to, source_reliability, confidence, explanation, suggested_action, last_detected_at)
SELECT f.*, now() FROM (%s) f
ON CONFLICT (fingerprint) DO UPDATE SET explanation = EXCLUDED.explanation, confidence = EXCLUDED.confidence,
  sources = EXCLUDED.sources, evidence_to = EXCLUDED.evidence_to, last_detected_at = now(),
  review_status = CASE WHEN r.review_status = 'resolved' THEN 'open' ELSE r.review_status END`

// Outcome summarises one run.
type Outcome struct {
	Detected int64 `json:"detected"`
	Resolved int64 `json:"resolved"`
}

// Run evaluates every rule. Findings no longer true are resolved. Findings a person
// dismissed stay dismissed, so a run doesn't bring them back.
func Run(ctx context.Context, conn db.Conn, actorID string) (Outcome, error) {
	var out Outcome
	err := db.InTx(ctx, conn, func(tx pgx.Tx) error {
		settings, err := loadSettings(ctx, tx)
		if err != nil {
			return err
		}
		disabled := []string{}
		for class, s := range settings {
			if !s.IsEnabled {
				disabled = append(disabled, class)
			}
		}
		for _, r := range rules {
			if s, ok := settings[classOf[r.name]]; ok && !s.IsEnabled {
				continue
			}
			tag, err := tx.Exec(ctx, fmt.Sprintf(insert, r.sql))
			if err != nil {
				return fmt.Errorf("rule %s: %w", r.name, err)
			}
			out.Detected += tag.RowsAffected()
		}
		for class, s := range settings {
			if err := applySettings(ctx, tx, class, s); err != nil {
				return fmt.Errorf("confidence for %s: %w", class, err)
			}
		}
		// Switching a classification off stops new detection. It doesn't mean its
		// findings were fixed, so they aren't resolved.
		tag, err := tx.Exec(ctx, `UPDATE reconciliation_results SET review_status = 'resolved'
			WHERE review_status IN ('open', 'investigating') AND last_detected_at < now() AND NOT classification = ANY($1)`, disabled)
		if err != nil {
			return err
		}
		out.Resolved = tag.RowsAffected()
		entry := audit.Entry{ActorID: actorID, Action: "reconciliation.run", ObjectType: "reconciliation", New: out}
		if actorID == "" {
			entry.ActorType = "system"
		}
		return audit.Write(ctx, tx, entry)
	})
	return out, err
}

type Finding struct {
	ID                string     `json:"id"`
	Classification    string     `json:"classification"`
	ObjectType        string     `json:"objectType"`
	ObjectID          *string    `json:"objectId"`
	RoutingDomainID   *string    `json:"routingDomainId"`
	RoutingDomainName *string    `json:"routingDomainName"`
	Address           *string    `json:"address"`
	Sources           []string   `json:"sources"`
	EvidenceFrom      *time.Time `json:"evidenceFrom"`
	EvidenceTo        *time.Time `json:"evidenceTo"`
	SourceReliability *int16     `json:"sourceReliability"`
	Confidence        *int16     `json:"confidence"`
	Explanation       string     `json:"explanation"`
	SuggestedAction   string     `json:"suggestedAction"`
	ReviewStatus      string     `json:"reviewStatus"`
	ReviewNote        string     `json:"reviewNote"`
	ReviewedBy        *string    `json:"reviewedBy"`
	ReviewedAt        *time.Time `json:"reviewedAt"`
	LastDetectedAt    time.Time  `json:"lastDetectedAt"`
	// ConfidenceBreakdown holds each contribution to the score, when it was calculated.
	ConfidenceBreakdown json.RawMessage `json:"confidenceBreakdown"`
}

// Applier makes an approved change to a record in its owning module, inside tx. It
// returns the fields before and after, for the audit row.
type Applier func(ctx context.Context, tx pgx.Tx, id, actor string, changes map[string]string) (before, after map[string]string, err error)

// Target says what a person may change on one kind of record. Allocation state isn't
// here on purpose: it changes only through the allocation workflow.
type Target struct {
	Fields map[string]bool
	Apply  Applier
}

type Handler struct {
	conn    db.Conn
	targets map[string]Target
}

// NewHandler takes the record types change requests can act on, keyed by the
// finding's object type.
func NewHandler(conn db.Conn, targets map[string]Target) *Handler { return &Handler{conn, targets} }

func (h *Handler) Register(g *auth.Guard) {
	g.Handle(http.MethodGet, "/reconciliation/results", h.list, auth.InventoryView)
	g.Handle(http.MethodGet, "/reconciliation/summary", h.summary, auth.InventoryView)
	g.Handle(http.MethodPost, "/reconciliation/run", h.run, auth.DiscoveryRun)
	g.Handle(http.MethodPost, "/reconciliation/results/:id/review", h.review, auth.InventoryEdit)
	g.Handle(http.MethodGet, "/reconciliation/rules", h.listSettings, auth.InventoryView)
	g.Handle(http.MethodPut, "/reconciliation/rules/:classification", h.saveSettings, auth.DiscoveryManage)
	g.Handle(http.MethodPost, "/reconciliation/results/:id/change-requests", h.proposeChange, auth.InventoryEdit)
	g.Handle(http.MethodGet, "/change-requests", h.listChanges, auth.InventoryView)
	g.Handle(http.MethodPost, "/change-requests/:id/approve", h.approveChange, auth.RequestsApprove)
	g.Handle(http.MethodPost, "/change-requests/:id/reject", h.rejectChange, auth.RequestsApprove)
}

func (h *Handler) list(c *gin.Context) {
	p := httpx.ParsePage(c)
	var where []string
	var args []any
	status := c.DefaultQuery("status", "active")
	switch status {
	case "active":
		where = append(where, "r.review_status IN ('open', 'investigating')")
	case "all":
	default:
		args = append(args, status)
		where = append(where, fmt.Sprintf("r.review_status = $%d", len(args)))
	}
	if v := c.Query("classification"); v != "" {
		args = append(args, v)
		where = append(where, fmt.Sprintf("r.classification = $%d", len(args)))
	}
	if v := strings.TrimSpace(c.Query("q")); v != "" {
		args = append(args, v)
		where = append(where, fmt.Sprintf("(host(r.address) ILIKE '%%' || $%d || '%%' OR r.explanation ILIKE '%%' || $%d || '%%')", len(args), len(args)))
	}
	clause := ""
	if len(where) > 0 {
		clause = " WHERE " + strings.Join(where, " AND ")
	}
	ctx := c.Request.Context()
	var total int64
	if err := h.conn.QueryRow(ctx, "SELECT count(*) FROM reconciliation_results r"+clause, args...).Scan(&total); err != nil {
		httpx.Fail(c, err)
		return
	}
	args = append(args, p.Size, p.Offset())
	rows, err := h.conn.Query(ctx, fmt.Sprintf(`
		SELECT r.id::text, r.classification, r.object_type, r.object_id::text, r.routing_domain_id::text, rd.name, host(r.address), r.sources,
		       r.evidence_from, r.evidence_to, r.source_reliability, r.confidence, r.explanation, r.suggested_action, r.review_status,
		       r.review_note, u.email, r.reviewed_at, r.last_detected_at, r.confidence_breakdown
		FROM reconciliation_results r LEFT JOIN routing_domains rd ON rd.id = r.routing_domain_id LEFT JOIN users u ON u.id = r.reviewed_by
		%s ORDER BY r.confidence DESC NULLS LAST, r.last_detected_at DESC, r.id LIMIT $%d OFFSET $%d`, clause, len(args)-1, len(args)), args...)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	out, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (Finding, error) {
		var f Finding
		return f, r.Scan(&f.ID, &f.Classification, &f.ObjectType, &f.ObjectID, &f.RoutingDomainID, &f.RoutingDomainName, &f.Address, &f.Sources,
			&f.EvidenceFrom, &f.EvidenceTo, &f.SourceReliability, &f.Confidence, &f.Explanation, &f.SuggestedAction, &f.ReviewStatus,
			&f.ReviewNote, &f.ReviewedBy, &f.ReviewedAt, &f.LastDetectedAt, &f.ConfidenceBreakdown)
	})
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.List(c, out, p, total)
}

func (h *Handler) summary(c *gin.Context) {
	rows, err := h.conn.Query(c.Request.Context(), `SELECT classification, count(*) FROM reconciliation_results
		WHERE review_status IN ('open', 'investigating') GROUP BY classification ORDER BY count(*) DESC`)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	type count struct {
		Classification string `json:"classification"`
		Count          int64  `json:"count"`
	}
	out, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (count, error) {
		var v count
		return v, r.Scan(&v.Classification, &v.Count)
	})
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, http.StatusOK, append([]count{}, out...))
}

func (h *Handler) run(c *gin.Context) {
	out, err := Run(c.Request.Context(), h.conn, auth.Current(c).UserID)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, http.StatusOK, out)
}

var reviewStates = map[string]bool{"open": true, "investigating": true, "accepted": true, "dismissed": true, "resolved": true}

func (h *Handler) review(c *gin.Context) {
	id, ok := httpx.PathID(c)
	if !ok {
		return
	}
	var req struct {
		Status string `json:"status"`
		Note   string `json:"note"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Fail(c, httpx.ErrBadRequest)
		return
	}
	if !reviewStates[req.Status] {
		httpx.Fail(c, httpx.Validation("status", "Choose a valid review status."))
		return
	}
	ctx, actor := c.Request.Context(), auth.Current(c).UserID
	err := db.InTx(ctx, h.conn, func(tx pgx.Tx) error {
		var old string
		err := tx.QueryRow(ctx, `SELECT review_status FROM reconciliation_results WHERE id = $1::uuid FOR UPDATE`, id).Scan(&old)
		if errors.Is(err, pgx.ErrNoRows) {
			return httpx.ErrNotFound
		}
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE reconciliation_results SET review_status = $2, review_note = $3, reviewed_by = $4::uuid, reviewed_at = now()
			WHERE id = $1::uuid`, id, req.Status, req.Note, actor); err != nil {
			return err
		}
		return audit.Write(ctx, tx, audit.Entry{ActorID: actor, Action: "finding.review", ObjectType: "finding", ObjectID: id,
			Old: gin.H{"status": old}, New: gin.H{"status": req.Status, "note": req.Note}})
	})
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}
