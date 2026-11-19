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
			for ip := range jobs {
				if !plan.pace.wait(ctx) {
					return
				}
				if found, ok := identify(ctx, plan, ip); ok {
					results <- found
				}
			}
		}()
	}
	go func() {
		defer close(jobs)
		for _, ip := range hosts {
			select {
			case <-ctx.Done():
				return
			case jobs <- ip:
			}
		}
	}()
	go func() { workers.Wait(); close(results) }()
	found := make([]sighting, 0)
	for item := range results {
		found = append(found, item)
	}
	return found
}
