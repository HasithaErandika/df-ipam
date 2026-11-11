// Package seed loads reference data. Every statement is safe to run repeatedly, and
// never overwrites a value someone has changed.
package seed

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
)

// Environment is a default server environment from docs/new.md.
type Environment struct {
	Name, Colour, Severity string
	Monitor, Critical      bool
}

var Environments = []Environment{
	{"Production", "#dc2626", "critical", true, true},
	{"DR", "#9333ea", "critical", true, true},
	{"UAT", "#d97706", "high", true, false},
	{"Development", "#2563eb", "medium", false, false},
	{"Test", "#64748b", "medium", false, false},
}

// Settings are starting values for an Admin to review.
var Settings = []struct{ Key, Value string }{
	{"scan.interval_minutes", "10"},
	{"scan.method", `"ping"`},
	{"server_check.interval_seconds", "86400"},
	{"server_check.down_after_failures", "3"},
	{"reservations.reminder_days", "7"},
	{"email.enabled", "false"}, // email stays off until an Admin turns it on in Settings
	{"session.idle_timeout_minutes", "30"},
	{"fortigate.enabled", "false"}, // optional, read-only; set up in Settings > Integrations
	{"fortigate.verify_tls", "true"},
	{"fortigate.interval_minutes", "10"},
}

func quote(s string) string { return "'" + strings.ReplaceAll(s, "'", "''") + "'" }

// SQL returns the seed as plain SQL, used by Run and by the setup file.
func SQL() string {
	var b strings.Builder
	for i, e := range Environments {
		fmt.Fprintf(&b, "INSERT INTO environments (name, colour, sort_order, default_severity, default_monitor, default_critical) "+
			"VALUES (%s, %s, %d, %s, %t, %t) ON CONFLICT ((lower(name))) DO NOTHING;\n",
			quote(e.Name), quote(e.Colour), (i+1)*10, quote(e.Severity), e.Monitor, e.Critical)
	}
	for _, s := range Settings {
		fmt.Fprintf(&b, "INSERT INTO settings (key, value) VALUES (%s, %s::jsonb) ON CONFLICT (key) DO NOTHING;\n", quote(s.Key), quote(s.Value))
	}
	return b.String()
}

func Run(ctx context.Context, tx pgx.Tx) error {
	_, err := tx.Exec(ctx, SQL())
	return err
}
