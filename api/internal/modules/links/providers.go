package links

import (
	"net/http"

	"dfipam/api/internal/platform/db"
	"dfipam/api/internal/platform/httpx"
	"dfipam/api/internal/validate"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
)

type providerDTO struct {
	ID             string `db:"id" json:"id"`
	Name           string `db:"name" json:"name"`
	AccountManager string `db:"account_manager" json:"accountManager"`
	ContactNumber  string `db:"contact_number" json:"contactNumber"`
	Email          string `db:"email" json:"email"`
	SupportHotline string `db:"support_hotline" json:"supportHotline"`
	SupportEmail   string `db:"support_email" json:"supportEmail"`
	Website        string `db:"website" json:"website"`
	Remarks        string `db:"remarks" json:"remarks"`
	Active         bool   `db:"is_active" json:"active"`
	Links          int64  `db:"links" json:"links"`
}

func (h *Handler) providers(c *gin.Context) {
	out, err := db.All[providerDTO](c, h.conn, `SELECT p.id::text AS id, p.name, p.account_manager, p.contact_number, p.email, p.support_hotline,
		p.support_email, p.website, p.remarks, p.is_active,
		(SELECT count(*) FROM network_links l WHERE l.provider_id = p.id AND l.archived_at IS NULL) AS links
		FROM network_link_providers p ORDER BY NOT p.is_active, lower(p.name)`)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, http.StatusOK, out)
}

type providerInput struct {
	Name           string `json:"name"`
	AccountManager string `json:"accountManager"`
	ContactNumber  string `json:"contactNumber"`
	Email          string `json:"email"`
	SupportHotline string `json:"supportHotline"`
	SupportEmail   string `json:"supportEmail"`
	Website        string `json:"website"`
	Remarks        string `json:"remarks"`
	Active         bool   `json:"active"`
}

func (in *providerInput) validate() error {
	var v validate.V
	v.Text("name", &in.Name, 120, true, "Provider name")
	v.Text("accountManager", &in.AccountManager, 120, false, "Account manager")
	phone(&v, "contactNumber", &in.ContactNumber, "Contact number")
	phone(&v, "supportHotline", &in.SupportHotline, "Support hotline")
	email(&v, "email", &in.Email, "Email address")
	email(&v, "supportEmail", &in.SupportEmail, "Support email")
	website(&v, "website", &in.Website)
	v.Text("remarks", &in.Remarks, 1000, false, "Remarks")
	return v.Err()
}

func (h *Handler) createProvider(c *gin.Context) { h.saveProvider(c, "") }

func (h *Handler) updateProvider(c *gin.Context) {
	if id, ok := httpx.PathID(c); ok {
		h.saveProvider(c, id)
	}
}

// saveProvider adds or edits one. Providers are switched off, never deleted,
// so links keep their provider.
func (h *Handler) saveProvider(c *gin.Context, id string) {
	var in providerInput
	if !httpx.Bind(c, &in) {
		return
	}
	if id == "" {
		in.Active = true
	}
	if err := in.validate(); err != nil {
		httpx.Fail(c, err)
		return
	}
	args := []any{in.Name, in.AccountManager, in.ContactNumber, in.Email, in.SupportHotline, in.SupportEmail, in.Website, in.Remarks, in.Active}
	err := db.InTx(c, h.conn, func(tx pgx.Tx) error {
		if id == "" {
			if err := tx.QueryRow(c, `INSERT INTO network_link_providers (name, account_manager, contact_number, email, support_hotline, support_email, website, remarks, is_active)
				VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9) RETURNING id::text`, args...).Scan(&id); err != nil {
				return err
			}
			return record(c, tx, "create", "link_provider", id, nil, in)
		}
		var old []byte
		if err := tx.QueryRow(c, `SELECT to_jsonb(p) FROM network_link_providers p WHERE id = $1::uuid FOR UPDATE`, id).Scan(&old); err != nil {
			return err
		}
		if _, err := tx.Exec(c, `UPDATE network_link_providers SET name = $1, account_manager = $2, contact_number = $3, email = $4, support_hotline = $5,
			support_email = $6, website = $7, remarks = $8, is_active = $9 WHERE id = $10::uuid`, append(args, id)...); err != nil {
			return err
		}
		return record(c, tx, "update", "link_provider", id, rawJSON(old), in)
	})
	if err != nil {
		httpx.Fail(c, db.Friendly(err, "A provider with this name already exists."))
		return
	}
	status := http.StatusOK
	if c.Request.Method == http.MethodPost {
		status = http.StatusCreated
	}
	httpx.OK(c, status, gin.H{"id": id})
}
