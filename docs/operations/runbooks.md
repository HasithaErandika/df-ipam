# Recovery Runbooks

For the people on call, assuming the setup in [setup.md](setup.md). **None has been rehearsed yet.** Record each rehearsal here: a runbook nobody has run isn't proven.

| Runbook | Last rehearsed | Result |
| --- | --- | --- |
| Restore the database | Not yet | |
| Rebuild the server | Not yet | |
| Rotate the encryption key | Not yet | |
| Deploy or roll back Network Links | Not yet | |
| Restore link attachments | Not yet | |

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

## Restore link attachments

Network link attachments are backed up next to the database dump, as `df-ipam-<stamp>.attachments.tar.gpg`. Restore them with the database from the same backup, so records and files match.

```bash
sha256sum -c df-ipam-<stamp>.attachments.tar.gpg.sha256
gpg --decrypt --output attachments.tar df-ipam-<stamp>.attachments.tar.gpg
cd /opt/df-ipam
sudo bash -c 'set -a; . api/.env; set +a; docker compose run --rm -T --no-deps --user root -v "$PWD/attachments.tar:/restore.tar:ro" --entrypoint sh backend \
  -c "tar -C /data -xf /restore.tar && chown -R 10001:10001 /data/attachments && chmod 700 /data/attachments"'
rm attachments.tar
```

A file whose record exists but whose file is missing downloads as "This file is missing from the server's attachment storage".

## Deploy or roll back Network Links

**Deploy.** Take a backup (`sudo make db-backup`), then `git pull` and `sudo make docker-up`. Migration 000023 adds five tables and the `links.*` settings and gives the starting roles their `links:*` permissions; nothing existing changes. Docker creates the `df-ipam-lite-link-files` volume. If Apache serves the app, reload it so the 15 MB attachment limit applies (`deploy/apache/df-ipam.conf`). Then check: sign in as an Administrator, open **Network Links > Overview**, add a provider and a link, attach a small PDF, and download it again.

**Roll back.** This deletes every network link, provider and attachment record, so export the links first (Import / Export > Export all links) and keep the attachment volume.

```bash
cd /opt/df-ipam
sudo bash -c 'set -a; . api/.env; set +a; docker compose run --rm --no-deps migrate /app/migrate down'   # undoes 000023 only; run once
git checkout <the previous release>
sudo make docker-up
```

The attachment volume is left as it is; remove it with `docker volume rm df-ipam-lite-link-files` only once nothing more is needed from it.

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
4. In System > FortiGate, paste the token again and save, so it is encrypted with the new key. Press **Test connection**.
5. Empty `SECRET_ENCRYPTION_KEY_PREVIOUS`, `sudo make docker-up` again, and store the new key in your vault.

**If the key leaked,** also create a new token on the FortiGate and revoke the old one. If the key is lost, set a new one and enter the token again; nothing else is affected.

## Stop discovery at once

Stop the backend: `sudo bash -c 'set -a; . api/.env; set +a; docker compose stop backend'`. This stops the scanner, server checks and the API together. Before starting it again, raise the scan interval in System, or remove the segment in Network setup. Turning off **Check common ports** on a segment stops port checks from its next scan.

## Microsoft sign-in or the internet is down

Nobody new can sign in, but data is safe on the server, and open sessions keep working until they end (10 hours, or the idle timeout). Discovery, server checks and alerts carry on. There is no other way in, by design. If an administrator must get in, wait for Microsoft, or reach the database with `make db-shell` for what can't wait (audited changes go through the app, so note anything done there).
