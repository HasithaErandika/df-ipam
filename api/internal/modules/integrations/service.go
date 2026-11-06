package integrations

import (
	"net/http"
	"regexp"
	"strings"
	"time"

	"dfipam/api/internal/platform/db"
	"dfipam/api/internal/platform/httpx"
	"dfipam/api/internal/platform/safehttp"
	"dfipam/api/internal/platform/secrets"
)

// A sync that runs longer than this is cut off, and one left running by a worker
// that died is failed so the connection can sync again.
const (
	syncTimeout = 30 * time.Minute
	stuckAfter  = 45 * time.Minute
	testTimeout = 20 * time.Second
)

var (
	errNoKey = httpx.NewError(http.StatusServiceUnavailable, "ENCRYPTION_NOT_CONFIGURED",
		"Credentials can't be stored until SECRET_ENCRYPTION_KEY is set on the server.")
	errUnavailable  = httpx.NewError(http.StatusUnprocessableEntity, "CONNECTOR_NOT_AVAILABLE", "This connector isn't available yet.")
	errNoCredential = httpx.NewError(http.StatusConflict, "CREDENTIALS_MISSING", "Add credentials to this connection first.")
	errHasHistory   = httpx.NewError(http.StatusConflict, "CONNECTION_HAS_HISTORY", "This connection has sync history, which is kept as evidence. Disable it instead.")
	errSyncActive   = httpx.NewError(http.StatusConflict, "SYNC_ACTIVE", "A sync is already queued or running for this connection.")
	errNameTaken    = httpx.NewError(http.StatusConflict, "NAME_TAKEN", "Another connection has this name.")
)

// Builder creates a connector. Tests replace it with one that returns fixtures.
type Builder func(c Connection, credentials map[string]string, policy safehttp.Policy) (Connector, error)

// Service manages connections and runs syncs.
type Service struct {
	conn   db.Conn
	keys   *secrets.Keyring
	policy safehttp.Policy
	build  Builder
}

// NewService returns a service. keys may be nil, in which case credentials can't
// be saved or used and the API says so.
func NewService(conn db.Conn, keys *secrets.Keyring, policy safehttp.Policy) *Service {
	return &Service{conn, keys, policy, buildConnector}
}

func buildConnector(c Connection, credentials map[string]string, policy safehttp.Policy) (Connector, error) {
	switch c.Kind {
	case KindFortiGate:
		return newFortiGate(c, credentials, policy)
	case KindVCenter:
		return newVCenter(c, credentials, policy)
	case KindAWS:
		return newAWS(c, credentials)
	case KindAzure:
		return newAzure(c, credentials)
	}
	return nil, errUnavailable
}

// Input is what a person can set on a connection. Nil Credentials keeps the stored ones.
type Input struct {
	Kind                string            `json:"kind"`
	Name                string            `json:"name"`
	BaseURL             string            `json:"baseUrl"`
	Config              map[string]string `json:"config"`
	RoutingDomains      map[string]string `json:"routingDomains"`
	Credentials         map[string]string `json:"credentials"`
	IsEnabled           *bool             `json:"isEnabled"`
	SyncIntervalMinutes int               `json:"syncIntervalMinutes"`
}

var (
	roleARN   = regexp.MustCompile(`^arn:aws[\w-]*:iam::\d{12}:role/[\w+=,.@/-]+$`)
	awsRegion = regexp.MustCompile(`^[a-z]{2}(-[a-z]+)+-\d$`)
)

// checkSettings refuses values a connector could never use, so the mistake shows
// when saving instead of at the first sync.
func checkSettings(kind string, config map[string]string) error {
	switch kind {
	case KindAWS:
		if !roleARN.MatchString(strings.TrimSpace(config["roleArn"])) {
			return httpx.Validation("roleArn", "Use a role ARN such as arn:aws:iam::123456789012:role/df-ipam-read.")
		}
		regions := splitRegions(config["regions"])
		if len(regions) == 0 {
			return httpx.Validation("regions", "List at least one region.")
		}
		for _, r := range regions {
			if !awsRegion.MatchString(r) {
				return httpx.Validation("regions", "List regions such as ap-south-1, separated by commas.")
			}
		}
	case KindAzure:
		for _, key := range []string{"tenantId", "clientId", "subscriptionId"} {
			if !httpx.IsUUID(strings.TrimSpace(config[key])) {
				return httpx.Validation(key, "Use the ID in its GUID form.")
			}
		}
	}
	return nil
}

func translate(err error) error {
	if db.ErrorCode(err) == "23505" {
		return errNameTaken
	}
	if db.ErrorCode(err) == "23503" {
		return httpx.Validation("routingDomains", "Choose a valid routing domain.")
	}
	return err
}

// WithBuilder replaces how connectors are built, for tests.
func (s *Service) WithBuilder(b Builder) *Service {
	s.build = b
	return s
}
