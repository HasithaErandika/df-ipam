package server

import (
	"bytes"
	"context"
	"dfipam/api/internal/modules/integrations"
	"dfipam/api/internal/platform/safehttp"
	"dfipam/api/internal/platform/secrets"
	"testing"

	"github.com/jackc/pgx/v5"
)

var testPolicy = safehttp.Policy{Hosts: []string{"fw.test.example"}}

func testKeys(t *testing.T) *secrets.Keyring {
	t.Helper()
	k, err := secrets.NewKeyring(1, map[int][]byte{1: bytes.Repeat([]byte{7}, 32)})
	if err != nil {
		t.Fatal(err)
	}
	return k
}

// fixedConnector returns canned evidence, or fails.
type fixedConnector struct {
	result *integrations.Result
	err    error
}

func (f fixedConnector) Test(context.Context) error { return f.err }

func (f fixedConnector) Collect(context.Context) (*integrations.Result, error) {
	return f.result, f.err
}

func text(t *testing.T, tx pgx.Tx, sql string, args ...any) string {
	t.Helper()
	var v string
	if err := tx.QueryRow(context.Background(), sql, args...).Scan(&v); err != nil {
		t.Fatalf("%s: %v", sql, err)
	}
	return v
}
