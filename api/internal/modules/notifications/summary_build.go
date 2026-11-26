package notifications

import (
	"fmt"
	"time"

	"dfipam/api/internal/auth"
	"dfipam/api/internal/notify"
)

// summary builds each reader's daily summary from the day's facts.
type summary struct {
	f      facts
	top    []openAlert
	health []notify.Row
	day    time.Time
}

// build returns the summary for one person (p), or for the team mailbox (p
// nil). team says whether to include the team sections, which a person gets
// only when no team mailbox takes them. ok is false when there is nothing to say.
func (s summary) build(p *auth.Principal, team bool) (notify.Email, bool) {
	can := func(perm string) bool { return team && (p == nil || p.Has(perm)) }
	var sections []notify.Section
	add := func(sec notify.Section, show bool) {
		if show && len(sec.Rows) > 0 {
			sections = append(sections, sec)
		}
	}
	add(s.alerts(), can(auth.AlertsManage))
	add(s.waiting(can(auth.InventoryEdit), can(auth.RequestsApprove)), true)
	add(s.capacity(), can(auth.NetworkManage))
	add(s.background(), can(auth.SystemManage))
	add(s.contracts(p, can(auth.LinksCommercialView)), true)
	add(s.holds(p), p != nil)
	if len(sections) == 0 {
		return notify.Email{}, false
	}
	why := "You get this summary because of your role in DF-Atlas."
	if p == nil {
		why = teamWhy
	}
	date := s.day.Format("Monday 2 January")
	return notify.Email{
		Subject: "Daily summary · " + s.day.Format("Mon 2 Jan"), Preheader: "What needs attention today",
		Heading:  "Daily summary for " + date,
		Intro:    "What needs attention today. Only items that need someone are listed; everything else is in DF-Atlas.",
		Sections: sections, Action: notify.Action{Label: "Open DF-Atlas", Path: "/"}, Why: why,
	}, true
}

func (s summary) alerts() notify.Section {
	a := s.f.Alerts
	if a.Critical+a.High+a.Medium+a.Low == 0 {
		return notify.Section{}
	}
	sec := notify.Section{Title: "Open alerts",
		Note: fmt.Sprintf("%d Critical · %d High · %d Medium · %d Low · %d opened in the last 24 hours", a.Critical, a.High, a.Medium, a.Low, a.New)}
	for _, x := range s.top {
		sv := severities[x.Severity]
		sec.Rows = append(sec.Rows, notify.Row{Chip: sv[0], Tone: sv[1], Text: x.Message, Meta: "Open since " + when(x.At), Path: "/alerts"})
	}
	if len(sec.Rows) == 0 {
		sec.Rows = []notify.Row{{Tone: "ok", Chip: "OK", Text: "No Critical or High alerts are open.", Path: "/alerts"}}
	}
	if more := a.Critical + a.High - len(s.top); more > 0 {
		sec.More = more
	}
	return sec
}

func (s summary) waiting(devices, requests bool) notify.Section {
	sec := notify.Section{Title: "Waiting for a decision"}
	if devices && s.f.Devices > 0 {
		sec.Rows = append(sec.Rows, notify.Row{Tone: "accent", Chip: "Devices", Text: plural(s.f.Devices, "new device to approve or reject", "new devices to approve or reject"), Path: "/devices"})
	}
	if r := s.f.Requests; requests && r.Count > 0 {
		row := notify.Row{Tone: "accent", Chip: "Requests", Text: plural(r.Count, "IP request to approve", "IP requests to approve"), Path: "/requests"}
		if r.Oldest != nil {
			row.Meta = "Oldest from " + when(*r.Oldest)
		}
		sec.Rows = append(sec.Rows, row)
	}
	return sec
}

func (s summary) capacity() notify.Section {
	sec := notify.Section{Title: "Networks running out of addresses", Note: "90% or more in use. DHCP pools count addresses seen in the last 24 hours."}
	for _, c := range s.f.Capacity {
		pct := c.Used * 100 / c.Size
		tone := "warn"
		if pct >= 98 {
			tone = "danger"
		}
		sec.Rows = append(sec.Rows, notify.Row{Tone: tone, Chip: fmt.Sprintf("%d%%", pct), Text: c.Name + " · " + c.What,
			Meta: fmt.Sprintf("%d of %d addresses", c.Used, c.Size), Path: "/segments/" + c.ID})
	}
	return sec
}

func (s summary) background() notify.Section {
	sec := notify.Section{Title: "Background reads and scans", Rows: append([]notify.Row(nil), s.health...)}
	for _, f := range s.f.Failing {
		sec.Rows = append(sec.Rows, notify.Row{Tone: "danger", Chip: "Scan", Text: "Scanning " + f.Name + " is failing",
			Meta: "Since " + when(f.Since), Path: "/segments/" + f.ID})
	}
	return sec
}

// contracts lists contracts ending within 30 days: all of them for those who
// see link costs, otherwise only links the person is responsible for.
func (s summary) contracts(p *auth.Principal, all bool) notify.Section {
	sec := notify.Section{Title: "Network link contracts ending soon"}
	for _, c := range s.f.Contracts {
		mine := p != nil && c.Officer != nil && *c.Officer == p.UserID
		if !all && !mine {
			continue
		}
		days := int(c.End.Sub(time.Date(s.day.Year(), s.day.Month(), s.day.Day(), 0, 0, 0, 0, time.UTC)).Hours() / 24)
		tone, left := "warn", fmt.Sprintf("in %d days", days)
		if days <= 7 {
			tone = "danger"
		}
		if days == 0 {
			left = "today"
		}
		sec.Rows = append(sec.Rows, notify.Row{Tone: tone, Chip: "Ends " + left, Text: c.Code + " · " + c.Name,
			Meta: "Contract ends " + c.End.Format("2 Jan 2006"), Path: "/network-links/" + c.ID})
	}
	return sec
}

func (s summary) holds(p *auth.Principal) notify.Section {
	sec := notify.Section{Title: "Your reservations expiring soon", Note: "Extend, assign or release them before they expire."}
	for _, h := range s.f.Holds {
		if p != nil && h.Owner == p.UserID {
			sec.Rows = append(sec.Rows, notify.Row{Tone: "warn", Chip: "Expires", Text: h.Reason + " (" + h.Range + ")",
				Meta: "Held until " + when(h.Expires), Path: "/reservations"})
		}
	}
	return sec
}

func when(t time.Time) string { return t.In(sriLanka).Format("Mon 2 Jan, 15:04") }
