package signin

import (
	"strings"
	"time"

	"dfipam/api/internal/auth"
)

// remember keeps a Microsoft sign-in attempt until its callback, clearing
// stale ones on the way. A client with perClient live attempts loses its own
// oldest; only when every slot is taken does the oldest of all go.
func (h *Handler) remember(a auth.LoginAttempt, client string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	oldest, oldestOwn, own := "", "", 0
	for k, v := range h.attempts {
		switch {
		case time.Since(v.Started) > attemptTTL:
			delete(h.attempts, k)
			continue
		case v.client == client:
			own++
			if oldestOwn == "" || v.Started.Before(h.attempts[oldestOwn].Started) {
				oldestOwn = k
			}
		}
		if oldest == "" || v.Started.Before(h.attempts[oldest].Started) {
			oldest = k
		}
	}
	if own >= perClient {
		delete(h.attempts, oldestOwn)
	} else if len(h.attempts) >= maxAttempts {
		delete(h.attempts, oldest)
	}
	h.attempts[a.State] = heldAttempt{LoginAttempt: a, client: client}
}

// take returns the attempt for state once, and only while it is fresh.
func (h *Handler) take(state string) (auth.LoginAttempt, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	a, ok := h.attempts[state]
	delete(h.attempts, state)
	return a.LoginAttempt, ok && time.Since(a.Started) <= attemptTTL
}

// safeReturn keeps the return address inside the app.
func safeReturn(p string) string {
	if !strings.HasPrefix(p, "/") || strings.HasPrefix(p, "//") || strings.HasPrefix(p, "/\\") || strings.HasPrefix(p, "/api/") {
		return "/"
	}
	return p
}
