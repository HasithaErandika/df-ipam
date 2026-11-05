package httpx

import (
	"net/http"
	"net/netip"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/time/rate"
)

type visitor struct {
	limiter *rate.Limiter
	seen    time.Time
}

// maxVisitors bounds the limiter's memory during a flood from many addresses.
const maxVisitors = 100_000

// overflowKey is the bucket everyone shares once the table is full.
const overflowKey = "overflow"

func tooMany(c *gin.Context) {
	c.Header("Retry-After", "1")
	Fail(c, NewError(http.StatusTooManyRequests, "RATE_LIMITED", "Too many requests. Try again shortly."))
}

// ClientKey identifies a client for limiting. An IPv6 client usually controls a whole
// /64, so counting addresses one by one would let it step around any limit.
func ClientKey(ip string) string {
	a, err := netip.ParseAddr(ip)
	if err != nil {
		return ip
	}
	a = a.Unmap()
	if a.Is6() {
		if p, err := a.WithZone("").Prefix(64); err == nil {
			return p.String()
		}
	}
	return a.String()
}

// RateLimit applies a token bucket per client address.
func RateLimit(perSecond float64, burst int) gin.HandlerFunc {
	return RateLimitBy(func(c *gin.Context) string { return ClientKey(c.ClientIP()) }, perSecond, burst)
}

// visitorTable holds one rate limiter per key, within a memory limit.
type visitorTable struct {
	mu        sync.Mutex
	visitors  map[string]*visitor
	lastSweep time.Time
	limit     rate.Limit
	burst     int
}

func (t *visitorTable) sweep(now time.Time, idle time.Duration) {
	for k, v := range t.visitors {
		if now.Sub(v.seen) > idle {
			delete(t.visitors, k)
		}
	}
	t.lastSweep = now
}

// lookup finds or adds a key's limiter. Once the table is full, new keys share one
// limiter, so a flood of addresses can't use up memory.
func (t *visitorTable) lookup(k string, now time.Time) *visitor {
	if v, ok := t.visitors[k]; ok {
		return v
	}
	if len(t.visitors) >= maxVisitors {
		t.sweep(now, time.Minute)
	}
	if len(t.visitors) >= maxVisitors {
		k = overflowKey
	}
	v := t.visitors[k]
	if v == nil {
		v = &visitor{limiter: rate.NewLimiter(t.limit, t.burst)}
		t.visitors[k] = v
	}
	return v
}

func (t *visitorTable) allow(k string, now time.Time) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	if now.Sub(t.lastSweep) > time.Minute {
		t.sweep(now, 10*time.Minute)
	}
	v := t.lookup(k, now)
	v.seen = now
	return v.limiter.Allow()
}

// RateLimitBy applies a token bucket per key, such as a signed-in user.
func RateLimitBy(key func(*gin.Context) string, perSecond float64, burst int) gin.HandlerFunc {
	table := &visitorTable{visitors: map[string]*visitor{}, lastSweep: time.Now(), limit: rate.Limit(perSecond), burst: burst}
	return func(c *gin.Context) {
		if !table.allow(key(c), time.Now()) {
			tooMany(c)
			return
		}
		c.Next()
	}
}

// MaxInFlight sheds load when too many requests run at once, so a flood from many
// addresses can't use up every database connection. It refuses at once rather than
// queueing, because a queue only makes everyone slower. Health checks always pass.
func MaxInFlight(limit int) gin.HandlerFunc {
	slots := make(chan struct{}, limit)
	return func(c *gin.Context) {
		if p := c.Request.URL.Path; p == "/health" || p == "/ready" {
			c.Next()
			return
		}
		select {
		case slots <- struct{}{}:
			defer func() { <-slots }()
			c.Next()
		default:
			c.Header("Retry-After", "2")
			Fail(c, NewError(http.StatusServiceUnavailable, "SERVER_BUSY", "The server is busy. Try again shortly."))
		}
	}
}
