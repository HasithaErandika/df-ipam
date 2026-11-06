package ipaddresses

import (
	"context"
	"errors"
	"net/http"
	"net/netip"
	"strings"
	"time"

	"dfipam/api/internal/audit"
	"dfipam/api/internal/platform/httpx"

	"github.com/jackc/pgx/v5"
)

// NewAddress is an address record to create from an approved import.
type NewAddress = writeRequest

// Insert creates an address under the same rules as the screen. Whether the people
// involved may set an allocation state is the caller's to check.
func Insert(ctx context.Context, tx pgx.Tx, actor string, req NewAddress) (string, error) {
	addr, err := netip.ParseAddr(strings.TrimSpace(req.Address))
	if err != nil {
		return "", httpx.Validation("address", "Enter a valid IP address.")
	}
	if err := req.checkValues(); err != nil {
		return "", err
	}
	id, err := insertAddress(ctx, tx, actor, addr, req)
	return id, translate(err)
}

// RemoveImported deletes addresses an import created, with the allocations and
// reservations it made for them. Any address changed since the import stops it.
func RemoveImported(ctx context.Context, tx pgx.Tx, actor string, ids []string, committedAt time.Time) error {
	var changed string
	err := tx.QueryRow(ctx, `SELECT host(address) FROM ip_addresses WHERE id = ANY($1::uuid[]) AND updated_at > $2 LIMIT 1`, ids, committedAt).Scan(&changed)
	if err == nil {
		return httpx.NewError(http.StatusConflict, "ADDRESS_CHANGED", changed+" has changed since the import.")
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	for _, table := range []string{"ip_allocations", "ip_reservations"} {
		if _, err := tx.Exec(ctx, "DELETE FROM "+table+" WHERE ip_address_id = ANY($1::uuid[])", ids); err != nil {
			return err
		}
	}
	if _, err := tx.Exec(ctx, `DELETE FROM ip_addresses WHERE id = ANY($1::uuid[])`, ids); err != nil {
		return err
	}
	entries := make([]audit.Entry, len(ids))
	for i, id := range ids {
		entries[i] = audit.Entry{ActorID: actor, Action: "ip_address.delete", ObjectType: "ip_address", ObjectID: id}
	}
	return audit.WriteMany(ctx, tx, entries)
}
