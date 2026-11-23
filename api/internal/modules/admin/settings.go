package admin

import (
	"encoding/json"
	"net/http"
	"strconv"

	"dfipam/api/internal/platform/db"
	"dfipam/api/internal/platform/httpx"
	"dfipam/api/internal/seed"
	"dfipam/api/internal/validate"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
)

// numberSetting is a whole-number setting an Admin may change, with its limits.
type numberSetting struct {
	Key, Field, Label string
	Min, Max          int
}

// Settings people can change. Keys are fixed here, never taken from a request.
var numbers = []numberSetting{
	{"scan.interval_minutes", "scanIntervalMinutes", "User network scan interval", 5, 1440},
	{"scan.server_interval_minutes", "serverScanIntervalMinutes", "Server network scan interval", 5, 1440},
	{"scan.rate_per_second", "scanRatePerSecond", "Scan speed", 5, 100},
	{"scan.port_check_hours", "portCheckHours", "Port check interval", 6, 168},
	{"server_check.interval_seconds", "serverCheckSeconds", "Server check interval", 60, 86400},
	{"server_check.down_after_failures", "downAfterFailures", "Failed checks before Down", 1, 20},
	{"server_check.slow_ms", "slowMs", "Slow server limit", 10, 5000},
	{"reservations.reminder_days", "reminderDays", "Reminder days", 1, 60},
	{"session.idle_timeout_minutes", "sessionTimeoutMinutes", "Session timeout", 5, 480},
}

const emailKey = "email.enabled"

// switches are on/off settings an Admin may change. Keys are fixed here.
var switches = []struct{ Key, Field string }{
	{"scan.auto_devices", "autoScanDevices"},
	{"scan.auto_servers", "autoScanServers"},
	{"server_check.enabled", "serverChecks"},
}

// Settings is what the Settings screen shows. SMTP comes from api/.env and is
// reported, never sent: only whether it is configured.
type settingsDTO struct {
	Values         map[string]int  `json:"values"`
	Switches       map[string]bool `json:"switches"`
	EmailEnabled   bool            `json:"emailEnabled"`
	SMTPConfigured bool            `json:"smtpConfigured"`
	ScanMethod     string          `json:"scanMethod"`
}

func (h *Handler) read(c *gin.Context) (settingsDTO, error) {
	out := settingsDTO{Values: map[string]int{}, Switches: map[string]bool{}, SMTPConfigured: h.smtpConfigured, ScanMethod: "ping"}
	rows, err := h.conn.Query(c, "SELECT key, value FROM settings")
	if err != nil {
		return out, err
	}
	defer rows.Close()
	raw := map[string]json.RawMessage{}
	for rows.Next() {
		var k string
		var v json.RawMessage
		if err := rows.Scan(&k, &v); err != nil {
			return out, err
		}
		raw[k] = v
	}
	for _, n := range numbers {
		var v int
		if json.Unmarshal(raw[n.Key], &v) != nil {
			_ = json.Unmarshal([]byte(seed.Default(n.Key)), &v)
		}
		if v < n.Min {
			v = n.Min
		}
		out.Values[n.Field] = v
	}
	for _, s := range switches {
		on := true
		if json.Unmarshal(raw[s.Key], &on) != nil {
			on = seed.Default(s.Key) != "false"
		}
		out.Switches[s.Field] = on
	}
	_ = json.Unmarshal(raw[emailKey], &out.EmailEnabled)
	_ = json.Unmarshal(raw["scan.method"], &out.ScanMethod)
	return out, rows.Err()
}

func (h *Handler) settings(c *gin.Context) {
	out, err := h.read(c)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, http.StatusOK, out)
}

type settingsInput struct {
	Values       map[string]int  `json:"values"`
	Switches     map[string]bool `json:"switches"`
	EmailEnabled *bool           `json:"emailEnabled"`
}

// changes checks each field sent and returns them as setting keys and values.
func (in settingsInput) changes() (map[string]string, error) {
	var v validate.V
	changes := map[string]string{}
	for _, n := range numbers {
		value, sent := in.Values[n.Field]
		if !sent {
			continue
		}
		if value < n.Min || value > n.Max {
			v.Add("values."+n.Field, n.Label+" must be from "+strconv.Itoa(n.Min)+" to "+strconv.Itoa(n.Max)+".")
		}
		changes[n.Key] = strconv.Itoa(value)
	}
	for _, s := range switches {
		if on, sent := in.Switches[s.Field]; sent {
			changes[s.Key] = strconv.FormatBool(on)
		}
	}
	if in.EmailEnabled != nil {
		changes[emailKey] = strconv.FormatBool(*in.EmailEnabled)
	}
	if err := v.Err(); err != nil {
		return nil, err
	}
	if len(changes) == 0 {
		return nil, httpx.Validation("values", "Change at least one setting.")
	}
	return changes, nil
}

// updateSettings saves the fields sent, each checked against its limits.
func (h *Handler) updateSettings(c *gin.Context) {
	var in settingsInput
	if !httpx.Bind(c, &in) {
		return
	}
	changes, err := in.changes()
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	before, err := h.read(c)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	keys, values := make([]string, 0, len(changes)), make([]string, 0, len(changes))
	for k, val := range changes {
		keys, values = append(keys, k), append(values, val)
	}
	err = db.InTx(c, h.conn, func(tx pgx.Tx) error {
		if _, err := tx.Exec(c, `INSERT INTO settings (key, value, updated_at, updated_by)
			SELECT k, v::jsonb, now(), $3::uuid FROM unnest($1::text[], $2::text[]) AS x(k, v)
			ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, updated_at = now(), updated_by = EXCLUDED.updated_by`,
			keys, values, actorID(c)); err != nil {
			return err
		}
		return record(c, tx, "update", "settings", "", before, changes)
	})
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	h.settings(c)
}
