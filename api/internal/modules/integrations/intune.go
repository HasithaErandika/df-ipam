package integrations

import (
	"errors"
	"net/http"
	"strconv"

	"dfipam/api/internal/audit"
	"dfipam/api/internal/auth"
	"dfipam/api/internal/integrations/intune"
	"dfipam/api/internal/platform/db"
	"dfipam/api/internal/platform/httpx"
	"dfipam/api/internal/validate"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
)

// IntuneHandler configures the read-only Microsoft Intune connection, which
// names the person who uses each discovered device.
type IntuneHandler struct {
	conn db.Conn
	sync *intune.Syncer
}

func NewIntune(conn db.Conn, sync *intune.Syncer) *IntuneHandler {
	return &IntuneHandler{conn: conn, sync: sync}
}

func (h *IntuneHandler) Register(g *auth.Guard) {
	g.Handle(http.MethodGet, "/integrations/intune", h.get, auth.SystemManage)
	g.Handle(http.MethodPut, "/integrations/intune", h.save, auth.SystemManage)
	g.Handle(http.MethodPost, "/integrations/intune/sync", h.syncNow, auth.SystemManage)
}

var errIntuneOff = httpx.NewError(http.StatusServiceUnavailable, "INTEGRATION_UNAVAILABLE", "The Intune connection is not running in this process.")

func (h *IntuneHandler) get(c *gin.Context) {
	if h.sync == nil {
		httpx.Fail(c, errIntuneOff)
		return
	}
	cfg, err := intune.Load(c, h.conn)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, http.StatusOK, gin.H{"config": cfg, "status": h.sync.Snapshot(), "configured": h.sync.Configured()})
}

func (h *IntuneHandler) save(c *gin.Context) {
	if h.sync == nil {
		httpx.Fail(c, errIntuneOff)
		return
	}
	var in intune.Config
	if !httpx.Bind(c, &in) {
		return
	}
	var v validate.V
	v.IntRange("intervalMinutes", &in.IntervalMinutes, 15, 1440, "Read interval")
	if err := v.Err(); err != nil {
		httpx.Fail(c, err)
		return
	}
	values := map[string]string{"intune.enabled": strconv.FormatBool(in.Enabled), "intune.interval_minutes": strconv.Itoa(in.IntervalMinutes)}
	err := db.InTx(c, h.conn, func(tx pgx.Tx) error {
		for k, val := range values {
			if _, err := tx.Exec(c, `INSERT INTO settings (key, value, updated_by) VALUES ($1, $2::jsonb, $3::uuid)
				ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, updated_at = now(), updated_by = EXCLUDED.updated_by`,
				k, val, auth.Current(c).UserID); err != nil {
				return err
			}
		}
		return audit.Write(c, tx, audit.Entry{ActorID: auth.Current(c).UserID, Action: "update", ObjectType: "integration", ObjectID: "intune", New: in})
	})
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	h.get(c)
}

func (h *IntuneHandler) syncNow(c *gin.Context) {
	if h.sync == nil {
		httpx.Fail(c, errIntuneOff)
		return
	}
	err := db.InTx(c, h.conn, func(tx pgx.Tx) error {
		if err := audit.Write(c, tx, audit.Entry{ActorID: auth.Current(c).UserID, Action: "sync", ObjectType: "integration", ObjectID: "intune"}); err != nil {
			return err
		}
		return h.sync.Trigger() // last, so a refusal rolls the audit row back
	})
	switch {
	case errors.Is(err, intune.ErrNotConfigured):
		httpx.Fail(c, httpx.NewError(http.StatusConflict, "INTEGRATION_NOT_CONFIGURED", "Set ENTRA_TENANT_ID, ENTRA_CLIENT_ID and ENTRA_CLIENT_SECRET in api/.env first."))
	case errors.Is(err, intune.ErrBusy):
		httpx.Fail(c, httpx.NewError(http.StatusConflict, "SYNC_RUNNING", "An Intune read is already running."))
	case errors.Is(err, intune.ErrTooSoon):
		httpx.Fail(c, httpx.NewError(http.StatusTooManyRequests, "SYNC_TOO_SOON", "Intune was read less than a minute ago. Try again shortly."))
	case err != nil:
		httpx.Fail(c, err)
	default:
		httpx.OK(c, http.StatusAccepted, gin.H{"queued": true})
	}
}
