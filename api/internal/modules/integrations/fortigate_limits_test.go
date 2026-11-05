package integrations

import (
	"context"
	"dfipam/api/internal/platform/safehttp"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"golang.org/x/time/rate"
)

func TestFortiGateRetriesAreBounded(t *testing.T) {
	s, f := newFortiServer(t)
	var calls int
	s.override["/api/v2/monitor/system/status"] = func(w http.ResponseWriter) { calls++; w.WriteHeader(http.StatusTooManyRequests) }
	if _, err := f.Collect(context.Background()); err == nil {
		t.Fatal("expected a failure")
	}
	if calls != fortiRetries {
		t.Fatalf("%d calls", calls)
	}
}

func TestFortiGatePagination(t *testing.T) {
	var starts []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		starts = append(starts, r.URL.Query().Get("start"))
		n := fortiPageSize
		if r.URL.Query().Get("start") != "0" {
			n = 3
		}
		items := make([]map[string]string, n)
		for i := range items {
			items[i] = map[string]string{"name": "x"}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"results": items})
	}))
	defer srv.Close()
	base, _ := url.Parse(srv.URL)
	f := &fortiGate{base, "t", srv.Client(), rate.NewLimiter(rate.Inf, 1), time.Millisecond}
	out, err := fortiList[fortiGroup](context.Background(), f, "/x", "root")
	if err != nil || len(out) != fortiPageSize+3 || len(starts) != 2 {
		t.Fatalf("%d items, %v, %v", len(out), starts, err)
	}
}

func TestErrorsNeverCarryTheToken(t *testing.T) {
	s, f := newFortiServer(t)
	s.override["/api/v2/monitor/system/status"] = func(w http.ResponseWriter) { w.WriteHeader(http.StatusInternalServerError) }
	_, err := f.Collect(context.Background())
	if err == nil || strings.Contains(err.Error(), "secret-token") {
		t.Fatalf("got %v", err)
	}
	f.token = "wrong"
	if err := f.Test(context.Background()); err == nil || strings.Contains(err.Error(), "wrong") {
		t.Fatalf("got %v", err)
	}
}

func TestNewFortiGateAppliesTheOutboundPolicy(t *testing.T) {
	c := Connection{BaseURL: "https://169.254.169.254"}
	if _, err := newFortiGate(c, nil, safehttp.Policy{}); !errors.Is(err, safehttp.ErrBlocked) {
		t.Fatalf("got %v", err)
	}
	if _, err := newVCenter(c, nil, safehttp.Policy{}); !errors.Is(err, safehttp.ErrBlocked) {
		t.Fatalf("got %v", err)
	}
}

func TestSplitIPMask(t *testing.T) {
	a, p, ok := splitIPMask("10.1.2.3 255.255.255.192")
	if !ok || a.String() != "10.1.2.3" || p.Masked().String() != "10.1.2.0/26" {
		t.Fatal(a, p, ok)
	}
	for _, bad := range []string{"", "10.1.2.3", "x 255.0.0.0", "10.0.0.1 255.0.255.0", "10.0.0.1 nonsense"} {
		if _, _, ok := splitIPMask(bad); ok {
			t.Errorf("%q accepted", bad)
		}
	}
}
