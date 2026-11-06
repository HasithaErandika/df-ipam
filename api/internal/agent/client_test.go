package agent

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPlainHTTPOnlyOnPrivatePaths(t *testing.T) {
	for url, ok := range map[string]bool{
		"http://backend:8080":      true,
		"http://localhost:8080":    true,
		"https://ipam.example.com": true,
		"http://ipam.example.com":  false,
		"http://backend.evil.com":  false,
	} {
		t.Setenv("AGENT_API_URL", url)
		if _, err := ConfigFromEnv(); (err == nil) != ok {
			t.Errorf("%s: %v", url, err)
		}
	}
}

// registrar answers /register with the credential, or refuses when it is empty.
func registrar(t *testing.T, credential string) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if credential == "" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_, _ = w.Write([]byte(`{"data":{"credential":"` + credential + `"}}`))
	}))
	t.Cleanup(srv.Close)
	return srv.URL
}

func authenticate(t *testing.T, url, token, stored string) (*Client, string, error) {
	t.Helper()
	file := filepath.Join(t.TempDir(), "credential")
	if stored != "" {
		if err := os.WriteFile(file, []byte(stored+"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	c := NewClient(Config{APIURL: url, RegistrationToken: token, CredentialFile: file})
	return c, file, c.Authenticate(context.Background())
}

func TestUsedTokenFallsBackToStoredCredential(t *testing.T) {
	c, _, err := authenticate(t, registrar(t, ""), "used", "stored")
	if err != nil || c.credential != "stored" {
		t.Fatalf("credential %q, error %v", c.credential, err)
	}
}

func TestNewTokenReplacesStoredCredential(t *testing.T) {
	c, file, err := authenticate(t, registrar(t, "rotated"), "fresh", "stored")
	if err != nil || c.credential != "rotated" {
		t.Fatalf("credential %q, error %v", c.credential, err)
	}
	if raw, _ := os.ReadFile(file); strings.TrimSpace(string(raw)) != "rotated" {
		t.Fatalf("stored: %q", raw)
	}
}

func TestRefusedTokenWithNothingStoredFails(t *testing.T) {
	if _, _, err := authenticate(t, registrar(t, ""), "bad", ""); err == nil {
		t.Fatal("an agent with no credential must not start")
	}
}
