package scanner

import (
	"context"
	"net/netip"
	"sync"
	"time"
)

// sweep probes every host, at most 100 a second with 32 workers.
func sweep(ctx context.Context, plan probePlan, hosts []netip.Addr) []sighting {
	results := make(chan sighting, 64)
	jobs := make(chan netip.Addr)
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	var workers sync.WaitGroup
	for range 32 {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for ip := range jobs {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
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
