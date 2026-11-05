package integrations

import (
	"context"
	"dfipam/api/internal/audit"
	"dfipam/api/internal/platform/db"
	"dfipam/api/internal/platform/httpx"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
)

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
