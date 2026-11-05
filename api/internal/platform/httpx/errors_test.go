package httpx

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func parse(query string) Page {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodGet, "/?"+query, nil)
	return ParsePage(c)
}

func TestParsePage(t *testing.T) {
	for query, want := range map[string]Page{
		"":                       {1, 50},
		"page=3&pageSize=25":     {3, 25},
		"page=0&pageSize=0":      {1, 50},
		"page=-2&pageSize=-1":    {1, 50},
		"page=abc&pageSize=xyz":  {1, 50},
		"pageSize=5000":          {1, 200},
		"page=99999999999999999": {1_000_000, 50},
		"page=5000000":           {1_000_000, 50},
	} {
		if got := parse(query); got != want {
			t.Errorf("%q: got %+v want %+v", query, got, want)
		}
	}
	if got := (Page{3, 25}).Offset(); got != 50 {
		t.Fatal(got)
	}
}

func TestMeta(t *testing.T) {
	for _, c := range []struct {
		page  Page
		total int64
		want  PageMeta
	}{
		{Page{1, 50}, 0, PageMeta{1, 50, 0, 0, false, false}},
		{Page{1, 50}, 50, PageMeta{1, 50, 50, 1, false, false}},
		{Page{1, 50}, 51, PageMeta{1, 50, 51, 2, true, false}},
		{Page{2, 50}, 51, PageMeta{2, 50, 51, 2, false, true}},
		{Page{9, 50}, 51, PageMeta{9, 50, 51, 2, false, true}},
		{Page{1, 0}, 0, PageMeta{1, 0, 0, 0, false, false}},
	} {
		if got := c.page.Meta(c.total); got != c.want {
			t.Errorf("%+v total %d: got %+v", c.page, c.total, got)
		}
	}
}

func limited(t *testing.T, h gin.HandlerFunc, ip string) int {
	t.Helper()
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/x", nil)
	c.Request.RemoteAddr = ip + ":1234"
	h(c)
	if c.IsAborted() {
		return w.Code
	}
	return http.StatusOK
}

func TestClientKey(t *testing.T) {
	for in, want := range map[string]string{
		"10.0.0.1":             "10.0.0.1",
		"::ffff:10.0.0.1":      "10.0.0.1",
		"2001:db8:1:2:aaaa::1": "2001:db8:1:2::/64",
		"2001:db8:1:2:bbbb::9": "2001:db8:1:2::/64",
		"2001:db8:1:3::1":      "2001:db8:1:3::/64",
		"not an address":       "not an address",
	} {
		if got := ClientKey(in); got != want {
			t.Errorf("%s: got %s", in, got)
		}
	}
}

func TestRateLimitStopsOneClientOnly(t *testing.T) {
	h := RateLimit(0.001, 2)
	for range 2 {
		if got := limited(t, h, "10.0.0.1"); got != http.StatusOK {
			t.Fatalf("within the burst: %d", got)
		}
	}
	if got := limited(t, h, "10.0.0.1"); got != http.StatusTooManyRequests {
		t.Fatalf("over the limit: %d", got)
	}
	if got := limited(t, h, "10.0.0.2"); got != http.StatusOK {
		t.Fatalf("another client was limited: %d", got)
	}
}

// Rotating through one /64 must not escape the limit.
func TestRateLimitCountsAnIPv6NetworkAsOneClient(t *testing.T) {
	h := RateLimit(0.001, 1)
	if got := limited(t, h, "[2001:db8::1]"); got != http.StatusOK {
		t.Fatalf("first: %d", got)
	}
	if got := limited(t, h, "[2001:db8::2]"); got != http.StatusTooManyRequests {
		t.Fatalf("same /64: %d", got)
	}
}

func TestMaxInFlightShedsLoad(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	release, started := make(chan struct{}), make(chan struct{})
	r.Use(MaxInFlight(1))
	r.GET("/slow", func(*gin.Context) { close(started); <-release })
	r.GET("/health", func(c *gin.Context) { c.Status(http.StatusOK) })
	done := make(chan struct{})
	go func() {
		defer close(done)
		r.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/slow", nil))
	}()
	<-started
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/slow", nil))
	if w.Code != http.StatusServiceUnavailable || w.Header().Get("Retry-After") == "" {
		t.Fatalf("got %d", w.Code)
	}
	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/health", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("health was shed: %d", w.Code)
	}
	close(release)
	<-done
}
