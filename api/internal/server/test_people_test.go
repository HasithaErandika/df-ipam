package server

import (
	"context"
	"dfipam/api/internal/auth"
	"dfipam/api/internal/seed"
	"os"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
)

// actingResolver lets one test act as several people.
type actingResolver struct{ current **auth.Principal }

func (a actingResolver) Resolve(context.Context, auth.Identity) (*auth.Principal, error) {
	return *a.current, nil
}

type people struct {
	t       *testing.T
	tx      pgx.Tx
	current *auth.Principal
	client  client
}

func (p *people) user(email string, roles ...string) *auth.Principal {
	p.t.Helper()
	ctx := context.Background()
	var id string
	if err := p.tx.QueryRow(ctx, `INSERT INTO users (email, auth_provider, auth_subject) VALUES ($1, 'test', gen_random_uuid()::text) RETURNING id::text`, email).Scan(&id); err != nil {
		p.t.Fatal(err)
	}
	perms := map[string]bool{}
	for _, role := range roles {
		if _, err := p.tx.Exec(ctx, `INSERT INTO user_roles (user_id, role_id) SELECT $1::uuid, id FROM roles WHERE name = $2`, id, role); err != nil {
			p.t.Fatal(err)
		}
		for _, r := range seed.Roles {
			if r.Name == role {
				for _, perm := range r.Permissions {
					perms[perm] = true
				}
			}
		}
	}
	return &auth.Principal{UserID: id, Email: email, Roles: roles, Permissions: perms}
}

func (p *people) as(u *auth.Principal) client { p.current = u; return p.client }

func setupPeople(t *testing.T) (*people, pgx.Tx) {
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
	p := &people{t: t, tx: tx}
	gin.SetMode(gin.TestMode)
	engine, _ := New(Deps{Conn: tx, Verifier: stubVerifier{}, Resolver: actingResolver{&p.current}, Keys: testKeys(t), Outbound: testPolicy})
	p.client = client{t, engine}
	return p, tx
}

func count(t *testing.T, tx pgx.Tx, sql string, args ...any) int {
	t.Helper()
	var n int
	if err := tx.QueryRow(context.Background(), sql, args...).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func errCode(m map[string]any) string {
	e, _ := m["error"].(map[string]any)
	s, _ := e["code"].(string)
	return s
}
