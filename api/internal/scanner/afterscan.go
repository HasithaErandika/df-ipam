package scanner

import (
	"context"

	"dfipam/api/internal/modules/monitoring"
)

// afterScan evaluates the alert rules once more and clears old evidence.
func (s *Scanner) afterScan(ctx context.Context) []error {
	var scanErrors []error
	if err := monitoring.New(s.conn).Evaluate(ctx); err != nil {
		scanErrors = append(scanErrors, err)
	}
	// Evidence is kept for 90 days after a device was last seen. Phones that
	// change their private MAC would otherwise leave rows behind forever.
	// Closed alerts are kept for a year, for the Alert history report.
	if _, err := s.conn.Exec(ctx, `DELETE FROM sightings WHERE last_seen < now() - interval '90 days'`); err != nil {
		scanErrors = append(scanErrors, err)
	}
	if _, err := s.conn.Exec(ctx, `DELETE FROM alerts WHERE status = 'closed' AND closed_at < now() - interval '365 days'`); err != nil {
		scanErrors = append(scanErrors, err)
	}
	return scanErrors
}

// markScanned records whether a segment's scan failed, writing only when that
// changes. The alert rules keep a failed segment's alerts open.
func (s *Scanner) markScanned(ctx context.Context, segmentID string, failed bool) error {
	_, err := s.conn.Exec(ctx, `UPDATE segments SET scan_failed_at = CASE WHEN $2 THEN now() END
		WHERE id = $1::uuid AND (scan_failed_at IS NULL) = $2`, segmentID, failed)
	return err
}
