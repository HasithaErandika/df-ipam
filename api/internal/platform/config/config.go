// Package config loads settings from the environment.
package config

import (
	"fmt"
	"net/url"
	"os"
	"sort"
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
	// AttachmentsDir holds network link attachments: a Docker volume in production.
	AttachmentsDir      string
	AllowedEmailDomains []string
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
		AttachmentsDir:    get("ATTACHMENTS_DIR", "data/attachments"),
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
	return c, c.checkProduction()
}

// checkProduction refuses to start a server nobody can sign in to, or one that
// would send the session cookie over plain HTTP.
func (c *Config) checkProduction() error {
	if !strings.EqualFold(c.Env, "production") {
		return nil
	}
	var missing []string
	for key, v := range map[string]string{"ENTRA_TENANT_ID": c.EntraTenantID, "ENTRA_CLIENT_ID": c.EntraClientID, "ENTRA_CLIENT_SECRET": c.EntraClientSecret} {
		if v == "" {
			missing = append(missing, key)
		}
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		return fmt.Errorf("APP_ENV=production needs Microsoft Entra ID sign-in: missing %s", strings.Join(missing, ", "))
	}
	if !strings.HasPrefix(c.AppBaseURL, "https://") && !loopback(c.AppBaseURL) {
		return fmt.Errorf("APP_ENV=production needs APP_BASE_URL to start with https:// (plain http only for localhost)")
	}
	return nil
}

// loopback reports whether the address is this machine, where plain HTTP never
// crosses the network.
func loopback(address string) bool {
	u, err := url.Parse(address)
	if err != nil || u.Scheme != "http" {
		return false
	}
	switch u.Hostname() {
	case "localhost", "127.0.0.1", "::1":
		return true
	}
	return false
}
