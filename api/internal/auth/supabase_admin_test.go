package auth

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestInviteSendsServiceKeyOnlyToSupabase(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/auth/v1/invite" || r.Header.Get("Authorization") != "Bearer service" || r.Header.Get("apikey") != "service" {
			t.Errorf("unexpected request: %s %v", r.URL.Path, r.Header)
		}
	}))
	defer srv.Close()
	if err := NewSupabaseAdmin(srv.URL, "anon", "service").Invite(context.Background(), "a@example.com"); err != nil {
		t.Fatal(err)
	}
}

func TestInviteErrors(t *testing.T) {
	status := http.StatusUnprocessableEntity
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(status) }))
	defer srv.Close()
	admin := NewSupabaseAdmin(srv.URL, "anon", "secret-service-key")
	if err := admin.Invite(context.Background(), "a@example.com"); !errors.Is(err, ErrAlreadyRegistered) {
		t.Fatalf("got %v", err)
	}
	status = http.StatusInternalServerError
	err := admin.Invite(context.Background(), "a@example.com")
	if err == nil || strings.Contains(err.Error(), "secret-service-key") {
		t.Fatalf("got %v", err)
	}
}

func TestCheckUsesAnonKey(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("apikey") != "anon" {
			w.WriteHeader(http.StatusUnauthorized)
		}
	}))
	defer srv.Close()
	if err := NewSupabaseAdmin(srv.URL, "anon", "s").Check(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := NewSupabaseAdmin(srv.URL, "wrong", "s").Check(context.Background()); err == nil {
		t.Fatal("expected failure")
	}
}
