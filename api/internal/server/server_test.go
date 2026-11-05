package server

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"strings"
	"testing"

	"dfipam/api/internal/auth"

	"github.com/gin-gonic/gin"
)

type stubVerifier struct{}

func (stubVerifier) Verify(context.Context, string) (auth.Identity, error) {
	return auth.Identity{Subject: "u", Email: "u@example.com"}, nil
}

type stubResolver struct{ permissions map[string]bool }

func (s stubResolver) Resolve(context.Context, auth.Identity) (*auth.Principal, error) {
	return &auth.Principal{UserID: "u", Permissions: s.permissions}, nil
}

func engine(perms map[string]bool) (*gin.Engine, *auth.Guard) {
	gin.SetMode(gin.TestMode)
	return New(Deps{Verifier: stubVerifier{}, Resolver: stubResolver{perms}})
}

// Fails when a route is added without going through the permission guard.
func TestEveryRouteIsGuarded(t *testing.T) {
	r, guard := engine(nil)
	public := map[string]bool{"GET /health": true, "GET /ready": true}
	for _, route := range r.Routes() {
		key := route.Method + " " + route.Path
		if !public[key] && !guard.Registered[key] {
			t.Errorf("unguarded route: %s", key)
		}
	}
}

// Fails when a route is added without documenting it in the API contract.
func TestEveryRouteIsInOpenAPI(t *testing.T) {
	raw, err := os.ReadFile("../../openapi.yaml")
	if err != nil {
		t.Fatal(err)
	}
	blocks := map[string]string{}
	current := ""
	for _, line := range strings.Split(string(raw), "\n") {
		if strings.HasPrefix(line, "  /") {
			current = strings.TrimSuffix(strings.TrimSpace(line), ":")
			continue
		}
		if line != "" && !strings.HasPrefix(line, " ") {
			current = ""
		}
		if current != "" {
			blocks[current] += line + "\n"
		}
	}
	_, guard := engine(nil)
	for key := range guard.Registered {
		method, path, _ := strings.Cut(key, " ")
		path = strings.TrimPrefix(path, "/api/v1")
		path = regexp.MustCompile(`:(\w+)`).ReplaceAllString(path, "{$1}")
		if !strings.Contains(blocks[path], strings.ToLower(method)+":") {
			t.Errorf("not in openapi.yaml: %s %s", method, path)
		}
	}
}

func TestRequestsWithoutTokenAreRejected(t *testing.T) {
	r, _ := engine(nil)
	for i, route := range r.Routes() {
		if !strings.HasPrefix(route.Path, "/api/") {
			continue
		}
		w := httptest.NewRecorder()
		req := httptest.NewRequest(route.Method, strings.ReplaceAll(route.Path, ":id", "x"), nil)
		// One client per route, so the rate limiter doesn't answer before auth does.
		req.RemoteAddr = fmt.Sprintf("10.0.%d.%d:1234", i/250, i%250+1)
		r.ServeHTTP(w, req)
		if w.Code != http.StatusUnauthorized {
			t.Errorf("%s %s: got %d", route.Method, route.Path, w.Code)
		}
	}
}

func TestMissingPermissionIsForbidden(t *testing.T) {
	r, _ := engine(map[string]bool{})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/subnets", nil)
	req.Header.Set("Authorization", "Bearer token")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden || !strings.Contains(w.Body.String(), `"FORBIDDEN"`) {
		t.Fatalf("got %d %s", w.Code, w.Body.String())
	}
	if w.Header().Get("X-Correlation-ID") == "" {
		t.Error("missing correlation ID")
	}
}

func TestCORS(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r, _ := New(Deps{Verifier: stubVerifier{}, Resolver: stubResolver{}, Origins: []string{"http://localhost:5173"}})
	preflight := func(origin string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodOptions, "/api/v1/subnets", nil)
		req.Header.Set("Origin", origin)
		req.Header.Set("Access-Control-Request-Method", "POST")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		return w
	}
	ok := preflight("http://localhost:5173")
	if ok.Code != http.StatusNoContent || ok.Header().Get("Access-Control-Allow-Origin") != "http://localhost:5173" ||
		!strings.Contains(ok.Header().Get("Access-Control-Allow-Headers"), "Authorization") {
		t.Fatalf("allowed origin: %d %v", ok.Code, ok.Header())
	}
	if got := preflight("https://evil.example").Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("unknown origin was allowed: %q", got)
	}
}

func TestSecurityHeaders(t *testing.T) {
	r, _ := engine(nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/health", nil))
	if w.Code != http.StatusOK || w.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatalf("got %d %v", w.Code, w.Header())
	}
}
