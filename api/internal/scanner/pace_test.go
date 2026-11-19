package scanner

import (
	"context"
	"testing"
	"time"
)

func TestPaceSpacesProbesAndSlowsDownOnce(t *testing.T) {
	p := newPace(100) // one every 10 ms
	start := time.Now()
	for range 6 {
		p.wait(context.Background())
	}
	if took := time.Since(start); took < 45*time.Millisecond {
		t.Fatalf("6 probes at 100 a second took %v: not paced", took)
	}
	if !p.slowDown() || p.perSecond() != 50 {
		t.Fatalf("slowDown should halve the rate, got %d a second", p.perSecond())
	}
	if p.slowDown() || p.perSecond() != 50 {
		t.Fatal("the rate halves only once per scan")
	}
	p.reset(30)
	if p.perSecond() != 30 || !p.slowDown() {
		t.Fatal("reset sets the rate and allows a new slow-down")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if p.wait(ctx) && p.wait(ctx) {
		t.Fatal("a cancelled scan must stop waiting")
	}
}
