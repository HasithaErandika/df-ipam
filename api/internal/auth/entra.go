package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/lestrrat-go/jwx/v3/jwt"
)

// ProviderEntra marks users who sign in with Microsoft Entra ID.
const ProviderEntra = "entra"

// Reasons a Microsoft sign-in is refused, shown to the person by code.
var (
	ErrEntraDomain = errors.New("domain") // the account isn't on an allowed email domain
	ErrEntraTenant = errors.New("tenant") // the token came from another directory
	ErrEntraFailed = errors.New("failed") // anything else: a bad token, a failed exchange
)

// EntraConfig connects to one Microsoft Entra ID tenant. Authority is
// https://login.microsoftonline.com unless a sovereign cloud is used.
type EntraConfig struct {
	Authority, TenantID, ClientID, ClientSecret, RedirectURL string
	Domains                                                  []string
}

// Enabled reports whether Microsoft sign-in is set up.
func (c EntraConfig) Enabled() bool {
	return c.TenantID != "" && c.ClientID != "" && c.ClientSecret != ""
}

// EntraUser is who a verified Microsoft ID token says signed in.
type EntraUser struct {
	ObjectID, Email, Name string
}

// Entra signs people in with the authorization code flow, PKCE and a nonce,
// against one pinned tenant. The client secret never leaves the backend.
type Entra struct {
	cfg  EntraConfig
	keys *keyCache
	http *http.Client
}

func NewEntra(cfg EntraConfig) *Entra {
	cfg.Authority = strings.TrimRight(cfg.Authority, "/")
	return &Entra{cfg: cfg, keys: &keyCache{url: cfg.base() + "/discovery/v2.0/keys"}, http: &http.Client{Timeout: 10 * time.Second}}
}

func (c EntraConfig) base() string   { return c.Authority + "/" + url.PathEscape(c.TenantID) }
func (c EntraConfig) issuer() string { return c.base() + "/v2.0" }

// LoginAttempt is what the callback must match: kept server-side, keyed by state.
type LoginAttempt struct {
	State, Nonce, Verifier, ReturnTo string
	Started                          time.Time
}

// NewAttempt makes fresh random values for one sign-in.
func NewAttempt(returnTo string) LoginAttempt {
	return LoginAttempt{State: random(), Nonce: random(), Verifier: random() + random(), ReturnTo: returnTo, Started: time.Now()}
}

func random() string {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}

// AuthCodeURL is where the browser goes to sign in.
func (e *Entra) AuthCodeURL(a LoginAttempt) string {
	sum := sha256.Sum256([]byte(a.Verifier))
	q := url.Values{
		"client_id": {e.cfg.ClientID}, "response_type": {"code"}, "redirect_uri": {e.cfg.RedirectURL}, "response_mode": {"query"},
		"scope": {"openid profile email"}, "state": {a.State}, "nonce": {a.Nonce}, "prompt": {"select_account"},
		"code_challenge": {base64.RawURLEncoding.EncodeToString(sum[:])}, "code_challenge_method": {"S256"},
	}
	return e.cfg.base() + "/oauth2/v2.0/authorize?" + q.Encode()
}

// SignIn exchanges the code from the callback and verifies the ID token.
func (e *Entra) SignIn(ctx context.Context, code string, a LoginAttempt) (EntraUser, error) {
	idToken, err := e.exchange(ctx, code, a.Verifier)
	if err != nil {
		return EntraUser{}, fmt.Errorf("%w: %v", ErrEntraFailed, err)
	}
	return e.verify(ctx, idToken, a.Nonce)
}

func (e *Entra) exchange(ctx context.Context, code, verifier string) (string, error) {
	form := url.Values{
		"client_id": {e.cfg.ClientID}, "client_secret": {e.cfg.ClientSecret}, "grant_type": {"authorization_code"},
		"code": {code}, "redirect_uri": {e.cfg.RedirectURL}, "code_verifier": {verifier}, "scope": {"openid profile email"},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, e.cfg.base()+"/oauth2/v2.0/token", strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	res, err := e.http.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	var out struct {
		IDToken string `json:"id_token"`
		Error   string `json:"error"`
	}
	if err := json.NewDecoder(io.LimitReader(res.Body, 1<<20)).Decode(&out); err != nil {
		return "", err
	}
	if res.StatusCode != http.StatusOK || out.IDToken == "" {
		return "", fmt.Errorf("token endpoint answered %d %s", res.StatusCode, out.Error) // never the description: it may echo input
	}
	return out.IDToken, nil
}

func (e *Entra) verify(ctx context.Context, idToken, nonce string) (EntraUser, error) {
	alg, kid, err := tokenHeader(idToken)
	if err != nil || alg != "RS256" {
		return EntraUser{}, ErrEntraFailed
	}
	keys, err := e.keys.get(ctx, kid)
	if err != nil {
		return EntraUser{}, fmt.Errorf("%w: %v", ErrEntraFailed, err)
	}
	t, err := jwt.Parse([]byte(idToken), jwt.WithKeySet(keys), jwt.WithValidate(true),
		jwt.WithIssuer(e.cfg.issuer()), jwt.WithAudience(e.cfg.ClientID), jwt.WithAcceptableSkew(2*time.Minute))
	if err != nil {
		return EntraUser{}, fmt.Errorf("%w: %v", ErrEntraFailed, err)
	}
	claim := func(name string) string {
		var v string
		_ = t.Get(name, &v)
		return strings.TrimSpace(v)
	}
	if claim("nonce") == "" || claim("nonce") != nonce {
		return EntraUser{}, ErrEntraFailed
	}
	if !strings.EqualFold(claim("tid"), e.cfg.TenantID) {
		return EntraUser{}, ErrEntraTenant
	}
	u := EntraUser{ObjectID: claim("oid"), Name: claim("name"), Email: strings.ToLower(claim("email"))}
	if u.Email == "" {
		u.Email = strings.ToLower(claim("preferred_username"))
	}
	if u.ObjectID == "" {
		return EntraUser{}, ErrEntraFailed
	}
	if !AllowedDomain(u.Email, e.cfg.Domains) {
		return u, ErrEntraDomain
	}
	return u, nil
}

// AllowedDomain is true when the part after the last @ is exactly one of the
// domains. Guests (x_gmail.com#EXT#@...) and look-alikes never match.
func AllowedDomain(email string, domains []string) bool {
	at := strings.LastIndex(email, "@")
	if at <= 0 || strings.ContainsAny(email, "# \t") {
		return false
	}
	return slices.Contains(domains, strings.ToLower(email[at+1:]))
}
