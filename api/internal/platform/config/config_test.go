package config

import "testing"

func TestBypassIsRefusedInProduction(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://x")
	t.Setenv("AUTH_BYPASS", "true")
	t.Setenv("AUTH_BYPASS_EMAIL", "a@dialogfinance.lk")
	t.Setenv("APP_ENV", "production")
	if _, err := Load(); err == nil {
		t.Fatal("bypass was allowed in production")
	}
	t.Setenv("APP_ENV", "development")
	if c, err := Load(); err != nil || !c.AuthBypass || c.AuthBypassEmail != "a@dialogfinance.lk" {
		t.Fatalf("development: %+v %v", c, err)
	}
	t.Setenv("AUTH_BYPASS_EMAIL", "")
	if _, err := Load(); err == nil {
		t.Fatal("bypass without an email was allowed")
	}
}
