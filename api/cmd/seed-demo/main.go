// Command seed-demo fills a development database with a sample network. It drives the
// real API in-process as sample people, so every record passes the same rules and
// approvals as real use, and is audited. It sends no packets and calls no real system.
// It refuses to run in production, and to run twice.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"time"

	"dfipam/api/internal/auth"
	"dfipam/api/internal/platform/db"
	"dfipam/api/internal/platform/safehttp"
	"dfipam/api/internal/platform/secrets"
	"dfipam/api/internal/server"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
)

// sample is everything the steps share.
type sample struct {
	ctx    context.Context
	pool   *pgxpool.Pool
	engine http.Handler
	keys   *secrets.Keyring
	policy safehttp.Policy
	people map[string]string // sample person to user ID
	ref    map[string]string // name to record ID
	nets   []network
	calls  int
}

type tokenVerifier struct{}

// Verify treats the bearer token as the sample person's name. This verifier exists
// only in this command, which never listens on a port.
func (tokenVerifier) Verify(_ context.Context, token string) (auth.Identity, error) {
	return auth.Identity{Subject: token, Email: emailOf(token)}, nil
}

func main() {
	defer func() {
		if r := recover(); r != nil {
			fmt.Fprintln(os.Stderr, "seed failed:", r)
			os.Exit(1)
		}
	}()
	if os.Getenv("APP_ENV") == "production" {
		panic("refusing to load sample data in production")
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError})))
	gin.SetMode(gin.ReleaseMode)
	ctx := context.Background()
	pool, err := db.Connect(ctx, os.Getenv("DATABASE_URL"))
	if err != nil {
		panic(err)
	}
	defer pool.Close()
	s := newSample(ctx, pool)
	if s.alreadySeeded() {
		fmt.Println("sample data is already loaded")
		return
	}
	start := time.Now()
	for _, step := range []struct {
		name string
		run  func()
	}{
		{"people", s.addPeople}, {"hierarchy", s.addHierarchy}, {"sites and VLANs", s.addSites}, {"blocks", s.addBlocks},
		{"subnets", s.addSubnets}, {"devices", s.addDevices}, {"addresses", s.addAddresses}, {"workflows", s.addWorkflows},
		{"requests", s.addRequests}, {"imports", s.addImports}, {"discovery", s.addDiscovery}, {"integrations", s.addIntegrations},
		{"findings", s.addFindings}, {"reports", s.addReports},
	} {
		step.run()
		fmt.Printf("%-16s done, %d calls so far\n", step.name, s.calls)
	}
	fmt.Printf("sample data loaded in %s\n", time.Since(start).Round(time.Second))
}

func newSample(ctx context.Context, pool *pgxpool.Pool) *sample {
	keys, err := secrets.FromEnv()
	if err != nil {
		panic("SECRET_ENCRYPTION_KEY is needed for the sample integrations: " + err.Error())
	}
	policy := safehttp.Policy{Hosts: []string{"fw.sample.example"}}
	engine, _ := server.New(server.Deps{Conn: pool, Ready: pool.Ping, Verifier: tokenVerifier{}, Keys: keys, Outbound: policy,
		Resolver: &auth.Resolver{Pool: pool, Provider: provider}})
	return &sample{ctx: ctx, pool: pool, engine: engine, keys: keys, policy: policy, people: map[string]string{}, ref: map[string]string{}}
}
