package reconciliation

import (
	"context"
	"dfipam/api/internal/audit"
	"dfipam/api/internal/auth"
	"dfipam/api/internal/platform/db"
	"dfipam/api/internal/platform/httpx"
	"encoding/json"
	"net/http"
	"slices"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
)

func (h *Handler) listSettings(c *gin.Context) {
	saved, err := loadSettings(c.Request.Context(), h.conn)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	out := make([]Settings, 0, len(Classifications))
	for _, class := range Classifications {
		s, ok := saved[class]
		if !ok {
			s = Settings{Classification: class, IsEnabled: true}
		}
		if s.SourcePriority == nil {
			s.SourcePriority = []string{}
		}
		out = append(out, s)
	}
	httpx.OK(c, http.StatusOK, out)
}

func (h *Handler) saveSettings(c *gin.Context) {
	class := c.Param("classification")
	if !slices.Contains(Classifications, class) {
		httpx.Fail(c, httpx.ErrNotFound)
		return
	}
	var in Settings
	if err := c.ShouldBindJSON(&in); err != nil {
		httpx.Fail(c, httpx.ErrBadRequest)
		return
	}
	if err := in.Weights.validate(); err != nil {
		httpx.Fail(c, err)
		return
	}
	if in.SourcePriority == nil {
		in.SourcePriority = []string{}
	}
	in.Classification = class
	ctx, actor := c.Request.Context(), auth.Current(c).UserID
	err := db.InTx(ctx, h.conn, func(tx pgx.Tx) error { return storeSettings(ctx, tx, actor, in) })
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, http.StatusOK, in)
}

func storeSettings(ctx context.Context, tx pgx.Tx, actor string, in Settings) error {
	var unknown int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM unnest($1::text[]) p(name) WHERE NOT EXISTS (SELECT 1 FROM observation_sources os WHERE os.name = p.name)`,
		in.SourcePriority).Scan(&unknown); err != nil {
		return err
	}
	if unknown > 0 {
		return httpx.Validation("sourcePriority", "Use only known evidence sources.")
	}
	old, err := loadSettings(ctx, tx)
	if err != nil {
		return err
	}
	priority, err := json.Marshal(in.SourcePriority)
	if err != nil {
		return err
	}
	weights, err := json.Marshal(in.Weights)
	if err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO reconciliation_rules (classification, source_priority, confidence_weights, is_enabled)
		VALUES ($1, $2::jsonb, $3::jsonb, $4)
		ON CONFLICT (classification) DO UPDATE SET source_priority = EXCLUDED.source_priority, confidence_weights = EXCLUDED.confidence_weights,
		  is_enabled = EXCLUDED.is_enabled`, in.Classification, priority, weights, in.IsEnabled); err != nil {
		return err
	}
	return audit.Write(ctx, tx, audit.Entry{ActorID: actor, Action: "reconciliation_rule.update", ObjectType: "reconciliation_rule", Old: old[in.Classification], New: in})
}
