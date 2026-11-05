package discovery

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"net/http"
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
