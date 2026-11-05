package discovery

import (
	"dfipam/api/internal/audit"
	"dfipam/api/internal/auth"
	"dfipam/api/internal/platform/db"
	"dfipam/api/internal/platform/httpx"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
)

// Agent is a discovery agent. Its credential never leaves the server.
type Agent struct {
	ID              string     `json:"id"`
	Name            string     `json:"name"`
	SiteName        *string    `json:"siteName"`
	Status          string     `json:"status"`
	Version         string     `json:"version"`
	LastHeartbeatAt *time.Time `json:"lastHeartbeatAt"`
	RotatedAt       *time.Time `json:"credentialRotatedAt"`
	RevokedAt       *time.Time `json:"revokedAt"`
	PendingToken    bool       `json:"registrationPending"`
	CreatedAt       time.Time  `json:"createdAt"`
}

const agentCols = `a.id::text, a.name, st.name, a.status, a.version, a.last_heartbeat_at, a.credential_rotated_at, a.revoked_at,
	EXISTS (SELECT 1 FROM agent_registration_tokens t WHERE t.agent_id = a.id AND t.used_at IS NULL AND t.expires_at > now()), a.created_at`

func scanAgent(r pgx.Row) (Agent, error) {
	var a Agent
	err := r.Scan(&a.ID, &a.Name, &a.SiteName, &a.Status, &a.Version, &a.LastHeartbeatAt, &a.RotatedAt, &a.RevokedAt, &a.PendingToken, &a.CreatedAt)
	return a, err
}

func (h *Handler) agents(c *gin.Context) {
	rows, err := h.svc.conn.Query(c.Request.Context(), "SELECT "+agentCols+" FROM discovery_agents a LEFT JOIN sites st ON st.id = a.site_id ORDER BY a.name")
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	out, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (Agent, error) { return scanAgent(r) })
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.List(c, out, httpx.Page{Number: 1, Size: len(out)}, int64(len(out)))
}

type tokenResponse struct {
	Agent     Agent     `json:"agent"`
	Token     string    `json:"registrationToken"`
	ExpiresAt time.Time `json:"expiresAt"`
}

func (h *Handler) loadAgent(c *gin.Context, tx pgx.Tx, id string) (Agent, error) {
	a, err := scanAgent(tx.QueryRow(c.Request.Context(), "SELECT "+agentCols+" FROM discovery_agents a LEFT JOIN sites st ON st.id = a.site_id WHERE a.id = $1::uuid", id))
	if errors.Is(err, pgx.ErrNoRows) {
		return a, httpx.ErrNotFound
	}
	return a, err
}

// createAgent registers an agent and returns its one-time registration token.
// Only the token's hash is stored; it is shown once.
func (h *Handler) createAgent(c *gin.Context) {
	var req struct {
		Name   string  `json:"name"`
		SiteID *string `json:"siteId"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Fail(c, httpx.ErrBadRequest)
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		httpx.Fail(c, httpx.Validation("name", "Name the agent."))
		return
	}
	if req.SiteID != nil && !httpx.IsUUID(*req.SiteID) {
		req.SiteID = nil
	}
	ctx, actor := c.Request.Context(), auth.Current(c).UserID
	var out tokenResponse
	err := db.InTx(ctx, h.svc.conn, func(tx pgx.Tx) error {
		var id string
		if err := tx.QueryRow(ctx, `INSERT INTO discovery_agents (name, site_id) VALUES ($1, $2::uuid) RETURNING id::text`, req.Name, req.SiteID).Scan(&id); err != nil {
			return err
		}
		var err error
		if out.Token, out.ExpiresAt, err = IssueToken(ctx, tx, actor, id); err != nil {
			return err
		}
		if out.Agent, err = h.loadAgent(c, tx, id); err != nil {
			return err
		}
		return audit.Write(ctx, tx, audit.Entry{ActorID: actor, Action: "discovery_agent.create", ObjectType: "discovery_agent", ObjectID: id, New: out.Agent})
	})
	switch db.ErrorCode(err) {
	case db.CodeUnique:
		err = httpx.NewError(http.StatusConflict, "ALREADY_EXISTS", "An agent with this name already exists.")
	case db.CodeForeignKey:
		err = httpx.Validation("siteId", "That site doesn't exist.")
	}
	respond(c, http.StatusCreated, out, err)
}

// rotateAgent issues a fresh registration token. The agent re-registers with
// it and gets a new credential; the old credential stops working then.
func (h *Handler) rotateAgent(c *gin.Context) {
	id, ok := httpx.PathID(c)
	if !ok {
		return
	}
	ctx, actor := c.Request.Context(), auth.Current(c).UserID
	var out tokenResponse
	err := db.InTx(ctx, h.svc.conn, func(tx pgx.Tx) error {
		a, err := h.loadAgent(c, tx, id)
		if err != nil {
			return err
		}
		if a.Status == "revoked" {
			return httpx.NewError(http.StatusConflict, "AGENT_REVOKED", "A revoked agent can't be rotated. Create a new agent.")
		}
		if out.Token, out.ExpiresAt, err = IssueToken(ctx, tx, actor, id); err != nil {
			return err
		}
		out.Agent = a
		return audit.Write(ctx, tx, audit.Entry{ActorID: actor, Action: "discovery_agent.rotate", ObjectType: "discovery_agent", ObjectID: id})
	})
	respond(c, http.StatusOK, out, err)
}

// revokeAgent ends an agent's access at once and hands its jobs back.
func (h *Handler) revokeAgent(c *gin.Context) {
	id, ok := httpx.PathID(c)
	if !ok {
		return
	}
	ctx, actor := c.Request.Context(), auth.Current(c).UserID
	var out Agent
	err := db.InTx(ctx, h.svc.conn, func(tx pgx.Tx) error {
		if _, err := h.loadAgent(c, tx, id); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE discovery_agents SET status = 'revoked', revoked_at = now(), credential_hash = NULL WHERE id = $1::uuid`, id); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE agent_registration_tokens SET expires_at = now() WHERE agent_id = $1::uuid AND used_at IS NULL`, id); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE discovery_jobs SET status = 'failed', finished_at = now(), error = 'Agent revoked' WHERE agent_id = $1::uuid AND status IN ('running','paused')`, id); err != nil {
			return err
		}
		var err error
		if out, err = h.loadAgent(c, tx, id); err != nil {
			return err
		}
		return audit.Write(ctx, tx, audit.Entry{ActorID: actor, Action: "discovery_agent.revoke", ObjectType: "discovery_agent", ObjectID: id, New: out})
	})
	respond(c, http.StatusOK, out, err)
}
