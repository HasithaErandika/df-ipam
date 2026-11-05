// Command worker runs background work: the hourly reconciliation run, and every
// minute the discovery scheduler, the allocation workflow's expiry jobs and email delivery. Several
// workers can run at once, because every job locks its rows with SKIP LOCKED.
package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"dfipam/api/internal/modules/allocation"
	"dfipam/api/internal/modules/discovery"
	"dfipam/api/internal/modules/notifications"
	"dfipam/api/internal/modules/reconciliation"
	"dfipam/api/internal/platform/db"
	"dfipam/api/internal/platform/mail"
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

	var mailer mail.Mailer
	cfg, enabled, err := mail.FromEnv()
	switch {
	case err != nil:
		slog.Error("email settings are invalid, so email is off", "error", err)
	case enabled:
		mailer = mail.NewSMTP(cfg)
		slog.Info("email is on", "host", cfg.Host, "port", cfg.Port)
	default:
		slog.Warn("SMTP_HOST is not set, so email is off; in-app alerts still work")
	}
	baseURL := strings.TrimRight(strings.TrimSpace(os.Getenv("APP_BASE_URL")), "/")
	workflow := allocation.NewService(pool)
	scheduler := discovery.NewService(pool)

	reconcile := func() {
		out, err := reconciliation.Run(ctx, pool, "")
		if err != nil {
			slog.Error("reconciliation failed", "error", err)
			return
		}
		slog.Info("reconciliation finished", "detected", out.Detected, "resolved", out.Resolved)
	}
	minutely := func() {
		if res, err := scheduler.RunScheduler(ctx, time.Now()); err != nil {
			slog.Error("discovery scheduler failed", "error", err)
		} else if res != (discovery.SchedulerResult{}) {
			slog.Info("discovery scheduler finished", "started", res.Started, "failed", res.Failed, "agentsOffline", res.AgentsOffline)
		}
		res, err := workflow.RunJobs(ctx)
		if err != nil {
			slog.Error("allocation jobs failed", "error", err)
		} else if res != (allocation.JobResult{}) {
			slog.Info("allocation jobs finished", "reservationsExpired", res.ReservationsExpired,
				"remindersSent", res.RemindersSent, "requestsExpired", res.RequestsExpired)
		}
		sent, err := notifications.Deliver(ctx, pool, mailer, baseURL, 50)
		if err != nil {
			slog.Error("email delivery failed", "error", err)
		} else if sent != (notifications.DeliveryResult{}) {
			slog.Info("email delivery finished", "sent", sent.Sent, "failed", sent.Failed, "suppressed", sent.Suppressed)
		}
	}
	reconcile()
	minutely()
	hourly, everyMinute := time.NewTicker(time.Hour), time.NewTicker(time.Minute)
	defer hourly.Stop()
	defer everyMinute.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-hourly.C:
			reconcile()
		case <-everyMinute.C:
			minutely()
		}
	}
}
