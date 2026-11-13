package seed

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestEnvironmentsFollowThePlan(t *testing.T) {
	want := map[string]string{"Production": "critical", "DR": "critical", "UAT": "high", "Development": "medium", "Test": "medium"}
	if len(Environments) != len(want) {
		t.Fatalf("want %d environments, got %d", len(want), len(Environments))
	}
	for _, e := range Environments {
		if want[e.Name] != e.Severity {
			t.Errorf("%s: severity %s, want %s", e.Name, e.Severity, want[e.Name])
		}
	}
}

func TestSettingsAreJSON(t *testing.T) {
	for _, s := range Settings {
		if !json.Valid([]byte(s.Value)) {
			t.Errorf("%s: %q is not JSON", s.Key, s.Value)
		}
	}
}

func TestSQLIsRepeatable(t *testing.T) {
	for _, line := range strings.Split(strings.TrimSpace(SQL()), "\n") {
		if !strings.Contains(line, "ON CONFLICT") {
			t.Errorf("not repeatable: %s", line)
		}
	}
}

// Email starts off: the plan is to test with it disabled (docs/product/alerts.md).
func TestEmailStartsDisabled(t *testing.T) {
	for _, s := range Settings {
		if s.Key == "email.enabled" {
			if s.Value != "false" {
				t.Fatalf("email.enabled seeds as %s", s.Value)
			}
			return
		}
	}
	t.Fatal("email.enabled is not seeded")
}
