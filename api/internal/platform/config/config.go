// Package config loads settings from the environment.
package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

type Config struct {
	Env            string
	ListenAddr     string
	LogLevel       string
	DatabaseURL    string
	AllowedOrigins []string
	TrustedProxies []string
	MaxInFlight    int
	AppBaseURL     string
	// Microsoft Entra ID sign-in. Off until the tenant, client ID and secret are set.
	EntraAuthority, EntraTenantID, EntraClientID, EntraClientSecret, EntraRedirectURL string
	AllowedEmailDomains                                                               []string
	// Development sign-in: no password, for while Entra ID isn't set up. Never in production.
	AuthBypass      bool
	AuthBypassEmail string
}

func get(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}

func splitList(value string) []string {
	var out []string
	for _, item := range strings.Split(value, ",") {
		if item = strings.TrimRight(strings.TrimSpace(item), "/"); item != "" {
			out = append(out, item)
		}
	}
	return out
}

// Load reads the API settings and reports every missing value at once.
func Load() (Config, error) {
	c := Config{
		Env:               get("APP_ENV", "development"),
		ListenAddr:        get("API_LISTEN_ADDR", "127.0.0.1:8080"),
		LogLevel:          get("LOG_LEVEL", "info"),
		DatabaseURL:       get("DATABASE_URL", ""),
		AllowedOrigins:    splitList(get("CORS_ALLOWED_ORIGINS", "")),
		TrustedProxies:    splitList(get("TRUSTED_PROXIES", "127.0.0.1,::1")),
		AppBaseURL:        strings.TrimRight(get("APP_BASE_URL", "http://localhost:5173"), "/"),
		EntraAuthority:    strings.TrimRight(get("ENTRA_AUTHORITY", "https://login.microsoftonline.com"), "/"),
		EntraTenantID:     get("ENTRA_TENANT_ID", ""),
		EntraClientID:     get("ENTRA_CLIENT_ID", ""),
		EntraClientSecret: get("ENTRA_CLIENT_SECRET", ""),
	}
	c.EntraRedirectURL = get("ENTRA_REDIRECT_URL", c.AppBaseURL+"/api/v1/auth/microsoft/callback")
	for _, d := range splitList(strings.ToLower(get("AUTH_ALLOWED_EMAIL_DOMAINS", "dialogfinance.lk"))) {
		c.AllowedEmailDomains = append(c.AllowedEmailDomains, strings.TrimPrefix(d, "@"))
	}
	if v := get("API_MAX_IN_FLIGHT", ""); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 {
			return c, fmt.Errorf("API_MAX_IN_FLIGHT must be a positive number")
		}
		c.MaxInFlight = n
	}
	if c.DatabaseURL == "" {
		return c, fmt.Errorf("missing settings: DATABASE_URL")
	}
	return c, c.loadBypass()
}

// loadBypass reads the development sign-in. It needs a work email to sign in
// as, and is refused outright in production.
func (c *Config) loadBypass() error {
	c.AuthBypass = strings.EqualFold(get("AUTH_BYPASS", "false"), "true")
	if !c.AuthBypass {
		return nil
	}
	if strings.EqualFold(c.Env, "production") {
		return fmt.Errorf("AUTH_BYPASS can't be used with APP_ENV=production: set up Microsoft Entra ID sign-in")
	}
	c.AuthBypassEmail = strings.ToLower(get("AUTH_BYPASS_EMAIL", ""))
	if _, domain, ok := strings.Cut(c.AuthBypassEmail, "@"); !ok || domain == "" || strings.Contains(domain, "@") {
		return fmt.Errorf("AUTH_BYPASS needs AUTH_BYPASS_EMAIL, the work email to sign in as")
	}
	return nil
}
