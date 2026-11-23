package admin

import (
	"encoding/json"
	"net/http"
	"regexp"
	"slices"
	"strings"

	"dfipam/api/internal/auth"
	"dfipam/api/internal/platform/db"
	"dfipam/api/internal/platform/httpx"
	"dfipam/api/internal/validate"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
)

type roleDTO struct {
	Key         string   `db:"key" json:"key"`
	Name        string   `db:"name" json:"name"`
	Description string   `db:"description" json:"description"`
	Permissions []string `db:"permissions" json:"permissions"`
	FullAccess  bool     `db:"full_access" json:"fullAccess"`
	IsSystem    bool     `db:"is_system" json:"isSystem"`
	Users       int64    `db:"users" json:"users"`
}

func (h *Handler) roles(c *gin.Context) {
	out, err := db.All[roleDTO](c, h.conn, `SELECT r.key, r.name, r.description, r.permissions, r.full_access, r.is_system,
		(SELECT count(*) FROM users u WHERE u.role = r.key) AS users FROM roles r ORDER BY r.sort_order, lower(r.name)`)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	for i := range out {
		if out[i].FullAccess {
			out[i].Permissions = auth.AllPermissions
		}
	}
	httpx.OK(c, http.StatusOK, out)
}

type roleInput struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Permissions []string `json:"permissions"`
}

func (in *roleInput) validate() error {
	var v validate.V
	v.Text("name", &in.Name, 60, true, "Name")
	v.Text("description", &in.Description, 300, false, "Description")
	clean := []string{}
	for _, p := range in.Permissions {
		if !auth.KnownPermission(p) {
			v.Add("permissions", "Choose listed permissions only.")
			break
		}
		if !slices.Contains(clean, p) {
			clean = append(clean, p)
		}
	}
	// Managing link costs means seeing them: a role with manage but not view
	// would be sent the costs blank and could wipe them on save.
	if slices.Contains(clean, auth.LinksCommercialManage) && !slices.Contains(clean, auth.LinksCommercialView) {
		clean = append(clean, auth.LinksCommercialView)
	}
	in.Permissions = clean
	return v.Err()
}

var nonKey = regexp.MustCompile(`[^a-z0-9]+`)

// roleKey makes a stable key from a new role's name, such as "Network engineer"
// → "network_engineer". The key never changes after a rename.
func roleKey(name string) string {
	k := strings.Trim(nonKey.ReplaceAllString(strings.ToLower(name), "_"), "_")
	if k == "" || k[0] < 'a' || k[0] > 'z' {
		k = "role_" + k
	}
	if len(k) > 40 {
		k = strings.TrimRight(k[:40], "_")
	}
	return k
}

func (h *Handler) createRole(c *gin.Context) {
	var in roleInput
	if !httpx.Bind(c, &in) {
		return
	}
	if err := in.validate(); err != nil {
		httpx.Fail(c, err)
		return
	}
	if err := checkRoleGrant(c, "", nil, in.Permissions); err != nil {
		httpx.Fail(c, err)
		return
	}
	key := roleKey(in.Name)
	err := db.InTx(c, h.conn, func(tx pgx.Tx) error {
		if _, err := tx.Exec(c, `INSERT INTO roles (key, name, description, permissions, sort_order)
			VALUES ($1, $2, $3, $4, 100)`, key, in.Name, in.Description, in.Permissions); err != nil {
			return err
		}
		return record(c, tx, "create", "role", key, nil, in)
	})
	if err != nil {
		httpx.Fail(c, db.Friendly(err, "A role with this name already exists."))
		return
	}
	httpx.OK(c, http.StatusCreated, gin.H{"key": key})
}

// updateRole renames or describes a role and, unless it is a system role, sets
// its permissions. People holding it get the change on their next request.
func (h *Handler) updateRole(c *gin.Context) {
	key := c.Param("key")
	var in roleInput
	if !httpx.Bind(c, &in) {
		return
	}
	if err := in.validate(); err != nil {
		httpx.Fail(c, err)
		return
	}
	err := db.InTx(c, h.conn, func(tx pgx.Tx) error {
		return h.applyRoleUpdate(c, tx, key, in)
	})
	if err != nil {
		httpx.Fail(c, db.Friendly(err, "A role with this name already exists."))
		return
	}
	httpx.OK(c, http.StatusOK, gin.H{"key": key})
}

// deleteRole removes a role nobody holds. System roles stay.
func (h *Handler) deleteRole(c *gin.Context) {
	key := c.Param("key")
	err := db.InTx(c, h.conn, func(tx pgx.Tx) error {
		var old []byte
		var system bool
		var users int
		if err := tx.QueryRow(c, `SELECT to_jsonb(r), r.is_system, (SELECT count(*) FROM users u WHERE u.role = r.key)
			FROM roles r WHERE r.key = $1 FOR UPDATE`, key).Scan(&old, &system, &users); err != nil {
			return err
		}
		if system {
			return httpx.NewError(http.StatusConflict, "ROLE_SYSTEM", "The Administrator role can't be deleted.")
		}
		if users > 0 {
			return httpx.NewError(http.StatusConflict, "ROLE_IN_USE", "People still hold this role. Give them another role first.")
		}
		if _, err := tx.Exec(c, "DELETE FROM roles WHERE key = $1", key); err != nil {
			return err
		}
		return record(c, tx, "delete", "role", key, json.RawMessage(old), nil)
	})
	if err != nil {
		httpx.Fail(c, db.Friendly(err))
		return
	}
	c.Status(http.StatusNoContent)
}
