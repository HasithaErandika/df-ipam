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
	WorkerEnabled      bool
	MaxInFlight        int
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
		WorkerEnabled:      get("WORKER_ENABLED", "true") != "false",
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
