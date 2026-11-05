package auth

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"
)

// ErrAlreadyRegistered means the invited email already has an account.
var ErrAlreadyRegistered = errors.New("already registered")

// SupabaseAdmin calls the Supabase Auth API. The service role key bypasses all
// Supabase security, so it is used only for invitations, and only here. It must
// never appear in a response, a log line or an error.
type SupabaseAdmin struct {
	url, anonKey, serviceKey string
	client                   *http.Client
}

func NewSupabaseAdmin(url, anonKey, serviceKey string) *SupabaseAdmin {
	return &SupabaseAdmin{url, anonKey, serviceKey, &http.Client{Timeout: 10 * time.Second}}
}

// Check confirms the project is reachable and accepts the anon key.
func (a *SupabaseAdmin) Check(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, a.url+"/auth/v1/settings", nil)
	if err != nil {
		return err
	}
	req.Header.Set("apikey", a.anonKey)
	res, err := a.client.Do(req)
	if err != nil {
		return fmt.Errorf("contact supabase: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("supabase settings returned %d", res.StatusCode)
	}
	return nil
}

// Invite emails a sign-up link to a new user.
func (a *SupabaseAdmin) Invite(ctx context.Context, email string) error {
	body, err := json.Marshal(map[string]string{"email": email})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.url+"/auth/v1/invite", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("apikey", a.serviceKey)
	req.Header.Set("Authorization", "Bearer "+a.serviceKey)
	req.Header.Set("Content-Type", "application/json")
	res, err := a.client.Do(req)
	if err != nil {
		return fmt.Errorf("contact supabase: %w", err)
	}
	defer res.Body.Close()
	switch {
	case res.StatusCode == http.StatusUnprocessableEntity:
		return ErrAlreadyRegistered
	case res.StatusCode >= 300:
		return fmt.Errorf("supabase invite returned %d", res.StatusCode)
	}
	return nil
}
