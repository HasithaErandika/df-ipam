package agent

import (
	"net/netip"
	"time"
)

// The agent's own ceilings. They hold even if a job, the server or a stored policy
// asks for more, because this is the last point before packets reach the network.
const (
	hardMaxPPS         = 500
	hardMaxConcurrency = 64
	hardMaxRetries     = 5
	hardMaxTimeoutMS   = 30000
	maxDNSPerSecond    = 20 // internal resolvers serve everything else too

	// A silent server can't deliver the emergency stop, so the agent stops itself.
	heartbeatEvery = 15 * time.Second
	contactTimeout = 90 * time.Second
)

// limited returns the job's settings within the agent's ceilings. localMax is the
// operator's own limit for this agent, and 0 means none.
func limited(c JobConfig, localMax int) JobConfig {
	ceiling := hardMaxPPS
	if localMax > 0 {
		ceiling = min(ceiling, localMax)
	}
	c.MaxPPS = min(max(c.MaxPPS, 1), ceiling)
	c.MaxConcurrency = min(max(c.MaxConcurrency, 1), hardMaxConcurrency)
	c.Retries = min(max(c.Retries, 0), hardMaxRetries)
	c.TimeoutMS = min(max(c.TimeoutMS, 100), hardMaxTimeoutMS)
	return c
}

// probeable refuses addresses that reach many hosts or none: multicast fans out, and
// the others never belong on a sweep.
func probeable(a netip.Addr) bool {
	return !a.IsMulticast() && !a.IsLoopback() && !a.IsUnspecified() && !a.IsLinkLocalUnicast() && !a.IsLinkLocalMulticast()
}
