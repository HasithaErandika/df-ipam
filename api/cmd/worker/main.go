// Command worker runs background work: hourly reconciliation and utilisation
// snapshots; every minute the discovery scheduler, allocation expiry and email
// delivery; and, in their own loops so a long run holds nothing else up, integration
// syncs and reports. Several workers can run at once, because every job locks its
// rows with SKIP LOCKED.
package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"dfipam/api/internal/modules/admin"
	"dfipam/api/internal/modules/allocation"
	"dfipam/api/internal/modules/discovery"
	"dfipam/api/internal/modules/integrations"
	"dfipam/api/internal/modules/notifications"
	"dfipam/api/internal/modules/reconciliation"
	"dfipam/api/internal/modules/reports"
	"dfipam/api/internal/modules/subnets"
	"dfipam/api/internal/platform/db"
	"dfipam/api/internal/platform/mail"
	"dfipam/api/internal/platform/safehttp"
	"dfipam/api/internal/platform/secrets"

	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	pool, err := db.Connect(ctx, os.Getenv("DATABASE_URL"))
	if err != nil {
		slog.Error("worker stopped", "error", err)
		os.Exit(1)
	}
	defer pool.Close()
	w := &worker{pool: pool, mailer: newMailer(), workflow: allocation.NewService(pool), scheduler: discovery.NewService(pool),
		baseURL: strings.TrimRight(strings.TrimSpace(os.Getenv("APP_BASE_URL")), "/")}
	connectors := newConnectors(pool)
	go every(ctx, time.Minute, func() { runSyncs(ctx, connectors) })
	go every(ctx, 15*time.Second, func() { w.runReports(ctx) })
	w.reconcile(ctx)
	w.minutely(ctx)
	hourly, everyMinute := time.NewTicker(time.Hour), time.NewTicker(time.Minute)
	defer hourly.Stop()
	defer everyMinute.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-hourly.C:
			w.reconcile(ctx)
		case <-everyMinute.C:
			w.minutely(ctx)
		}
	}
}

// worker holds what the background jobs share.
type worker struct {
	pool      *pgxpool.Pool
	mailer    mail.Mailer
	workflow  *allocation.Service
	scheduler *discovery.Service
	baseURL   string
}

func newMailer() mail.Mailer {
	cfg, enabled, err := mail.FromEnv()
	switch {
	case err != nil:
		slog.Error("email settings are invalid, so email is off", "error", err)
	case enabled:
		slog.Info("email is on", "host", cfg.Host, "port", cfg.Port)
		return mail.NewSMTP(cfg)
	default:
		slog.Warn("SMTP_HOST is not set, so email is off; in-app alerts still work")
	}
	return nil
}

func newConnectors(pool *pgxpool.Pool) *integrations.Service {
	keys, err := secrets.FromEnv()
	if err != nil {
		keys = nil
		slog.Warn("integration credentials can't be read, so syncs that need them will fail", "reason", err)
	}
	outbound, err := safehttp.FromEnv()
	if err != nil {
		slog.Error("outbound policy is invalid, so every integration URL is refused", "error", err)
		outbound = safehttp.Policy{}
	}
	return integrations.NewService(pool, keys, outbound)
}

func runSyncs(ctx context.Context, connectors *integrations.Service) {
	res, err := connectors.RunSyncs(ctx)
	if err != nil {
		slog.Error("integration syncs failed", "error", err)
	} else if res != (integrations.RunResult{}) {
		slog.Info("integration syncs finished", "queued", res.Queued, "completed", res.Completed, "failed", res.Failed, "stuck", res.Stuck)
	}
}

func (w *worker) runReports(ctx context.Context) {
	res, err := reports.Run(ctx, w.pool)
	if err != nil {
		slog.Error("report generation failed", "error", err)
	} else if res != (reports.RunResult{}) {
		slog.Info("reports finished", "completed", res.Completed, "failed", res.Failed, "stuck", res.Stuck)
	}
}

func (w *worker) reconcile(ctx context.Context) {
	out, err := reconciliation.Run(ctx, w.pool, "")
	if err != nil {
		slog.Error("reconciliation failed", "error", err)
		return
	}
	slog.Info("reconciliation finished", "detected", out.Detected, "resolved", out.Resolved)
	if n, err := subnets.SnapshotDay(ctx, w.pool, time.Now()); err != nil {
		slog.Error("utilisation snapshot failed", "error", err)
	} else if n > 0 {
		slog.Info("utilisation snapshot finished", "subnets", n)
	}
}

func (w *worker) minutely(ctx context.Context) {
	if err := admin.Heartbeat(ctx, w.pool, time.Now()); err != nil {
		slog.Error("worker heartbeat failed", "error", err)
	}
	if res, err := w.scheduler.RunScheduler(ctx, time.Now()); err != nil {
		slog.Error("discovery scheduler failed", "error", err)
	} else if res != (discovery.SchedulerResult{}) {
		slog.Info("discovery scheduler finished", "started", res.Started, "failed", res.Failed, "agentsOffline", res.AgentsOffline)
	}
	res, err := w.workflow.RunJobs(ctx)
	if err != nil {
		slog.Error("allocation jobs failed", "error", err)
	} else if res != (allocation.JobResult{}) {
		slog.Info("allocation jobs finished", "reservationsExpired", res.ReservationsExpired,
			"remindersSent", res.RemindersSent, "requestsExpired", res.RequestsExpired)
	}
	sent, err := notifications.Deliver(ctx, w.pool, w.mailer, w.baseURL, 50)
	if err != nil {
		slog.Error("email delivery failed", "error", err)
	} else if sent != (notifications.DeliveryResult{}) {
		slog.Info("email delivery finished", "sent", sent.Sent, "failed", sent.Failed, "suppressed", sent.Suppressed)
	}
}

// every runs fn now and then on a schedule. A run that overruns delays the next one
// instead of overlapping it.
func every(ctx context.Context, interval time.Duration, fn func()) {
	fn()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			fn()
		}
	}
}
