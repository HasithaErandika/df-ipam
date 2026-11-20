package links

import (
	"net/http"
	"strconv"
	"time"

	"dfipam/api/internal/auth"
	"dfipam/api/internal/platform/db"
	"dfipam/api/internal/platform/httpx"

	"github.com/gin-gonic/gin"
)

type count struct {
	Key   string `db:"key" json:"key"`
	Label string `db:"label" json:"label"`
	Count int64  `db:"n" json:"count"`
}

type amount struct {
	Key      string  `db:"key" json:"key"`
	Label    string  `db:"label" json:"label"`
	Currency string  `db:"currency" json:"currency"`
	Amount   float64 `db:"amount" json:"amount"`
}

type recentLink struct {
	ID        string    `db:"id" json:"id"`
	Code      string    `db:"code" json:"code"`
	Name      string    `db:"name" json:"name"`
	Status    string    `db:"status" json:"status"`
	UpdatedAt time.Time `db:"updated_at" json:"updatedAt"`
}

type totals struct {
	Total    int64 `db:"total" json:"total"`
	Active   int64 `db:"active" json:"active"`
	Inactive int64 `db:"inactive" json:"inactive"`
	Pending  int64 `db:"pending" json:"pending"`
	VPN      int64 `db:"vpn" json:"vpn"`
	Expiring int64 `db:"expiring" json:"expiring"`
}

// live are the links that count: not archived.
const live = ` FROM network_links l LEFT JOIN network_link_providers p ON p.id = l.provider_id WHERE l.archived_at IS NULL`

// recurring are the costs that count: not terminated, not paid through another link.
const recurring = live + ` AND l.status <> 'terminated' AND NOT l.no_separate_cost AND l.monthly_rental IS NOT NULL`

// colomboToday is today's date in Sri Lanka.
const colomboToday = `(now() AT TIME ZONE 'Asia/Colombo')::date`

// dashboard summarises the links: counts for everyone, money and contract
// dates only for people who may see them. Currencies are never added together.
func (h *Handler) dashboard(c *gin.Context) {
	s, err := loadSettings(c, h.conn)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	var t totals
	rows, err := db.All[totals](c, h.conn, `SELECT count(*) AS total, count(*) FILTER (WHERE l.status = 'active') AS active,
		count(*) FILTER (WHERE l.status IN ('inactive', 'terminated')) AS inactive, count(*) FILTER (WHERE l.status = 'pending') AS pending,
		count(*) FILTER (WHERE l.link_type = ANY($1::text[]) AND l.status <> 'terminated') AS vpn,
		count(*) FILTER (WHERE l.status <> 'terminated' AND l.contract_end BETWEEN `+colomboToday+` AND `+colomboToday+` + 90) AS expiring`+live, s.vpnKeys())
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	t = rows[0]
	out := gin.H{"totals": t}
	groups := map[string]string{
		"byType":     `SELECT l.link_type AS key, l.link_type AS label, count(*) AS n` + live + ` GROUP BY 1 ORDER BY 3 DESC`,
		"byProvider": `SELECT COALESCE(p.id::text, '') AS key, COALESCE(p.name, 'No provider') AS label, count(*) AS n` + live + ` GROUP BY 1, 2 ORDER BY 3 DESC LIMIT 12`,
		"bySite":     `SELECT l.source_site AS key, l.source_site AS label, count(*) AS n` + live + ` GROUP BY 1 ORDER BY 3 DESC LIMIT 12`,
		"byStatus":   `SELECT l.status AS key, l.status AS label, count(*) AS n` + live + ` GROUP BY 1 ORDER BY 3 DESC`,
	}
	for key, q := range groups {
		if out[key], err = db.All[count](c, h.conn, q); err != nil {
			httpx.Fail(c, err)
			return
		}
	}
	if out["recent"], err = db.All[recentLink](c, h.conn, `SELECT l.id::text AS id, l.code, l.name, l.status, l.updated_at`+live+
		` ORDER BY l.updated_at DESC LIMIT 8`); err != nil {
		httpx.Fail(c, err)
		return
	}
	if !can(c, auth.LinksCommercialView) {
		t.Expiring = 0
		out["totals"], out["commercialHidden"] = t, true
		httpx.OK(c, http.StatusOK, out)
		return
	}
	if err := h.costs(c, out, s); err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, http.StatusOK, out)
}

// costs adds monthly totals per currency and per provider, and the combined
// LKR total when an exchange rate is set.
func (h *Handler) costs(c *gin.Context, out gin.H, s Settings) error {
	var err error
	if out["monthlyByCurrency"], err = db.All[amount](c, h.conn, `SELECT l.currency AS key, l.currency AS label, l.currency,
		sum(l.monthly_rental)::float8 AS amount`+recurring+` GROUP BY 1 ORDER BY 1`); err != nil {
		return err
	}
	if out["monthlyByProvider"], err = db.All[amount](c, h.conn, `SELECT COALESCE(p.id::text, '') AS key, COALESCE(p.name, 'No provider') AS label,
		l.currency, sum(l.monthly_rental)::float8 AS amount`+recurring+` GROUP BY 1, 2, 3 ORDER BY 4 DESC LIMIT 20`); err != nil {
		return err
	}
	if s.USDRate != nil {
		var lkr float64
		if err := h.conn.QueryRow(c, `SELECT COALESCE(sum(CASE l.currency WHEN 'LKR' THEN l.monthly_rental WHEN 'USD' THEN l.monthly_rental * $1 END), 0)::float8`+
			recurring+` AND l.currency IN ('LKR', 'USD')`, *s.USDRate).Scan(&lkr); err != nil {
			return err
		}
		out["monthlyCombinedLkr"] = lkr
	}
	out["renewals"], err = renewalRows(c, h.conn, 90, 8)
	return err
}

// renewals lists contracts ending within ?days (default 90), and those already
// ended on links that aren't terminated.
func (h *Handler) renewals(c *gin.Context) {
	days, err := strconv.Atoi(c.DefaultQuery("days", "90"))
	if err != nil || days < 1 || days > 730 {
		httpx.Fail(c, httpx.Validation("days", "Choose from 1 to 730 days."))
		return
	}
	rows, err := renewalRows(c, h.conn, days, 500)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, http.StatusOK, rows)
}

type renewal struct {
	ID            string   `db:"id" json:"id"`
	Code          string   `db:"code" json:"code"`
	Name          string   `db:"name" json:"name"`
	Provider      *string  `db:"provider" json:"provider"`
	ContractEnd   string   `db:"contract_end" json:"contractEnd"`
	DaysLeft      int32    `db:"days_left" json:"daysLeft"`
	NoticeDays    *int32   `db:"renewal_notice_days" json:"noticeDays"`
	MonthlyRental *float64 `db:"monthly_rental" json:"monthlyRental"`
	Currency      string   `db:"currency" json:"currency"`
	Officer       *string  `db:"officer" json:"officer"`
	ContractRef   string   `db:"contract_ref" json:"contractRef"`
}

func renewalRows(c *gin.Context, conn db.Conn, days, limit int) ([]renewal, error) {
	return db.All[renewal](c, conn, `SELECT l.id::text AS id, l.code, l.name, p.name AS provider, l.contract_end::text AS contract_end,
		(l.contract_end - `+colomboToday+`) AS days_left, l.renewal_notice_days, l.monthly_rental::float8 AS monthly_rental, l.currency,
		COALESCE(NULLIF(u.display_name, ''), u.email) AS officer, l.contract_ref
		FROM network_links l LEFT JOIN network_link_providers p ON p.id = l.provider_id LEFT JOIN users u ON u.id = l.officer_id
		WHERE l.archived_at IS NULL AND l.status <> 'terminated' AND l.contract_end <= `+colomboToday+` + $1::integer
		ORDER BY l.contract_end, l.code LIMIT $2`, days, limit)
}
