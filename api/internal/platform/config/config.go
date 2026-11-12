// Package config loads settings from the environment.
package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

type Config struct {
	Env                string
	ListenAddr         string
	LogLevel           string
	DatabaseURL        string
	AllowedOrigins     []string
	SupabaseURL        string
	SupabaseAnonKey    string
	SupabaseServiceKey string
	AuthAudience       string
	TrustedProxies     []string
	MaxInFlight        int
	AppBaseURL         string
	// Microsoft Entra ID sign-in. Off until the tenant, client ID and secret are set.
	EntraAuthority, EntraTenantID, EntraClientID, EntraClientSecret, EntraRedirectURL string
	AllowedEmailDomains                                                               []string
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
		Env:                get("APP_ENV", "development"),
		ListenAddr:         get("API_LISTEN_ADDR", "127.0.0.1:8080"),
		LogLevel:           get("LOG_LEVEL", "info"),
		DatabaseURL:        get("DATABASE_URL", ""),
		AllowedOrigins:     splitList(get("CORS_ALLOWED_ORIGINS", "")),
		SupabaseURL:        strings.TrimRight(get("SUPABASE_URL", ""), "/"),
		SupabaseAnonKey:    get("SUPABASE_ANON_KEY", ""),
		SupabaseServiceKey: get("SUPABASE_SERVICE_ROLE_KEY", ""),
		AuthAudience:       get("AUTH_AUDIENCE", "authenticated"),
		TrustedProxies:     splitList(get("TRUSTED_PROXIES", "127.0.0.1,::1")),
		AppBaseURL:         strings.TrimRight(get("APP_BASE_URL", "http://localhost:5173"), "/"),
		EntraAuthority:     strings.TrimRight(get("ENTRA_AUTHORITY", "https://login.microsoftonline.com"), "/"),
		EntraTenantID:      get("ENTRA_TENANT_ID", ""),
		EntraClientID:      get("ENTRA_CLIENT_ID", ""),
		EntraClientSecret:  get("ENTRA_CLIENT_SECRET", ""),
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
	var missing []string
	for name, v := range map[string]string{
		"DATABASE_URL": c.DatabaseURL, "SUPABASE_URL": c.SupabaseURL, "SUPABASE_ANON_KEY": c.SupabaseAnonKey,
	} {
		if v == "" {
			missing = append(missing, name)
		}
	}
	if len(missing) > 0 {
		return c, fmt.Errorf("missing settings: %s", strings.Join(missing, ", "))
	}
	return c, nil
}
