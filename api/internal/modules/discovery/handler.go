// Package discovery shows what the scanner is doing and everything it has seen,
// and lets an Admin start a scan. It never changes the register or devices.
package discovery

import (
	"errors"
	"net/http"

	"dfipam/api/internal/audit"
	"dfipam/api/internal/auth"
	"dfipam/api/internal/platform/db"
	"dfipam/api/internal/platform/httpx"
	"dfipam/api/internal/scanner"
	"dfipam/api/internal/validate"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
)

// Scanner is the part of the background scanner this module uses.
type Scanner interface {
	Trigger(segmentID, by string) error
	Snapshot() scanner.Status
}

type Handler struct {
	conn db.Conn
	scan Scanner
}

func New(conn db.Conn, scan Scanner) *Handler { return &Handler{conn: conn, scan: scan} }

func (h *Handler) Register(g *auth.Guard) {
	g.Handle(http.MethodGet, "/discovery/status", h.status, auth.InventoryView)
	g.Handle(http.MethodPost, "/discovery/scan", h.start, auth.DiscoveryScan)
	g.Handle(http.MethodGet, "/discovery/findings", h.findings, auth.InventoryView)
	g.Handle(http.MethodGet, "/discovery/summary", h.summary, auth.InventoryView)
}

var errUnavailable = httpx.NewError(http.StatusServiceUnavailable, "SCANNER_UNAVAILABLE", "The scanner is not running in this process.")

type statusDTO struct {
	scanner.Status
	Method          string `json:"method"`
	IntervalMinutes int    `json:"intervalMinutes"`
}

func (h *Handler) status(c *gin.Context) {
	if h.scan == nil {
		httpx.Fail(c, errUnavailable)
		return
	}
	out := statusDTO{Status: h.scan.Snapshot(), Method: "ping", IntervalMinutes: 5}
	_ = h.conn.QueryRow(c, `SELECT COALESCE((SELECT value #>> '{}' FROM settings WHERE key = 'scan.method'), 'ping'),
		COALESCE((SELECT (value #>> '{}')::integer FROM settings WHERE key = 'scan.interval_minutes'), 5)`).Scan(&out.Method, &out.IntervalMinutes)
	httpx.OK(c, http.StatusOK, out)
}

// start asks for a scan now. The scanner refuses while one runs and spaces manual
// scans a minute apart, so repeated clicks can't flood the network.
func (h *Handler) start(c *gin.Context) {
	if h.scan == nil {
		httpx.Fail(c, errUnavailable)
		return
	}
	var in struct {
		SegmentID string `json:"segmentId"`
	}
	if c.Request.ContentLength > 0 && !httpx.Bind(c, &in) {
		return
	}
	var v validate.V
	v.UUID("segmentId", in.SegmentID, false, "a segment")
	if err := v.Err(); err != nil {
		httpx.Fail(c, err)
		return
	}
	uid := auth.Current(c).UserID
	err := db.InTx(c, h.conn, func(tx pgx.Tx) error {
		if in.SegmentID != "" {
			var ok bool
			if err := tx.QueryRow(c, "SELECT EXISTS (SELECT 1 FROM segments WHERE id = $1::uuid)", in.SegmentID).Scan(&ok); err != nil || !ok {
				return httpx.ErrNotFound
			}
		}
		if err := audit.Write(c, tx, audit.Entry{ActorID: uid, Action: "scan", ObjectType: "discovery", ObjectID: in.SegmentID, New: in}); err != nil {
			return err
		}
		return h.scan.Trigger(in.SegmentID, "manual") // last, so a refusal rolls the audit row back
	})
	switch {
	case errors.Is(err, scanner.ErrBusy):
		httpx.Fail(c, httpx.NewError(http.StatusConflict, "SCAN_RUNNING", "A scan is already running. Wait for it to finish."))
	case errors.Is(err, scanner.ErrTooSoon):
		httpx.Fail(c, httpx.NewError(http.StatusTooManyRequests, "SCAN_TOO_SOON", "A scan was started less than a minute ago. Try again shortly."))
	case err != nil:
		httpx.Fail(c, db.Friendly(err))
	default:
		httpx.OK(c, http.StatusAccepted, gin.H{"queued": true})
	}
}
