package intune

import (
	"context"
	"encoding/hex"
	"net"
	"strings"
	"time"

	"dfipam/api/internal/platform/db"

	"github.com/jackc/pgx/v5"
)

// normalMAC reads Graph's "001A2B3C4D5E" (or a separated form) as aa:bb:… .
func normalMAC(s string) string {
	s = strings.Map(func(r rune) rune {
		if strings.ContainsRune("0123456789abcdefABCDEF", r) {
			return r
		}
		return -1
	}, s)
	if b, err := hex.DecodeString(s); err == nil && len(b) == 6 && s != "000000000000" {
		return strings.ToLower(net.HardwareAddr(b).String())
	}
	return ""
}

// nameKey turns a device name into the form its network hostname takes: a
// Mac called "Nimal's MacBook Pro" announces itself as Nimals-MacBook-Pro.
// Windows names (DESKTOP-7Q2) are unchanged apart from case. hostKey in SQL
// does the same to the hostname.
func nameKey(name string) string {
	name = strings.NewReplacer("'", "", "\u2019", "").Replace(strings.TrimSpace(name))
	var b strings.Builder
	dash := false
	for _, r := range strings.ToUpper(name) {
		if (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			dash = false
		} else if !dash && b.Len() > 0 {
			b.WriteByte('-')
			dash = true
		}
	}
	return strings.TrimRight(b.String(), "-")
}

// hostKey is nameKey for the first label of a sighting's hostname.
const hostKey = `upper(trim(both '-' from regexp_replace(regexp_replace(split_part(o.hostname, '.', 1), '[''’]', '', 'g'), '[^A-Za-z0-9]+', '-', 'g')))`

// index keeps, per MAC, the device synced most recently, and per computer name
// only names that one device has: a repeated name could point at the wrong PC.
// A shared device with no primary user is kept for its details.
func index(devs []Device) (byMAC, byName map[string]Device) {
	byMAC, byName = map[string]Device{}, map[string]Device{}
	names := map[string]int{}
	for _, d := range devs {
		d.User = strings.ToLower(strings.TrimSpace(d.User))
		for _, m := range []string{normalMAC(d.WiFiMAC), normalMAC(d.EtherMAC)} {
			if old, ok := byMAC[m]; m != "" && (!ok || d.LastSync.After(old.LastSync)) {
				byMAC[m] = d
			}
		}
		if n := nameKey(d.Name); n != "" {
			names[n]++
			byName[n] = d
		}
	}
	for n, count := range names {
		if count > 1 {
			delete(byName, n)
		}
	}
	return byMAC, byName
}

// rows turns a map into the parallel arrays the update unnests.
type rows struct {
	keys, users, names, devices, serials, makers, models, systems, comply []string
	encrypted                                                             []bool
	synced                                                                []*time.Time
}

func columns(m map[string]Device) rows {
	var r rows
	for k, d := range m {
		r.keys, r.users, r.names, r.devices = append(r.keys, k), append(r.users, d.User), append(r.names, d.UserName), append(r.devices, d.Name)
		r.serials, r.makers, r.models = append(r.serials, d.Serial), append(r.makers, d.Maker), append(r.models, d.Model)
		r.systems, r.comply, r.encrypted = append(r.systems, strings.TrimSpace(d.OS+" "+d.Version)), append(r.comply, d.Comply), append(r.encrypted, d.Encrypt)
		var at *time.Time
		if !d.LastSync.IsZero() {
			t := d.LastSync
			at = &t
		}
		r.synced = append(r.synced, at)
	}
	return r
}

const setDevice = `UPDATE sightings o SET intune_user = x.u, intune_user_name = x.n, intune_device = x.d, intune_seen_at = now(),
	intune_serial = x.serial, intune_maker = x.maker, intune_model = x.model, intune_os = x.os, intune_compliance = x.comply,
	intune_encrypted = x.enc, intune_last_sync = x.synced
	FROM unnest($1::text[], $2::text[], $3::text[], $4::text[], $5::text[], $6::text[], $7::text[], $8::text[], $9::text[], $10::boolean[], $11::timestamptz[])
	AS x(k, u, n, d, serial, maker, model, os, comply, enc, synced) WHERE `

func (r rows) args() []any {
	return []any{r.keys, r.users, r.names, r.devices, r.serials, r.makers, r.models, r.systems, r.comply, r.encrypted, r.synced}
}

// save records each Intune device on its sightings, by computer name first and
// then by MAC, which wins. Sightings no Intune device matched any more are
// cleared, so details don't linger after a device leaves Intune. It returns
// how many sightings are now matched to an Intune device.
func save(ctx context.Context, conn db.Conn, devs []Device) (int64, error) {
	byMAC, byName := index(devs)
	var matched int64
	err := db.InTx(ctx, conn, func(tx pgx.Tx) error {
		var start time.Time
		if err := tx.QueryRow(ctx, `SELECT now()`).Scan(&start); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, setDevice+hostKey+` = x.k`, columns(byName).args()...); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, setDevice+`o.mac = x.k::macaddr`, columns(byMAC).args()...); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE sightings SET intune_user = '', intune_user_name = '', intune_device = '', intune_seen_at = NULL,
			intune_serial = '', intune_maker = '', intune_model = '', intune_os = '', intune_compliance = '', intune_encrypted = NULL, intune_last_sync = NULL
			WHERE intune_seen_at < $1`, start); err != nil {
			return err
		}
		return tx.QueryRow(ctx, `SELECT count(*) FROM sightings WHERE intune_seen_at IS NOT NULL`).Scan(&matched)
	})
	return matched, err
}
