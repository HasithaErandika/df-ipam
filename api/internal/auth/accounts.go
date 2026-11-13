package auth

import (
	"context"
	"strings"

	"github.com/jackc/pgx/v5"
)

// PendingSubject marks an account added before its person first signs in with
// Microsoft. Their first Microsoft sign-in with that email takes it over once,
// so the role and history given beforehand stay with them.
func PendingSubject(email string) string {
	return "pending:" + strings.ToLower(strings.TrimSpace(email))
}

// AddPendingAdmin adds an Administrator who hasn't signed in yet, or makes the
// pending account for that email an active Administrator again. It returns the
// account's ID. The caller audits it in the same transaction.
func AddPendingAdmin(ctx context.Context, tx pgx.Tx, email string) (string, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	var id string
	err := tx.QueryRow(ctx, `INSERT INTO users (email, auth_provider, auth_subject, role)
		VALUES ($1, $2, $3, 'admin')
		ON CONFLICT (auth_provider, auth_subject) DO UPDATE SET role = 'admin', is_active = true
		RETURNING id::text`, email, ProviderEntra, PendingSubject(email)).Scan(&id)
	return id, err
}
