package notify

import (
	"bytes"
	"fmt"
	"html/template"
	"strings"

	"dfipam/api/internal/platform/mail"
)

// Email is what one message says, laid out the same way for every kind: a
// heading, a short introduction, sections of rows and one button. Render turns
// it into HTML and a plain-text copy.
type Email struct {
	Subject   string
	Preheader string // the line mail apps show after the subject
	Heading   string
	Intro     string
	Sections  []Section
	Action    Action
	Why       string // why this person gets it, at the foot
}

// Section is a titled list. More counts rows left out for length.
type Section struct {
	Title string
	Note  string
	Rows  []Row
	More  int
}

// Row is one line: an optional coloured label, the text, a quieter detail
// and a link into the app.
type Row struct {
	Tone string // danger, warn, ok, accent or neutral
	Chip string
	Text string
	Meta string
	Path string
}

// Action is the main button: a label and an app path.
type Action struct{ Label, Path string }

// tones are the app's own colours (web/src/index.css), as text and background.
var tones = map[string][2]string{
	"danger": {"#c02f2f", "#fcebea"}, "warn": {"#a15c00", "#fdf1dc"}, "ok": {"#17794f", "#e2f4ec"},
	"accent": {"#2257c9", "#e8effc"}, "neutral": {"#4d586a", "#f2f4f8"},
}

var page = template.Must(template.New("email").Funcs(template.FuncMap{
	"fg":  func(t string) string { return tone(t)[0] },
	"bg":  func(t string) string { return tone(t)[1] },
	"url": func(base, path string) string { return base + path },
}).Parse(layout))

func tone(t string) [2]string {
	if c, ok := tones[t]; ok {
		return c
	}
	return tones["neutral"]
}

// Render builds the message for one address. Every value is escaped in the
// HTML; links point into the app at baseURL.
func Render(e Email, baseURL, to string) (mail.Letter, error) {
	var html bytes.Buffer
	if err := page.Execute(&html, struct {
		Email
		Base string
	}{e, strings.TrimRight(baseURL, "/")}); err != nil {
		return mail.Letter{}, err
	}
	return mail.Letter{To: to, Subject: "DF-Atlas · " + e.Subject, Text: text(e, strings.TrimRight(baseURL, "/")), HTML: html.String()}, nil
}

// text is the plain version, for clients that don't show HTML.
func text(e Email, base string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s\n\n", e.Heading)
	if e.Intro != "" {
		fmt.Fprintf(&b, "%s\n\n", e.Intro)
	}
	for _, s := range e.Sections {
		fmt.Fprintf(&b, "%s\n%s\n", strings.ToUpper(s.Title), strings.Repeat("-", len(s.Title)))
		if s.Note != "" {
			fmt.Fprintf(&b, "%s\n", s.Note)
		}
		for _, r := range s.Rows {
			line := "- " + r.Text
			if r.Chip != "" {
				line = "- [" + r.Chip + "] " + r.Text
			}
			if r.Meta != "" {
				line += " (" + r.Meta + ")"
			}
			fmt.Fprintln(&b, line)
		}
		if s.More > 0 {
			fmt.Fprintf(&b, "- and %d more\n", s.More)
		}
		b.WriteString("\n")
	}
	if e.Action.Label != "" {
		fmt.Fprintf(&b, "%s: %s%s\n\n", e.Action.Label, base, e.Action.Path)
	}
	fmt.Fprintf(&b, "-- \nDF-Atlas, Dialog Finance network and IP address management (IPAM). %s\n", e.Why)
	return b.String()
}
