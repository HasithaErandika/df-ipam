package reconciliation

import (
	"context"
	"encoding/json"
	"math"
	"net/http"

	"dfipam/api/internal/audit"
	"dfipam/api/internal/auth"
	"dfipam/api/internal/platform/db"
	"dfipam/api/internal/platform/httpx"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
)

// Weights build a finding's confidence from three parts, each 0 to 100: the
// reliability of its best source, how fresh its evidence is, and how many sources
// agree. Empty weights keep the fixed confidence in the rule.
type Weights struct {
	Reliability   float64 `json:"reliability"`
	Freshness     float64 `json:"freshness"`
	Agreement     float64 `json:"agreement"`
	Cap           int     `json:"cap"`
	FreshnessDays int     `json:"freshnessDays"`
}

func (w Weights) empty() bool { return w == Weights{} }

func (w Weights) validate() error {
	if w.empty() {
		return nil
	}
	for name, v := range map[string]float64{"reliability": w.Reliability, "freshness": w.Freshness, "agreement": w.Agreement} {
		if v < 0 || v > 1 {
			return httpx.Validation(name, "Use a weight between 0 and 1.")
		}
	}
	if math.Abs(w.Reliability+w.Freshness+w.Agreement-1) > 0.001 {
		return httpx.Validation("reliability", "The three weights must add up to 1.")
	}
	if w.Cap < 1 || w.Cap > 100 {
		return httpx.Validation("cap", "Use a cap between 1 and 100.")
	}
	if w.FreshnessDays < 1 || w.FreshnessDays > 365 {
		return httpx.Validation("freshnessDays", "Use between 1 and 365 days.")
	}
	return nil
}

// Score is the calculation in Go, matching scoreSQL. It documents the formula and
// lets the tests pin it down without a database.
func Score(w Weights, reliability, ageDays float64, sources int) int {
	freshness := math.Max(0, 1-ageDays/float64(w.FreshnessDays)) * 100
	agreement := math.Min(1, float64(sources-1)/2) * 100
	total := w.Reliability*reliability + w.Freshness*freshness + w.Agreement*agreement
	return min(w.Cap, int(math.Round(total)))
}

// Settings configure one classification.
type Settings struct {
	Classification string   `json:"classification"`
	SourcePriority []string `json:"sourcePriority"`
	Weights        Weights  `json:"weights"`
	IsEnabled      bool     `json:"isEnabled"`
}

func loadSettings(ctx context.Context, q db.Conn) (map[string]Settings, error) {
	rows, err := q.Query(ctx, `SELECT classification, source_priority, confidence_weights, is_enabled FROM reconciliation_rules ORDER BY classification`)
	if err != nil {
		return nil, err
	}
	list, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (Settings, error) {
		var s Settings
		var priority, weights []byte
		if err := r.Scan(&s.Classification, &priority, &weights, &s.IsEnabled); err != nil {
			return s, err
		}
		if err := json.Unmarshal(priority, &s.SourcePriority); err != nil {
			return s, err
		}
		return s, json.Unmarshal(weights, &s.Weights)
	})
	if err != nil {
		return nil, err
	}
	out := make(map[string]Settings, len(list))
	for _, s := range list {
		out[s.Classification] = s
	}
	return out, nil
}

// Findings detected in this run take the reliability of their highest-priority
// source. now() is fixed for the transaction, so it marks this run's findings.
const prioritySQL = `
UPDATE reconciliation_results r SET source_reliability = best.reliability
FROM (
  SELECT f.id, (SELECT os.reliability FROM unnest($2::text[]) WITH ORDINALITY p(name, ord)
                JOIN observation_sources os ON os.name = p.name
                WHERE p.name = ANY(f.sources) ORDER BY p.ord LIMIT 1) AS reliability
  FROM reconciliation_results f WHERE f.classification = $1 AND f.last_detected_at = now()
) best
WHERE r.id = best.id AND best.reliability IS NOT NULL`

const scoreSQL = `
WITH parts AS (
  SELECT id, COALESCE(source_reliability, 0)::float8 AS reliability,
    greatest(0, 1 - extract(epoch FROM now() - COALESCE(evidence_to, now())) / 86400 / $5::float8) * 100 AS freshness,
    least(1, greatest(0, cardinality(sources) - 1) / 2.0)::float8 * 100 AS agreement
  FROM reconciliation_results WHERE classification = $1 AND last_detected_at = now()
)
UPDATE reconciliation_results r SET
  confidence = least($6::int, round($2::float8 * p.reliability + $3::float8 * p.freshness + $4::float8 * p.agreement)::int),
  confidence_breakdown = jsonb_build_object('reliability', round(p.reliability), 'freshness', round(p.freshness), 'agreement', round(p.agreement),
    'weights', jsonb_build_object('reliability', $2::float8, 'freshness', $3::float8, 'agreement', $4::float8), 'cap', $6::int)
FROM parts p WHERE r.id = p.id`

func applySettings(ctx context.Context, tx pgx.Tx, class string, s Settings) error {
	if !s.IsEnabled {
		return nil
	}
	if len(s.SourcePriority) > 0 {
		if _, err := tx.Exec(ctx, prioritySQL, class, s.SourcePriority); err != nil {
			return err
		}
	}
	if s.Weights.empty() {
		return nil
	}
	w := s.Weights
	_, err := tx.Exec(ctx, scoreSQL, class, w.Reliability, w.Freshness, w.Agreement, w.FreshnessDays, w.Cap)
	return err
}

// Classifications are the values reconciliation_results allows.
var Classifications = []string{"new_device", "unknown_ip", "ip_conflict_suspected", "duplicate_mac", "dns_mismatch", "allocated_not_observed",
	"observed_not_allocated", "subnet_overlap", "ownership_missing", "infrastructure_mismatch", "stale_evidence"}

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
	known := false
	for _, k := range Classifications {
		known = known || k == class
	}
	if !known {
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
	ctx, actor := c.Request.Context(), auth.Current(c).UserID
	err := db.InTx(ctx, h.conn, func(tx pgx.Tx) error {
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
			  is_enabled = EXCLUDED.is_enabled`, class, priority, weights, in.IsEnabled); err != nil {
			return err
		}
		in.Classification = class
		return audit.Write(ctx, tx, audit.Entry{ActorID: actor, Action: "reconciliation_rule.update", ObjectType: "reconciliation_rule", Old: old[class], New: in})
	})
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, http.StatusOK, in)
}
