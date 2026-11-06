package agent

import (
	"context"
	"errors"
	"log/slog"
	"runtime"
	"sync/atomic"
	"time"
)

// errHalt means the server said to pause or stop, or the emergency stop is on.
var errHalt = errors.New("halted by the server")

// Agent ties the loop together.
type Agent struct {
	client  *Client
	cfg     Config
	stopped atomic.Bool
	contact atomic.Int64 // when the server last answered, in Unix seconds
	jobs    atomic.Int64
}

func New(cfg Config) *Agent { return &Agent{client: NewClient(cfg), cfg: cfg} }

// Run registers if needed, then heartbeats and works until ctx ends.
func (a *Agent) Run(ctx context.Context) error {
	if err := a.client.Authenticate(ctx); err != nil {
		return err
	}
	slog.Info("agent authenticated", "api", a.cfg.APIURL, "version", Version)
	a.contact.Store(time.Now().Unix())
	go a.heartbeats(ctx)
	tick := time.NewTicker(a.cfg.Poll)
	defer tick.Stop()
	for {
		if !a.stopped.Load() {
			if err := a.work(ctx); err != nil && !errors.Is(err, context.Canceled) {
				slog.Error("job loop", "error", err)
			}
		}
		select {
		case <-ctx.Done():
			return nil
		case <-tick.C:
		}
	}
}

func (a *Agent) heartbeats(ctx context.Context) {
	tick := time.NewTicker(heartbeatEvery)
	defer tick.Stop()
	for {
		var mem runtime.MemStats
		runtime.ReadMemStats(&mem)
		stop, err := a.client.Heartbeat(ctx, map[string]any{"goroutines": runtime.NumGoroutine(), "heapBytes": mem.HeapAlloc, "jobsRun": a.jobs.Load()})
		if err != nil {
			slog.Warn("heartbeat failed", "error", err)
		} else {
			a.contact.Store(time.Now().Unix())
			if stop && !a.stopped.Load() {
				slog.Warn("emergency stop is on: all discovery halted")
			}
			a.stopped.Store(stop)
		}
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
	}
}

// mustHalt says the job has to stop now: the emergency stop is on, or the server has
// gone quiet and could no longer deliver it.
func (a *Agent) mustHalt() bool {
	return a.stopped.Load() || time.Since(time.Unix(a.contact.Load(), 0)) > contactTimeout
}

func has(methods []string, m string) bool {
	for _, x := range methods {
		if x == m {
			return true
		}
	}
	return false
}
