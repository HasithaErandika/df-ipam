package notifications

import (
	"strings"
	"testing"
	"time"
)

func TestUniqueSkipsActorAndDuplicates(t *testing.T) {
	got := unique([]string{"a", "b", "a", "", "me", "c"}, "me")
	if strings.Join(got, ",") != "a,b,c" {
		t.Fatalf("got %v", got)
	}
}

func TestMergePreferencesDefaultsOn(t *testing.T) {
	got := MergePreferences([]Preference{{ApprovalRequested, "email", false}})
	if len(got) != len(EventTypes)*len(Channels) {
		t.Fatalf("got %d entries", len(got))
	}
	for _, p := range got {
		want := !(p.EventType == ApprovalRequested && p.Channel == "email")
		if p.IsEnabled != want {
			t.Errorf("%s/%s: got %v", p.EventType, p.Channel, p.IsEnabled)
		}
	}
}

func TestEventTypesAreUnique(t *testing.T) {
	seen := map[string]bool{}
	for _, e := range EventTypes {
		if seen[e.Type] || !severities[e.Severity] || e.Label == "" {
			t.Errorf("bad event type %+v", e)
		}
		seen[e.Type] = true
	}
}

func TestBackoff(t *testing.T) {
	for attempts, want := range map[int]time.Duration{0: time.Minute, 1: time.Minute, 2: 4 * time.Minute, 3: 16 * time.Minute, 9: 4 * time.Hour} {
		if got := Backoff(attempts); got != want {
			t.Errorf("%d: got %v want %v", attempts, got, want)
		}
	}
}

func TestEmailText(t *testing.T) {
	subject, body := EmailText(Payload{Title: "Approval needed", Body: "Please look.", Link: "/requests?id=1"}, "https://ipam.example.com/")
	if subject != "[DF-IPAM] Approval needed" {
		t.Errorf("subject %q", subject)
	}
	if !strings.Contains(body, "https://ipam.example.com/requests?id=1") || !strings.HasPrefix(body, "Please look.") {
		t.Errorf("body %q", body)
	}
	if _, body := EmailText(Payload{Title: "x", Body: "y", Link: "/a"}, ""); strings.Contains(body, "/a") {
		t.Errorf("relative link without a base URL: %q", body)
	}
}
