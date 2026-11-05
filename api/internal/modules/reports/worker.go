package reports

import (
	"context"
	"dfipam/api/internal/modules/notifications"
	"dfipam/api/internal/platform/db"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
)

func nullable(v string) *string {
	if v == "" {
		return nil
	}
	return &v
}

// endOfDay makes an end date inclusive of that whole day.
func endOfDay(v string) *string {
	t, err := time.Parse("2006-01-02", v)
	if err != nil {
		return nil
	}
	s := t.AddDate(0, 0, 1).Format("2006-01-02")
	return &s
}

// RunResult summarises one pass of the report worker.
type RunResult struct{ Completed, Failed, Stuck int64 }

// Run fails stuck jobs and works through the queue. SKIP LOCKED lets several
// workers share it.
func Run(ctx context.Context, conn db.Conn) (RunResult, error) {
	var out RunResult
	tag, err := conn.Exec(ctx, `UPDATE report_jobs SET status = 'failed', error = 'The worker stopped before this report finished.', finished_at = now()
		WHERE status = 'running' AND started_at < now() - make_interval(mins => $1)`, stuckAfter)
	if err != nil {
		return out, err
	}
	out.Stuck = tag.RowsAffected()
	for ctx.Err() == nil {
		var id, kind, format, user string
		var raw []byte
		err := conn.QueryRow(ctx, `UPDATE report_jobs SET status = 'running', started_at = now()
			WHERE id = (SELECT id FROM report_jobs WHERE status = 'queued' ORDER BY created_at FOR UPDATE SKIP LOCKED LIMIT 1)
			RETURNING id::text, report_type, format, filters, requested_by::text`).Scan(&id, &kind, &format, &raw, &user)
		if errors.Is(err, pgx.ErrNoRows) {
			return out, nil
		}
		if err != nil {
			return out, err
		}
		if err := runOne(ctx, conn, id, kind, format, raw, user); err != nil {
			out.Failed++
			slog.Error("report failed", "job", id, "type", kind, "error", err)
			message := "The report could not be generated."
			if errors.Is(err, errTooLarge) {
				message = "The report is too large. Narrow the filters and try again."
			}
			fctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
			_, uerr := conn.Exec(fctx, `UPDATE report_jobs SET status = 'failed', error = $2, finished_at = now() WHERE id = $1::uuid`, id, message)
			cancel()
			if uerr != nil {
				return out, uerr
			}
			continue
		}
		out.Completed++
	}
	return out, ctx.Err()
}

func runOne(ctx context.Context, conn db.Conn, id, kind, format string, raw []byte, user string) error {
	d, ok := definition(kind)
	if !ok {
		return fmt.Errorf("unknown report %q", kind)
	}
	var f Filters
	if err := json.Unmarshal(raw, &f); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, jobTimeout)
	defer cancel()
	asOf := time.Now()
	content, count, truncated, err := generate(ctx, conn, d, format, f, asOf)
	if err != nil {
		return err
	}
	return db.InTx(ctx, conn, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `UPDATE report_jobs SET status = 'completed', content = $2, content_type = $3, size_bytes = $4, row_count = $5, truncated = $6,
			data_as_of = $7, finished_at = now() WHERE id = $1::uuid`, id, content, formats[format].contentType, len(content), count, truncated, asOf); err != nil {
			return err
		}
		return notifications.Enqueue(ctx, tx, "", notifications.Event{Type: notifications.ReportReady, ObjectType: "report", ObjectID: id, DedupeKey: "report-ready:" + id,
			Title: d.Title + " is ready", Body: fmt.Sprintf("%d rows.", count), Link: "/reports", Recipients: []string{user}})
	})
}

var errReportFinished = errors.New("report finished")

func generate(ctx context.Context, conn db.Conn, d Definition, format string, f Filters, asOf time.Time) ([]byte, int64, bool, error) {
	out, err := newRenderer(format, d, asOf)
	if err != nil {
		return nil, 0, false, err
	}
	limit := limitFor(format)
	var count int64
	var truncated bool
	// A report changes nothing, so its transaction always rolls back. That also
	// clears the read-only setting when this runs inside another transaction.
	err = db.InTx(ctx, conn, func(tx pgx.Tx) (err error) {
		if count, truncated, err = streamReport(ctx, tx, d, f, limit, out); err != nil {
			return err
		}
		return errReportFinished
	})
	if !errors.Is(err, errReportFinished) {
		return nil, 0, false, err
	}
	content, err := out.finish(truncated, limit)
	if err != nil {
		return nil, 0, false, err
	}
	if len(content) > maxFileBytes {
		return nil, 0, false, errTooLarge
	}
	return content, count, truncated, nil
}

// streamReport passes each row to the renderer, up to the limit.
func streamReport(ctx context.Context, tx pgx.Tx, d Definition, f Filters, limit int64, out renderer) (count int64, truncated bool, err error) {
	// Reports only read, and the database holds them to that.
	if _, err := tx.Exec(ctx, `SET TRANSACTION READ ONLY`); err != nil {
		return 0, false, err
	}
	if _, err := tx.Exec(ctx, `SET LOCAL statement_timeout = '9min'`); err != nil {
		return 0, false, err
	}
	var scope any
	if f.Scope != nil {
		scope = *f.Scope
	}
	rows, err := tx.Query(ctx, d.sql+fmt.Sprintf(" LIMIT %d", limit+1), nullable(f.RoutingDomainID), nullable(f.Environment), nullable(f.From), endOfDay(f.To), scope)
	if err != nil {
		return 0, false, err
	}
	defer rows.Close()
	cells := make([]string, len(d.Columns))
	dest := make([]any, len(cells))
	for i := range cells {
		dest[i] = &cells[i]
	}
	for rows.Next() {
		if count == limit {
			return count, true, nil
		}
		if err := rows.Scan(dest...); err != nil {
			return count, false, err
		}
		if err := out.row(cells); err != nil {
			return count, false, err
		}
		count++
	}
	return count, false, rows.Err()
}
