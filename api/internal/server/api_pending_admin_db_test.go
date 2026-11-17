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

// The first Administrator comes from `make admin` as a pending Microsoft
// account. Their first Microsoft sign-in takes that account over, role and all,
// without making a second one.
func TestPendingAdminTakenOverByMicrosoftAgainstDatabase(t *testing.T) {
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
	if _, err := auth.AddPendingAdmin(ctx, tx, "kamal@dialogfinance.lk"); err != nil {
		t.Fatal(err)
	}
	gin.SetMode(gin.TestMode)
	sessions := &auth.Sessions{Conn: tx}
	ms := &fakeMicrosoft{user: auth.EntraUser{ObjectID: "obj-77", Email: "Kamal@dialogfinance.lk", Name: "Kamal"}}
	r, _ := New(Deps{Conn: tx, Sessions: sessions, SignIn: signin.New(tx, ms, sessions, "http://localhost:5173")})
	do := func(path string, cookies []*http.Cookie) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.RemoteAddr = "10.78.0.1:1"
		for _, c := range cookies {
			req.AddCookie(c)
		}
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		return w
	}
	login := do("/api/v1/auth/microsoft/login", nil)
	state := strings.TrimPrefix(login.Header().Get("Location"), "https://login.example/authorize?state=")
	back := do("/api/v1/auth/microsoft/callback?code=c&state="+state, login.Result().Cookies())
	if back.Code != http.StatusFound || strings.Contains(back.Header().Get("Location"), "signin_error") {
		t.Fatalf("microsoft: %d %s", back.Code, back.Header().Get("Location"))
	}
	me := do("/api/v1/auth/me", back.Result().Cookies())
	if me.Code != http.StatusOK || !strings.Contains(me.Body.String(), `"role":"admin"`) {
		t.Fatalf("me: %d %s", me.Code, me.Body.String())
	}
	var users int
	var subject string
	if err := tx.QueryRow(ctx, `SELECT count(*), max(auth_subject) FROM users WHERE lower(email) = 'kamal@dialogfinance.lk'`).Scan(&users, &subject); err != nil {
		t.Fatal(err)
	}
	if users != 1 || subject != "obj-77" {
		t.Fatalf("microsoft sign-in did not take the account over: %d accounts, subject %q", users, subject)
	}
}

// There is no way in without Microsoft: the old development sign-in is gone.
func TestNoPasswordlessSignIn(t *testing.T) {
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
