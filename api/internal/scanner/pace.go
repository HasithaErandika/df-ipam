package scanner

import (
	"context"
	"sync"
	"time"
)

// DefaultRate is how many addresses a second discovery and server checks
// probe together, when the setting is missing. Steady, never in bursts.
const DefaultRate = 30

// pace spaces probes evenly: one per interval, shared by the sweep and the
// server checks, so together they never go over the rate. When the network
// starts refusing packets it halves the rate until the next scan.
type pace struct {
	mu     sync.Mutex
	every  time.Duration
	next   time.Time
	slowed bool
}

func newPace(perSecond int) *pace {
	p := &pace{}
	p.reset(perSecond)
	return p
}

// reset sets the rate for a new scan and forgets an earlier slow-down.
func (p *pace) reset(perSecond int) {
	if perSecond <= 0 {
		perSecond = DefaultRate
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.every, p.slowed = time.Second/time.Duration(perSecond), false
}

// wait blocks until this caller's turn. It reports false when ctx ends first.
func (p *pace) wait(ctx context.Context) bool {
	p.mu.Lock()
	now := time.Now()
	if p.next.Before(now) {
		p.next = now
	}
	at := p.next
	p.next = p.next.Add(p.every)
	p.mu.Unlock()
	timer := time.NewTimer(time.Until(at))
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

// slowDown halves the rate once per scan. It reports whether it did.
func (p *pace) slowDown() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.slowed {
		return false
	}
	p.every, p.slowed = 2*p.every, true
	return true
}

// perSecond is the current rate, for the status line.
func (p *pace) perSecond() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return int(time.Second / p.every)
}
