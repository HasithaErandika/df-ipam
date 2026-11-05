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
	client := &http.Client{Timeout: 30 * time.Second}

	var mu sync.Mutex
	latency := map[string][]time.Duration{}
	var failed, limited atomic.Int64
	var wg sync.WaitGroup
	for w := range *workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := w; ctx.Err() == nil; i++ {
				path := targets[i%len(targets)]
				req, err := http.NewRequestWithContext(ctx, http.MethodGet, *base+path, nil)
				if err != nil {
					failed.Add(1)
					continue
				}
				req.Header.Set("Authorization", "Bearer "+token)
				start := time.Now()
				res, err := client.Do(req)
				if err != nil {
					if ctx.Err() == nil {
						failed.Add(1)
					}
					continue
				}
				_, _ = io.Copy(io.Discard, res.Body)
				res.Body.Close()
				switch {
				case res.StatusCode == http.StatusTooManyRequests:
					limited.Add(1)
				case res.StatusCode != http.StatusOK:
					failed.Add(1)
				default:
					mu.Lock()
					latency[path] = append(latency[path], time.Since(start))
					mu.Unlock()
				}
			}
		}()
	}
	wg.Wait()

	var total int
	fmt.Printf("%-44s %8s %9s %9s %9s\n", "path", "requests", "p50", "p95", "p99")
	for _, path := range targets {
		v := latency[path]
		slices.Sort(v)
		total += len(v)
		fmt.Printf("%-44s %8d %9s %9s %9s\n", path, len(v), percentile(v, .5).Round(time.Millisecond), percentile(v, .95).Round(time.Millisecond), percentile(v, .99).Round(time.Millisecond))
	}
	fmt.Printf("\n%d ok, %.1f requests a second, %d failed, %d rate limited\n", total, float64(total)/duration.Seconds(), failed.Load(), limited.Load())
	if failed.Load() > 0 {
		os.Exit(1)
	}
}
