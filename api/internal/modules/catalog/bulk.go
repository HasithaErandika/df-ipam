package catalog

import (
	"context"
	"fmt"
	"strings"

	"dfipam/api/internal/audit"

	"github.com/jackc/pgx/v5"
)

// ByPath finds an entity definition.
func ByPath(path string) (Entity, bool) {
	for _, e := range Entities {
		if e.Path == path {
			return e, true
		}
	}
	return Entity{}, false
}

// BulkError says which row of a bulk insert failed.
type BulkError struct {
	Index int
	Err   error
}

func (e *BulkError) Error() string { return e.Err.Error() }
func (e *BulkError) Unwrap() error { return e.Err }

// InsertMany creates rows under the same field rules as the API, in one round trip,
// with their audit rows. It returns the new IDs in order.
func InsertMany(ctx context.Context, tx pgx.Tx, actor string, e Entity, bodies []map[string]any) ([]string, error) {
	batch := &pgx.Batch{}
	for i, body := range bodies {
		row, err := newRow(e, body, actor)
		if err != nil {
			return nil, &BulkError{i, err}
		}
		batch.Queue(fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s) RETURNING id::text", e.Table, strings.Join(row.cols, ", "), strings.Join(row.marks, ", ")), row.args...)
	}
	results := tx.SendBatch(ctx, batch)
	ids, entries := make([]string, len(bodies)), make([]audit.Entry, len(bodies))
	for i := range bodies {
		if err := results.QueryRow().Scan(&ids[i]); err != nil {
			_ = results.Close()
			return nil, &BulkError{i, translate(err)}
		}
		entries[i] = audit.Entry{ActorID: actor, Action: e.Object + ".create", ObjectType: e.Object, ObjectID: ids[i], New: bodies[i]}
	}
	if err := results.Close(); err != nil {
		return nil, translate(err)
	}
	return ids, audit.WriteMany(ctx, tx, entries)
}

// DeleteMany removes rows with their audit rows. A row still in use stops it all.
func DeleteMany(ctx context.Context, tx pgx.Tx, actor string, e Entity, ids []string) error {
	if _, err := tx.Exec(ctx, "DELETE FROM "+e.Table+" WHERE id = ANY($1::uuid[])", ids); err != nil {
		return translate(err)
	}
	entries := make([]audit.Entry, len(ids))
	for i, id := range ids {
		entries[i] = audit.Entry{ActorID: actor, Action: e.Object + ".delete", ObjectType: e.Object, ObjectID: id}
	}
	return audit.WriteMany(ctx, tx, entries)
}
