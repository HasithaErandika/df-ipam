package httpx

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log/slog"
	"net/http"
	"regexp"
	"time"

	"github.com/gin-gonic/gin"
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
