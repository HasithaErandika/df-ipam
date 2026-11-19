package scanner

import (
	"context"
	"errors"
	"testing"
)

func TestStopEndsOnlyARunningScan(t *testing.T) {
	s := New(nil)
	if err := s.Stop(); !errors.Is(err, ErrNotRunning) {
		t.Fatalf("stop with no scan running: %v", err)
	}
	ctx := context.Background()
	run, end := s.begin(ctx, "manual")
	if err := s.Stop(); err != nil {
		t.Fatalf("stop a running scan: %v", err)
	}
	if run.Err() == nil {
		t.Fatal("stop must cancel the scan's sweeps")
	}
	if ctx.Err() != nil {
		t.Fatal("stop must not cancel the scanner itself")
	}
	if st := s.Snapshot(); !st.Running || !st.Stopped {
		t.Fatalf("until it winds down the scan is running and marked stopped: %+v", st)
	}
	end()
	if st := s.Snapshot(); st.Running || !st.Stopped || st.FinishedAt == nil {
		t.Fatalf("after it ends: %+v", st)
	}
	if err := s.Stop(); !errors.Is(err, ErrNotRunning) {
		t.Fatalf("stop after the scan ended: %v", err)
	}
	_, end = s.begin(ctx, "schedule")
	defer end()
	if s.Snapshot().Stopped {
		t.Fatal("a new scan clears the stopped mark")
	}
}

// A segment cut short by a stop is shown as stopped, saves nothing, and isn't
// counted as a failed scan.
func TestSweepOneReportsAStoppedSegment(t *testing.T) {
	s := New(nil)
	ctx := context.Background()
	run, end := s.begin(ctx, "manual")
	defer end()
	_ = s.Stop()
	if errs := s.sweepOne(ctx, run, segment{id: "a", name: "Floor 1", network: "192.168.1.0/24"}, true); len(errs) != 0 {
		t.Fatalf("a stopped segment is not an error: %v", errs)
	}
	got := s.Snapshot().Segments
	if len(got) != 1 || got[0].Error != stoppedText || got[0].Found != 0 {
		t.Fatalf("stopped segment result: %+v", got)
	}
}
