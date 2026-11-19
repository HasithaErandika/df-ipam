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
	Stop() error
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
	g.Handle(http.MethodPost, "/discovery/stop", h.stop, auth.DiscoveryScan)
	g.Handle(http.MethodGet, "/discovery/findings", h.findings, auth.InventoryView)
	g.Handle(http.MethodGet, "/discovery/summary", h.summary, auth.InventoryView)
}

var errUnavailable = httpx.NewError(http.StatusServiceUnavailable, "SCANNER_UNAVAILABLE", "The scanner is not running in this process.")

type statusDTO struct {
	scanner.Status
	Method          string `json:"method"`
	IntervalMinutes int    `json:"intervalMinutes"`
	// Which kinds of segment the scheduled scan covers (Settings > System).
	AutoDevices bool `json:"autoScanDevices"`
	AutoServers bool `json:"autoScanServers"`
	// How much scanning costs: the server-network interval, the probe rate,
	// and how many addresses one round of each kind covers.
	ServerIntervalMinutes int   `json:"serverIntervalMinutes"`
	RatePerSecond         int   `json:"ratePerSecond"`
	UserAddresses         int64 `json:"userAddresses"`
	ServerAddresses       int64 `json:"serverAddresses"`
}

// sweptSizes counts the addresses a round sweeps, per kind: usable addresses
// of each segment small enough to sweep (/20 or smaller).
const sweptSizes = `SELECT
	COALESCE(sum(n) FILTER (WHERE kind = 'dhcp'), 0)::bigint, COALESCE(sum(n) FILTER (WHERE kind = 'server'), 0)::bigint
	FROM (SELECT kind, CASE WHEN masklen(network) >= 31 THEN 2 ^ (32 - masklen(network)) ELSE 2 ^ (32 - masklen(network)) - 2 END AS n
		FROM segments WHERE kind IN ('dhcp', 'server') AND masklen(network) >= 20) x`

func (h *Handler) status(c *gin.Context) {
	if h.scan == nil {
		httpx.Fail(c, errUnavailable)
		return
	}
	out := statusDTO{Status: h.scan.Snapshot(), Method: "ping", IntervalMinutes: int(scanner.DefaultScanInterval.Minutes()), AutoDevices: true, AutoServers: true,
		ServerIntervalMinutes: int(scanner.DefaultServerScanInterval.Minutes()), RatePerSecond: scanner.DefaultRate}
	_ = h.conn.QueryRow(c, `SELECT COALESCE((SELECT value #>> '{}' FROM settings WHERE key = 'scan.method'), 'ping'),
		COALESCE((SELECT (value #>> '{}')::integer FROM settings WHERE key = 'scan.interval_minutes'), $1),
		COALESCE((SELECT (value #>> '{}')::boolean FROM settings WHERE key = 'scan.auto_devices'), true),
		COALESCE((SELECT (value #>> '{}')::boolean FROM settings WHERE key = 'scan.auto_servers'), true),
		COALESCE((SELECT (value #>> '{}')::integer FROM settings WHERE key = 'scan.server_interval_minutes'), $2),
		COALESCE((SELECT (value #>> '{}')::integer FROM settings WHERE key = 'scan.rate_per_second'), $3)`,
		out.IntervalMinutes, out.ServerIntervalMinutes, out.RatePerSecond).Scan(&out.Method, &out.IntervalMinutes, &out.AutoDevices, &out.AutoServers,
		&out.ServerIntervalMinutes, &out.RatePerSecond)
	_ = h.conn.QueryRow(c, sweptSizes).Scan(&out.UserAddresses, &out.ServerAddresses)
	// With both kinds off the scheduled scan does nothing, so there is no next scan to show.
	if !out.AutoDevices && !out.AutoServers {
		out.NextScanAt = nil
	}
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
