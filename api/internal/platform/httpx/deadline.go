package httpx

import (
	"context"
	"time"

	"github.com/gin-gonic/gin"
)

// Deadline gives every request a time limit shorter than the server's write
// timeout. With the engine's ContextWithFallback on, database work done with
// the gin context stops at the limit, or when the client goes away, and its
// transaction rolls back: a slow import can't commit after the person has
// been told it failed.
func Deadline(limit time.Duration) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), limit)
		defer cancel()
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	}
}
