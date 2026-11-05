package httpx

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log/slog"
	"net/http"
	"net/netip"
	"regexp"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/time/rate"
)

const correlationKey = "correlationId"

type metaKey struct{}

// Meta is request information needed outside the HTTP layer, such as for audit.
type Meta struct {
	CorrelationID string
	SourceIP      string
}

func MetaFrom(ctx context.Context) Meta {
	m, _ := ctx.Value(metaKey{}).(Meta)
	return m
}

var safeID = regexp.MustCompile(`^[A-Za-z0-9-]{8,64}$`)

// Correlation accepts a well-formed X-Correlation-ID or generates one.
func Correlation() gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.GetHeader("X-Correlation-ID")
		if !safeID.MatchString(id) {
			b := make([]byte, 16)
			_, _ = rand.Read(b)
			id = hex.EncodeToString(b)
		}
		c.Set(correlationKey, id)
		c.Header("X-Correlation-ID", id)
		ctx := context.WithValue(c.Request.Context(), metaKey{}, Meta{CorrelationID: id, SourceIP: c.ClientIP()})
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	}
}

// Logger writes one JSON line per request. It never logs headers or bodies.
func Logger() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()
		slog.Info("request", "method", c.Request.Method, "route", c.FullPath(), "status", c.Writer.Status(),
			"durationMs", time.Since(start).Milliseconds(), "correlationId", c.GetString(correlationKey))
	}
}

func Recovery() gin.HandlerFunc {
	return gin.CustomRecoveryWithWriter(nil, func(c *gin.Context, v any) {
		Fail(c, fmt.Errorf("panic: %v", v))
	})
}

func SecurityHeaders() gin.HandlerFunc {
	return func(c *gin.Context) {
		h := c.Writer.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'")
		h.Set("Cache-Control", "no-store")
		c.Next()
	}
}

func BodyLimit(maxBytes int64) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxBytes)
		c.Next()
	}
}

// CORS allows only the listed web origins. Preflight requests are answered here,
// before authentication, because browsers send them without a token.
func CORS(allowed []string) gin.HandlerFunc {
	origins := make(map[string]bool, len(allowed))
	for _, origin := range allowed {
		origins[origin] = true
	}
	return func(c *gin.Context) {
		origin := c.GetHeader("Origin")
		h := c.Writer.Header()
		h.Add("Vary", "Origin")
		if origins[origin] {
			h.Set("Access-Control-Allow-Origin", origin)
			h.Set("Access-Control-Expose-Headers", "X-Correlation-ID")
		}
		if c.Request.Method == http.MethodOptions && c.GetHeader("Access-Control-Request-Method") != "" {
			if origins[origin] {
				h.Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE")
				h.Set("Access-Control-Allow-Headers", "Authorization, Content-Type, If-Match, X-Correlation-ID")
				h.Set("Access-Control-Max-Age", "600")
			}
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}

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

// RateLimitBy applies a token bucket per key, such as a signed-in user.
func RateLimitBy(key func(*gin.Context) string, perSecond float64, burst int) gin.HandlerFunc {
	var mu sync.Mutex
	visitors := map[string]*visitor{}
	lastSweep := time.Now()
	sweep := func(now time.Time, idle time.Duration) {
		for k, v := range visitors {
			if now.Sub(v.seen) > idle {
				delete(visitors, k)
			}
		}
		lastSweep = now
	}
	return func(c *gin.Context) {
		now := time.Now()
		k := key(c)
		mu.Lock()
		if now.Sub(lastSweep) > time.Minute {
			sweep(now, 10*time.Minute)
		}
		v, ok := visitors[k]
		if !ok {
			if len(visitors) >= maxVisitors {
				sweep(now, time.Minute)
			}
			if len(visitors) >= maxVisitors {
				k = overflowKey
				v = visitors[k]
			}
			if v == nil {
				v = &visitor{limiter: rate.NewLimiter(rate.Limit(perSecond), burst)}
				visitors[k] = v
			}
		}
		v.seen = now
		allowed := v.limiter.Allow()
		mu.Unlock()
		if !allowed {
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
