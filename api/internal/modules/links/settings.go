package links

import (
	"context"
	"encoding/json"
	"net/http"
	"regexp"
	"slices"
	"strings"

	"dfipam/api/internal/platform/db"
	"dfipam/api/internal/platform/httpx"
	"dfipam/api/internal/validate"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
)

// LinkType is one entry of the link type list. vpn shows the VPN section.
type LinkType struct {
	Key    string `json:"key"`
	Label  string `json:"label"`
	VPN    bool   `json:"vpn"`
	Active bool   `json:"active"`
}

// Settings are the Network Links master lists, kept in settings (links.*).
type Settings struct {
	Types        []LinkType `json:"types"`
	Sites        []string   `json:"sites"`
	ReminderDays []int      `json:"reminderDays"`
	// USDRate is the approved LKR per USD. Without it totals stay per currency.
	USDRate *float64 `json:"usdRate"`
}

var settingKeys = map[string]string{"links.types": "types", "links.sites": "sites", "links.reminder_days": "reminderDays", "links.usd_rate": "usdRate"}

func loadSettings(ctx context.Context, conn db.Conn) (Settings, error) {
	s := Settings{Types: []LinkType{}, Sites: []string{}, ReminderDays: []int{}}
	rows, err := conn.Query(ctx, `SELECT key, value FROM settings WHERE key LIKE 'links.%'`)
	if err != nil {
		return s, err
	}
	defer rows.Close()
	for rows.Next() {
		var k string
		var v json.RawMessage
		if err := rows.Scan(&k, &v); err != nil {
			return s, err
		}
		switch k {
		case "links.types":
			_ = json.Unmarshal(v, &s.Types)
		case "links.sites":
			_ = json.Unmarshal(v, &s.Sites)
		case "links.reminder_days":
			_ = json.Unmarshal(v, &s.ReminderDays)
		case "links.usd_rate":
			_ = json.Unmarshal(v, &s.USDRate)
		}
	}
	return s, rows.Err()
}

func (s Settings) typeOf(key string) (LinkType, bool) {
	for _, t := range s.Types {
		if t.Key == key {
			return t, true
		}
	}
	return LinkType{}, false
}

func (h *Handler) getSettings(c *gin.Context) {
	s, err := loadSettings(c, h.conn)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, http.StatusOK, s)
}

var typeKey = regexp.MustCompile(`^[a-z][a-z0-9_]{1,39}$`)

func (s *Settings) validate() error {
	var v validate.V
	s.checkTypes(&v)
	s.checkSites(&v)
	for _, d := range s.ReminderDays {
		if d < 0 || d > 365 {
			v.Add("reminderDays", "Reminder days must be from 0 (the expiry date) to 365.")
		}
	}
	if len(s.ReminderDays) > 10 {
		v.Add("reminderDays", "Use at most 10 reminder days.")
	}
	if s.USDRate != nil && (*s.USDRate <= 0 || *s.USDRate > 100000) {
		v.Add("usdRate", "Enter the LKR per USD rate, or leave it empty.")
	}
	return v.Err()
}

func (s *Settings) checkTypes(v *validate.V) {
	if len(s.Types) == 0 || len(s.Types) > 50 {
		v.Add("types", "Keep from 1 to 50 link types.")
	}
	seen := map[string]bool{}
	for i := range s.Types {
		t := &s.Types[i]
		v.Text("types", &t.Label, 80, true, "Each link type's name")
		if !typeKey.MatchString(t.Key) || seen[t.Key] {
			v.Add("types", "Each link type needs its own short key: lower-case letters, digits and _.")
		}
		seen[t.Key] = true
	}
}

func (s *Settings) checkSites(v *validate.V) {
	if len(s.Sites) == 0 || len(s.Sites) > 100 {
		v.Add("sites", "Keep from 1 to 100 sites.")
	}
	seen := map[string]bool{}
	for i := range s.Sites {
		v.Text("sites", &s.Sites[i], 120, true, "Each site")
		if seen[strings.ToLower(s.Sites[i])] {
			v.Add("sites", "Each site can be listed once.")
		}
		seen[strings.ToLower(s.Sites[i])] = true
	}
}

// saveSettings replaces the lists. A type in use can be switched off but not
// removed, so no link is left with a type nobody can read.
func (h *Handler) saveSettings(c *gin.Context) {
	var in Settings
	if !httpx.Bind(c, &in) {
		return
	}
	if err := in.validate(); err != nil {
		httpx.Fail(c, err)
		return
	}
	slices.Sort(in.ReminderDays)
	slices.Reverse(in.ReminderDays)
	in.ReminderDays = slices.Compact(in.ReminderDays)
	keys := make([]string, 0, len(in.Types))
	for _, t := range in.Types {
		keys = append(keys, t.Key)
	}
	err := db.InTx(c, h.conn, func(tx pgx.Tx) error {
		var missing string
		if err := tx.QueryRow(c, `SELECT COALESCE(min(link_type), '') FROM network_links WHERE NOT (link_type = ANY($1::text[]))`, keys).Scan(&missing); err != nil {
			return err
		}
		if missing != "" {
			return httpx.Validation("types", "Links still use the type "+missing+". Switch it off instead of removing it.")
		}
		old, err := loadSettings(c, tx)
		if err != nil {
			return err
		}
		values := map[string]any{"types": in.Types, "sites": in.Sites, "reminderDays": in.ReminderDays, "usdRate": in.USDRate}
		for key, field := range settingKeys {
			raw, _ := json.Marshal(values[field])
			if _, err := tx.Exec(c, `INSERT INTO settings (key, value, updated_by) VALUES ($1, $2::jsonb, $3::uuid)
				ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, updated_at = now(), updated_by = EXCLUDED.updated_by`, key, string(raw), actor(c)); err != nil {
				return err
			}
		}
		return record(c, tx, "update", "link_settings", "links", old, in)
	})
	if err != nil {
		httpx.Fail(c, db.Friendly(err))
		return
	}
	h.getSettings(c)
}
