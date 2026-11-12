package auth

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/lestrrat-go/jwx/v3/jwk"
	"github.com/lestrrat-go/jwx/v3/jwt"
)

const providerSupabase = "supabase"

// Only asymmetric algorithms: accepting others would allow key-confusion attacks.
var allowedAlgs = map[string]bool{"ES256": true, "RS256": true}

// SupabaseVerifier checks Supabase access tokens against the project's JWKS.
type SupabaseVerifier struct {
	issuer, audience string
	cache            *keyCache
}

func NewSupabaseVerifier(issuer, audience, jwksURL string) *SupabaseVerifier {
	return &SupabaseVerifier{issuer: issuer, audience: audience, cache: &keyCache{url: jwksURL}}
}

func (*SupabaseVerifier) Provider() string { return providerSupabase }

func (v *SupabaseVerifier) keySet(ctx context.Context, kid string) (jwk.Set, error) {
	return v.cache.get(ctx, kid)
}

func tokenHeader(token string) (alg, kid string, err error) {
	head, _, ok := strings.Cut(token, ".")
	if !ok {
		return "", "", ErrInvalidToken
	}
	raw, err := base64.RawURLEncoding.DecodeString(head)
	if err != nil {
		return "", "", ErrInvalidToken
	}
	var h struct {
		Alg string `json:"alg"`
		Kid string `json:"kid"`
	}
	if err := json.Unmarshal(raw, &h); err != nil {
		return "", "", ErrInvalidToken
	}
	return h.Alg, h.Kid, nil
}

func (v *SupabaseVerifier) Verify(ctx context.Context, token string) (Identity, error) {
	alg, kid, err := tokenHeader(token)
	if err != nil || !allowedAlgs[alg] {
		return Identity{}, ErrInvalidToken
	}
	keys, err := v.keySet(ctx, kid)
	if err != nil {
		return Identity{}, err
	}
	parsed, err := jwt.Parse([]byte(token), jwt.WithKeySet(keys), jwt.WithValidate(true),
		jwt.WithIssuer(v.issuer), jwt.WithAudience(v.audience))
	if err != nil {
		return Identity{}, fmt.Errorf("%w: %v", ErrInvalidToken, err)
	}
	subject, ok := parsed.Subject()
	if !ok || subject == "" {
		return Identity{}, ErrInvalidToken
	}
	var email string
	_ = parsed.Get("email", &email)
	return Identity{Subject: subject, Email: strings.ToLower(email)}, nil
}
