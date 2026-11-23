package scanner

import (
	"context"
	"net/netip"
	"sync"
)

// sweep probes every host with 32 workers, each probe waiting its turn on
// plan.pace: a steady rate shared with the server checks, never a burst.
func sweep(ctx context.Context, plan probePlan, hosts []netip.Addr) []sighting {
	results := make(chan sighting, 64)
	jobs := make(chan netip.Addr)
	var workers sync.WaitGroup
	for range 32 {
		workers.Add(1)
		go func() {
			defer workers.Done()
			probeAll(ctx, plan, jobs, results)
		}()
	}
	go feed(ctx, jobs, hosts)
	go func() { workers.Wait(); close(results) }()
	found := make([]sighting, 0)
	for item := range results {
		found = append(found, item)
	}
	return found
}

// probeAll identifies each host from jobs until jobs closes or ctx ends.
func probeAll(ctx context.Context, plan probePlan, jobs <-chan netip.Addr, results chan<- sighting) {
	for ip := range jobs {
		if !plan.pace.wait(ctx) {
			return
		}
		if found, ok := identify(ctx, plan, ip); ok {
			results <- found
		}
	}
}

// feed hands out the hosts one by one and stops early when ctx ends.
func feed(ctx context.Context, jobs chan<- netip.Addr, hosts []netip.Addr) {
	defer close(jobs)
	for _, ip := range hosts {
		select {
		case <-ctx.Done():
			return
		case jobs <- ip:
		}
	}
}
