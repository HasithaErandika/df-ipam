package notifications

import (
	"net/http"

	"dfipam/api/internal/auth"
	"dfipam/api/internal/notify"
	"dfipam/api/internal/platform/httpx"

	"github.com/gin-gonic/gin"
)

// testEmail sends a sample to the person asking, and to the team mailbox when
// one is set, so an Admin can check the mail server and how email looks. It
// works whether or not email is switched on, and writes no notice.
func (h *Handler) testEmail(c *gin.Context) {
	if h.mailer == nil {
		httpx.Fail(c, httpx.NewError(http.StatusConflict, "SMTP_OFF", "No mail server is set in api/.env (SMTP_HOST), so DF-Atlas can't send email."))
		return
	}
	to := []string{auth.Current(c).Email}
	if team := loadConfig(c, h.conn).Team; team != "" && team != to[0] {
		to = append(to, team)
	}
	for _, addr := range to {
		letter, err := notify.Render(sample(), h.base, addr)
		if err == nil {
			err = h.mailer.Send(c, letter)
		}
		if err != nil {
			httpx.Fail(c, httpx.NewError(http.StatusBadGateway, "EMAIL_FAILED", "The email to "+addr+" couldn't be sent: "+err.Error()))
			return
		}
	}
	httpx.OK(c, http.StatusOK, gin.H{"sentTo": to})
}

// sample looks like a real digest, with made-up rows, so the layout can be judged.
func sample() notify.Email {
	return notify.Email{
		Subject: "Test email", Preheader: "Email from DF-Atlas is working",
		Heading: "Email from DF-Atlas is working",
		Intro:   "This is a test from System › Settings › Email. Real emails look like this. The rows below are examples, not real alerts.",
		Sections: []notify.Section{
			{Title: "Alerts", Rows: []notify.Row{
				{Tone: "danger", Chip: "Critical", Text: "Server DB-01 answered with a different MAC address", Meta: "Example", Path: "/alerts"},
				{Tone: "warn", Chip: "High", Text: "Unregistered IP answers in a server network", Meta: "Example", Path: "/alerts"},
			}},
			{Title: "Waiting for a decision", Rows: []notify.Row{{Tone: "accent", Chip: "Requests", Text: "2 IP requests to approve", Meta: "Example", Path: "/requests"}}},
		},
		Action: notify.Action{Label: "Open DF-Atlas", Path: "/"},
		Why:    "You asked for this test email.",
	}
}
