package auth

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/lestrrat-go/jwx/v3/jwa"
	"github.com/lestrrat-go/jwx/v3/jwk"
	"github.com/lestrrat-go/jwx/v3/jwt"
)

const testIssuer = "https://example.supabase.co/auth/v1"

type fixture struct {
	key      jwk.Key
	verifier *SupabaseVerifier
}

func newFixture(t *testing.T) fixture {
	t.Helper()
	raw, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	private, err := jwk.Import(raw)
	if err != nil {
		t.Fatal(err)
	}
	_ = private.Set(jwk.KeyIDKey, "test-key")
	_ = private.Set(jwk.AlgorithmKey, jwa.ES256())
	public, err := private.PublicKey()
	if err != nil {
		t.Fatal(err)
	}
	set := jwk.NewSet()
	_ = set.AddKey(public)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(set)
	}))
	t.Cleanup(srv.Close)
	return fixture{private, NewSupabaseVerifier(testIssuer, "authenticated", srv.URL)}
}

func (f fixture) sign(t *testing.T, issuer, audience string, expires time.Time) string {
	t.Helper()
	tok, err := jwt.NewBuilder().Issuer(issuer).Audience([]string{audience}).Subject("user-1").
		Expiration(expires).Claim("email", "Person@Example.com").Build()
	if err != nil {
		t.Fatal(err)
	}
	signed, err := jwt.Sign(tok, jwt.WithKey(jwa.ES256(), f.key))
	if err != nil {
		t.Fatal(err)
	}
	return string(signed)
}

func TestVerifyAcceptsValidToken(t *testing.T) {
	f := newFixture(t)
	id, err := f.verifier.Verify(context.Background(), f.sign(t, testIssuer, "authenticated", time.Now().Add(time.Hour)))
	if err != nil {
		t.Fatal(err)
	}
	if id.Subject != "user-1" || id.Email != "person@example.com" {
		t.Fatalf("got %+v", id)
	}
}

func TestVerifyRejects(t *testing.T) {
	f := newFixture(t)
	other := newFixture(t)
	hour := time.Now().Add(time.Hour)

	hsTok, _ := jwt.NewBuilder().Issuer(testIssuer).Audience([]string{"authenticated"}).Subject("u").Expiration(hour).Build()
	hs, err := jwt.Sign(hsTok, jwt.WithKey(jwa.HS256(), []byte("shared-secret-shared-secret-shared")))
	if err != nil {
		t.Fatal(err)
	}

	cases := map[string]string{
		"wrong issuer":   f.sign(t, "https://evil.example/auth/v1", "authenticated", hour),
		"wrong audience": f.sign(t, testIssuer, "anon", hour),
		"expired":        f.sign(t, testIssuer, "authenticated", time.Now().Add(-time.Hour)),
		"wrong key":      other.sign(t, testIssuer, "authenticated", hour),
		"hs256":          string(hs),
		"garbage":        "not-a-token",
	}
	for name, token := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := f.verifier.Verify(context.Background(), token); !errors.Is(err, ErrInvalidToken) {
				t.Fatalf("want ErrInvalidToken, got %v", err)
			}
		})
	}
}
