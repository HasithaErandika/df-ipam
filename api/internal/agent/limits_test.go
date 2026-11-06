package agent

import (
	"net/netip"
	"testing"
	"time"
)

func TestLimitedHoldsEveryCeiling(t *testing.T) {
	got := limited(JobConfig{MaxPPS: 100000, MaxConcurrency: 100000, Retries: 99, TimeoutMS: 1 << 30}, 0)
	if got.MaxPPS != hardMaxPPS || got.MaxConcurrency != hardMaxConcurrency || got.Retries != hardMaxRetries || got.TimeoutMS != hardMaxTimeoutMS {
		t.Fatalf("%+v", got)
	}
	if got := limited(JobConfig{MaxPPS: 400}, 25); got.MaxPPS != 25 {
		t.Fatalf("the local limit must win: %d", got.MaxPPS)
	}
	if got := limited(JobConfig{MaxPPS: -5, MaxConcurrency: 0, Retries: -1}, 0); got.MaxPPS != 1 || got.MaxConcurrency != 1 || got.Retries != 0 {
		t.Fatalf("bad values must become the gentlest: %+v", got)
	}
}

func TestHostsSkipsWhatMustNotBeProbed(t *testing.T) {
	if hosts, err := Hosts("224.0.0.0/28"); err != nil || len(hosts) != 0 {
		t.Fatalf("multicast: %v %v", hosts, err)
	}
	if hosts, err := Hosts("127.0.0.0/30"); err != nil || len(hosts) != 0 {
		t.Fatalf("loopback: %v %v", hosts, err)
	}
	if _, err := Hosts("10.0.0.0/16"); err == nil {
		t.Fatal("a target past the ceiling must be refused")
	}
	if hosts, _ := Hosts("10.0.0.0/30"); len(hosts) != 2 {
		t.Fatalf("ordinary subnet: %v", hosts)
	}
	if !probeable(netip.MustParseAddr("10.1.2.3")) {
		t.Fatal("ordinary address")
	}
}

// Retries used to skip the limiter, so dead hosts multiplied the packet rate.
func TestRetriesWaitForTheLimiter(t *testing.T) {
	lim := newLimiter(50)
	defer lim.stop()
	start := time.Now()
	for range 5 {
		if err := lim.wait(t.Context()); err != nil {
			t.Fatal(err)
		}
	}
	if took := time.Since(start); took < 70*time.Millisecond {
		t.Fatalf("5 packets at 50 a second took only %s", took)
	}
}
