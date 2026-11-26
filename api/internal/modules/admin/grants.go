package admin

import (
	"net/http"
	"slices"

	"dfipam/api/internal/auth"
	"dfipam/api/internal/platform/httpx"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
)

// Nobody can hand out more than they hold. A person who manages users but isn't
// an Administrator could otherwise add every permission to a role, their own
// included, or give someone the Administrator role.

// lockUserManagers serialises changes that decide who can manage users, so two
// at once can't both pass the "somebody keeps it" check and lock everyone out.
func lockUserManagers(c *gin.Context, tx pgx.Tx) error {
	_, err := tx.Exec(c, "SELECT pg_advisory_xact_lock(hashtext('dfipam:user_managers'))")
	return err
}

type roleGrant struct {
	FullAccess  bool
	Permissions []string
}

func (g roleGrant) all() []string {
	if g.FullAccess {
		return auth.AllPermissions
	}
	return g.Permissions
}

func loadGrant(c *gin.Context, tx pgx.Tx, key string) (roleGrant, error) {
	var g roleGrant
	err := tx.QueryRow(c, "SELECT full_access, permissions FROM roles WHERE key = $1", key).Scan(&g.FullAccess, &g.Permissions)
	return g, err
}

// holdsAll says whether the person making the change holds every permission.
func holdsAll(c *gin.Context, perms []string) bool {
	p := auth.Current(c)
	for _, name := range perms {
		if !p.Has(name) {
			return false
		}
	}
	return true
}

var errBeyondOwn = httpx.NewError(http.StatusForbidden, "BEYOND_OWN_ACCESS",
	"You can only give or take away permissions you hold yourself. Ask an Administrator.")

// checkRoleGrant refuses adding or removing permissions the person doesn't
// hold, so nobody takes away more than they hold, and changing the permissions
// of their own role.
func checkRoleGrant(c *gin.Context, key string, old, next []string) error {
	added, removed := missing(next, old), missing(old, next)
	if !holdsAll(c, added) || !holdsAll(c, removed) {
		return errBeyondOwn
	}
	same := len(added) == 0 && len(removed) == 0
	if key != "" && key == auth.Current(c).Role && !same {
		return httpx.NewError(http.StatusConflict, "SELF_CHANGE", "You can't change the permissions of your own role. Ask another administrator.")
	}
	return nil
}

// missing lists the permissions in a that b lacks.
func missing(a, b []string) []string {
	out := []string{}
	for _, p := range a {
		if !slices.Contains(b, p) {
			out = append(out, p)
		}
	}
	return out
}

// checkUserGrant refuses giving a role, or changing a person whose role,
// holds more than the person making the change.
func checkUserGrant(c *gin.Context, tx pgx.Tx, oldRole, newRole string) error {
	for _, key := range []string{oldRole, newRole} {
		if key == "" {
			continue
		}
		g, err := loadGrant(c, tx, key)
		if err != nil {
			return httpx.Validation("role", "Choose a role from the list.")
		}
		if !holdsAll(c, g.all()) {
			return errBeyondOwn
		}
	}
	return nil
}
