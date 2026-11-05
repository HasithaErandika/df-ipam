package discovery

import (
	"context"

	"github.com/jackc/pgx/v5"
)

const maxLogMessage = 2000

// LogLine is one line of a job's log.
type LogLine struct{ Level, Message string }

// Log appends a job log line.
func Log(ctx context.Context, tx pgx.Tx, jobID, level, message string) error {
	return LogLines(ctx, tx, jobID, []LogLine{{level, message}})
}

// LogLines appends several lines to one job's log in one statement, in order.
func LogLines(ctx context.Context, tx pgx.Tx, jobID string, lines []LogLine) error {
	if len(lines) == 0 {
		return nil
	}
	levels, messages := make([]string, len(lines)), make([]string, len(lines))
	for i, l := range lines {
		levels[i], messages[i] = l.Level, clipLog(l.Message)
	}
	_, err := tx.Exec(ctx, `INSERT INTO discovery_job_logs (job_id, level, message)
		SELECT $1::uuid, level, message FROM unnest($2::text[], $3::text[]) WITH ORDINALITY AS t(level, message, n) ORDER BY n`, jobID, levels, messages)
	return err
}

// logEachJob appends the same line to many jobs in one statement.
func logEachJob(ctx context.Context, tx pgx.Tx, jobIDs []string, level, message string) error {
	if len(jobIDs) == 0 {
		return nil
	}
	_, err := tx.Exec(ctx, `INSERT INTO discovery_job_logs (job_id, level, message) SELECT id, $2, $3 FROM unnest($1::uuid[]) AS t(id)`,
		jobIDs, level, clipLog(message))
	return err
}

func clipLog(message string) string {
	if len(message) > maxLogMessage {
		return message[:maxLogMessage]
	}
	return message
}
