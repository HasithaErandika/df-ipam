package reconciliation

import (
	"dfipam/api/internal/audit"
	"dfipam/api/internal/auth"
	"dfipam/api/internal/platform/db"
	"dfipam/api/internal/platform/httpx"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
)

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
