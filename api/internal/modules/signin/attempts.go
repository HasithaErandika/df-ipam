package signin

import (
	"strings"
	"time"

	"dfipam/api/internal/auth"
)

// remember keeps a Microsoft sign-in attempt until its callback, clearing
// stale ones on the way.
func (h *Handler) remember(a auth.LoginAttempt) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for k, v := range h.attempts {
		if time.Since(v.Started) > attemptTTL || len(h.attempts) >= maxAttempts {
			delete(h.attempts, k)
		}
	}
	h.attempts[a.State] = a
}

// take returns the attempt for state once, and only while it is fresh.
func (h *Handler) take(state string) (auth.LoginAttempt, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	a, ok := h.attempts[state]
	delete(h.attempts, state)
	return a, ok && time.Since(a.Started) <= attemptTTL
}

// safeReturn keeps the return address inside the app.
func safeReturn(p string) string {
	if !strings.HasPrefix(p, "/") || strings.HasPrefix(p, "//") || strings.HasPrefix(p, "/\\") || strings.HasPrefix(p, "/api/") {
		return "/"
	}
	return p
}
