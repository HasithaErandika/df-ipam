package integrations

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"regexp"
	"strings"
	"time"

	"dfipam/api/internal/audit"
	"dfipam/api/internal/modules/notifications"
	"dfipam/api/internal/platform/db"
	"dfipam/api/internal/platform/httpx"
	"dfipam/api/internal/platform/safehttp"
	"dfipam/api/internal/platform/secrets"

	"github.com/jackc/pgx/v5"
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

// WithBuilder replaces how connectors are built, for tests.
func (s *Service) WithBuilder(b Builder) *Service {
	s.build = b
	return s
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

func (s *Service) validate(in *Input, k Kind, creating bool) error {
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" || len(in.Name) > 200 {
		return httpx.Validation("name", "Enter a name of up to 200 characters.")
	}
	if !k.Available {
		return errUnavailable
	}
	if k.NeedsURL {
		if _, err := s.policy.CheckURL(in.BaseURL); err != nil {
			return httpx.Validation("baseUrl", "Use an https URL on the approved host allowlist (INTEGRATION_ALLOWED_HOSTS).")
		}
	} else {
		in.BaseURL = ""
	}
	if in.SyncIntervalMinutes == 0 {
		in.SyncIntervalMinutes = 60
	}
	if in.SyncIntervalMinutes < 5 || in.SyncIntervalMinutes > 10080 {
		return httpx.Validation("syncIntervalMinutes", "Choose between 5 minutes and 7 days.")
	}
	known := map[string]bool{}
	for _, o := range k.Config {
		known[o.Key] = true
		if o.Required && strings.TrimSpace(in.Config[o.Key]) == "" {
			return httpx.Validation(o.Key, o.Label+" is required.")
		}
	}
	for key := range in.Config {
		if !known[key] {
			return httpx.Validation(key, "This setting isn't used by this connector.")
		}
	}
	if err := checkSettings(in.Kind, in.Config); err != nil {
		return err
	}
	for context, id := range in.RoutingDomains {
		if context == "" || (id != "" && !httpx.IsUUID(id)) {
			return httpx.Validation("routingDomains", "Choose a valid routing domain.")
		}
	}
	if in.Credentials == nil {
		if creating {
			for _, o := range k.Credentials {
				if o.Required {
					return httpx.Validation(o.Key, o.Label+" is required.")
				}
			}
		}
		return nil
	}
	allowed := map[string]bool{}
	for _, o := range k.Credentials {
		allowed[o.Key] = true
		if o.Required && strings.TrimSpace(in.Credentials[o.Key]) == "" {
			return httpx.Validation(o.Key, o.Label+" is required.")
		}
	}
	for key := range in.Credentials {
		if !allowed[key] {
			return httpx.Validation(key, "This credential isn't used by this connector.")
		}
	}
	return nil
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

const columns = `c.id::text, c.kind, c.name, COALESCE(c.base_url, ''), c.config, c.credential_ciphertext IS NOT NULL, c.is_enabled,
	c.sync_interval_minutes, c.health, c.last_success_at, c.last_sync_at, c.last_error, c.created_at, c.updated_at,
	COALESCE((SELECT j.contexts FROM integration_sync_jobs j WHERE j.connection_id = c.id AND j.status = 'completed' ORDER BY j.created_at DESC LIMIT 1), '{}')`

func scanConnection(row pgx.Row) (Connection, error) {
	var c Connection
	var raw []byte
	err := row.Scan(&c.ID, &c.Kind, &c.Name, &c.BaseURL, &raw, &c.HasCredentials, &c.IsEnabled, &c.SyncIntervalMinutes, &c.Health,
		&c.LastSuccessAt, &c.LastSyncAt, &c.LastError, &c.CreatedAt, &c.UpdatedAt, &c.Contexts)
	if err != nil {
		return c, err
	}
	var cfg storedConfig
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return c, fmt.Errorf("connection config: %w", err)
	}
	c.Config, c.RoutingDomains = cfg.Settings, cfg.RoutingDomains
	if c.Config == nil {
		c.Config = map[string]string{}
	}
	if c.RoutingDomains == nil {
		c.RoutingDomains = map[string]string{}
	}
	return c, nil
}

// List returns one page of connections, optionally matching a name or URL.
func (s *Service) List(ctx context.Context, p httpx.Page, q string) ([]Connection, int64, error) {
	const match = ` WHERE $1 = '' OR c.name ILIKE '%' || $1 || '%' OR c.base_url ILIKE '%' || $1 || '%'`
	var total int64
	if err := s.conn.QueryRow(ctx, `SELECT count(*) FROM integration_connections c`+match, q).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := s.conn.Query(ctx, `SELECT `+columns+` FROM integration_connections c`+match+` ORDER BY c.name, c.id LIMIT $2 OFFSET $3`, q, p.Size, p.Offset())
	if err != nil {
		return nil, 0, err
	}
	out, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (Connection, error) { return scanConnection(r) })
	return out, total, err
}

// Get returns one connection.
func (s *Service) Get(ctx context.Context, id string) (Connection, error) {
	return s.get(ctx, s.conn, id)
}

func (s *Service) get(ctx context.Context, q db.Conn, id string) (Connection, error) {
	c, err := scanConnection(q.QueryRow(ctx, `SELECT `+columns+` FROM integration_connections c WHERE c.id = $1::uuid`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return c, httpx.ErrNotFound
	}
	return c, err
}

// redacted is what the audit log records. Secret values never reach it.
func redacted(in Input) map[string]any {
	keys := make([]string, 0, len(in.Credentials))
	for k := range in.Credentials {
		keys = append(keys, k)
	}
	sortStrings(keys)
	return map[string]any{"name": in.Name, "baseUrl": in.BaseURL, "config": in.Config, "routingDomains": in.RoutingDomains,
		"syncIntervalMinutes": in.SyncIntervalMinutes, "isEnabled": in.IsEnabled, "credentialsChanged": keys}
}

func (s *Service) encrypt(id string, credentials map[string]string) ([]byte, *int, error) {
	if s.keys == nil {
		return nil, nil, errNoKey
	}
	raw, err := json.Marshal(credentials)
	if err != nil {
		return nil, nil, err
	}
	sealed, version, err := s.keys.Encrypt(raw, []byte(id))
	if err != nil {
		return nil, nil, err
	}
	return sealed, &version, nil
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

func (s *Service) checkDomains(ctx context.Context, q pgx.Tx, domains map[string]string) error {
	for _, id := range domains {
		if id == "" {
			continue
		}
		var exists bool
		if err := q.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM routing_domains WHERE id = $1::uuid)`, id).Scan(&exists); err != nil {
			return err
		}
		if !exists {
			return httpx.Validation("routingDomains", "Choose a valid routing domain.")
		}
	}
	return nil
}

// Create saves a new connection.
func (s *Service) Create(ctx context.Context, actor string, in Input) (Connection, error) {
	k, ok := kindByName(in.Kind)
	if !ok {
		return Connection{}, httpx.Validation("kind", "Choose a valid connector.")
	}
	if err := s.validate(&in, k, true); err != nil {
		return Connection{}, err
	}
	if s.keys == nil && len(in.Credentials) > 0 {
		return Connection{}, errNoKey
	}
	raw, err := json.Marshal(storedConfig{in.Config, in.RoutingDomains})
	if err != nil {
		return Connection{}, err
	}
	enabled := in.IsEnabled == nil || *in.IsEnabled
	var out Connection
	err = db.InTx(ctx, s.conn, func(tx pgx.Tx) error {
		if err := s.checkDomains(ctx, tx, in.RoutingDomains); err != nil {
			return err
		}
		var id string
		if err := tx.QueryRow(ctx, `INSERT INTO integration_connections (kind, name, base_url, config, is_enabled, sync_interval_minutes, created_by, updated_by)
			VALUES ($1, $2, NULLIF($3, ''), $4::jsonb, $5, $6, $7::uuid, $7::uuid) RETURNING id::text`,
			in.Kind, in.Name, in.BaseURL, raw, enabled, in.SyncIntervalMinutes, actor).Scan(&id); err != nil {
			return err
		}
		if len(in.Credentials) > 0 {
			sealed, version, err := s.encrypt(id, in.Credentials)
			if err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, `UPDATE integration_connections SET credential_ciphertext = $2, key_version = $3 WHERE id = $1::uuid`, id, sealed, version); err != nil {
				return err
			}
		}
		var err error
		if out, err = s.get(ctx, tx, id); err != nil {
			return err
		}
		return audit.Write(ctx, tx, audit.Entry{ActorID: actor, Action: "integration.create", ObjectType: "integration_connection", ObjectID: id, New: redacted(in)})
	})
	return out, translate(err)
}

// Update changes a connection. Credentials change only when new ones are sent.
func (s *Service) Update(ctx context.Context, actor, id string, in Input) (Connection, error) {
	var out Connection
	err := db.InTx(ctx, s.conn, func(tx pgx.Tx) error {
		var locked string
		if err := tx.QueryRow(ctx, `SELECT id::text FROM integration_connections WHERE id = $1::uuid FOR UPDATE`, id).Scan(&locked); errors.Is(err, pgx.ErrNoRows) {
			return httpx.ErrNotFound
		} else if err != nil {
			return err
		}
		old, err := s.get(ctx, tx, id)
		if err != nil {
			return err
		}
		k, _ := kindByName(old.Kind)
		in.Kind = old.Kind
		if err := s.validate(&in, k, false); err != nil {
			return err
		}
		if err := s.checkDomains(ctx, tx, in.RoutingDomains); err != nil {
			return err
		}
		raw, err := json.Marshal(storedConfig{in.Config, in.RoutingDomains})
		if err != nil {
			return err
		}
		enabled := old.IsEnabled
		if in.IsEnabled != nil {
			enabled = *in.IsEnabled
		}
		if _, err := tx.Exec(ctx, `UPDATE integration_connections SET name = $2, base_url = NULLIF($3, ''), config = $4::jsonb, is_enabled = $5,
			sync_interval_minutes = $6, updated_by = $7::uuid WHERE id = $1::uuid`, id, in.Name, in.BaseURL, raw, enabled, in.SyncIntervalMinutes, actor); err != nil {
			return err
		}
		if in.Credentials != nil {
			sealed, version, err := s.encrypt(id, in.Credentials)
			if err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, `UPDATE integration_connections SET credential_ciphertext = $2, key_version = $3 WHERE id = $1::uuid`, id, sealed, version); err != nil {
				return err
			}
		}
		if out, err = s.get(ctx, tx, id); err != nil {
			return err
		}
		return audit.Write(ctx, tx, audit.Entry{ActorID: actor, Action: "integration.update", ObjectType: "integration_connection", ObjectID: id,
			Old: map[string]any{"name": old.Name, "baseUrl": old.BaseURL, "config": old.Config, "routingDomains": old.RoutingDomains,
				"syncIntervalMinutes": old.SyncIntervalMinutes, "isEnabled": old.IsEnabled}, New: redacted(in)})
	})
	return out, translate(err)
}

// Delete removes a connection that has never synced. History is evidence, so a
// connection with history can only be disabled.
func (s *Service) Delete(ctx context.Context, actor, id string) error {
	return db.InTx(ctx, s.conn, func(tx pgx.Tx) error {
		old, err := s.get(ctx, tx, id)
		if err != nil {
			return err
		}
		var used bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM integration_sync_jobs WHERE connection_id = $1::uuid)
			OR EXISTS (SELECT 1 FROM cloud_accounts WHERE connection_id = $1::uuid)
			OR EXISTS (SELECT 1 FROM external_resource_mappings WHERE connection_id = $1::uuid)`, id).Scan(&used); err != nil {
			return err
		}
		if used {
			return errHasHistory
		}
		if _, err := tx.Exec(ctx, `DELETE FROM integration_connections WHERE id = $1::uuid`, id); err != nil {
			return err
		}
		return audit.Write(ctx, tx, audit.Entry{ActorID: actor, Action: "integration.delete", ObjectType: "integration_connection", ObjectID: id,
			Old: map[string]any{"name": old.Name, "kind": old.Kind}})
	})
}

func (s *Service) credentials(ctx context.Context, id string) (map[string]string, error) {
	var sealed []byte
	var version *int
	if err := s.conn.QueryRow(ctx, `SELECT credential_ciphertext, key_version FROM integration_connections WHERE id = $1::uuid`, id).Scan(&sealed, &version); err != nil {
		return nil, err
	}
	if sealed == nil || version == nil {
		return map[string]string{}, nil
	}
	if s.keys == nil {
		return nil, errNoKey
	}
	raw, err := s.keys.Decrypt(sealed, *version, []byte(id))
	if err != nil {
		return nil, err
	}
	out := map[string]string{}
	return out, json.Unmarshal(raw, &out)
}

func (s *Service) connector(ctx context.Context, c Connection) (Connector, error) {
	creds, err := s.credentials(ctx, c.ID)
	if err != nil {
		return nil, err
	}
	k, _ := kindByName(c.Kind)
	for _, o := range k.Credentials {
		if o.Required && creds[o.Key] == "" {
			return nil, errNoCredential
		}
	}
	return s.build(c, creds, s.policy)
}

// TestResult reports a connection test.
type TestResult struct {
	OK      bool   `json:"ok"`
	Message string `json:"message"`
}

// Test makes one read against the platform.
func (s *Service) Test(ctx context.Context, actor, id string) (TestResult, error) {
	c, err := s.Get(ctx, id)
	if err != nil {
		return TestResult{}, err
	}
	conn, err := s.connector(ctx, c)
	var e *httpx.Error
	if errors.As(err, &e) {
		return TestResult{}, err
	}
	if err == nil {
		tctx, cancel := context.WithTimeout(ctx, testTimeout)
		defer cancel()
		err = conn.Test(tctx)
	}
	out := TestResult{OK: err == nil, Message: "Connected. The credentials can read this platform."}
	if err != nil {
		out.Message = safeMessage(err)
	}
	werr := db.InTx(ctx, s.conn, func(tx pgx.Tx) error {
		return audit.Write(ctx, tx, audit.Entry{ActorID: actor, Action: "integration.test", ObjectType: "integration_connection", ObjectID: id, New: out})
	})
	return out, werr
}

// safeMessage turns a connector failure into text for people and the database.
// Connectors put no secrets in errors, and this keeps policy refusals readable.
func safeMessage(err error) string {
	if errors.Is(err, safehttp.ErrBlocked) {
		return "The connection was refused by the outbound policy. Check the allowlist."
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "The platform didn't answer in time."
	}
	return truncate(err.Error(), 500)
}

// RequestSync queues a sync started by a person.
func (s *Service) RequestSync(ctx context.Context, actor, id string) (SyncJob, error) {
	var job SyncJob
	err := db.InTx(ctx, s.conn, func(tx pgx.Tx) error {
		c, err := s.get(ctx, tx, id)
		if err != nil {
			return err
		}
		if !c.HasCredentials && c.Kind != KindAWS {
			return errNoCredential
		}
		var jobID string
		err = tx.QueryRow(ctx, `INSERT INTO integration_sync_jobs (connection_id, requested_by) VALUES ($1::uuid, $2::uuid)
			ON CONFLICT (connection_id) WHERE status IN ('queued', 'running') DO NOTHING RETURNING id::text`, id, actor).Scan(&jobID)
		if errors.Is(err, pgx.ErrNoRows) {
			return errSyncActive
		}
		if err != nil {
			return err
		}
		if err := audit.Write(ctx, tx, audit.Entry{ActorID: actor, Action: "integration.sync_requested", ObjectType: "integration_connection", ObjectID: id,
			New: map[string]string{"jobId": jobID}}); err != nil {
			return err
		}
		job, err = s.syncJob(ctx, tx, jobID)
		return err
	})
	return job, err
}

const syncColumns = `j.id::text, j.connection_id::text, j.status, j.counts, j.contexts, j.error,
	(SELECT count(*) FROM integration_errors e WHERE e.sync_job_id = j.id), u.email, j.created_at, j.started_at, j.finished_at`

func scanSync(row pgx.Row) (SyncJob, error) {
	var j SyncJob
	return j, row.Scan(&j.ID, &j.ConnectionID, &j.Status, &j.Counts, &j.Contexts, &j.Error, &j.ErrorCount, &j.RequestedBy, &j.CreatedAt, &j.StartedAt, &j.FinishedAt)
}

func (s *Service) syncJob(ctx context.Context, q db.Conn, id string) (SyncJob, error) {
	return scanSync(q.QueryRow(ctx, `SELECT `+syncColumns+` FROM integration_sync_jobs j LEFT JOIN users u ON u.id = j.requested_by WHERE j.id = $1::uuid`, id))
}

// Syncs returns one page of a connection's history.
func (s *Service) Syncs(ctx context.Context, id string, p httpx.Page) ([]SyncJob, int64, error) {
	var total int64
	if err := s.conn.QueryRow(ctx, `SELECT count(*) FROM integration_sync_jobs WHERE connection_id = $1::uuid`, id).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := s.conn.Query(ctx, `SELECT `+syncColumns+` FROM integration_sync_jobs j LEFT JOIN users u ON u.id = j.requested_by
		WHERE j.connection_id = $1::uuid ORDER BY j.created_at DESC, j.id LIMIT $2 OFFSET $3`, id, p.Size, p.Offset())
	if err != nil {
		return nil, 0, err
	}
	out, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (SyncJob, error) { return scanSync(r) })
	return out, total, err
}

// Errors returns the problems a sync recorded.
func (s *Service) Errors(ctx context.Context, jobID string, p httpx.Page) ([]SyncError, int64, error) {
	var total int64
	if err := s.conn.QueryRow(ctx, `SELECT count(*) FROM integration_errors WHERE sync_job_id = $1::uuid`, jobID).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := s.conn.Query(ctx, `SELECT code, message, context, occurred_at FROM integration_errors WHERE sync_job_id = $1::uuid
		ORDER BY occurred_at, id LIMIT $2 OFFSET $3`, jobID, p.Size, p.Offset())
	if err != nil {
		return nil, 0, err
	}
	out, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (SyncError, error) {
		var e SyncError
		return e, r.Scan(&e.Code, &e.Message, &e.Context, &e.OccurredAt)
	})
	return out, total, err
}

// RunResult summarises one pass of the sync loop.
type RunResult struct{ Queued, Completed, Failed, Stuck int64 }

// RunSyncs fails stuck jobs, queues connections that are due, then works through
// the queue. SKIP LOCKED lets several workers share it.
func (s *Service) RunSyncs(ctx context.Context) (RunResult, error) {
	var out RunResult
	tag, err := s.conn.Exec(ctx, `UPDATE integration_sync_jobs SET status = 'failed', error = 'The worker stopped before this sync finished.', finished_at = now()
		WHERE status = 'running' AND started_at < now() - make_interval(mins => $1)`, int(stuckAfter.Minutes()))
	if err != nil {
		return out, err
	}
	out.Stuck = tag.RowsAffected()
	tag, err = s.conn.Exec(ctx, `INSERT INTO integration_sync_jobs (connection_id)
		SELECT c.id FROM integration_connections c
		WHERE c.is_enabled AND (c.credential_ciphertext IS NOT NULL OR c.kind = 'aws')
		  AND c.kind IN ('fortigate', 'vcenter', 'aws', 'azure')
		  AND (c.last_sync_at IS NULL OR c.last_sync_at < now() - make_interval(mins => c.sync_interval_minutes))
		ON CONFLICT (connection_id) WHERE status IN ('queued', 'running') DO NOTHING`)
	if err != nil {
		return out, err
	}
	out.Queued = tag.RowsAffected()
	for ctx.Err() == nil {
		var jobID, connID string
		err := s.conn.QueryRow(ctx, `UPDATE integration_sync_jobs SET status = 'running', started_at = now()
			WHERE id = (SELECT id FROM integration_sync_jobs WHERE status = 'queued' ORDER BY created_at FOR UPDATE SKIP LOCKED LIMIT 1)
			RETURNING id::text, connection_id::text`).Scan(&jobID, &connID)
		if errors.Is(err, pgx.ErrNoRows) {
			return out, nil
		}
		if err != nil {
			return out, err
		}
		if err := s.runOne(ctx, jobID, connID); err != nil {
			out.Failed++
			slog.Error("integration sync failed", "connection", connID, "job", jobID, "error", safeMessage(err))
			continue
		}
		out.Completed++
	}
	return out, ctx.Err()
}

func (s *Service) runOne(ctx context.Context, jobID, connID string) error {
	c, err := s.Get(ctx, connID)
	if err != nil {
		return s.finishFailed(ctx, jobID, connID, "", err)
	}
	conn, err := s.connector(ctx, c)
	if err != nil {
		return s.finishFailed(ctx, jobID, connID, c.Name, err)
	}
	sctx, cancel := context.WithTimeout(ctx, syncTimeout)
	defer cancel()
	result, err := conn.Collect(sctx)
	if err != nil {
		return s.finishFailed(ctx, jobID, connID, c.Name, err)
	}
	now := time.Now()
	err = db.InTx(ctx, s.conn, func(tx pgx.Tx) error {
		counts, err := save(ctx, tx, connID, c.Kind, jobID, c.RoutingDomains, result, now)
		if err != nil {
			return err
		}
		raw, err := json.Marshal(counts)
		if err != nil {
			return err
		}
		for _, w := range result.Warnings {
			detail, _ := json.Marshal(map[string]string{"context": w.Context})
			if _, err := tx.Exec(ctx, `INSERT INTO integration_errors (sync_job_id, code, message, context) VALUES ($1::uuid, $2, $3, $4::jsonb)`,
				jobID, w.Code, w.Message, detail); err != nil {
				return err
			}
		}
		health := "healthy"
		if len(result.Warnings) > 0 {
			health = "degraded"
		}
		if _, err := tx.Exec(ctx, `UPDATE integration_sync_jobs SET status = 'completed', counts = $2::jsonb, contexts = $3, finished_at = now() WHERE id = $1::uuid`,
			jobID, raw, result.Contexts()); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE integration_connections SET health = $2, last_success_at = now(), last_sync_at = now(), last_error = '' WHERE id = $1::uuid`, connID, health); err != nil {
			return err
		}
		return audit.Write(ctx, tx, audit.Entry{ActorType: "system", Action: "integration.sync", ObjectType: "integration_connection", ObjectID: connID, New: counts})
	})
	if err != nil {
		return s.finishFailed(ctx, jobID, connID, c.Name, err)
	}
	return nil
}

func (s *Service) finishFailed(ctx context.Context, jobID, connID, name string, cause error) error {
	message := safeMessage(cause)
	var e *httpx.Error
	if errors.As(cause, &e) {
		message = e.Message
	}
	// The sync's own context may have ended, but the failure must still be recorded.
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
	defer cancel()
	err := db.InTx(ctx, s.conn, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `UPDATE integration_sync_jobs SET status = 'failed', error = $2, finished_at = now() WHERE id = $1::uuid`, jobID, message); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO integration_errors (sync_job_id, code, message) VALUES ($1::uuid, 'SYNC_FAILED', $2)`, jobID, message); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE integration_connections SET health = 'failed', last_sync_at = now(), last_error = $2 WHERE id = $1::uuid`, connID, message); err != nil {
			return err
		}
		if err := audit.Write(ctx, tx, audit.Entry{ActorType: "system", Action: "integration.sync_failed", ObjectType: "integration_connection", ObjectID: connID,
			New: map[string]string{"error": message}}); err != nil {
			return err
		}
		recipients, err := notifications.UsersWithPermission(ctx, tx, "integrations:manage", nil)
		if err != nil {
			return err
		}
		return notifications.Enqueue(ctx, tx, "", notifications.Event{Type: notifications.IntegrationFailed, ObjectType: "integration_connection", ObjectID: connID,
			// One alert per connection per day, however often it retries.
			DedupeKey: fmt.Sprintf("integration-failed:%s:%s", connID, time.Now().UTC().Format("2006-01-02")),
			Title:     "Integration sync failed: " + name, Body: message, Recipients: recipients})
	})
	if err != nil {
		return errors.Join(cause, err)
	}
	return cause
}

// RotateCredentials re-encrypts every credential not on the current key. All of them
// change together, so a failure leaves nothing half done.
func (s *Service) RotateCredentials(ctx context.Context) (int, error) {
	if s.keys == nil {
		return 0, errNoKey
	}
	count := 0
	err := db.InTx(ctx, s.conn, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `SELECT id::text, credential_ciphertext, key_version FROM integration_connections
			WHERE credential_ciphertext IS NOT NULL AND key_version <> $1 FOR UPDATE`, s.keys.Version())
		if err != nil {
			return err
		}
		type stored struct {
			id      string
			sealed  []byte
			version int
		}
		items, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (stored, error) {
			var v stored
			return v, r.Scan(&v.id, &v.sealed, &v.version)
		})
		if err != nil {
			return err
		}
		for _, item := range items {
			plain, err := s.keys.Decrypt(item.sealed, item.version, []byte(item.id))
			if err != nil {
				return fmt.Errorf("connection %s: %w", item.id, err)
			}
			sealed, version, err := s.keys.Encrypt(plain, []byte(item.id))
			if err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, `UPDATE integration_connections SET credential_ciphertext = $2, key_version = $3 WHERE id = $1::uuid`, item.id, sealed, version); err != nil {
				return err
			}
			count++
		}
		return audit.Write(ctx, tx, audit.Entry{ActorType: "system", Action: "integration.credentials_rotated", ObjectType: "integration_connection",
			New: map[string]int{"count": count, "keyVersion": s.keys.Version()}})
	})
	return count, err
}
