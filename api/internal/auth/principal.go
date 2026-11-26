// Package auth handles sign-in sessions, Microsoft Entra ID and permissions.
// DF-Atlas holds no passwords: people sign in with Microsoft (see the signin
// module).
package auth

// Principal is the authenticated user for one request. Role is empty until an
// Admin gives one, and then grants that role's permissions.
type Principal struct {
	UserID      string
	Email       string
	Role        string
	RoleName    string
	Permissions map[string]bool
}

func (p *Principal) Has(permission string) bool { return p.Permissions[permission] }

// NewPrincipal builds a principal from its role's permissions. A full-access
// role holds every permission, including ones added after it was set up.
func NewPrincipal(userID, email, role string, permissions []string, fullAccess bool) *Principal {
	p := &Principal{UserID: userID, Email: email, Role: role, Permissions: map[string]bool{}}
	if fullAccess {
		permissions = AllPermissions
	}
	for _, name := range permissions {
		if KnownPermission(name) {
			p.Permissions[name] = true
		}
	}
	return p
}
