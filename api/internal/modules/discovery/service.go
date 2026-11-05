package discovery

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"net/http"
	"net/netip"
	"time"

	"dfipam/api/internal/platform/db"
	"dfipam/api/internal/platform/httpx"

	"github.com/jackc/pgx/v5"
)

var (
	errStopped     = httpx.NewError(http.StatusConflict, "DISCOVERY_STOPPED", "The emergency stop is on. No discovery runs until it is lifted.")
	errNotApproved = httpx.NewError(http.StatusConflict, "POLICY_NOT_APPROVED", "This policy hasn't been approved, so it can't run.")
)

func targetError(msg string) error {
	return httpx.NewError(http.StatusUnprocessableEntity, "TARGET_NOT_ALLOWED", msg)
}

type Service struct{ conn db.Conn }

func NewService(conn db.Conn) *Service { return &Service{conn} }

type querier interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
	QueryRow(context.Context, string, ...any) pgx.Row
}

// Stopped reports whether the emergency stop is on.
func Stopped(ctx context.Context, q querier) (bool, error) {
	var on bool
	err := q.QueryRow(ctx, `SELECT coalesce((SELECT value = 'true'::jsonb FROM system_settings WHERE key = 'discovery.emergency_stop'), false)`).Scan(&on)
	return on, err
}

// JobRequest asks for discovery of whole subnets under a policy.
type JobRequest struct {
	PolicyID  string   `json:"policyId"`
	SubnetIDs []string `json:"subnetIds"`
	AgentID   *string  `json:"agentId"`
	Methods   []string `json:"methods"`
	Priority  int      `json:"priority"`
}

// Snapshot is a job's frozen configuration. The database refuses changes to it.
type Snapshot struct {
	PolicyID       string    `json:"policyId"`
	PolicyName     string    `json:"policyName"`
	Methods        []string  `json:"methods"`
	MaxConcurrency int       `json:"maxConcurrency"`
	MaxPPS         int       `json:"maxPps"`
	TimeoutMS      int       `json:"timeoutMs"`
	Retries        int       `json:"retries"`
	Windows        []Window  `json:"windows"`
	Targets        []Target  `json:"targets"`
	Skipped        []string  `json:"skipped"`
	FrozenAt       time.Time `json:"frozenAt"`
}

type subnetScope struct {
	ID, RoutingDomainID, CIDR, Name, Sensitivity string
	Enabled, ServiceSensitive                    bool
}

func activeExclusions(ctx context.Context, q querier) ([]Exclusion, error) {
	rows, err := q.Query(ctx, `SELECT coalesce(routing_domain_id::text, ''), target::text FROM discovery_exclusions WHERE expires_at IS NULL OR expires_at > now()`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Exclusion
	for rows.Next() {
		var rd, cidr string
		if err := rows.Scan(&rd, &cidr); err != nil {
			return nil, err
		}
		if p, err := netip.ParsePrefix(cidr); err == nil {
			out = append(out, Exclusion{rd, p})
		}
	}
	return out, rows.Err()
}

// NewSecret returns a random secret and its stored hash.
func NewSecret() (string, string) {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	secret := base64.RawURLEncoding.EncodeToString(b)
	return secret, Hash(secret)
}

// Hash is how agent secrets are stored: never in plain text.
func Hash(secret string) string {
	sum := sha256.Sum256([]byte(secret))
	return hex.EncodeToString(sum[:])
}

// RegistrationTTL is how long a registration token stays usable.
const RegistrationTTL = 24 * time.Hour

// IssueToken creates a single-use registration token for an agent.
func IssueToken(ctx context.Context, tx pgx.Tx, actor, agentID string) (string, time.Time, error) {
	token, hash := NewSecret()
	expires := time.Now().Add(RegistrationTTL).UTC()
	if _, err := tx.Exec(ctx, `UPDATE agent_registration_tokens SET expires_at = now() WHERE agent_id = $1::uuid AND used_at IS NULL AND expires_at > now()`, agentID); err != nil {
		return "", expires, err
	}
	_, err := tx.Exec(ctx, `INSERT INTO agent_registration_tokens (agent_id, token_hash, created_by, expires_at) VALUES ($1::uuid, $2, $3::uuid, $4)`,
		agentID, hash, actor, expires)
	return token, expires, err
}
