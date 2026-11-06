// Package agent is the discovery agent: it registers with the API, sends
// heartbeats, claims approved jobs and posts what it finds. It never talks to
// the database, never touches an address outside its job's frozen targets,
// and stops at once when the emergency stop is on.
package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

// Version is reported in heartbeats.
const Version = "0.4.0"

type Config struct {
	MaxPPS            int // this site's own limit on packets per second, 0 for none
	APIURL            string
	RegistrationToken string
	CredentialFile    string
	Poll              time.Duration
}

// ConfigFromEnv reads AGENT_API_URL, AGENT_REGISTRATION_TOKEN, AGENT_CREDENTIAL_FILE
// and AGENT_POLL_SECONDS. Plain http is refused except to plainHTTPAllowed hosts.
func ConfigFromEnv() (Config, error) {
	c := Config{
		APIURL:            strings.TrimRight(strings.TrimSpace(os.Getenv("AGENT_API_URL")), "/"),
		RegistrationToken: strings.TrimSpace(os.Getenv("AGENT_REGISTRATION_TOKEN")),
		CredentialFile:    strings.TrimSpace(os.Getenv("AGENT_CREDENTIAL_FILE")),
		Poll:              30 * time.Second,
	}
	if c.CredentialFile == "" {
		c.CredentialFile = "agent.credential"
	}
	if n, err := strconv.Atoi(os.Getenv("AGENT_MAX_PPS")); err == nil && n > 0 {
		c.MaxPPS = n
	}
	if v := os.Getenv("AGENT_POLL_SECONDS"); v != "" {
		if d, err := time.ParseDuration(v + "s"); err == nil && d >= 5*time.Second {
			c.Poll = d
		}
	}
	u, err := url.Parse(c.APIURL)
	if c.APIURL == "" || err != nil {
		return c, errors.New("AGENT_API_URL must be the DF-IPAM address, such as https://ipam.example.com")
	}
	if u.Scheme != "https" && !plainHTTPAllowed(u.Hostname()) {
		return c, errors.New("AGENT_API_URL must use https")
	}
	return c, nil
}

// plainHTTPAllowed names the hosts reached without crossing a network: this machine,
// and the backend container from the built-in agent on Docker's private network.
func plainHTTPAllowed(host string) bool {
	return host == "localhost" || host == "127.0.0.1" || host == "backend"
}

// Client calls the agent API.
type Client struct {
	cfg        Config
	http       *http.Client
	credential string
}

func NewClient(cfg Config) *Client {
	return &Client{cfg: cfg, http: &http.Client{Timeout: 60 * time.Second}}
}

type apiError struct {
	Status int
	Body   string
}

func (e *apiError) Error() string { return fmt.Sprintf("api returned %d: %s", e.Status, e.Body) }

func (c *Client) call(ctx context.Context, method, path string, body, out any) (int, error) {
	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return 0, err
		}
		reader = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.cfg.APIURL+"/api/v1/agent-api"+path, reader)
	if err != nil {
		return 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.credential != "" {
		req.Header.Set("Authorization", "Agent "+c.credential)
	}
	res, err := c.http.Do(req)
	if err != nil {
		return 0, err
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(res.Body, 16<<20))
	if res.StatusCode >= 300 {
		return res.StatusCode, &apiError{res.StatusCode, strings.TrimSpace(string(raw))}
	}
	if out != nil && res.StatusCode != http.StatusNoContent {
		var env struct {
			Data json.RawMessage `json:"data"`
		}
		if err := json.Unmarshal(raw, &env); err != nil {
			return res.StatusCode, err
		}
		return res.StatusCode, json.Unmarshal(env.Data, out)
	}
	return res.StatusCode, nil
}

// Authenticate loads the stored credential, or registers with the one-time token
// and stores the credential with owner-only permissions. A token is tried even with
// a stored credential, because rotation works that way. A token that was already
// used then fails, and the stored credential carries on, so a restart is safe.
func (c *Client) Authenticate(ctx context.Context) error {
	var stored string
	if raw, err := os.ReadFile(c.cfg.CredentialFile); err == nil {
		stored = strings.TrimSpace(string(raw))
	}
	if c.cfg.RegistrationToken == "" {
		if stored == "" {
			return errors.New("no stored credential: set AGENT_REGISTRATION_TOKEN to register this agent")
		}
		c.credential = stored
		return nil
	}
	var out struct {
		Credential string `json:"credential"`
	}
	c.credential = ""
	if _, err := c.call(ctx, http.MethodPost, "/register", map[string]string{"token": c.cfg.RegistrationToken, "version": Version}, &out); err != nil {
		if stored == "" {
			return fmt.Errorf("register: %w", err)
		}
		slog.Warn("registration token not accepted: using the stored credential", "error", err)
		c.credential = stored
		return nil
	}
	c.credential = out.Credential
	return os.WriteFile(c.cfg.CredentialFile, []byte(out.Credential+"\n"), 0o600)
}

// Heartbeat reports health and returns whether the emergency stop is on.
func (c *Client) Heartbeat(ctx context.Context, metrics map[string]any) (bool, error) {
	var out struct {
		EmergencyStop bool `json:"emergencyStop"`
	}
	_, err := c.call(ctx, http.MethodPost, "/heartbeat", map[string]any{"version": Version, "metrics": metrics}, &out)
	return out.EmergencyStop, err
}
