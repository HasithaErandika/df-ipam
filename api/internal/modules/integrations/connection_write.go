package integrations

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"dfipam/api/internal/audit"
	"dfipam/api/internal/platform/db"
	"dfipam/api/internal/platform/httpx"

	"github.com/jackc/pgx/v5"
)

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
	var out Connection
	err := db.InTx(ctx, s.conn, func(tx pgx.Tx) (err error) {
		out, err = s.insertConnection(ctx, tx, actor, in)
		return err
	})
	return out, translate(err)
}

func (s *Service) insertConnection(ctx context.Context, tx pgx.Tx, actor string, in Input) (Connection, error) {
	if err := s.checkDomains(ctx, tx, in.RoutingDomains); err != nil {
		return Connection{}, err
	}
	raw, err := json.Marshal(storedConfig{in.Config, in.RoutingDomains})
	if err != nil {
		return Connection{}, err
	}
	enabled := in.IsEnabled == nil || *in.IsEnabled
	var id string
	if err := tx.QueryRow(ctx, `INSERT INTO integration_connections (kind, name, base_url, config, is_enabled, sync_interval_minutes, created_by, updated_by)
		VALUES ($1, $2, NULLIF($3, ''), $4::jsonb, $5, $6, $7::uuid, $7::uuid) RETURNING id::text`,
		in.Kind, in.Name, in.BaseURL, raw, enabled, in.SyncIntervalMinutes, actor).Scan(&id); err != nil {
		return Connection{}, err
	}
	if len(in.Credentials) > 0 {
		if err := s.storeCredentials(ctx, tx, id, in.Credentials); err != nil {
			return Connection{}, err
		}
	}
	out, err := s.get(ctx, tx, id)
	if err != nil {
		return out, err
	}
	return out, audit.Write(ctx, tx, audit.Entry{ActorID: actor, Action: "integration.create", ObjectType: "integration_connection", ObjectID: id, New: redacted(in)})
}

func (s *Service) storeCredentials(ctx context.Context, tx pgx.Tx, id string, credentials map[string]string) error {
	sealed, version, err := s.encrypt(id, credentials)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `UPDATE integration_connections SET credential_ciphertext = $2, key_version = $3 WHERE id = $1::uuid`, id, sealed, version)
	return err
}

// Update changes a connection. Credentials change only when new ones are sent.
func (s *Service) Update(ctx context.Context, actor, id string, in Input) (Connection, error) {
	var out Connection
	err := db.InTx(ctx, s.conn, func(tx pgx.Tx) (err error) {
		out, err = s.updateConnection(ctx, tx, actor, id, in)
		return err
	})
	return out, translate(err)
}

func (s *Service) updateConnection(ctx context.Context, tx pgx.Tx, actor, id string, in Input) (Connection, error) {
	old, err := s.lockConnection(ctx, tx, id)
	if err != nil {
		return old, err
	}
	k, _ := kindByName(old.Kind)
	in.Kind = old.Kind
	if err := s.validate(&in, k, false); err != nil {
		return old, err
	}
	if err := s.checkDomains(ctx, tx, in.RoutingDomains); err != nil {
		return old, err
	}
	raw, err := json.Marshal(storedConfig{in.Config, in.RoutingDomains})
	if err != nil {
		return old, err
	}
	enabled := old.IsEnabled
	if in.IsEnabled != nil {
		enabled = *in.IsEnabled
	}
	if _, err := tx.Exec(ctx, `UPDATE integration_connections SET name = $2, base_url = NULLIF($3, ''), config = $4::jsonb, is_enabled = $5,
		sync_interval_minutes = $6, updated_by = $7::uuid WHERE id = $1::uuid`, id, in.Name, in.BaseURL, raw, enabled, in.SyncIntervalMinutes, actor); err != nil {
		return old, err
	}
	if in.Credentials != nil {
		if err := s.storeCredentials(ctx, tx, id, in.Credentials); err != nil {
			return old, err
		}
	}
	out, err := s.get(ctx, tx, id)
	if err != nil {
		return out, err
	}
	return out, audit.Write(ctx, tx, audit.Entry{ActorID: actor, Action: "integration.update", ObjectType: "integration_connection", ObjectID: id,
		Old: map[string]any{"name": old.Name, "baseUrl": old.BaseURL, "config": old.Config, "routingDomains": old.RoutingDomains,
			"syncIntervalMinutes": old.SyncIntervalMinutes, "isEnabled": old.IsEnabled}, New: redacted(in)})
}

func (s *Service) lockConnection(ctx context.Context, tx pgx.Tx, id string) (Connection, error) {
	var locked string
	err := tx.QueryRow(ctx, `SELECT id::text FROM integration_connections WHERE id = $1::uuid FOR UPDATE`, id).Scan(&locked)
	if errors.Is(err, pgx.ErrNoRows) {
		return Connection{}, httpx.ErrNotFound
	}
	if err != nil {
		return Connection{}, err
	}
	return s.get(ctx, tx, id)
}

// RotateCredentials re-encrypts every credential not on the current key. All of them
// change together, so a failure leaves nothing half done.
func (s *Service) RotateCredentials(ctx context.Context) (int, error) {
	if s.keys == nil {
		return 0, errNoKey
	}
	count := 0
	err := db.InTx(ctx, s.conn, func(tx pgx.Tx) error {
		items, err := sealedCredentials(ctx, tx, s.keys.Version())
		if err != nil {
			return err
		}
		for _, item := range items {
			if err := s.reseal(ctx, tx, item); err != nil {
				return err
			}
			count++
		}
		return audit.Write(ctx, tx, audit.Entry{ActorType: "system", Action: "integration.credentials_rotated", ObjectType: "integration_connection",
			New: map[string]int{"count": count, "keyVersion": s.keys.Version()}})
	})
	return count, err
}

type sealedCredential struct {
	id      string
	sealed  []byte
	version int
}

func sealedCredentials(ctx context.Context, tx pgx.Tx, current int) ([]sealedCredential, error) {
	rows, err := tx.Query(ctx, `SELECT id::text, credential_ciphertext, key_version FROM integration_connections
		WHERE credential_ciphertext IS NOT NULL AND key_version <> $1 FOR UPDATE`, current)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (sealedCredential, error) {
		var v sealedCredential
		return v, r.Scan(&v.id, &v.sealed, &v.version)
	})
}

func (s *Service) reseal(ctx context.Context, tx pgx.Tx, item sealedCredential) error {
	plain, err := s.keys.Decrypt(item.sealed, item.version, []byte(item.id))
	if err != nil {
		return fmt.Errorf("connection %s: %w", item.id, err)
	}
	sealed, version, err := s.keys.Encrypt(plain, []byte(item.id))
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `UPDATE integration_connections SET credential_ciphertext = $2, key_version = $3 WHERE id = $1::uuid`, item.id, sealed, version)
	return err
}
