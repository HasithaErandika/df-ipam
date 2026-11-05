#!/bin/sh
# Runs once, when the data volume is first created. The migrations find this role
# already there, so its password never appears in a migration.
set -eu
psql -v ON_ERROR_STOP=1 -v app_password="$DFIPAM_APP_PASSWORD" --username "$POSTGRES_USER" --dbname "$POSTGRES_DB" <<'SQL'
CREATE ROLE dfipam_app LOGIN PASSWORD :'app_password';
SQL
