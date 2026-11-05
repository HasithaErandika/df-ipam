#!/usr/bin/env bash
# Dumps the Docker database, encrypts it to a GPG public key, and proves the dump can
# be read. Run through `make db-backup`, which loads api/.env. The private key stays
# off this server, so a stolen backup can't be opened here.
set -euo pipefail

dir="${BACKUP_DIR:-/var/backups/df-ipam}"
keep="${BACKUP_KEEP_DAYS:-30}"
database="${POSTGRES_DB:-dfipam}"
: "${BACKUP_GPG_RECIPIENT:?BACKUP_GPG_RECIPIENT is not set in api/.env}"

stamp="$(date -u +%Y%m%dT%H%M%SZ)"
work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT
umask 077
mkdir -p "$dir"

docker compose exec -T db pg_dump -U postgres --format=custom --no-owner "$database" > "$work/dump"

# A finished command doesn't prove a usable backup, so read the table of contents back.
docker compose exec -T db pg_restore --list < "$work/dump" | grep -q 'TABLE DATA public audit_logs'

gpg --batch --yes --trust-model always --encrypt --recipient "$BACKUP_GPG_RECIPIENT" \
    --output "$dir/df-ipam-$stamp.dump.gpg" "$work/dump"
sha256sum "$dir/df-ipam-$stamp.dump.gpg" > "$dir/df-ipam-$stamp.dump.gpg.sha256"

find "$dir" -name 'df-ipam-*.dump.gpg*' -mtime +"$keep" -delete

# Shown on the System Administration page.
docker compose exec -T db psql -U postgres -d "$database" -q -c "INSERT INTO system_settings (key, value)
  VALUES ('backup.last_success', to_jsonb(now())) ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, updated_at = now()"
echo "backup written: $dir/df-ipam-$stamp.dump.gpg"
