# Recovery Runbooks

For the people on call, assuming the setup in [setup.md](setup.md). **None has been rehearsed yet.** Record each rehearsal here: a runbook nobody has run isn't proven.

| Runbook | Last rehearsed | Result |
| --- | --- | --- |
| Restore the database | Not yet | |
| Rebuild the server | Not yet | |
| Rotate the encryption key | Not yet | |

## Restore the database

First agree with the business whether the data lost since the last backup is acceptable.

1. On the machine with the GPG private key, check and decrypt, then copy `restore.dump` to the server:

   ```bash
   sha256sum -c df-ipam-<stamp>.dump.gpg.sha256
   gpg --decrypt --output restore.dump df-ipam-<stamp>.dump.gpg
   ```

2. **Restore into a scratch database first,** never straight over the live one:

   ```bash
   cd /opt/df-ipam
   sudo make db-shell     # then: CREATE DATABASE dfipam_restore;
   sudo bash -c 'set -a; . api/.env; set +a; docker compose exec -T db pg_restore -U postgres --no-owner -d dfipam_restore < restore.dump'
   sudo make db-shell     # then: \c dfipam_restore
   ```

   ```sql
   SELECT count(*) FROM segments;
   SELECT count(*) FROM ip_addresses;
   SELECT max(occurred_at) FROM audit_logs;
   ```

3. If it looks right, stop the app and swap the databases, in `make db-shell` connected to `postgres`:

   ```bash
   sudo bash -c 'set -a; . api/.env; set +a; docker compose stop web backend'
   ```

   ```sql
   ALTER DATABASE dfipam RENAME TO dfipam_old;
   ALTER DATABASE dfipam_restore RENAME TO dfipam;
   ```

4. `sudo make docker-up`, then check `/ready` and sign in.
5. Delete `restore.dump`, which is unencrypted, and drop `dfipam_old` once you are sure.

## Rebuild the server

The database lives on the server, so this needs a backup from off the server.

1. Follow setup steps 1 to 4 on the new server.
2. Restore `api/.env` from your vault. **`SECRET_ENCRYPTION_KEY` must be the same,** or the FortiGate token has to be entered again.
3. `make db-up`, then restore the latest backup as above.
4. `sudo make docker-up`, and move the host name and certificate.

## Rotate the encryption key

The key protects only the FortiGate token. Rotate on a schedule, and at once if it may have leaked.

1. Generate a new key: `openssl rand -base64 32`.
2. In `api/.env`, move the current key to `SECRET_ENCRYPTION_KEY_PREVIOUS`, set the new one as `SECRET_ENCRYPTION_KEY`, and add 1 to `SECRET_ENCRYPTION_KEY_VERSION`.
3. `sudo make docker-up`. The stored token is still read with the previous key.
4. In Settings > FortiGate, paste the token again and save, so it is encrypted with the new key. Press **Test connection**.
5. Empty `SECRET_ENCRYPTION_KEY_PREVIOUS`, `sudo make docker-up` again, and store the new key in your vault.

**If the key leaked,** also create a new token on the FortiGate and revoke the old one. If the key is lost, set a new one and enter the token again; nothing else is affected.

## Stop discovery at once

Stop the backend: `sudo bash -c 'set -a; . api/.env; set +a; docker compose stop backend'`. This stops the scanner, server checks and the API together. Before starting it again, raise the scan interval or remove the segment in Settings. Turning off **Check common ports** on a segment stops port checks from its next scan.

## Supabase or the internet is down

Nobody can sign in, but data is safe on the server, and open sessions keep working until their tokens expire. Discovery, server checks and alerts carry on.
