package allocation

import (
	"context"
	"fmt"
	"sync"
	"testing"
)

// These tests need committed rows, because concurrency only shows between
// separate connections. Each test builds its own organisation, subnet and users,
// then deletes everything it made, including its audit rows, through the
// privileged retention path. They need TEST_DATABASE_URL (the postgres role);
// the service itself runs as the app role when TEST_APP_DATABASE_URL is set.

// Forty automatic allocations, twenty at a time, on one subnet: every one gets
// a different address, with no gaps.
func TestConcurrentAutomaticAllocation(t *testing.T) {
	f := newFixture(t)
	subnet := f.subnet("10.250.0.0/24")
	requester, fulfiller := f.user("req"), f.user("ful")
	var ids []string
	for i := 0; i < 40; i++ {
		ids = append(ids, f.approved(TypeAllocation, subnet, requester.UserID, nil))
	}
	addresses := make([]string, len(ids))
	errs := make([]error, len(ids))
	var wg sync.WaitGroup
	sem := make(chan struct{}, 20)
	for i, id := range ids {
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			a, _, err := f.svc.Allocate(context.Background(), fulfiller, id)
			addresses[i], errs[i] = a.Address, err
		}()
	}
	wg.Wait()
	seen := map[string]bool{}
	for i, a := range addresses {
		if errs[i] != nil {
			t.Fatalf("allocation %d: %v", i, errs[i])
		}
		if seen[a] {
			t.Fatalf("address %s given twice", a)
		}
		seen[a] = true
	}
	for i := 1; i <= 40; i++ {
		if !seen[fmt.Sprintf("10.250.0.%d", i)] {
			t.Fatalf("10.250.0.%d was skipped: %v", i, addresses)
		}
	}
}

// More requests than addresses: the subnet fills, and the rest fail cleanly.
func TestConcurrentAllocationExhaustsCleanly(t *testing.T) {
	f := newFixture(t)
	subnet := f.subnet("10.250.1.0/29") // six usable addresses
	requester, fulfiller := f.user("req"), f.user("ful")
	var ids []string
	for i := 0; i < 10; i++ {
		ids = append(ids, f.approved(TypeAllocation, subnet, requester.UserID, nil))
	}
	codes := make([]string, len(ids))
	var wg sync.WaitGroup
	for i, id := range ids {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _, err := f.svc.Allocate(context.Background(), fulfiller, id)
			codes[i] = errorCode(err)
		}()
	}
	wg.Wait()
	ok, exhausted := 0, 0
	for _, c := range codes {
		switch c {
		case "":
			ok++
		case "SUBNET_EXHAUSTED":
			exhausted++
		default:
			t.Fatalf("unexpected result %q", c)
		}
	}
	if ok != 6 || exhausted != 4 {
		t.Fatalf("got %d allocated, %d exhausted", ok, exhausted)
	}
}
