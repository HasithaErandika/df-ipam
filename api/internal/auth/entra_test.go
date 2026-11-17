package auth

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/lestrrat-go/jwx/v3/jwa"
	"github.com/lestrrat-go/jwx/v3/jwk"
	"github.com/lestrrat-go/jwx/v3/jwt"
)

const testTenant = "11111111-2222-3333-4444-555555555555"

// fakeEntra is a tenant's token endpoint and keys. claims are put in the next
// ID token it issues.
type fakeEntra struct {
	*httptest.Server
	key    jwk.Key
	claims map[string]any
	code   string
}

func newFakeEntra(t *testing.T) *fakeEntra {
	raw, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	private, _ := jwk.Import(raw)
	_ = private.Set(jwk.KeyIDKey, "k1")
	_ = private.Set(jwk.AlgorithmKey, jwa.RS256())
	public, _ := private.PublicKey()
	_ = public.Remove(jwk.AlgorithmKey) // Microsoft publishes its keys without "alg"
	set := jwk.NewSet()
	_ = set.AddKey(public)
	f := &fakeEntra{key: private, code: "good-code"}
	mux := http.NewServeMux()
	mux.HandleFunc("/"+testTenant+"/discovery/v2.0/keys", func(w http.ResponseWriter, _ *http.Request) { _ = json.NewEncoder(w).Encode(set) })
	mux.HandleFunc("/"+testTenant+"/oauth2/v2.0/token", func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		if r.PostForm.Get("code") != f.code || r.PostForm.Get("client_secret") != "secret" || r.PostForm.Get("code_verifier") == "" {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":"invalid_grant"}`))
			return
		}
		b := jwt.NewBuilder().Expiration(time.Now().Add(time.Hour)).IssuedAt(time.Now())
		for k, v := range f.claims {
			b = b.Claim(k, v)
		}
		tok, _ := b.Build()
		signed, err := jwt.Sign(tok, jwt.WithKey(jwa.RS256(), f.key))
		if err != nil {
			t.Error(err)
		}
		_ = json.NewEncoder(w).Encode(map[string]string{"id_token": string(signed)})
	})
	f.Server = httptest.NewServer(mux)
	t.Cleanup(f.Close)
	return f
}

func (f *fakeEntra) client() *Entra {
	return NewEntra(EntraConfig{Authority: f.URL, TenantID: testTenant, ClientID: "app-id", ClientSecret: "secret",
		RedirectURL: "https://ipam.example/api/v1/auth/microsoft/callback", Domains: []string{"dialogfinance.lk"}})
}

func (f *fakeEntra) good(nonce string) map[string]any {
	return map[string]any{"iss": f.URL + "/" + testTenant + "/v2.0", "aud": "app-id", "tid": testTenant, "oid": "obj-1",
		"nonce": nonce, "name": "Nimal Perera", "preferred_username": "Nimal.Perera@DialogFinance.lk"}
}

func TestEntraSignIn(t *testing.T) {
	f := newFakeEntra(t)
	e := f.client()
	a := NewAttempt("/")
	f.claims = f.good(a.Nonce)
	u, err := e.SignIn(context.Background(), "good-code", a)
	if err != nil || u.ObjectID != "obj-1" || u.Email != "nimal.perera@dialogfinance.lk" || u.Name != "Nimal Perera" {
		t.Fatalf("got %+v, %v", u, err)
	}
}

func TestEntraRefusals(t *testing.T) {
	f := newFakeEntra(t)
	e := f.client()
	a := NewAttempt("/")
	cases := map[string]struct {
		change func(map[string]any)
		code   string
		want   error
	}{
		"wrong nonce":    {func(c map[string]any) { c["nonce"] = "other" }, "good-code", ErrEntraFailed},
		"another tenant": {func(c map[string]any) { c["tid"] = "99999999-0000-0000-0000-000000000000" }, "good-code", ErrEntraTenant},
		"wrong audience": {func(c map[string]any) { c["aud"] = "another-app" }, "good-code", ErrEntraFailed},
		"wrong issuer":   {func(c map[string]any) { c["iss"] = "https://login.microsoftonline.com/common/v2.0" }, "good-code", ErrEntraFailed},
		"guest":          {func(c map[string]any) { c["preferred_username"] = "x_gmail.com#EXT#@dialogfinance.lk" }, "good-code", ErrEntraDomain},
		"other domain":   {func(c map[string]any) { c["preferred_username"] = "x@dialog.lk" }, "good-code", ErrEntraDomain},
		"look-alike":     {func(c map[string]any) { c["preferred_username"] = "x@dialogfinance.lk.evil.com" }, "good-code", ErrEntraDomain},
		"expired":        {func(c map[string]any) { c["exp"] = time.Now().Add(-time.Hour).Unix() }, "good-code", ErrEntraFailed},
		"no object id":   {func(c map[string]any) { delete(c, "oid") }, "good-code", ErrEntraFailed},
		"code refused":   {func(map[string]any) {}, "stolen-code", ErrEntraFailed},
	}
	for name, tc := range cases {
		f.claims = f.good(a.Nonce)
		tc.change(f.claims)
		if _, err := e.SignIn(context.Background(), tc.code, a); !errors.Is(err, tc.want) {
			t.Errorf("%s: got %v, want %v", name, err, tc.want)
		}
	}
}

func TestEntraAuthCodeURL(t *testing.T) {
	f := newFakeEntra(t)
	a := NewAttempt("/")
	u, _ := url.Parse(f.client().AuthCodeURL(a))
	q := u.Query()
	if !strings.HasSuffix(u.Path, "/"+testTenant+"/oauth2/v2.0/authorize") || q.Get("state") != a.State || q.Get("nonce") != a.Nonce ||
		q.Get("code_challenge_method") != "S256" || q.Get("code_challenge") == "" || q.Get("client_secret") != "" {
		t.Fatalf("bad URL: %s", u)
	}
}

func TestAllowedDomain(t *testing.T) {
	d := []string{"dialogfinance.lk"}
	for email, want := range map[string]bool{
		"a@dialogfinance.lk": true, "a@dialogfinance.lk.evil.com": false, "a@evil-dialogfinance.lk": false,
		"a_gmail.com#EXT#@dialogfinance.lk": false, "@dialogfinance.lk": false, "a@sub.dialogfinance.lk": false, "": false,
	} {
		if AllowedDomain(email, d) != want {
			t.Errorf("%q: want %v", email, want)
		}
	}
}
