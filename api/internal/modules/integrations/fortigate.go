// Package integrations configures the optional, read-only FortiGate connection.
package integrations

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"dfipam/api/internal/audit"
	"dfipam/api/internal/auth"
	"dfipam/api/internal/integrations/fortigate"
	"dfipam/api/internal/platform/db"
	"dfipam/api/internal/platform/httpx"
	"dfipam/api/internal/validate"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
)

type Handler struct {
	conn db.Conn
	sync *fortigate.Syncer
}

func New(conn db.Conn, sync *fortigate.Syncer) *Handler { return &Handler{conn: conn, sync: sync} }

func (h *Handler) Register(g *auth.Guard) {
	g.Handle(http.MethodGet, "/integrations/fortigate", h.get, auth.SystemManage)
	g.Handle(http.MethodPut, "/integrations/fortigate", h.save, auth.SystemManage)
	g.Handle(http.MethodPost, "/integrations/fortigate/test", h.test, auth.SystemManage)
	g.Handle(http.MethodPost, "/integrations/fortigate/sync", h.syncNow, auth.SystemManage)
	g.Handle(http.MethodDelete, "/integrations/fortigate/token", h.deleteToken, auth.SystemManage)
}

var errOff = httpx.NewError(http.StatusServiceUnavailable, "INTEGRATION_UNAVAILABLE", "The FortiGate connection is not running in this process.")

// The token is never sent back: only whether one is stored.
func (h *Handler) get(c *gin.Context) {
	if h.sync == nil {
		httpx.Fail(c, errOff)
		return
	}
	cfg, err := fortigate.Load(c, h.conn, h.sync.Keys())
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, http.StatusOK, gin.H{"config": cfg, "status": h.sync.Snapshot(), "encryptionReady": h.sync.Keys() != nil})
}

type saveInput struct {
	Active          *bool  `json:"active"` // absent keeps the stored value
	Enabled         bool   `json:"enabled"`
	URL             string `json:"url"`
	VDOM            string `json:"vdom"`
	VerifyTLS       bool   `json:"verifyTls"`
	IntervalMinutes int    `json:"intervalMinutes"`
	Token           string `json:"token"`      // empty keeps the stored one
	ClearToken      bool   `json:"clearToken"` // removes the stored one
}

func (in *saveInput) validate() (int, error) {
	var v validate.V
	v.Text("url", &in.URL, 200, in.Enabled, "Address")
	if in.URL != "" && !strings.HasPrefix(in.URL, "https://") {
		v.Add("url", "Use an https:// address. The API token must never travel unencrypted.")
	}
	v.Text("vdom", &in.VDOM, 40, false, "VDOM")
	interval := in.IntervalMinutes
	v.IntRange("intervalMinutes", &interval, 5, 1440, "Read interval")
	in.Token = strings.TrimSpace(in.Token)
	if len(in.Token) > 200 {
		v.Add("token", "The token is too long.")
	}
	return interval, v.Err()
}

func (h *Handler) save(c *gin.Context) {
	if h.sync == nil {
		httpx.Fail(c, errOff)
		return
	}
	var in saveInput
	if !httpx.Bind(c, &in) {
		return
	}
	interval, err := in.validate()
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	values, err := h.settingValues(in, interval)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	err = db.InTx(c, h.conn, func(tx pgx.Tx) error { return saveSettings(c, tx, in, values) })
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	h.get(c)
}

func (h *Handler) settingValues(in saveInput, interval int) (map[string]string, error) {
	values := map[string]string{
		"fortigate.enabled": strconv.FormatBool(in.Enabled), "fortigate.url": quote(in.URL), "fortigate.vdom": quote(in.VDOM),
		"fortigate.verify_tls": strconv.FormatBool(in.VerifyTLS), "fortigate.interval_minutes": strconv.Itoa(interval),
	}
	if in.Active != nil {
		values["fortigate.active"] = strconv.FormatBool(*in.Active)
	}
	if in.Token != "" {
		sealed, err := fortigate.SealToken(h.sync.Keys(), in.Token)
		if err != nil {
			return nil, httpx.Validation("token", err.Error()+".")
		}
		values["fortigate.token"] = sealed
	}
	return values, nil
}

// saveSettings stores the connection. A token belongs to the firewall it was
// made on: when the address changes without a new token, the old one is
// removed, so it is never sent to a different host.
func saveSettings(c *gin.Context, tx pgx.Tx, in saveInput, values map[string]string) error {
	var oldURL string
	_ = tx.QueryRow(c, `SELECT value #>> '{}' FROM settings WHERE key = 'fortigate.url'`).Scan(&oldURL)
	movedHost := oldURL != "" && !strings.EqualFold(strings.TrimRight(oldURL, "/"), strings.TrimRight(in.URL, "/"))
	for k, val := range values {
		if _, err := tx.Exec(c, `INSERT INTO settings (key, value, updated_by) VALUES ($1, $2::jsonb, $3::uuid)
			ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, updated_at = now(), updated_by = EXCLUDED.updated_by`,
			k, val, auth.Current(c).UserID); err != nil {
			return err
		}
	}
	if (in.ClearToken || movedHost) && in.Token == "" {
		if _, err := tx.Exec(c, `DELETE FROM settings WHERE key = 'fortigate.token'`); err != nil {
			return err
		}
	}
	logged := in
	logged.Token = map[bool]string{true: "(changed)", false: ""}[in.Token != ""]
	if movedHost && in.Token == "" {
		logged.ClearToken = true
	}
	return audit.Write(c, tx, audit.Entry{ActorID: auth.Current(c).UserID, Action: "update", ObjectType: "integration", ObjectID: "fortigate", New: logged})
}

func quote(s string) string { return strconv.Quote(s) }

// test checks the saved connection without recording anything.
func (h *Handler) test(c *gin.Context) {
	if h.sync == nil {
		httpx.Fail(c, errOff)
		return
	}
	cfg, err := fortigate.Load(c, h.conn, h.sync.Keys())
	if err == nil {
		var client *fortigate.Client
		if client, err = cfg.Client(); err == nil {
			ctx, cancel := context.WithTimeout(c, 20*time.Second)
			defer cancel()
			var name string
			if name, err = client.Status(ctx); err == nil {
				httpx.OK(c, http.StatusOK, gin.H{"ok": true, "firewall": name})
				return
			}
		}
	}
	httpx.OK(c, http.StatusOK, gin.H{"ok": false, "error": err.Error()})
}

func (h *Handler) syncNow(c *gin.Context) {
	if h.sync == nil {
		httpx.Fail(c, errOff)
		return
	}
	err := db.InTx(c, h.conn, func(tx pgx.Tx) error {
		if err := audit.Write(c, tx, audit.Entry{ActorID: auth.Current(c).UserID, Action: "sync", ObjectType: "integration", ObjectID: "fortigate"}); err != nil {
			return err
		}
		return h.sync.Trigger() // last, so a refusal rolls the audit row back
	})
	switch {
	case errors.Is(err, fortigate.ErrBusy):
		httpx.Fail(c, httpx.NewError(http.StatusConflict, "SYNC_RUNNING", "A FortiGate read is already running."))
	case errors.Is(err, fortigate.ErrTooSoon):
		httpx.Fail(c, httpx.NewError(http.StatusTooManyRequests, "SYNC_TOO_SOON", "The FortiGate was read less than a minute ago. Try again shortly."))
	case err != nil:
		httpx.Fail(c, err)
	default:
		httpx.OK(c, http.StatusAccepted, gin.H{"queued": true})
	}
}
