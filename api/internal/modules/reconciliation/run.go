package reconciliation

import (
	"context"
	"dfipam/api/internal/audit"
	"dfipam/api/internal/platform/db"
	"fmt"

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
	Detected int64 `json:"detected"` // everything the rules matched, old and new
	New      int64 `json:"new"`      // findings that didn't exist before this run
	Resolved int64 `json:"resolved"`
	Open     int64 `json:"open"` // findings needing review after this run
}

// Run applies every enabled rule, refreshes confidence, and resolves what no rule
// detected this time.
func Run(ctx context.Context, conn db.Conn, actorID string) (Outcome, error) {
	var out Outcome
	err := db.InTx(ctx, conn, func(tx pgx.Tx) (err error) {
		out, err = runRules(ctx, tx, actorID)
		return err
	})
	return out, err
}

func runRules(ctx context.Context, tx pgx.Tx, actorID string) (Outcome, error) {
	var out Outcome
	settings, err := loadSettings(ctx, tx)
	if err != nil {
		return out, err
	}
	before, err := countFindings(ctx, tx)
	if err != nil {
		return out, err
	}
	if out.Detected, err = detect(ctx, tx, settings); err != nil {
		return out, err
	}
	after, err := countFindings(ctx, tx)
	if err != nil {
		return out, err
	}
	out.New = after - before
	disabled := []string{}
	for class, s := range settings {
		if !s.IsEnabled {
			disabled = append(disabled, class)
		}
		if err := applySettings(ctx, tx, class, s); err != nil {
			return out, fmt.Errorf("confidence for %s: %w", class, err)
		}
	}
	// Switching a classification off stops new detection. It doesn't mean its
	// findings were fixed, so they aren't resolved.
	tag, err := tx.Exec(ctx, `UPDATE reconciliation_results SET review_status = 'resolved'
		WHERE review_status IN ('open', 'investigating') AND last_detected_at < now() AND NOT classification = ANY($1)`, disabled)
	if err != nil {
		return out, err
	}
	out.Resolved = tag.RowsAffected()
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM reconciliation_results WHERE review_status IN ('open', 'investigating')`).Scan(&out.Open); err != nil {
		return out, err
	}
	entry := audit.Entry{ActorID: actorID, Action: "reconciliation.run", ObjectType: "reconciliation", New: out}
	if actorID == "" {
		entry.ActorType = "system"
	}
	return out, audit.Write(ctx, tx, entry)
}

func countFindings(ctx context.Context, tx pgx.Tx) (int64, error) {
	var n int64
	err := tx.QueryRow(ctx, `SELECT count(*) FROM reconciliation_results`).Scan(&n)
	return n, err
}

func detect(ctx context.Context, tx pgx.Tx, settings map[string]Settings) (int64, error) {
	var detected int64
	for _, r := range rules {
		if s, ok := settings[classOf[r.name]]; ok && !s.IsEnabled {
			continue
		}
		tag, err := tx.Exec(ctx, fmt.Sprintf(insert, r.sql))
		if err != nil {
			return detected, fmt.Errorf("rule %s: %w", r.name, err)
		}
		detected += tag.RowsAffected()
	}
	return detected, nil
}
