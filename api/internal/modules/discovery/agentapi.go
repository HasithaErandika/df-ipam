package discovery

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"strings"
	"time"

	"dfipam/api/internal/audit"
	"dfipam/api/internal/platform/db"
	"dfipam/api/internal/platform/httpx"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
)

const agentKey = "agentId"

// MaxObservations bounds one results call.
const MaxObservations = 5000

// RegisterAgentAPI adds the routes agents call. They authenticate with their
// own credential, not a person's sign-in. It returns the registered routes,
// so the route-guard test can account for them.
func (h *Handler) RegisterAgentAPI(g *gin.RouterGroup) map[string]bool {
	keys := map[string]bool{}
	add := func(method, path string, handlers ...gin.HandlerFunc) {
		keys[method+" "+strings.TrimRight(g.BasePath(), "/")+path] = true
		g.Handle(method, path, handlers...)
	}
	// One-time tokens are guessed here if anywhere, so this is far stricter than the rest.
	add(http.MethodPost, "/register", httpx.RateLimit(0.2, 5), h.register)
	add(http.MethodPost, "/heartbeat", h.agentAuth, h.heartbeat)
	add(http.MethodGet, "/jobs", h.agentAuth, h.claim)
	add(http.MethodPost, "/jobs/:id/results", h.agentAuth, h.results)
	return keys
}

var errAgentAuth = httpx.NewError(http.StatusUnauthorized, "UNAUTHENTICATED", "Agent credential missing or not valid.")

// agentAuth checks "Authorization: Agent <agent id>.<secret>" against the stored hash.
func (h *Handler) agentAuth(c *gin.Context) {
	scheme, value, ok := strings.Cut(c.GetHeader("Authorization"), " ")
	id, secret, ok2 := strings.Cut(value, ".")
	if !ok || !ok2 || !strings.EqualFold(scheme, "Agent") || !httpx.IsUUID(id) || secret == "" {
		httpx.Fail(c, errAgentAuth)
		return
	}
	var hash *string
	var status string
	err := h.svc.conn.QueryRow(c.Request.Context(), `SELECT credential_hash, status FROM discovery_agents WHERE id = $1::uuid`, id).Scan(&hash, &status)
	if err != nil || hash == nil || status == "revoked" || subtle.ConstantTimeCompare([]byte(*hash), []byte(Hash(secret))) != 1 {
		httpx.Fail(c, errAgentAuth)
		return
	}
	c.Set(agentKey, id)
	c.Next()
}

// register trades a single-use registration token for a credential.
func (h *Handler) register(c *gin.Context) {
	var req struct {
		Token   string `json:"token"`
		Version string `json:"version"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.Token == "" {
		httpx.Fail(c, errAgentAuth)
		return
	}
	ctx := c.Request.Context()
	var agentID, credential string
	err := db.InTx(ctx, h.svc.conn, func(tx pgx.Tx) error {
		var tokenID string
		err := tx.QueryRow(ctx, `SELECT t.id::text, t.agent_id::text FROM agent_registration_tokens t JOIN discovery_agents a ON a.id = t.agent_id
			WHERE t.token_hash = $1 AND t.used_at IS NULL AND t.expires_at > now() AND a.status <> 'revoked' FOR UPDATE OF t`, Hash(req.Token)).Scan(&tokenID, &agentID)
		if errors.Is(err, pgx.ErrNoRows) {
			return errAgentAuth
		}
		if err != nil {
			return err
		}
		secret, hash := NewSecret()
		credential = agentID + "." + secret
		if _, err := tx.Exec(ctx, `UPDATE agent_registration_tokens SET used_at = now() WHERE id = $1::uuid`, tokenID); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE discovery_agents SET credential_hash = $2, credential_rotated_at = now(), status = 'active', version = $3,
			last_heartbeat_at = now() WHERE id = $1::uuid`, agentID, hash, truncate(req.Version, 64)); err != nil {
			return err
		}
		return audit.Write(ctx, tx, audit.Entry{ActorType: "agent", Action: "discovery_agent.register", ObjectType: "discovery_agent", ObjectID: agentID,
			New: map[string]string{"version": req.Version}})
	})
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, http.StatusOK, gin.H{"agentId": agentID, "credential": credential})
}

func truncate(s string, n int) string {
	if len(s) > n {
		return s[:n]
	}
	return s
}

func (h *Handler) heartbeat(c *gin.Context) {
	var req struct {
		Version string          `json:"version"`
		Metrics json.RawMessage `json:"metrics"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Fail(c, httpx.ErrBadRequest)
		return
	}
	if len(req.Metrics) == 0 || len(req.Metrics) > 8192 || !json.Valid(req.Metrics) {
		req.Metrics = json.RawMessage(`{}`)
	}
	ctx, id := c.Request.Context(), c.GetString(agentKey)
	stopped := false
	err := db.InTx(ctx, h.svc.conn, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `INSERT INTO agent_heartbeats (agent_id, version, metrics) VALUES ($1::uuid, $2, $3::jsonb)`, id, truncate(req.Version, 64), req.Metrics); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE discovery_agents SET last_heartbeat_at = now(), version = $2, status = 'active' WHERE id = $1::uuid AND status <> 'revoked'`, id, truncate(req.Version, 64)); err != nil {
			return err
		}
		var err error
		stopped, err = Stopped(ctx, tx)
		return err
	})
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, http.StatusOK, gin.H{"emergencyStop": stopped, "serverTime": time.Now().UTC()})
}

type claimedTarget struct {
	ID              string  `json:"id"`
	RoutingDomainID string  `json:"routingDomainId"`
	CIDR            string  `json:"cidr"`
	Checkpoint      *string `json:"checkpoint"`
	Status          string  `json:"status"`
}

// defaultMaxTotalPPS caps all discovery running at once, in packets per second. Each
// policy has its own limit, but several agents together could otherwise add up to a
// flood on the network this system is meant to protect.
const defaultMaxTotalPPS = 500

// fitsBudget says whether a job can start. A job alone on the network always may,
// because its policy was approved at that rate, and it would otherwise never run.
func fitsBudget(running, jobPPS, ceiling int) bool {
	return running == 0 || running+jobPPS <= ceiling
}

// trafficBudget reads the ceiling, and what other agents use now. Agents that have
// gone silent don't count, so a crashed agent can't block discovery for good.
func trafficBudget(ctx context.Context, q querier, agent string) (ceiling, running int, err error) {
	err = q.QueryRow(ctx, `SELECT
		  COALESCE((SELECT (value #>> '{}')::int FROM system_settings WHERE key = 'discovery.max_total_pps'), $2),
		  COALESCE((SELECT sum((j.config_snapshot->>'maxPps')::int) FROM discovery_jobs j JOIN discovery_agents a ON a.id = j.agent_id
		            WHERE j.status = 'running' AND a.id <> $1::uuid AND a.last_heartbeat_at > now() - interval '5 minutes'), 0)`,
		agent, defaultMaxTotalPPS).Scan(&ceiling, &running)
	return ceiling, running, err
}

// claim hands the agent its next job, if discovery may run now. Jobs pinned to
// another agent, or outside their policy's windows, are left alone.
func (h *Handler) claim(c *gin.Context) {
	ctx, agent := c.Request.Context(), c.GetString(agentKey)
	var out *gin.H
	err := db.InTx(ctx, h.svc.conn, func(tx pgx.Tx) error {
		if on, err := Stopped(ctx, tx); err != nil || on {
			return err
		}
		// One claim at a time, so two agents can't both pass the checks below.
		if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended('discovery-claim', 7))`); err != nil {
			return err
		}
		// A job this agent already runs is handed back first, so a restart resumes it.
		rows, err := tx.Query(ctx, `SELECT id::text, config_snapshot FROM discovery_jobs
			WHERE (status = 'running' AND agent_id = $1::uuid) OR (status = 'queued' AND (agent_id IS NULL OR agent_id = $1::uuid))
			ORDER BY (status = 'running') DESC, priority, created_at LIMIT 20 FOR UPDATE SKIP LOCKED`, agent)
		if err != nil {
			return err
		}
		type candidate struct {
			id   string
			snap Snapshot
		}
		var list []candidate
		for rows.Next() {
			var cd candidate
			var raw []byte
			if err := rows.Scan(&cd.id, &raw); err != nil {
				rows.Close()
				return err
			}
			_ = json.Unmarshal(raw, &cd.snap)
			list = append(list, cd)
		}
		rows.Close()
		loc, _ := time.LoadLocation("Asia/Colombo")
		now := time.Now().In(loc)
		ceiling, running, err := trafficBudget(ctx, tx, agent)
		if err != nil {
			return err
		}
		for _, cd := range list {
			if !InWindow(cd.snap.Windows, now) {
				continue
			}
			var resuming, busy bool
			if err := tx.QueryRow(ctx, `SELECT status = 'running', EXISTS (
				  SELECT 1 FROM discovery_job_targets a
				  JOIN discovery_job_targets b ON b.routing_domain_id = a.routing_domain_id AND b.target && a.target AND b.job_id <> a.job_id
				  JOIN discovery_jobs j ON j.id = b.job_id AND j.status = 'running'
				  WHERE a.job_id = $1::uuid)
				FROM discovery_jobs WHERE id = $1::uuid`, cd.id).Scan(&resuming, &busy); err != nil {
				return err
			}
			// Waiting jobs stay queued and go at a later claim. Nothing is lost.
			if !resuming && (busy || !fitsBudget(running, cd.snap.MaxPPS, ceiling)) {
				continue
			}
			if _, err := tx.Exec(ctx, `UPDATE discovery_jobs SET status = 'running', agent_id = $2::uuid, started_at = coalesce(started_at, now()), claimed_at = now()
				WHERE id = $1::uuid`, cd.id, agent); err != nil {
				return err
			}
			trows, err := tx.Query(ctx, `SELECT id::text, routing_domain_id::text, target::text, checkpoint, status FROM discovery_job_targets
				WHERE job_id = $1::uuid AND status IN ('pending','running') ORDER BY target`, cd.id)
			if err != nil {
				return err
			}
			targets, err := pgx.CollectRows(trows, func(r pgx.CollectableRow) (claimedTarget, error) {
				var t claimedTarget
				return t, r.Scan(&t.ID, &t.RoutingDomainID, &t.CIDR, &t.Checkpoint, &t.Status)
			})
			if err != nil {
				return err
			}
			if err := Log(ctx, tx, cd.id, "info", "Claimed by agent."); err != nil {
				return err
			}
			out = &gin.H{"jobId": cd.id, "config": cd.snap, "targets": targets}
			return nil
		}
		return nil
	})
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	if out == nil {
		c.Status(http.StatusNoContent)
		return
	}
	httpx.OK(c, http.StatusOK, out)
}

// Observation is one fact an agent saw.
type Observation struct {
	Method     string          `json:"method"`
	Address    string          `json:"address"`
	MAC        string          `json:"mac"`
	Hostname   string          `json:"hostname"`
	Alive      bool            `json:"alive"`
	Payload    json.RawMessage `json:"payload"`
	ObservedAt time.Time       `json:"observedAt"`
}

// ResultBatch is what an agent posts while working through a job.
type ResultBatch struct {
	Observations []Observation `json:"observations"`
	Targets      []struct {
		ID         string `json:"id"`
		Status     string `json:"status"`
		Checkpoint string `json:"checkpoint"`
	} `json:"targets"`
	Logs []struct {
		Level   string `json:"level"`
		Message string `json:"message"`
	} `json:"logs"`
	Progress int    `json:"progress"`
	Done     bool   `json:"done"`
	Error    string `json:"error"`
}

var targetStatuses = map[string]bool{"running": true, "done": true, "failed": true, "skipped": true}

// results records an agent's findings. Each address is checked again against
// the frozen scope and current exclusions; anything outside is dropped and
// logged. Only evidence and observation fields change: never allocation
// state, never ownership. The reply tells the agent to continue, pause or stop.
func (h *Handler) results(c *gin.Context) {
	jobID, ok := httpx.PathID(c)
	if !ok {
		return
	}
	var req ResultBatch
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 8<<20)
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Fail(c, httpx.ErrBadRequest)
		return
	}
	if len(req.Observations) > MaxObservations {
		httpx.Fail(c, httpx.Validation("observations", fmt.Sprintf("Send at most %d observations per call.", MaxObservations)))
		return
	}
	ctx, agent := c.Request.Context(), c.GetString(agentKey)
	instruction := "continue"
	err := db.InTx(ctx, h.svc.conn, func(tx pgx.Tx) error {
		var status string
		var owner *string
		err := tx.QueryRow(ctx, `SELECT status, agent_id::text FROM discovery_jobs WHERE id = $1::uuid FOR UPDATE`, jobID).Scan(&status, &owner)
		if errors.Is(err, pgx.ErrNoRows) || (err == nil && (owner == nil || *owner != agent)) {
			return httpx.ErrNotFound
		}
		if err != nil {
			return err
		}
		if on, err := Stopped(ctx, tx); err != nil {
			return err
		} else if on {
			instruction = "stop"
		}
		switch status {
		case "paused":
			instruction = "pause"
		case "running":
		default:
			instruction = "stop"
		}
		// The frozen scope, and exclusions as they are now: an exclusion added
		// mid-job still wins.
		trows, err := tx.Query(ctx, `SELECT routing_domain_id::text, target::text FROM discovery_job_targets WHERE job_id = $1::uuid`, jobID)
		if err != nil {
			return err
		}
		var scope []Target
		for trows.Next() {
			var t Target
			if err := trows.Scan(&t.RoutingDomainID, &t.CIDR); err != nil {
				trows.Close()
				return err
			}
			scope = append(scope, t)
		}
		trows.Close()
		exclusions, err := activeExclusions(ctx, tx)
		if err != nil {
			return err
		}
		sources := map[string]string{}
		srows, err := tx.Query(ctx, `SELECT name, id::text FROM observation_sources`)
		if err != nil {
			return err
		}
		for srows.Next() {
			var n, id string
			if err := srows.Scan(&n, &id); err != nil {
				srows.Close()
				return err
			}
			sources[n] = id
		}
		srows.Close()
		accepted, dropped := 0, 0
		for _, o := range req.Observations {
			a, err := netip.ParseAddr(strings.TrimSpace(o.Address))
			source, known := sources[SourceName[o.Method]]
			if err != nil || !known {
				dropped++
				continue
			}
			a = a.Unmap()
			rd := inScope(a, scope, exclusions)
			if rd == "" {
				dropped++
				continue
			}
			var mac *string
			if hw, err := net.ParseMAC(o.MAC); err == nil {
				s := hw.String()
				mac = &s
			}
			observed := o.ObservedAt
			if observed.IsZero() || observed.After(time.Now()) {
				observed = time.Now()
			}
			payload := o.Payload
			if len(payload) == 0 || len(payload) > 16384 || !json.Valid(payload) {
				payload = json.RawMessage(`{}`)
			}
			if _, err := tx.Exec(ctx, `INSERT INTO discovery_observations (job_id, source_id, routing_domain_id, address, mac, hostname, payload, observed_at)
				VALUES ($1::uuid, $2::uuid, $3::uuid, $4::inet, $5::macaddr, NULLIF($6, ''), $7::jsonb, $8)`,
				jobID, source, rd, a.String(), mac, truncate(o.Hostname, 255), payload, observed); err != nil {
				return err
			}
			if o.Alive {
				// Observation fields only. Allocation state and ownership are never touched.
				if _, err := tx.Exec(ctx, `UPDATE ip_addresses SET observation_state = CASE WHEN observation_state = 'conflict_suspected'
					THEN observation_state ELSE 'observed_active' END, last_seen = greatest(coalesce(last_seen, $3), $3),
					first_seen = coalesce(first_seen, $3), discovery_source_id = $4::uuid
					WHERE routing_domain_id = $1::uuid AND address = $2::inet`, rd, a.String(), observed, source); err != nil {
					return err
				}
			}
			accepted++
		}
		for _, t := range req.Targets {
			if !httpx.IsUUID(t.ID) || !targetStatuses[t.Status] {
				continue
			}
			if _, err := tx.Exec(ctx, `UPDATE discovery_job_targets SET status = $3, checkpoint = NULLIF($4, '') WHERE id = $1::uuid AND job_id = $2::uuid`,
				t.ID, jobID, t.Status, truncate(t.Checkpoint, 200)); err != nil {
				return err
			}
		}
		for _, l := range req.Logs {
			level := l.Level
			if level != "info" && level != "warning" && level != "error" {
				level = "info"
			}
			if err := Log(ctx, tx, jobID, level, l.Message); err != nil {
				return err
			}
		}
		if dropped > 0 {
			if err := Log(ctx, tx, jobID, "warning", fmt.Sprintf("Dropped %d results outside the job's approved scope or with an unknown method.", dropped)); err != nil {
				return err
			}
		}
		progress := req.Progress
		if progress < 0 || progress > 100 {
			progress = 0
		}
		if status == "running" {
			switch {
			case req.Done && req.Error != "":
				_, err = tx.Exec(ctx, `UPDATE discovery_jobs SET status = 'failed', progress = $2, error = $3, finished_at = now() WHERE id = $1::uuid`, jobID, progress, truncate(req.Error, 2000))
			case req.Done:
				_, err = tx.Exec(ctx, `UPDATE discovery_jobs SET status = 'completed', progress = 100, finished_at = now() WHERE id = $1::uuid`, jobID)
			default:
				_, err = tx.Exec(ctx, `UPDATE discovery_jobs SET progress = $2 WHERE id = $1::uuid`, jobID, progress)
			}
			if err != nil {
				return err
			}
			if req.Done {
				if err := Log(ctx, tx, jobID, "info", fmt.Sprintf("Agent finished. %d results accepted in the last batch.", accepted)); err != nil {
					return err
				}
			}
		}
		return nil
	})
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, http.StatusOK, gin.H{"instruction": instruction})
}

// inScope returns the routing domain of the frozen target holding the
// address, or "" when the address is outside the scope or excluded.
func inScope(a netip.Addr, scope []Target, exclusions []Exclusion) string {
	for _, t := range scope {
		p, err := netip.ParsePrefix(t.CIDR)
		if err != nil || !p.Contains(a) {
			continue
		}
		for _, e := range exclusions {
			if (e.RoutingDomainID == "" || e.RoutingDomainID == t.RoutingDomainID) && e.CIDR.Contains(a) {
				return ""
			}
		}
		return t.RoutingDomainID
	}
	return ""
}
