package intune

import (
	"context"
	"time"

	"dfipam/api/internal/platform/db"

	"github.com/jackc/pgx/v5"
)

const setEntra = `UPDATE sightings o SET entra_user = x.u, entra_user_name = x.n, entra_device = x.d, entra_join = x.j,
	entra_last_sign_in = x.signed, entra_seen_at = now()
	FROM unnest($1::text[], $2::text[], $3::text[], $4::text[], $5::text[], $6::timestamptz[]) AS x(k, u, n, d, j, signed)
	WHERE ` + hostKey + ` = x.k`

// saveEntra records each Entra device's owner on the sightings with its
// computer name, and clears sightings no Entra device matched any more. It
// returns how many sightings are now matched to an Entra device.
func saveEntra(ctx context.Context, conn db.Conn, devs []EntraDevice) (int64, error) {
	var keys, users, names, devices, joins []string
	var signed []*time.Time
	for k, d := range entraIndex(devs) {
		u, n := d.owner()
		keys, users, names, devices, joins = append(keys, k), append(users, u), append(names, n), append(devices, d.Name), append(joins, d.Join)
		signed = append(signed, d.LastSignIn)
	}
	var matched int64
	err := db.InTx(ctx, conn, func(tx pgx.Tx) error {
		var start time.Time
		if err := tx.QueryRow(ctx, `SELECT now()`).Scan(&start); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, setEntra, keys, users, names, devices, joins, signed); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE sightings SET entra_user = '', entra_user_name = '', entra_device = '', entra_join = '',
			entra_last_sign_in = NULL, entra_seen_at = NULL WHERE entra_seen_at < $1`, start); err != nil {
			return err
		}
		return tx.QueryRow(ctx, `SELECT count(*) FROM sightings WHERE entra_seen_at IS NOT NULL`).Scan(&matched)
	})
	return matched, err
}
