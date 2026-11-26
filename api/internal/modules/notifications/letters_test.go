package notifications

import (
	"strings"
	"testing"
	"time"

	"dfipam/api/internal/auth"
	"dfipam/api/internal/notify"
)

// A digest puts alerts first, most severe first, and says what it holds.
func TestDigestGroupsAndOrders(t *testing.T) {
	at := time.Date(2026, 11, 26, 4, 0, 0, 0, time.UTC)
	e := digest([]queued{
		{Key: "u1", Kind: "request", Message: "New IP request", At: at},
		{Key: "u1", Kind: "alert", Severity: "high", Message: "High one", At: at},
		{Key: "u1", Kind: "alert", Severity: "critical", Message: "Critical one", At: at},
	}, 30)
	if e.Subject != "2 new alerts and 1 IP request to approve" {
		t.Errorf("subject %q", e.Subject)
	}
	if len(e.Sections) != 2 || e.Sections[0].Title != "Alerts" || e.Sections[0].Rows[0].Chip != "Critical" {
		t.Errorf("sections %+v", e.Sections)
	}
	if e.Sections[0].Rows[0].Meta != "Thu 26 Nov, 09:30" {
		t.Errorf("time not in Sri Lanka: %q", e.Sections[0].Rows[0].Meta)
	}
}

// Each person sees only what their role covers; a quiet day sends nothing; the
// team mailbox gets the team sections and people then get only their own.
func TestSummaryFollowsPermissions(t *testing.T) {
	day := time.Date(2026, 11, 26, 8, 0, 0, 0, sriLanka)
	s := summary{day: day, f: facts{Alerts: alertFacts{High: 2}, Devices: 4,
		Holds: []hold{{Owner: "u2", Reason: "Lab", Range: "10.0.0.5", Expires: day.Add(48 * time.Hour)}}},
		top: []openAlert{{Severity: "high", Message: "Unregistered IP", At: day}}}
	viewer := auth.NewPrincipal("u1", "v@example.com", "viewer", []string{auth.InventoryView}, false)
	if _, ok := s.build(viewer, true); ok {
		t.Error("a viewer with nothing of their own got a summary")
	}
	handler := auth.NewPrincipal("u3", "h@example.com", "it_officer", []string{auth.AlertsManage, auth.InventoryEdit}, false)
	e, ok := s.build(handler, true)
	if !ok || titles(e) != "Open alerts|Waiting for a decision" {
		t.Errorf("alert handler got %q", titles(e))
	}
	if _, ok := s.build(handler, false); ok {
		t.Error("with a team mailbox, a person got team sections")
	}
	owner := auth.NewPrincipal("u2", "o@example.com", "requester", []string{auth.IPReserve}, false)
	if e, ok := s.build(owner, false); !ok || titles(e) != "Your reservations expiring soon" {
		t.Errorf("owner got %q", titles(e))
	}
	if e, ok := s.build(nil, true); !ok || !strings.Contains(titles(e), "Open alerts") || strings.Contains(titles(e), "Your reservations") {
		t.Errorf("team mailbox got %q", titles(e))
	}
}

func titles(e notify.Email) string {
	var out []string
	for _, s := range e.Sections {
		out = append(out, s.Title)
	}
	return strings.Join(out, "|")
}
