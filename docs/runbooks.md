# Recovery Runbooks

For the people on call. Each one assumes the setup in [setup.md](setup.md). **None of these has been rehearsed yet.** Record the date and result of each rehearsal here, because a runbook nobody has run isn't proven.

| Runbook | Last rehearsed | Result |
| --- | --- | --- |
| Restore the database | Not yet | |
| Rebuild the app server | Not yet | |
| Rotate the encryption key | Not yet | |
| Replace an agent | Not yet | |

## Restore the database

Use when data is lost or corrupted. First decide with the business whether the data lost since the last backup is acceptable.

1. On the machine holding the GPG private key, check and decrypt the file, then copy `restore.dump` to the server:

   ```bash
   sha256sum -c df-ipam-<stamp>.dump.gpg.sha256
   gpg --decrypt --output restore.dump df-ipam-<stamp>.dump.gpg
   ```

2. **Restore into a scratch database first,** and check it. Never go straight over the live one:

   ```bash
   cd /opt/df-ipam
   sudo make db-shell     # then: CREATE DATABASE dfipam_restore;
   sudo bash -c 'set -a; . api/.env; set +a; docker compose exec -T db pg_restore -U postgres --no-owner -d dfipam_restore < restore.dump'
   sudo make db-shell     # then: \c dfipam_restore
   ```

   ```sql
   SELECT count(*) FROM subnets;
   SELECT count(*) FROM ip_addresses;
   SELECT max(occurred_at) FROM audit_logs;
   ```

3. If it looks right, stop the app, and swap the databases in `make db-shell`, connected to the `postgres` database:

   ```bash
   sudo bash -c 'set -a; . api/.env; set +a; COMPOSE_FILE=docker-compose.yml:docker-compose.prod.yml docker compose stop web backend'
   ```

   ```sql
   ALTER DATABASE dfipam RENAME TO dfipam_old;
   ALTER DATABASE dfipam_restore RENAME TO dfipam;
   ```

4. Run `sudo make docker-up`, and check `/ready` and System Administration.
5. Delete `restore.dump`, which is unencrypted, and drop `dfipam_old` once you are sure.

## Rebuild the app server

The database is on this server, so this needs a backup from off the server.

1. Follow setup steps 3, 4, 6, 7, 9 and 10 on the new server.
2. Restore `api/.env` from your vault. **The master key must be the same,** or stored credentials can't be read.
3. Run `make db-up`, then restore the latest backup as above.
4. Run `sudo make docker-up`.
5. Move the host name and certificate, and update firewall rules. Agents reconnect once the name resolves.

## Rotate the encryption key

Do this on a schedule, and at once if the key may have leaked.

1. Generate a new key: `openssl rand -base64 32`.
2. In `api/.env`, move the current key to `SECRET_ENCRYPTION_KEY_PREVIOUS`, set the new one as `SECRET_ENCRYPTION_KEY`, and add 1 to `SECRET_ENCRYPTION_KEY_VERSION`.
3. Run `sudo make docker-up`, then `sudo make docker-rotate-secrets`. All credentials change in one transaction, so a failure leaves them as they were.
4. Check a connection with **Test**.
5. Empty `SECRET_ENCRYPTION_KEY_PREVIOUS`, run `sudo make docker-up` again, and store the new key in your vault.

**If the key leaked, rotating isn't enough.** Anyone with the key and a database copy can read the credentials, so change the password or token on each platform too.

### If the key is lost

Stored credentials can't be recovered. Generate a new key, then enter every connection's credentials again in Integrations. Nothing else is affected.

## Replace an agent

1. In Discovery Centre, **revoke** the old agent. Do this first if it may be compromised: it stops at once.
2. Create a new agent, and install it as in Discovery agents in [setup.md](setup.md).
3. Assign it to the same policies, and check it comes online.

If the server's certificate moved to a new CA, install that CA on agent hosts first, or they refuse to connect.

## Emergency: stop all discovery

Discovery Centre has an emergency stop, which reaches each agent at its next heartbeat, within 15 seconds. An agent that loses the server for 90 seconds stops its job by itself. If the app can't be reached, stop the agents on their hosts: `sudo systemctl stop df-ipam-agent`.

## Supabase or the internet is down

Nobody can sign in, but the data is safe on your server, and sessions already open keep working until their tokens expire. Agents and integrations on the private network carry on. This ends when the company auth replaces Supabase.

## Redis

Redis isn't in use: jobs run from PostgreSQL. Nothing is needed here unless it is approved.
