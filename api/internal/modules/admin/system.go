package admin

import (
	"context"
	"dfipam/api/internal/audit"
	"dfipam/api/internal/auth"
	"dfipam/api/internal/platform/db"
	"dfipam/api/internal/platform/httpx"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
)

// Heartbeat records that a worker is alive, for the health page.
func Heartbeat(ctx context.Context, conn db.Conn, now time.Time) error {
	value, err := json.Marshal(now.UTC())
	if err != nil {
		return err
	}
	_, err = conn.Exec(ctx, `INSERT INTO system_settings (key, value) VALUES ('worker.heartbeat', $1::jsonb)
		ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, updated_at = now()`, value)
	return err
}

// Health describes the platform, for the System Administration page.
type Health struct {
	Database             string     `json:"database"`
	WorkerHeartbeat      *time.Time `json:"workerHeartbeat"`
	WorkerHealthy        bool       `json:"workerHealthy"`
	EncryptionConfigured bool       `json:"encryptionConfigured"`
	EmergencyStop        bool       `json:"emergencyStop"`
	LastBackup           *time.Time `json:"lastBackup"`
	DiscoveryQueued      int64      `json:"discoveryQueued"`
	SyncsQueued          int64      `json:"syncsQueued"`
	ReportsQueued        int64      `json:"reportsQueued"`
	EmailPending         int64      `json:"emailPending"`
	AgentsOnline         int64      `json:"agentsOnline"`
	AgentsOffline        int64      `json:"agentsOffline"`
	IntegrationsHealthy  int64      `json:"integrationsHealthy"`
	IntegrationsFailing  int64      `json:"integrationsFailing"`
}

func timeSetting(ctx context.Context, q db.Conn, key string) (*time.Time, error) {
	var raw []byte
	err := q.QueryRow(ctx, `SELECT value FROM system_settings WHERE key = $1`, key).Scan(&raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var t time.Time
	if err := json.Unmarshal(raw, &t); err != nil {
		return nil, nil // a value that isn't a time means it hasn't been reported
	}
	return &t, nil
}

func (h *Handler) health(c *gin.Context) {
	ctx := c.Request.Context()
	out := Health{Database: "ok", EncryptionConfigured: h.encryptionSet}
	err := h.conn.QueryRow(ctx, `SELECT
		  COALESCE((SELECT value = 'true'::jsonb FROM system_settings WHERE key = 'discovery.emergency_stop'), false),
		  (SELECT count(*) FROM discovery_jobs WHERE status = 'queued'),
		  (SELECT count(*) FROM integration_sync_jobs WHERE status = 'queued'),
		  (SELECT count(*) FROM report_jobs WHERE status = 'queued'),
		  (SELECT count(*) FROM notifications WHERE channel = 'email' AND status = 'pending'),
		  (SELECT count(*) FROM discovery_agents WHERE status = 'active' AND last_heartbeat_at > now() - interval '5 minutes'),
		  (SELECT count(*) FROM discovery_agents WHERE status = 'active' AND (last_heartbeat_at IS NULL OR last_heartbeat_at <= now() - interval '5 minutes')),
		  (SELECT count(*) FROM integration_connections WHERE is_enabled AND health = 'healthy'),
		  (SELECT count(*) FROM integration_connections WHERE is_enabled AND health IN ('degraded', 'failed'))`).
		Scan(&out.EmergencyStop, &out.DiscoveryQueued, &out.SyncsQueued, &out.ReportsQueued, &out.EmailPending, &out.AgentsOnline, &out.AgentsOffline,
			&out.IntegrationsHealthy, &out.IntegrationsFailing)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	if out.WorkerHeartbeat, err = timeSetting(ctx, h.conn, "worker.heartbeat"); err != nil {
		httpx.Fail(c, err)
		return
	}
	out.WorkerHealthy = out.WorkerHeartbeat != nil && time.Since(*out.WorkerHeartbeat) < 3*time.Minute
	if out.LastBackup, err = timeSetting(ctx, h.conn, "backup.last_success"); err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, http.StatusOK, out)
}

// RetentionPolicy says how long one class of data is kept.
type RetentionPolicy struct {
	DataClass  string     `json:"dataClass"`
	RetainDays int        `json:"retainDays"`
	UpdatedAt  *time.Time `json:"updatedAt"`
	UpdatedBy  *string    `json:"updatedBy"`
}

func (h *Handler) retention(c *gin.Context) {
	rows, err := h.conn.Query(c.Request.Context(), `SELECT r.data_class, r.retain_days, r.updated_at, u.email FROM retention_policies r
		LEFT JOIN users u ON u.id = r.updated_by ORDER BY r.data_class`)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	out, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (RetentionPolicy, error) {
		var p RetentionPolicy
		return p, r.Scan(&p.DataClass, &p.RetainDays, &p.UpdatedAt, &p.UpdatedBy)
	})
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	if out == nil {
		out = []RetentionPolicy{}
	}
	httpx.OK(c, http.StatusOK, out)
}

// minimumDays stops a slip wiping evidence. Audit history has the highest floor.
func minimumDays(class string) int {
	if class == "audit_logs" {
		return 365
	}
	return 7
}

func (h *Handler) setRetention(c *gin.Context) {
	class := c.Param("class")
	var req struct {
		RetainDays int `json:"retainDays"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Fail(c, httpx.ErrBadRequest)
		return
	}
	if req.RetainDays < minimumDays(class) || req.RetainDays > 36500 {
		httpx.Fail(c, httpx.Validation("retainDays", "This period is outside the allowed range."))
		return
	}
	ctx, actor := c.Request.Context(), auth.Current(c).UserID
	err := db.InTx(ctx, h.conn, func(tx pgx.Tx) error {
		var old int
		err := tx.QueryRow(ctx, `SELECT retain_days FROM retention_policies WHERE data_class = $1 FOR UPDATE`, class).Scan(&old)
		if errors.Is(err, pgx.ErrNoRows) {
			return httpx.ErrNotFound
		}
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE retention_policies SET retain_days = $2, updated_by = $3::uuid WHERE data_class = $1`, class, req.RetainDays, actor); err != nil {
			return err
		}
		return audit.Write(ctx, tx, audit.Entry{ActorID: actor, Action: "retention.update", ObjectType: "retention_policy",
			Old: gin.H{"dataClass": class, "retainDays": old}, New: gin.H{"dataClass": class, "retainDays": req.RetainDays}})
	})
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}
