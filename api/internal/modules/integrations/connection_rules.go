package integrations

import (
	"context"
	"strings"

	"dfipam/api/internal/platform/httpx"

	"github.com/jackc/pgx/v5"
)

func (s *Service) validate(in *Input, k Kind, creating bool) error {
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" || len(in.Name) > 200 {
		return httpx.Validation("name", "Enter a name of up to 200 characters.")
	}
	if !k.Available {
		return errUnavailable
	}
	if err := s.checkEndpoint(in, k); err != nil {
		return err
	}
	if err := checkOptions(k.Config, in.Config, "This setting isn't used by this connector."); err != nil {
		return err
	}
	if err := checkSettings(in.Kind, in.Config); err != nil {
		return err
	}
	for context, id := range in.RoutingDomains {
		if context == "" || (id != "" && !httpx.IsUUID(id)) {
			return httpx.Validation("routingDomains", "Choose a valid routing domain.")
		}
	}
	return checkCredentials(in, k, creating)
}

func (s *Service) checkEndpoint(in *Input, k Kind) error {
	if !k.NeedsURL {
		in.BaseURL = ""
	} else if _, err := s.policy.CheckURL(in.BaseURL); err != nil {
		return httpx.Validation("baseUrl", "Use an https URL on the approved host allowlist (INTEGRATION_ALLOWED_HOSTS).")
	}
	if in.SyncIntervalMinutes == 0 {
		in.SyncIntervalMinutes = 60
	}
	if in.SyncIntervalMinutes < 5 || in.SyncIntervalMinutes > 10080 {
		return httpx.Validation("syncIntervalMinutes", "Choose between 5 minutes and 7 days.")
	}
	return nil
}

// checkOptions needs every required option, and refuses any the connector doesn't use.
func checkOptions(options []Option, values map[string]string, unknown string) error {
	known := make(map[string]bool, len(options))
	for _, o := range options {
		known[o.Key] = true
		if o.Required && strings.TrimSpace(values[o.Key]) == "" {
			return httpx.Validation(o.Key, o.Label+" is required.")
		}
	}
	for key := range values {
		if !known[key] {
			return httpx.Validation(key, unknown)
		}
	}
	return nil
}

// checkCredentials applies when credentials are sent, and always to a new connection.
func checkCredentials(in *Input, k Kind, creating bool) error {
	if in.Credentials == nil && !creating {
		return nil
	}
	return checkOptions(k.Credentials, in.Credentials, "This credential isn't used by this connector.")
}

// checkDomains confirms every mapped routing domain exists, in one query.
func (s *Service) checkDomains(ctx context.Context, q pgx.Tx, domains map[string]string) error {
	wanted := map[string]bool{}
	for _, id := range domains {
		if id != "" {
			wanted[strings.ToLower(id)] = true
		}
	}
	if len(wanted) == 0 {
		return nil
	}
	ids := make([]string, 0, len(wanted))
	for id := range wanted {
		ids = append(ids, id)
	}
	var found int
	if err := q.QueryRow(ctx, `SELECT count(*) FROM routing_domains WHERE id = ANY($1::uuid[])`, ids).Scan(&found); err != nil {
		return err
	}
	if found != len(ids) {
		return httpx.Validation("routingDomains", "Choose a valid routing domain.")
	}
	return nil
}
