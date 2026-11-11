# Setup, Run and Deploy

How to run DF-IPAM on your machine, and deploy it to a server. What runs is described in [architecture.md](architecture.md).

## Run it on your machine

### What you need

- Docker with the Compose plugin
- Go 1.26.10+, and Node 24 with pnpm, for tests and commands outside Docker
- A Supabase project for sign-in (URL and keys under **Project Settings, API**). Turn public sign-ups off

### First run

```bash
make setup       # installs dependencies, creates api/.env and web/.env
```

Fill in `api/.env`:

| Setting | Value |
| --- | --- |
| `SUPABASE_URL`, `SUPABASE_ANON_KEY`, `SUPABASE_SERVICE_ROLE_KEY` | From your Supabase project |
| `POSTGRES_PASSWORD`, `DFIPAM_APP_PASSWORD` | `openssl rand -hex 24` each. They take effect when the database is first created |
| `DATABASE_URL`, `MIGRATION_DATABASE_URL` | As in the example, with those passwords |
| `SECRET_ENCRYPTION_KEY` | `openssl rand -base64 32`. Needed to store a FortiGate token |
| `HOST_NETWORK` | `true` on Linux to let the scanner read MAC addresses (see below) |

Fill in `web/.env` with `VITE_SUPABASE_URL` and `VITE_SUPABASE_ANON_KEY`. **The `service_role` key never goes in `web/.env`.**

```bash
make env-check   # lists anything missing
make dev         # starts the containers, then the web app with instant reloads
```

| Address | What |
| --- | --- |
| <http://localhost:5173> | The web app with instant reloads |
| <http://localhost:8088> | The web app in its container |
| <http://127.0.0.1:8080> | The API |

Everything is published on localhost only.

### Make yourself an Admin

1. In Supabase, **Authentication, Users**: add a user with a password and auto-confirm on.
2. Sign in to DF-IPAM. You see "Waiting for access".
3. In `make db-shell`:

   ```sql
   UPDATE users SET role = 'admin' WHERE lower(email) = lower('<your email>');
   ```

4. Press **Check again**. Give others roles in Settings > Users.

### First data

In Settings: add the segments (with each floor's DHCP pool, and each server segment's environment), departments, and check the environments. Discovery starts on the next scan, or press **Scan now** in Discovery. Import existing devices and register entries from CSV or Excel to avoid a flood of unknown devices.

### MAC addresses and host networking

The scanner sees MAC addresses only on networks the server itself is on. With `HOST_NETWORK=true` in `api/.env` (Linux only), `make` adds `docker-compose.hostnet.yml`: the backend shares the host's network and reads its ARP table. The API then listens on loopback and the Docker gateway only, never on the Wi-Fi or Ethernet address.

### Daily commands

| Command | What it does |
| --- | --- |
| `make dev` | Start everything |
| `make docker-up` | Rebuild and start the containers, migrating first |
| `make docker-down` | Stop the containers. Data stays |
| `make docker-logs`, `make docker-ps` | Logs and status |
| `make lint test` | Formatting, types and unit tests |
| `make test-db` | Database and API tests on a throwaway database |
| `make db-shell` | `psql` as the owner |
| `make db-reset CONFIRM=yes` | **Delete all local data** and start empty |

A restart doesn't scan straight away: the first scan waits until the scan interval has passed since the last one. Use **Scan now** to scan at once.

## Deploy to a server

Nothing here has been rehearsed on a real server yet. Try it on a test system first.

### 1. The server

One Ubuntu Server in a management VLAN, able to reach every floor and server segment. 2 vCPU, 4 GB of memory and 50 GB of SSD is plenty for one building (an estimate). For MAC addresses on every floor it needs to sit on those VLANs (see [new.md](new.md#not-built)).

Install Docker Engine with the Compose plugin from Docker's apt repository, then `sudo apt install -y make gnupg git`. Anyone in the `docker` group is effectively root, so keep it to administrators.

### 2. Firewall

| Direction | To or from | Port |
| --- | --- | --- |
| Out | `<project>.supabase.co` | 443/tcp |
| Out | Mail server | 587/tcp |
| Out | The FortiGate's management address, if used | 443/tcp or its admin port |
| Out | Floors and server segments | ICMP; UDP 53, 137, 1900, 5353; TCP 80 for UPnP descriptions; the check ports of monitored servers; the common ports, where port checks are on |
| In | Office and VPN ranges | 443/tcp, and 80/tcp for the redirect |

Browsers also need `<project>.supabase.co` on 443, because sign-in happens in the browser.

### 3. Configure

```bash
sudo git clone <repository> /opt/df-ipam && cd /opt/df-ipam
sudo cp api/.env.example api/.env && sudo cp web/.env.example web/.env
sudo chmod 600 api/.env web/.env
```

In `api/.env`, as on your machine, plus:

| Setting | Value |
| --- | --- |
| `APP_ENV` | `production` |
| `COMPOSE_FILE` | `docker-compose.yml:docker-compose.prod.yml`. **Without it nothing is published on 80 and 443** |
| `CORS_ALLOWED_ORIGINS`, `APP_BASE_URL` | `https://<host name>` |
| `SMTP_HOST`, `SMTP_PORT`, `SMTP_USERNAME`, `SMTP_PASSWORD`, `MAIL_FROM` | Your mail server. Empty `SMTP_HOST` keeps email off |
| `BACKUP_GPG_RECIPIENT` | See step 6 |
| `DOCKER_SUBNET`, `DOCKER_GATEWAY` | Change only if `172.30.0.0/24` is in use |

Store `SECRET_ENCRYPTION_KEY` in your vault, apart from backups. In Supabase, set the site URL to `https://<host name>`.

### 4. Certificates and access list

```bash
sudo mkdir -p /etc/ssl/df-ipam
sudo cp fullchain.pem privkey.pem /etc/ssl/df-ipam/
sudo chown root:101 /etc/ssl/df-ipam/privkey.pem && sudo chmod 640 /etc/ssl/df-ipam/privkey.pem
```

List your office and VPN ranges in `deploy/nginx/access-private.conf`. **Don't leave the example `10.0.0.0/8`.** Only if security approves public access, use `deploy/nginx/access-public.conf` through `NGINX_ACCESS_FILE` in `api/.env`, behind a WAF.

### 5. Start and check

```bash
sudo make docker-up
sudo make docker-ps      # web, backend and db healthy; migrate Exited (0)
```

Host flood protection:

```bash
sudo cp deploy/sysctl/99-df-ipam.conf /etc/sysctl.d/ && sudo sysctl --system
sudo nft -c -f deploy/nftables/df-ipam-flood.nft && sudo nft -f deploy/nftables/df-ipam-flood.nft
```

Add `include "/opt/df-ipam/deploy/nftables/df-ipam-flood.nft"` to `/etc/nftables.conf` and enable `nftables` to keep it after a reboot. Volumetric floods must be stopped upstream, on the FortiGate.

| Check | Expected |
| --- | --- |
| Open the site from the office or VPN, sign in | Works |
| Open it from outside | Times out, or 403 |
| `nc -zv <server> 5432` and `8080` from another machine | Refused or times out |
| Make a small change, read Settings > Audit log | The source address is your machine, not `172.30.0.1` |

### 6. Backups

Create a GPG key on a machine **other than the server**, import only its public key on the server as root, and set `BACKUP_GPG_RECIPIENT`.

```bash
sudo cp deploy/systemd/df-ipam-backup.* /etc/systemd/system/
sudo systemctl daemon-reload && sudo systemctl enable --now df-ipam-backup.timer
sudo systemctl start df-ipam-backup.service
```

It runs `make db-backup` daily at 01:00 into `/var/backups/df-ipam`, checks the dump can be read, and keeps 30 days. **Copy that folder off the server every day,** and test a restore with [runbooks.md](runbooks.md).

### 7. Go-live

Run the checklist in [security.md](security.md), and make a second Admin straight away.

## Upgrade

```bash
cd /opt/df-ipam
sudo systemctl start df-ipam-backup.service
sudo git pull
sudo make docker-up
```

This rebuilds the images, runs new migrations and the seed, and restarts what changed.

## Common problems

| Symptom | Cause | Fix |
| --- | --- | --- |
| The page loads, sign-in fails | The browser can't reach Supabase, or `VITE_` values were wrong at build | Fix `web/.env`, then `make docker-up` |
| "Waiting for access" | Signed in with no role | An Admin assigns one |
| `required variable ... is missing` | Compose run directly | Use the `make` commands |
| Nothing on 443 | `COMPOSE_FILE` not set | See step 3 |
| `web` keeps restarting | Certificates missing or unreadable by user 101 | See step 4, and `make docker-logs` |
| Can't sign in to PostgreSQL | Passwords changed after the volume was created | Set them back, or change them in PostgreSQL |
| Discovery shows no MACs | The server isn't on the devices' network | `HOST_NETWORK=true`, or a trunk port (not built) |
| A segment is skipped | More than 4,096 addresses | Split it into smaller segments |
| Can't save the FortiGate token | No `SECRET_ENCRYPTION_KEY` | Set it, then `make docker-up` |
| `port is already allocated` | Something else has the port | Change `POSTGRES_PORT`, `API_PORT` or `WEB_PORT` |
