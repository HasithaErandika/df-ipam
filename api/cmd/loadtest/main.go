// Command loadtest measures the API under read load and prints latency percentiles.
// It only sends GET requests. Run it against a test system: it is not a safe thing to
// point at production. The token comes from LOADTEST_TOKEN, so it stays out of shell history.
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const defaultPaths = "/dashboard/summary,/subnets?pageSize=50,/ip-addresses?pageSize=50,/devices?pageSize=50,/reconciliation/results?pageSize=50,/search?q=10.0"

func percentile(sorted []time.Duration, p float64) time.Duration {
	if len(sorted) == 0 {
		return 0
	}
	return sorted[min(int(float64(len(sorted))*p), len(sorted)-1)]
}

// tally counts results across the workers.
type tally struct {
	mu              sync.Mutex
	latency         map[string][]time.Duration
	failed, limited atomic.Int64
}

func (t *tally) observe(path string, status int, took time.Duration) {
	switch {
	case status == http.StatusTooManyRequests:
		t.limited.Add(1)
	case status != http.StatusOK:
		t.failed.Add(1)
	default:
		t.mu.Lock()
		t.latency[path] = append(t.latency[path], took)
		t.mu.Unlock()
	}
}

// reader sends requests to its share of the paths until the context ends.
type reader struct {
	client      *http.Client
	base, token string
	targets     []string
	tally       *tally
}

func (r reader) run(ctx context.Context, first int) {
	for i := first; ctx.Err() == nil; i++ {
		r.fetch(ctx, r.targets[i%len(r.targets)])
	}
}

func (r reader) fetch(ctx context.Context, path string) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, r.base+path, nil)
	if err != nil {
		r.tally.failed.Add(1)
		return
	}
	req.Header.Set("Authorization", "Bearer "+r.token)
	start := time.Now()
	res, err := r.client.Do(req)
	if err != nil {
		if ctx.Err() == nil {
			r.tally.failed.Add(1)
		}
		return
	}
	_, _ = io.Copy(io.Discard, res.Body)
	res.Body.Close()
	r.tally.observe(path, res.StatusCode, time.Since(start))
}

func report(targets []string, t *tally, duration time.Duration) {
	var total int
	fmt.Printf("%-44s %8s %9s %9s %9s\n", "path", "requests", "p50", "p95", "p99")
	for _, path := range targets {
		v := t.latency[path]
		slices.Sort(v)
		total += len(v)
		fmt.Printf("%-44s %8d %9s %9s %9s\n", path, len(v), percentile(v, .5).Round(time.Millisecond), percentile(v, .95).Round(time.Millisecond), percentile(v, .99).Round(time.Millisecond))
	}
	fmt.Printf("\n%d ok, %.1f requests a second, %d failed, %d rate limited\n", total, float64(total)/duration.Seconds(), t.failed.Load(), t.limited.Load())
}

func main() {
	base := flag.String("url", "http://127.0.0.1:8080/api/v1", "API base URL")
	paths := flag.String("paths", defaultPaths, "comma separated paths to read")
	workers := flag.Int("concurrency", 10, "parallel clients")
	duration := flag.Duration("duration", 30*time.Second, "how long to run")
	flag.Parse()
	token := os.Getenv("LOADTEST_TOKEN")
	if token == "" {
		fmt.Fprintln(os.Stderr, "LOADTEST_TOKEN is not set")
		os.Exit(1)
	}
	targets := strings.Split(*paths, ",")
	ctx, cancel := context.WithTimeout(context.Background(), *duration)
	defer cancel()
	t := &tally{latency: map[string][]time.Duration{}}
	r := reader{&http.Client{Timeout: 30 * time.Second}, *base, token, targets, t}
	var wg sync.WaitGroup
	for w := range *workers {
		wg.Go(func() { r.run(ctx, w) })
	}
	wg.Wait()
	report(targets, t, *duration)
	if t.failed.Load() > 0 {
		os.Exit(1)
	}
}
