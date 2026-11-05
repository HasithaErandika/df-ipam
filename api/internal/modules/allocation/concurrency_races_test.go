package allocation

import (
	"context"
	"dfipam/api/internal/auth"
	"sync"
	"testing"
)

// Allocations and reservations racing for one chosen address: exactly one wins,
// and the address is never both allocated and reserved.
func TestConcurrentAllocationAndReservationOfOneAddress(t *testing.T) {
	f := newFixture(t)
	subnet := f.subnet("10.250.2.0/24")
	requester, fulfiller := f.user("req"), f.user("ful")
	addr := "10.250.2.77"
	type job struct{ kind, id string }
	var jobs []job
	for i := 0; i < 10; i++ {
		jobs = append(jobs, job{TypeAllocation, f.approved(TypeAllocation, subnet, requester.UserID, &addr)})
		jobs = append(jobs, job{TypeReservation, f.approved(TypeReservation, subnet, requester.UserID, &addr)})
	}
	codes := make([]string, len(jobs))
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i, j := range jobs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			var err error
			if j.kind == TypeAllocation {
				_, _, err = f.svc.Allocate(context.Background(), fulfiller, j.id)
			} else {
				_, _, err = f.svc.Reserve(context.Background(), fulfiller, j.id)
			}
			codes[i] = errorCode(err)
		}()
	}
	close(start)
	wg.Wait()
	wins := 0
	for _, c := range codes {
		switch c {
		case "":
			wins++
		case "ADDRESS_UNAVAILABLE":
		default:
			t.Fatalf("unexpected result %q", c)
		}
	}
	if wins != 1 {
		t.Fatalf("%d claims won, want exactly 1: %v", wins, codes)
	}
	var allocations, reservations int
	f.row(&allocations, `SELECT count(*) FROM ip_allocations WHERE subnet_id = $1::uuid AND status = 'active'`, subnet)
	f.row(&reservations, `SELECT count(*) FROM ip_reservations WHERE subnet_id = $1::uuid AND status = 'active'`, subnet)
	if allocations+reservations != 1 {
		t.Fatalf("%d active allocations and %d active reservations", allocations, reservations)
	}
}

// Two approvers deciding the same step at once: one decision counts.
func TestConcurrentApprovalOfOneStep(t *testing.T) {
	f := newFixture(t)
	subnet := f.subnet("10.250.3.0/24")
	requester, a1, a2 := f.user("req"), f.user("a1"), f.user("a2")
	var request, step string
	f.row(&request, `INSERT INTO ip_requests (request_type, subnet_id, justification, status, requested_by, submitted_at)
		VALUES ('allocation', $1::uuid, 'test', 'pending_approval', $2::uuid, now()) RETURNING id::text`, subnet, requester.UserID)
	f.row(&step, `INSERT INTO ip_request_approvals (request_id, step_order) VALUES ($1::uuid, 1) RETURNING id::text`, request)
	codes := make([]string, 2)
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i, who := range []*auth.Principal{a1, a2} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_, err := f.svc.Decide(context.Background(), who, step, i == 0, "racing")
			codes[i] = errorCode(err)
		}()
	}
	close(start)
	wg.Wait()
	if (codes[0] == "") == (codes[1] == "") || (codes[0] != "" && codes[0] != "APPROVAL_NOT_CURRENT") || (codes[1] != "" && codes[1] != "APPROVAL_NOT_CURRENT") {
		t.Fatalf("want one decision and one APPROVAL_NOT_CURRENT, got %q", codes)
	}
	var decided int
	f.row(&decided, `SELECT count(*) FROM ip_request_approvals WHERE request_id = $1::uuid AND decision <> 'pending'`, request)
	if decided != 1 {
		t.Fatalf("%d decisions recorded", decided)
	}
}

// Repeating a fulfilment at the same moment, as a retried HTTP call would,
// produces one allocation.
func TestConcurrentRepeatedFulfilment(t *testing.T) {
	f := newFixture(t)
	subnet := f.subnet("10.250.4.0/24")
	requester, fulfiller := f.user("req"), f.user("ful")
	id := f.approved(TypeAllocation, subnet, requester.UserID, nil)
	results := make([]string, 8)
	var wg sync.WaitGroup
	for i := range results {
		wg.Add(1)
		go func() {
			defer wg.Done()
			a, _, err := f.svc.Allocate(context.Background(), fulfiller, id)
			if err != nil {
				results[i] = "error: " + err.Error()
				return
			}
			results[i] = a.ID
		}()
	}
	wg.Wait()
	for _, r := range results[1:] {
		if r != results[0] {
			t.Fatalf("repeats disagree: %v", results)
		}
	}
	var n int
	f.row(&n, `SELECT count(*) FROM ip_allocations WHERE request_id = $1::uuid`, id)
	if n != 1 {
		t.Fatalf("%d allocations for one request", n)
	}
}
