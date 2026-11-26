package notifications

import (
	"fmt"
	"strings"
	"time"

	"dfipam/api/internal/notify"
)

// sriLanka is the building's time zone (no daylight saving), fixed so the
// container needs no time zone database.
var sriLanka = time.FixedZone("Sri Lanka", 5*3600+1800)

// severities label and colour alerts as the app does.
var severities = map[string][2]string{
	"critical": {"Critical", "danger"}, "high": {"High", "warn"}, "medium": {"Medium", "accent"}, "low": {"Low", "neutral"},
}

// kindText is how each kind reads: the heading of its own email, the title of
// its digest section, and why the person gets it.
var kindText = map[string]struct{ Heading, Section, Why, Action, Path string }{
	"alert":            {"New alert", "Alerts", "You get this because you handle alerts.", "Review in Alerts", "/alerts"},
	"request":          {"New IP request", "IP requests waiting for approval", "You get this because you approve IP requests.", "Review requests", "/requests"},
	"request_decision": {"Your IP request was decided", "Your IP requests", "You get this because you asked for this address.", "Open your requests", "/requests"},
}

const teamWhy = "Sent to the team mailbox set in System › Settings › Email."

func row(n queued) notify.Row {
	r := notify.Row{Text: n.Message, Meta: n.At.In(sriLanka).Format("Mon 2 Jan, 15:04"), Path: n.Link}
	if s, ok := severities[n.Severity]; ok {
		r.Chip, r.Tone = s[0], s[1]
	}
	return r
}

func why(n queued) string {
	if n.Key == "team" {
		return teamWhy
	}
	return kindText[n.Kind].Why
}

// single is the email for one notice sent at once.
func single(n queued) notify.Email {
	t := kindText[n.Kind]
	heading, subject := t.Heading, t.Heading+": "+clip(n.Message, 90)
	if s, ok := severities[n.Severity]; ok && n.Kind == "alert" {
		heading, subject = s[0]+" alert", s[0]+" alert: "+clip(n.Message, 90)
	}
	path := n.Link
	if path == "" {
		path = t.Path
	}
	return notify.Email{
		Subject: subject, Preheader: n.Message, Heading: heading,
		Intro:    intro(n.Kind),
		Sections: []notify.Section{{Title: t.Section, Rows: []notify.Row{row(n)}}},
		Action:   notify.Action{Label: t.Action, Path: path}, Why: why(n),
	}
}

func intro(kind string) string {
	if kind == "alert" {
		return "DF-IPAM found this on the network just now. It stays open in Alerts until the cause is gone or someone acknowledges it."
	}
	return ""
}

// digest gathers one recipient's waiting notices: a section per kind, alerts
// first and most severe first.
func digest(items []queued, minutes int) notify.Email {
	order := []string{"alert", "request", "request_decision"}
	counts := map[string]int{}
	sections := []notify.Section{}
	for _, kind := range order {
		var rows []notify.Row
		for _, n := range sortBySeverity(items) {
			if n.Kind == kind {
				rows = append(rows, row(n))
			}
		}
		if len(rows) == 0 {
			continue
		}
		counts[kind] = len(rows)
		s := notify.Section{Title: kindText[kind].Section, Rows: rows}
		if len(rows) > 15 {
			s.Rows, s.More = rows[:15], len(rows)-15
		}
		sections = append(sections, s)
	}
	action := kindText[order[0]]
	if counts["alert"] == 0 {
		action = kindText["request"]
	}
	subject := summaryLine(counts)
	return notify.Email{
		Subject: subject, Preheader: subject, Heading: "What's new in DF-IPAM",
		Intro:    fmt.Sprintf("Gathered over the last %d minutes, so you get one email instead of many. Critical alerts are sent on their own as they happen.", minutes),
		Sections: sections, Action: notify.Action{Label: action.Action, Path: action.Path}, Why: why(items[0]),
	}
}

func summaryLine(counts map[string]int) string {
	var parts []string
	if n := counts["alert"]; n > 0 {
		parts = append(parts, plural(n, "new alert", "new alerts"))
	}
	if n := counts["request"]; n > 0 {
		parts = append(parts, plural(n, "IP request to approve", "IP requests to approve"))
	}
	if n := counts["request_decision"]; n > 0 {
		parts = append(parts, plural(n, "request decided", "requests decided"))
	}
	return strings.Join(parts, " and ")
}

func sortBySeverity(items []queued) []queued {
	rank := map[string]int{"critical": 0, "high": 1, "medium": 2, "low": 3}
	out := append([]queued(nil), items...)
	for i := 1; i < len(out); i++ { // insertion sort: short lists, keeps time order within a severity
		for j := i; j > 0 && rank[out[j].Severity] < rank[out[j-1].Severity]; j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out
}

func plural(n int, one, many string) string {
	if n == 1 {
		return "1 " + one
	}
	return fmt.Sprintf("%d %s", n, many)
}

func clip(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
}
