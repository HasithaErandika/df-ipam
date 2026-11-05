// Package admin serves the permission matrix, system health and retention settings.
package admin

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"strings"
	"time"

	"dfipam/api/internal/audit"
	"dfipam/api/internal/auth"
	"dfipam/api/internal/platform/db"
	"dfipam/api/internal/platform/httpx"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
)

var (
	errProtected = httpx.NewError(http.StatusForbidden, "ROLE_PROTECTED", "The Super Administrator role always holds every permission.")
	errOwnRole   = httpx.NewError(http.StatusForbidden, "SELF_ROLE_CHANGE_DENIED", "Another administrator must change a role you hold.")
	errLockout   = httpx.NewError(http.StatusConflict, "LAST_ADMINISTRATOR", "This would leave nobody able to manage users.")
	errRoleInUse = httpx.NewError(http.StatusConflict, "ROLE_IN_USE", "Remove this role from its users first.")
	errSystem    = httpx.NewError(http.StatusConflict, "ROLE_IS_SYSTEM", "Default roles can be edited but not deleted.")
	errNameTaken = httpx.NewError(http.StatusConflict, "NAME_TAKEN", "Another role has this name.")
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

// Handler serves administration.
type Handler struct {
	conn          db.Conn
	encryptionSet bool
}

// NewHandler returns a handler. encryptionSet says whether the master key is loaded.
func NewHandler(conn db.Conn, encryptionSet bool) *Handler { return &Handler{conn, encryptionSet} }

// Register adds the routes.
func (h *Handler) Register(g *auth.Guard) {
	g.Handle(http.MethodGet, "/admin/permission-matrix", h.matrix, auth.UsersManage, auth.AuditView)
	g.Handle(http.MethodPost, "/roles", h.createRole, auth.UsersManage)
	g.Handle(http.MethodPut, "/roles/:id", h.updateRole, auth.UsersManage)
	g.Handle(http.MethodPut, "/roles/:id/permissions", h.setPermissions, auth.UsersManage)
	g.Handle(http.MethodDelete, "/roles/:id", h.deleteRole, auth.UsersManage)
	g.Handle(http.MethodGet, "/system/health", h.health, auth.SystemManage, auth.AuditView)
	g.Handle(http.MethodGet, "/system/retention", h.retention, auth.SystemManage, auth.AuditView)
	g.Handle(http.MethodPut, "/system/retention/:class", h.setRetention, auth.SystemManage)
}

// MatrixRole is a role with its permissions and how many people hold it.
type MatrixRole struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	IsSystem    bool     `json:"isSystem"`
	Locked      bool     `json:"locked"`
	Customised  bool     `json:"isCustomised"`
	UserCount   int64    `json:"userCount"`
	Permissions []string `json:"permissions"`
}

// Matrix is the whole permission matrix.
type Matrix struct {
	Permissions []string     `json:"permissions"`
	Roles       []MatrixRole `json:"roles"`
}

func (h *Handler) matrix(c *gin.Context) {
	ctx := c.Request.Context()
	rows, err := h.conn.Query(ctx, `SELECT resource || ':' || action FROM permissions ORDER BY resource, action`)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	permissions, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	rows, err = h.conn.Query(ctx, `
		SELECT r.id::text, r.name, r.description, r.is_system, r.is_customised, (SELECT count(DISTINCT ur.user_id) FROM user_roles ur WHERE ur.role_id = r.id),
		  COALESCE((SELECT array_agg(p.resource || ':' || p.action ORDER BY p.resource, p.action) FROM role_permissions rp
		            JOIN permissions p ON p.id = rp.permission_id WHERE rp.role_id = r.id), '{}')
		FROM roles r ORDER BY r.is_system DESC, r.name`)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	roles, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (MatrixRole, error) {
		var m MatrixRole
		err := r.Scan(&m.ID, &m.Name, &m.Description, &m.IsSystem, &m.Customised, &m.UserCount, &m.Permissions)
		m.Locked = m.Name == auth.SuperAdminRole
		return m, err
	})
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, http.StatusOK, Matrix{permissions, roles})
}

func (h *Handler) createRole(c *gin.Context) {
	var req struct {
		Name        string `json:"name"`
		Description string `json:"description"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Fail(c, httpx.ErrBadRequest)
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" || len(req.Name) > 100 {
		httpx.Fail(c, httpx.Validation("name", "Enter a name of up to 100 characters."))
		return
	}
	ctx, actor := c.Request.Context(), auth.Current(c).UserID
	var id string
	err := db.InTx(ctx, h.conn, func(tx pgx.Tx) error {
		err := tx.QueryRow(ctx, `INSERT INTO roles (name, description, is_customised) VALUES ($1, $2, true) RETURNING id::text`, req.Name, req.Description).Scan(&id)
		if db.ErrorCode(err) == "23505" {
			return errNameTaken
		}
		if err != nil {
			return err
		}
		return audit.Write(ctx, tx, audit.Entry{ActorID: actor, Action: "role.create", ObjectType: "role", ObjectID: id, New: req})
	})
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, http.StatusCreated, gin.H{"id": id, "name": req.Name, "description": req.Description})
}

// updateRole changes a role's name and description. Default roles keep their names,
// because the seed and setup steps find them by name.
func (h *Handler) updateRole(c *gin.Context) {
	id, ok := httpx.PathID(c)
	if !ok {
		return
	}
	var req struct {
		Name        string `json:"name"`
		Description string `json:"description"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Fail(c, httpx.ErrBadRequest)
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" || len(req.Name) > 100 {
		httpx.Fail(c, httpx.Validation("name", "Enter a name of up to 100 characters."))
		return
	}
	ctx, actor := c.Request.Context(), auth.Current(c).UserID
	err := db.InTx(ctx, h.conn, func(tx pgx.Tx) error {
		name, system, err := lockRole(ctx, tx, id)
		if err != nil {
			return err
		}
		if system && req.Name != name {
			return httpx.Validation("name", "Default roles keep their names. Their descriptions can change.")
		}
		var old string
		if err := tx.QueryRow(ctx, `SELECT description FROM roles WHERE id = $1::uuid`, id).Scan(&old); err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `UPDATE roles SET name = $2, description = $3, is_customised = true WHERE id = $1::uuid`, id, req.Name, req.Description)
		if db.ErrorCode(err) == "23505" {
			return errNameTaken
		}
		if err != nil {
			return err
		}
		return audit.Write(ctx, tx, audit.Entry{ActorID: actor, Action: "role.update", ObjectType: "role", ObjectID: id,
			Old: gin.H{"name": name, "description": old}, New: req})
	})
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func lockRole(ctx context.Context, tx pgx.Tx, id string) (name string, system bool, err error) {
	err = tx.QueryRow(ctx, `SELECT name, is_system FROM roles WHERE id = $1::uuid FOR UPDATE`, id).Scan(&name, &system)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", false, httpx.ErrNotFound
	}
	return name, system, err
}

// setPermissions replaces a role's permissions. Separation of duties and the lockout
// check both run inside the transaction, against the state after the change.
func (h *Handler) setPermissions(c *gin.Context) {
	id, ok := httpx.PathID(c)
	if !ok {
		return
	}
	var req struct {
		Permissions []string `json:"permissions"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.Permissions == nil {
		httpx.Fail(c, httpx.ErrBadRequest)
		return
	}
	slices.Sort(req.Permissions)
	req.Permissions = slices.Compact(req.Permissions)
	ctx, actor := c.Request.Context(), auth.Current(c).UserID
	err := db.InTx(ctx, h.conn, func(tx pgx.Tx) error {
		// One role change at a time, so two edits can't each pass the lockout check.
		if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended('role-permissions', 7))`); err != nil {
			return err
		}
		name, _, err := lockRole(ctx, tx, id)
		if err != nil {
			return err
		}
		if name == auth.SuperAdminRole {
			return errProtected
		}
		var holds bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM user_roles WHERE user_id = $1::uuid AND role_id = $2::uuid)`, actor, id).Scan(&holds); err != nil {
			return err
		}
		if holds {
			return errOwnRole
		}
		rows, err := tx.Query(ctx, `SELECT p.resource || ':' || p.action FROM role_permissions rp JOIN permissions p ON p.id = rp.permission_id
			WHERE rp.role_id = $1::uuid ORDER BY 1`, id)
		if err != nil {
			return err
		}
		old, err := pgx.CollectRows(rows, pgx.RowTo[string])
		if err != nil {
			return err
		}
		var known int
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM permissions WHERE resource || ':' || action = ANY($1)`, req.Permissions).Scan(&known); err != nil {
			return err
		}
		if known != len(req.Permissions) {
			return httpx.Validation("permissions", "Use only known permissions.")
		}
		if _, err := tx.Exec(ctx, `DELETE FROM role_permissions WHERE role_id = $1::uuid`, id); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO role_permissions (role_id, permission_id)
			SELECT $1::uuid, id FROM permissions WHERE resource || ':' || action = ANY($2)`, id, req.Permissions); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE roles SET is_customised = true WHERE id = $1::uuid`, id); err != nil {
			return err
		}
		var admins int
		if err := tx.QueryRow(ctx, `SELECT count(DISTINCT u.id) FROM users u JOIN user_roles ur ON ur.user_id = u.id
			JOIN role_permissions rp ON rp.role_id = ur.role_id JOIN permissions p ON p.id = rp.permission_id
			WHERE u.is_active AND p.resource = 'users' AND p.action = 'manage'`).Scan(&admins); err != nil {
			return err
		}
		if admins == 0 {
			return errLockout
		}
		return audit.Write(ctx, tx, audit.Entry{ActorID: actor, Action: "role.set_permissions", ObjectType: "role", ObjectID: id,
			Old: gin.H{"permissions": old}, New: gin.H{"role": name, "permissions": req.Permissions}})
	})
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *Handler) deleteRole(c *gin.Context) {
	id, ok := httpx.PathID(c)
	if !ok {
		return
	}
	ctx, actor := c.Request.Context(), auth.Current(c).UserID
	err := db.InTx(ctx, h.conn, func(tx pgx.Tx) error {
		name, system, err := lockRole(ctx, tx, id)
		if err != nil {
			return err
		}
		if system {
			return errSystem
		}
		var used bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM user_roles WHERE role_id = $1::uuid)
			OR EXISTS (SELECT 1 FROM approval_workflow_steps WHERE approver_role_id = $1::uuid)
			OR EXISTS (SELECT 1 FROM ip_request_approvals WHERE approver_role_id = $1::uuid)`, id).Scan(&used); err != nil {
			return err
		}
		if used {
			return errRoleInUse
		}
		if _, err := tx.Exec(ctx, `DELETE FROM role_permissions WHERE role_id = $1::uuid`, id); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `DELETE FROM roles WHERE id = $1::uuid`, id); err != nil {
			return err
		}
		return audit.Write(ctx, tx, audit.Entry{ActorID: actor, Action: "role.delete", ObjectType: "role", ObjectID: id, Old: gin.H{"name": name}})
	})
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	c.Status(http.StatusNoContent)
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
