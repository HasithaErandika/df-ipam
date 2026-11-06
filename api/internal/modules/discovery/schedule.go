package discovery

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"
)

// Window is a time of week when active discovery may run.
type Window struct {
	Days  []int  `json:"days"` // 1 Monday ... 7 Sunday; empty means every day
	Start string `json:"start"`
	End   string `json:"end"` // may be earlier than Start, crossing midnight
}

func minutes(hhmm string) (int, error) {
	h, m, ok := strings.Cut(hhmm, ":")
	if !ok {
		return 0, fmt.Errorf("time %q must be HH:MM", hhmm)
	}
	hi, err1 := strconv.Atoi(h)
	mi, err2 := strconv.Atoi(m)
	if err1 != nil || err2 != nil || hi < 0 || hi > 23 || mi < 0 || mi > 59 {
		return 0, fmt.Errorf("time %q must be HH:MM", hhmm)
	}
	return hi*60 + mi, nil
}

// ValidateWindows checks window definitions.
func ValidateWindows(ws []Window) error {
	for _, w := range ws {
		if _, err := minutes(w.Start); err != nil {
			return err
		}
		if _, err := minutes(w.End); err != nil {
			return err
		}
		for _, d := range w.Days {
			if d < 1 || d > 7 {
				return fmt.Errorf("days run from 1 (Monday) to 7 (Sunday)")
			}
		}
	}
	return nil
}

// InWindow reports whether t falls in any window. No windows means any time.
func InWindow(ws []Window, t time.Time) bool {
	if len(ws) == 0 {
		return true
	}
	day := int(t.Weekday())
	if day == 0 {
		day = 7
	}
	now := t.Hour()*60 + t.Minute()
	return slices.ContainsFunc(ws, func(w Window) bool { return windowOpen(w, day, now) })
}

func windowOpen(w Window, day, now int) bool {
	start, err1 := minutes(w.Start)
	end, err2 := minutes(w.End)
	if err1 != nil || err2 != nil {
		return false
	}
	crossesMidnight := end < start
	if !windowDayMatches(w.Days, day, crossesMidnight && now < end) {
		return false
	}
	if crossesMidnight {
		return now >= start || now < end
	}
	return now >= start && now < end
}

// windowDayMatches checks the day. A window crossing midnight belongs to the day
// it started, so its early hours also count for the day before.
func windowDayMatches(days []int, day int, afterMidnight bool) bool {
	if len(days) == 0 {
		return true
	}
	previous := day - 1
	if previous == 0 {
		previous = 7
	}
	return slices.ContainsFunc(days, func(d int) bool { return d == day || (afterMidnight && d == previous) })
}

// Cron is a parsed five-field schedule: minute, hour, day of month, month, day of week.
type Cron struct{ fields [5]map[int]bool }

var cronRanges = [5][2]int{{0, 59}, {0, 23}, {1, 31}, {1, 12}, {0, 6}}

// ParseCron reads "*", "n", "a-b", "*/n", "a-b/n" and comma lists.
func ParseCron(spec string) (Cron, error) {
	var c Cron
	parts := strings.Fields(spec)
	if len(parts) != 5 {
		return c, fmt.Errorf("a schedule needs five fields: minute hour day month weekday")
	}
	for i, part := range parts {
		set := map[int]bool{}
		for _, item := range strings.Split(part, ",") {
			if err := addCronItem(set, i, item); err != nil {
				return c, err
			}
		}
		c.fields[i] = set
	}
	return c, nil
}

// addCronItem adds one comma-separated item of field i to set.
func addCronItem(set map[int]bool, field int, item string) error {
	step := 1
	if base, s, ok := strings.Cut(item, "/"); ok {
		n, err := strconv.Atoi(s)
		if err != nil || n < 1 {
			return fmt.Errorf("bad step in %q", item)
		}
		step, item = n, base
	}
	lo, hi := cronRanges[field][0], cronRanges[field][1]
	from, to, err := cronBounds(item, lo, hi)
	if err != nil {
		return err
	}
	if field == 4 && to == 7 { // Sunday as 7
		to = 6
		set[0] = true
	}
	if from < lo || to > hi || from > to {
		return fmt.Errorf("%q is out of range %d-%d", item, lo, hi)
	}
	for v := from; v <= to; v += step {
		set[v] = true
	}
	return nil
}

func cronBounds(item string, lo, hi int) (from, to int, err error) {
	if item == "*" {
		return lo, hi, nil
	}
	if a, b, ok := strings.Cut(item, "-"); ok {
		x, err1 := strconv.Atoi(a)
		y, err2 := strconv.Atoi(b)
		if err1 != nil || err2 != nil {
			return 0, 0, fmt.Errorf("bad range %q", item)
		}
		return x, y, nil
	}
	x, err := strconv.Atoi(item)
	if err != nil {
		return 0, 0, fmt.Errorf("bad value %q", item)
	}
	return x, x, nil
}

// Matches reports whether the schedule fires in the minute of t.
func (c Cron) Matches(t time.Time) bool {
	return c.fields[0][t.Minute()] && c.fields[1][t.Hour()] && c.fields[2][t.Day()] && c.fields[3][int(t.Month())] && c.fields[4][int(t.Weekday())]
}
