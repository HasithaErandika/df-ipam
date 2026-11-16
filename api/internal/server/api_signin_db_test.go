package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"

	"dfipam/api/internal/auth"
	"dfipam/api/internal/modules/signin"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
)

// fakeMicrosoft stands in for Entra ID: it signs in whoever user says, or
// refuses with err.
type fakeMicrosoft struct {
	user auth.EntraUser
	err  error
}

func (fakeMicrosoft) AuthCodeURL(a auth.LoginAttempt) string {
	return "https://login.example/authorize?state=" + url.QueryEscape(a.State)
}

func (f *fakeMicrosoft) SignIn(context.Context, string, auth.LoginAttempt) (auth.EntraUser, error) {
	return f.user, f.err
}

func TestMicrosoftSignInAgainstDatabase(t *testing.T) {
	dsn := os.Getenv("TEST_APP_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_APP_DATABASE_URL is not set")
	}
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close(ctx) })
	tx, err := conn.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { tx.Rollback(ctx) })
	gin.SetMode(gin.TestMode)
	ms := &fakeMicrosoft{user: auth.EntraUser{ObjectID: "obj-42", Email: "nimal@dialogfinance.lk", Name: "Nimal"}}
	sessions := &auth.Sessions{Conn: tx}
	r, _ := New(Deps{Conn: tx, Sessions: sessions,
		SignIn: signin.New(tx, ms, sessions, "https://ipam.example")})

	n := 0
	do := func(method, path string, cookies []*http.Cookie, csrf bool) *httptest.ResponseRecorder {
		n++
		req := httptest.NewRequest(method, path, nil)
		req.RemoteAddr = "10.77.0." + string(rune('0'+n%10)) + ":1"
		for _, c := range cookies {
			req.AddCookie(c)
		}
		if csrf {
			req.Header.Set("X-Requested-With", "DF-IPAM")
		}
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		return w
	}
	signIn := func() (*httptest.ResponseRecorder, []*http.Cookie) {
		login := do("GET", "/api/v1/auth/microsoft/login?returnTo=/servers", nil, false)
		if login.Code != http.StatusFound {
			t.Fatalf("login: %d %s", login.Code, login.Body.String())
		}
		state := strings.TrimPrefix(login.Header().Get("Location"), "https://login.example/authorize?state=")
		state, _ = url.QueryUnescape(state)
		back := do("GET", "/api/v1/auth/microsoft/callback?code=c&state="+url.QueryEscape(state), login.Result().Cookies(), false)
		return back, back.Result().Cookies()
	}

	back, cookies := signIn()
	if back.Code != http.StatusFound || back.Header().Get("Location") != "https://ipam.example/servers" {
		t.Fatalf("callback: %d %s", back.Code, back.Header().Get("Location"))
	}
	var session []*http.Cookie
	for _, c := range cookies {
		if c.Name == auth.SessionCookieSecure && c.Value != "" {
			if !c.HttpOnly || !c.Secure || c.Path != "/" {
				t.Fatalf("unsafe cookie: %+v", c)
			}
			session = append(session, c)
		}
	}
	if len(session) != 1 {
		t.Fatalf("no session cookie: %v", cookies)
	}
	me := do("GET", "/api/v1/auth/me", session, false)
	if me.Code != http.StatusOK || !strings.Contains(me.Body.String(), "nimal@dialogfinance.lk") || !strings.Contains(me.Body.String(), `"role":null`) {
		t.Fatalf("me: %d %s", me.Code, me.Body.String())
	}
	if w := do("GET", "/api/v1/auth/status", session, false); !strings.Contains(w.Body.String(), `"session":true`) {
		t.Fatalf("status: %s", w.Body.String())
	}
	if w := do("POST", "/api/v1/auth/logout", session, false); w.Code != http.StatusForbidden {
		t.Fatalf("logout without the safety header: %d", w.Code)
	}
	if w := do("POST", "/api/v1/auth/logout", session, true); w.Code != http.StatusNoContent {
		t.Fatalf("logout: %d", w.Code)
	}
	if w := do("GET", "/api/v1/auth/me", session, false); w.Code != http.StatusUnauthorized {
		t.Fatalf("after logout: %d", w.Code)
	}

	// A replayed callback finds no attempt, and a refused domain says so.
	login := do("GET", "/api/v1/auth/microsoft/login", nil, false)
	if w := do("GET", "/api/v1/auth/microsoft/callback?code=c&state=forged", login.Result().Cookies(), false); !strings.HasSuffix(w.Header().Get("Location"), "signin_error=expired") {
		t.Fatalf("forged state: %s", w.Header().Get("Location"))
	}
	ms.err = auth.ErrEntraDomain
	if back, _ := signIn(); !strings.HasSuffix(back.Header().Get("Location"), "signin_error=domain") {
		t.Fatalf("domain: %s", back.Header().Get("Location"))
	}
	ms.err = nil
	if _, err := tx.Exec(ctx, "UPDATE users SET is_active = false WHERE auth_subject = 'obj-42'"); err != nil {
		t.Fatal(err)
	}
	if back, _ := signIn(); !strings.HasSuffix(back.Header().Get("Location"), "signin_error=disabled") {
		t.Fatalf("disabled: %s", back.Header().Get("Location"))
	}
	if w := do("GET", "/api/v1/auth/microsoft/login?returnTo=//evil.example", nil, false); w.Code != http.StatusFound {
		t.Fatalf("login: %d", w.Code)
	}
}
