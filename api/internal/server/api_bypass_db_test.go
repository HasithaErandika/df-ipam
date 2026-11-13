package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"dfipam/api/internal/auth"
	"dfipam/api/internal/modules/signin"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
)

// The development sign-in starts an ordinary session as one person, an
// Administrator added as a pending Microsoft account. Their later Microsoft
// sign-in with the same email takes that account over, role and all.
func TestBypassThenMicrosoftAgainstDatabase(t *testing.T) {
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
	sessions := &auth.Sessions{Conn: tx}
	ms := &fakeMicrosoft{user: auth.EntraUser{ObjectID: "obj-77", Email: "Kamal@dialogfinance.lk", Name: "Kamal"}}
	r, _ := New(Deps{Conn: tx, Sessions: sessions, SignIn: signin.New(tx, ms, sessions, "http://localhost:5173").WithBypass("kamal@dialogfinance.lk")})
	n := 0
	do := func(method, path string, cookies []*http.Cookie, csrf bool) *httptest.ResponseRecorder {
		n++
		req := httptest.NewRequest(method, path, nil)
		req.RemoteAddr = "10.78.0." + string(rune('0'+n%10)) + ":1"
		for _, c := range cookies {
			req.AddCookie(c)
		}
		if csrf {
			req.Header.Set(auth.CSRFHeader, "DF-IPAM")
		}
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		return w
	}
	if w := do("GET", "/api/v1/auth/status", nil, false); !strings.Contains(w.Body.String(), `"bypass":true`) {
		t.Fatalf("status: %s", w.Body.String())
	}
	if w := do("POST", "/api/v1/auth/bypass", nil, false); w.Code != http.StatusForbidden {
		t.Fatalf("bypass without the safety header: %d", w.Code)
	}
	in := do("POST", "/api/v1/auth/bypass", nil, true)
	if in.Code != http.StatusNoContent {
		t.Fatalf("bypass: %d %s", in.Code, in.Body.String())
	}
	me := do("GET", "/api/v1/auth/me", in.Result().Cookies(), false)
	if me.Code != http.StatusOK || !strings.Contains(me.Body.String(), "kamal@dialogfinance.lk") || !strings.Contains(me.Body.String(), `"role":"admin"`) {
		t.Fatalf("me: %d %s", me.Code, me.Body.String())
	}
	if again := do("POST", "/api/v1/auth/bypass", nil, true); again.Code != http.StatusNoContent {
		t.Fatalf("second bypass: %d", again.Code)
	}
	var users int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM users WHERE lower(email) = 'kamal@dialogfinance.lk'`).Scan(&users); err != nil || users != 1 {
		t.Fatalf("bypass made %d accounts (%v)", users, err)
	}

	login := do("GET", "/api/v1/auth/microsoft/login", nil, false)
	state := strings.TrimPrefix(login.Header().Get("Location"), "https://login.example/authorize?state=")
	back := do("GET", "/api/v1/auth/microsoft/callback?code=c&state="+state, login.Result().Cookies(), false)
	if back.Code != http.StatusFound || strings.Contains(back.Header().Get("Location"), "signin_error") {
		t.Fatalf("microsoft: %d %s", back.Code, back.Header().Get("Location"))
	}
	var subject, role string
	if err := tx.QueryRow(ctx, `SELECT auth_subject, role FROM users WHERE lower(email) = 'kamal@dialogfinance.lk'`).Scan(&subject, &role); err != nil {
		t.Fatal(err)
	}
	if subject != "obj-77" || role != "admin" {
		t.Fatalf("microsoft sign-in did not take the account over: %s %s", subject, role)
	}
}

func TestBypassIsOffByDefault(t *testing.T) {
	r, _ := engine(nil)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/bypass", nil)
	req.Header.Set(auth.CSRFHeader, "DF-IPAM")
	req.RemoteAddr = "10.201.0.1:1"
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("got %d %s", w.Code, w.Body.String())
	}
}
