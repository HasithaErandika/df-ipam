// Package auth handles token verification, user provisioning and permissions.
package auth

import (
	"context"
	"errors"
)

// Identity is who a verified token says the caller is.
type Identity struct {
	Subject string
	Email   string
}

// ErrInvalidToken means the token must be rejected.
var ErrInvalidToken = errors.New("invalid token")

// Verifier is the only thing the application needs from an auth provider.
// Moving to company auth means adding another implementation of it.
type Verifier interface {
	Verify(ctx context.Context, bearerToken string) (Identity, error)
}
