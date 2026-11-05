// Package dashboard serves summary counts.
package dashboard

import (
	"net/http"
	"strconv"

	"dfipam/api/internal/auth"
	"dfipam/api/internal/platform/db"
	"dfipam/api/internal/platform/httpx"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
)

type Summary struct {
	SubnetsV4          int64 `json:"subnetsV4"`
	SubnetsV6          int64 `json:"subnetsV6"`
	IPRecords          int64 `json:"ipRecords"`
	Allocated          int64 `json:"allocated"`
	Reserved           int64 `json:"reserved"`
	AvailableCandidate int64 `json:"availableCandidates"`
	ObservedActive     int64 `json:"observedActive"`
	UnknownObservation int64 `json:"unknownObservation"`
	ConflictSuspected  int64 `json:"conflictSuspected"`
	Devices            int64 `json:"devices"`
	Quarantined        int64 `json:"quarantined"`
	OpenFindings       int64 `json:"openFindings"`
	PendingApprovals   int64 `json:"pendingApprovals"`
	ReadyToFulfil      int64 `json:"readyToFulfil"`
	ExpiringSoon       int64 `json:"reservationsExpiringSoon"`
	HighUtilisation    int64 `json:"highUtilisationSubnets"`
	IntegrationsOK     int64 `json:"integrationsHealthy"`
	IntegrationsBad    int64 `json:"integrationsFailing"`
	AgentsOnline       int64 `json:"agentsOnline"`
	AgentsOffline      int64 `json:"agentsOffline"`
	JobsRunning        int64 `json:"discoveryJobsRunning"`
	JobsFailed         int64 `json:"discoveryJobsFailedToday"`
}

// TrendPoint is one day. Counts are strings because IPv6 totals can pass what a
// JSON number holds exactly.
type TrendPoint struct {
	Date           string `json:"date"`
	Allocated      string `json:"allocated"`
	Reserved       string `json:"reserved"`
	ObservedActive string `json:"observedActive"`
	AveragePct     string `json:"averagePct"`
	NewFindings    int64  `json:"newFindings"`
	NewConflicts   int64  `json:"newConflicts"`
	Observations   int64  `json:"observations"`
}

// Slice is one part of a distribution.
type Slice struct {
	Label string `json:"label"`
	Count int64  `json:"count"`
}

// Distribution groups active subnets.
type Distribution struct {
	Environments []Slice `json:"environments"`
	Sites        []Slice `json:"sites"`
	Families     []Slice `json:"families"`
}

type Handler struct{ pool db.Conn }

func NewHandler(pool db.Conn) *Handler { return &Handler{pool} }

func (h *Handler) Register(g *auth.Guard) {
	g.Handle(http.MethodGet, "/dashboard/summary", h.summary, auth.InventoryView)
	g.Handle(http.MethodGet, "/dashboard/trends", h.trends, auth.InventoryView)
	g.Handle(http.MethodGet, "/dashboard/distribution", h.distribution, auth.InventoryView)
}

func (h *Handler) summary(c *gin.Context) {
	var s Summary
	err := h.pool.QueryRow(c.Request.Context(), `
		SELECT
		  (SELECT count(*) FROM subnets WHERE archived_at IS NULL AND family(cidr) = 4),
		  (SELECT count(*) FROM subnets WHERE archived_at IS NULL AND family(cidr) = 6),
		  count(*),
		  count(*) FILTER (WHERE allocation_state = 'allocated'),
		  count(*) FILTER (WHERE allocation_state = 'reserved'),
		  count(*) FILTER (WHERE allocation_state = 'available_candidate'),
		  count(*) FILTER (WHERE observation_state = 'observed_active'),
		  count(*) FILTER (WHERE observation_state = 'unknown'),
		  count(*) FILTER (WHERE observation_state = 'conflict_suspected'),
		  (SELECT count(*) FROM devices),
		  count(*) FILTER (WHERE allocation_state = 'quarantined'),
		  (SELECT count(*) FROM reconciliation_results WHERE review_status IN ('open', 'investigating')),
		  (SELECT count(*) FROM ip_requests WHERE status = 'pending_approval'),
		  (SELECT count(*) FROM ip_requests WHERE status = 'approved'),
		  (SELECT count(*) FROM ip_reservations WHERE status = 'active' AND expires_at <= now() + interval '7 days'),
		  (SELECT count(*) FROM subnet_utilisation_snapshots n JOIN subnets s ON s.id = n.subnet_id
		   WHERE s.archived_at IS NULL AND n.utilisation_pct >= s.warn_threshold
		     AND n.captured_at = (SELECT max(captured_at) FROM subnet_utilisation_snapshots)),
		  (SELECT count(*) FROM integration_connections WHERE is_enabled AND health = 'healthy'),
		  (SELECT count(*) FROM integration_connections WHERE is_enabled AND health IN ('degraded', 'failed')),
		  (SELECT count(*) FROM discovery_agents WHERE status = 'active' AND last_heartbeat_at > now() - interval '5 minutes'),
		  (SELECT count(*) FROM discovery_agents WHERE status = 'active' AND (last_heartbeat_at IS NULL OR last_heartbeat_at <= now() - interval '5 minutes')),
		  (SELECT count(*) FROM discovery_jobs WHERE status = 'running'),
		  (SELECT count(*) FROM discovery_jobs WHERE status = 'failed' AND finished_at > now() - interval '1 day')
		FROM ip_addresses`).Scan(&s.SubnetsV4, &s.SubnetsV6, &s.IPRecords, &s.Allocated, &s.Reserved,
		&s.AvailableCandidate, &s.ObservedActive, &s.UnknownObservation, &s.ConflictSuspected, &s.Devices, &s.Quarantined, &s.OpenFindings,
		&s.PendingApprovals, &s.ReadyToFulfil, &s.ExpiringSoon, &s.HighUtilisation, &s.IntegrationsOK, &s.IntegrationsBad,
		&s.AgentsOnline, &s.AgentsOffline, &s.JobsRunning, &s.JobsFailed)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, http.StatusOK, s)
}

func (h *Handler) trends(c *gin.Context) {
	days, err := strconv.Atoi(c.DefaultQuery("days", "30"))
	if err != nil || days < 7 || days > 365 {
		httpx.Fail(c, httpx.Validation("days", "Choose between 7 and 365 days."))
		return
	}
	rows, err := h.pool.Query(c.Request.Context(), `
		WITH days AS (SELECT d::date AS day FROM generate_series(current_date - ($1::int - 1), current_date, interval '1 day') d),
		usage AS (SELECT captured_at::date AS day, sum(allocated) a, sum(reserved) r, sum(observed_active) o, round(avg(utilisation_pct), 2) p
		          FROM subnet_utilisation_snapshots WHERE captured_at >= current_date - ($1::int - 1) GROUP BY 1),
		found AS (SELECT created_at::date AS day, count(*) n, count(*) FILTER (WHERE classification = 'ip_conflict_suspected') c
		          FROM reconciliation_results WHERE created_at >= current_date - ($1::int - 1) GROUP BY 1),
		seen AS (SELECT observed_at::date AS day, count(*) n FROM discovery_observations WHERE observed_at >= current_date - ($1::int - 1) GROUP BY 1)
		SELECT to_char(d.day, 'YYYY-MM-DD'), COALESCE(u.a::text, ''), COALESCE(u.r::text, ''), COALESCE(u.o::text, ''), COALESCE(u.p::text, ''),
		       COALESCE(f.n, 0), COALESCE(f.c, 0), COALESCE(s.n, 0)
		FROM days d LEFT JOIN usage u USING (day) LEFT JOIN found f USING (day) LEFT JOIN seen s USING (day) ORDER BY d.day`, days)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	out, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (TrendPoint, error) {
		var p TrendPoint
		return p, r.Scan(&p.Date, &p.Allocated, &p.Reserved, &p.ObservedActive, &p.AveragePct, &p.NewFindings, &p.NewConflicts, &p.Observations)
	})
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, http.StatusOK, out)
}

func (h *Handler) slices(c *gin.Context, sql string) ([]Slice, error) {
	rows, err := h.pool.Query(c.Request.Context(), sql)
	if err != nil {
		return nil, err
	}
	out, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (Slice, error) {
		var s Slice
		return s, r.Scan(&s.Label, &s.Count)
	})
	if out == nil {
		out = []Slice{}
	}
	return out, err
}

func (h *Handler) distribution(c *gin.Context) {
	var d Distribution
	var err error
	if d.Environments, err = h.slices(c, `SELECT environment, count(*) FROM subnets WHERE archived_at IS NULL GROUP BY 1 ORDER BY 2 DESC, 1`); err != nil {
		httpx.Fail(c, err)
		return
	}
	if d.Sites, err = h.slices(c, `SELECT COALESCE(si.name, 'No site'), count(*) FROM subnets s LEFT JOIN sites si ON si.id = s.site_id
		WHERE s.archived_at IS NULL GROUP BY 1 ORDER BY 2 DESC, 1 LIMIT 10`); err != nil {
		httpx.Fail(c, err)
		return
	}
	if d.Families, err = h.slices(c, `SELECT 'IPv' || family(cidr), count(*) FROM subnets WHERE archived_at IS NULL GROUP BY 1 ORDER BY 1`); err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, http.StatusOK, d)
}
