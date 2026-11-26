package notify

import (
	"strings"
	"testing"
)

// Values from the network (host names, device names) are escaped in the HTML,
// and both versions carry the content and the link into the app.
func TestRenderEscapesAndLinks(t *testing.T) {
	e := Email{Subject: "Test", Heading: "Heading", Sections: []Section{{Title: "Alerts", Rows: []Row{
		{Tone: "danger", Chip: "Critical", Text: `<script>alert(1)</script> answered`, Path: "/alerts?q=10.0.0.5"},
	}, More: 3}}, Action: Action{Label: "Open", Path: "/alerts"}, Why: "Because."}
	l, err := Render(e, "https://ipam.example/", "a@example.com")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(l.HTML, "<script>") || !strings.Contains(l.HTML, "&lt;script&gt;") {
		t.Error("network text not escaped")
	}
	if !strings.Contains(l.HTML, `href="https://ipam.example/alerts?q=10.0.0.5"`) {
		t.Errorf("row link missing:\n%s", l.HTML)
	}
	for _, want := range []string{"[Critical]", "and 3 more", "Open: https://ipam.example/alerts"} {
		if !strings.Contains(l.Text, want) {
			t.Errorf("text version missing %q:\n%s", want, l.Text)
		}
	}
	if l.Subject != "DF-Atlas · Test" {
		t.Errorf("subject %q", l.Subject)
	}
}
