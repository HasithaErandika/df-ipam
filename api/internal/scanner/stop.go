package scanner

import (
	"context"
	"errors"
	"time"
)

// ErrNotRunning means there is no scan to stop.
var ErrNotRunning = errors.New("no scan is running")

// stoppedText is the result shown for the segment a stop cut short.
const stoppedText = "Stopped before it finished. Nothing from this segment was saved."

// Stop ends the running scan. Segments already swept keep their results, the one
// in progress saves nothing, and the alert rules still run on what was saved.
// Server checks are separate and carry on.
func (s *Scanner) Stop() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.status.Running || s.cancel == nil {
		return ErrNotRunning
	}
	s.cancel()
	s.status.Stopped = true
	return nil
}

// begin marks a scan as running and returns the context its sweeps use, which
// Stop cancels, and the function that marks it finished.
func (s *Scanner) begin(ctx context.Context, by string) (context.Context, func()) {
	run, cancel := context.WithCancel(ctx)
	started := time.Now()
	s.mu.Lock()
	s.status.Running, s.status.Stopped, s.status.Trigger, s.status.StartedAt, s.status.NextScanAt = true, false, by, &started, nil
	s.cancel = cancel
	s.mu.Unlock()
	return run, func() {
		cancel()
		done := time.Now()
		s.mu.Lock()
		s.status.Running, s.status.FinishedAt, s.cancel = false, &done, nil
		s.mu.Unlock()
	}
}
