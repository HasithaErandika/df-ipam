package server

import (
	"bytes"
	"context"
	"dfipam/api/internal/auth"
	"dfipam/api/internal/seed"
	"encoding/json"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
)

// client drives the real router against a real database, as the limited app role,
// inside one transaction that is rolled back. Only sign-in is stubbed.
type client struct {
	t      *testing.T
	engine *gin.Engine
}

func (c client) call(method, path string, body any, headers ...string) (int, map[string]any) {
	c.t.Helper()
	var reader *bytes.Reader = bytes.NewReader(nil)
	if body != nil {
		raw, _ := json.Marshal(body)
		reader = bytes.NewReader(raw)
	}
	req := httptest.NewRequest(method, "/api/v1"+path, reader)
	req.Header.Set("Authorization", "Bearer token")
	req.Header.Set("Content-Type", "application/json")
	for i := 0; i+1 < len(headers); i += 2 {
		req.Header.Set(headers[i], headers[i+1])
	}
	w := httptest.NewRecorder()
	c.engine.ServeHTTP(w, req)
	out := map[string]any{}
	_ = json.Unmarshal(w.Body.Bytes(), &out)
	return w.Code, out
}

func (c client) must(want int, method, path string, body any, headers ...string) map[string]any {
	c.t.Helper()
	code, out := c.call(method, path, body, headers...)
	if code != want {
		c.t.Fatalf("%s %s: got %d want %d: %v", method, path, code, want, out)
	}
	return out
}

func data(m map[string]any) map[string]any { return m["data"].(map[string]any) }

func id(m map[string]any) string { return data(m)["id"].(string) }

func setup(t *testing.T) client {
	t.Helper()
	url := os.Getenv("TEST_APP_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_APP_DATABASE_URL is not set")
	}
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close(ctx) })
	tx, err := conn.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { tx.Rollback(ctx) })
	var user string
	if err := tx.QueryRow(ctx, `INSERT INTO users (email, auth_provider, auth_subject) VALUES ('it@example.com', 'test', gen_random_uuid()::text) RETURNING id::text`).Scan(&user); err != nil {
		t.Fatal(err)
	}
	perms := map[string]bool{}
	for _, p := range seed.AllPermissions {
		perms[p] = true
	}
	gin.SetMode(gin.TestMode)
	engine, _ := New(Deps{Conn: tx, Verifier: stubVerifier{}, Resolver: fixedResolver{&auth.Principal{UserID: user, Email: "it@example.com", Permissions: perms}}})
	return client{t, engine}
}

type fixedResolver struct{ p *auth.Principal }

func (f fixedResolver) Resolve(context.Context, auth.Identity) (*auth.Principal, error) {
	return f.p, nil
}
