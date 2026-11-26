package config

import (
	"strings"
	"testing"
)

func TestProductionNeedsEntraAndHTTPS(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://x")
	t.Setenv("APP_ENV", "production")
	t.Setenv("APP_BASE_URL", "https://ipam.dialogfinance.lk")
	t.Setenv("ENTRA_TENANT_ID", "")
	t.Setenv("ENTRA_CLIENT_ID", "client")
	t.Setenv("ENTRA_CLIENT_SECRET", "")
	_, err := Load()
	if err == nil || !strings.Contains(err.Error(), "ENTRA_CLIENT_SECRET, ENTRA_TENANT_ID") {
		t.Fatalf("production without Entra ID: %v", err)
	}
	t.Setenv("ENTRA_TENANT_ID", "tenant")
	t.Setenv("ENTRA_CLIENT_SECRET", "secret")
	if _, err := Load(); err != nil {
		t.Fatalf("production with Entra ID: %v", err)
	}
	for _, local := range []string{"http://localhost:5173", "http://127.0.0.1:8088"} {
		t.Setenv("APP_BASE_URL", local)
		if _, err := Load(); err != nil {
			t.Fatalf("production on %s: %v", local, err)
		}
	}
	for _, remote := range []string{"http://ipam.dialogfinance.lk", "http://localhost.evil.example", "http://10.0.0.5"} {
		t.Setenv("APP_BASE_URL", remote)
		if _, err := Load(); err == nil {
			t.Fatalf("production over plain HTTP was allowed on %s", remote)
		}
	}
}

func TestDevelopmentStartsWithoutEntra(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://x")
	t.Setenv("APP_ENV", "development")
	t.Setenv("ENTRA_TENANT_ID", "")
	if _, err := Load(); err != nil {
		t.Fatalf("development: %v", err)
	}
}

func TestBadTrustedProxyStopsStartup(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://x")
	t.Setenv("APP_ENV", "development")
	t.Setenv("TRUSTED_PROXIES", "172.30.0.10,web-container")
	if _, err := Load(); err == nil || !strings.Contains(err.Error(), "web-container") {
		t.Fatalf("want a TRUSTED_PROXIES error, got %v", err)
	}
	t.Setenv("TRUSTED_PROXIES", "172.30.0.10,10.0.0.0/8")
	if _, err := Load(); err != nil {
		t.Fatal(err)
	}
}
