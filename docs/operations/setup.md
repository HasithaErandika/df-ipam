# Setup, Run and Deploy

How to run DF-Atlas on your machine, and deploy it to a server. What runs is described in [architecture.md](../engineering/architecture.md).

## Run it on your machine

### What you need

- Docker with the Compose plugin
- Go 1.26.11+, and Node 24 with pnpm, for tests and commands outside Docker
- For sign-in, an Entra ID app registration ([authentication.md](../engineering/authentication.md#setting-it-up-in-entra-id)). Nobody can sign in without one

### First run

```bash
make setup       # installs dependencies, creates api/.env and web/.env
```

Fill in `api/.env`:

| Setting | Value |
| --- | --- |
| `POSTGRES_PASSWORD`, `DFIPAM_APP_PASSWORD` | `openssl rand -hex 24` each. They take effect when the database is first created |
| `DATABASE_URL`, `MIGRATION_DATABASE_URL` | As in the example, with those passwords |
| `SECRET_ENCRYPTION_KEY` | `openssl rand -base64 32`. Needed to store a FortiGate token |
| `ENTRA_TENANT_ID`, `ENTRA_CLIENT_ID`, `ENTRA_CLIENT_SECRET` | From the Entra ID app registration. Required: Microsoft sign-in is the only way in. See [authentication.md](../engineering/authentication.md#setting-it-up-in-entra-id) |
| `HOST_NETWORK` | `true` on Linux to let the scanner read MAC addresses (see below) |

Leave `VITE_API_URL` empty in `web/.env`, so the app and the API share one address and the session cookie is sent. Never put secrets in `web/.env`: it ends up in the browser.

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

1. `make admin EMAIL=<you>@dialogfinance.lk`. It promotes your account if you have signed in, or adds a pending one that your first sign-in takes over.
2. **Sign in with Microsoft.** Your first sign-in takes over the pending account, with its Administrator role.
3. Give others roles in Users and roles.

### First data

In Network setup: add the segments (by hand, or paste FortiGate `show system interface` output into Import from text; set each floor's DHCP pool and each server segment's environment), departments, and check the environments. Give people roles in Users and roles. Discovery starts on the next scan, or press **Scan now** in Discovery. Import existing devices and register entries from CSV, text or Excel to avoid a flood of unknown devices.

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

One Ubuntu Server in a management VLAN, able to reach every floor and server segment. 2 vCPU, 4 GB of memory and 50 GB of SSD is plenty for one building (an estimate). For MAC addresses on every floor it needs to sit on those VLANs (see [status.md](../status.md#not-built)).

Install Docker Engine with the Compose plugin from Docker's apt repository, then `sudo apt install -y make gnupg git`. Anyone in the `docker` group is effectively root, so keep it to administrators.

### 2. Firewall

| Direction | To or from | Port |
| --- | --- | --- |
| Out | `login.microsoftonline.com` (Entra ID) | 443/tcp |
| Out | Mail server | 587/tcp |
| Out | The FortiGate's management address, if used | 443/tcp or its admin port |
| Out | `login.microsoftonline.com` and `graph.microsoft.com`, for sign-in and the optional Intune read | 443/tcp |
| Out | Floors and server segments | ICMP; UDP 53, 137, 1900, 5353; TCP 80 for UPnP descriptions; the check ports of monitored servers; the common ports, where port checks are on |
| In | Office and VPN ranges | 443/tcp, and 80/tcp for the redirect |

Browsers also need `login.microsoftonline.com` on 443, because people sign in to Microsoft in the browser.

### 3. Configure

```bash
sudo git clone <repository> /opt/df-ipam && cd /opt/df-ipam
sudo cp api/.env.example api/.env && sudo cp web/.env.example web/.env
sudo chmod 600 api/.env web/.env
```

In `api/.env`, as on your machine, plus:

| Setting | Value |
| --- | --- |
| `APP_ENV` | `production` (also fixed by `docker-compose.prod.yml`). The API then needs the `ENTRA_*` settings and an `https://` `APP_BASE_URL` |
| `ENTRA_TENANT_ID`, `ENTRA_CLIENT_ID`, `ENTRA_CLIENT_SECRET` | Required: the API won't start in production without them, or without an `https://` `APP_BASE_URL` |
| `COMPOSE_FILE` | `docker-compose.yml:docker-compose.prod.yml`. **Without it nothing is published on 80 and 443** |
| `CORS_ALLOWED_ORIGINS`, `APP_BASE_URL` | `https://<host name>` |
| `SMTP_HOST`, `SMTP_PORT`, `SMTP_USERNAME`, `SMTP_PASSWORD`, `MAIL_FROM` | Your mail server. Empty `SMTP_HOST` keeps email off |
| `BACKUP_GPG_RECIPIENT` | See step 6 |
| `DOCKER_SUBNET`, `DOCKER_GATEWAY` | Change only if `172.30.0.0/24` is in use |

Store `SECRET_ENCRYPTION_KEY` and `ENTRA_CLIENT_SECRET` in your vault, apart from backups. In Entra ID, add the redirect URI `https://<host name>/api/v1/auth/microsoft/callback`.

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
| Make a small change, read the Audit log | The source address is your machine, not `172.30.0.1` |

### 6. Backups

Create a GPG key on a machine **other than the server**, import only its public key on the server as root, and set `BACKUP_GPG_RECIPIENT`.

```bash
sudo cp deploy/systemd/df-ipam-backup.* /etc/systemd/system/
sudo systemctl daemon-reload && sudo systemctl enable --now df-ipam-backup.timer
sudo systemctl start df-ipam-backup.service
```

It runs `make db-backup` daily at 01:00 into `/var/backups/df-ipam`, checks the dump can be read, and keeps 30 days. **Copy that folder off the server every day,** and test a restore with [runbooks.md](runbooks.md).

### 7. Go-live

Run the checklist in [security.md](../engineering/security.md), and make a second Admin straight away.

## Run it behind Apache

Use this when the server already runs Apache, or Apache is the standard. Apache takes the place of the `web` container: it serves the built web app and passes `/api/` to the backend container, which is published on `127.0.0.1:8080` only. Steps 1, 2, 4 and 6 above still apply.

1. **Install Apache and its modules.**

   ```bash
   sudo apt install -y apache2
   sudo a2enmod ssl headers proxy proxy_http rewrite reqtimeout
   ```

2. **Configure `api/.env`** as in step 3, but **don't** set `COMPOSE_FILE`: `make apache-up` chooses the files. Set `APP_BASE_URL` and `CORS_ALLOWED_ORIGINS` to `https://<host name>`. In `web/.env`, leave `VITE_API_URL` empty.

3. **Start the database and backend** (no web container):

   ```bash
   sudo make apache-up        # db and backend; the API answers on 127.0.0.1:8080 only
   curl -s http://127.0.0.1:8080/ready
   ```

4. **Build the web app and put it where Apache serves it:**

   ```bash
   make apache-web                                  # builds web/dist
   sudo mkdir -p /var/www/df-ipam
   sudo rsync -a --delete web/dist/ /var/www/df-ipam/
   ```

5. **Add the site.** Copy the site file, then edit the host name, the certificate paths, your office and VPN ranges (both `Require ip` lines; don't leave `10.0.0.0/8`).

   ```bash
   sudo cp deploy/apache/df-ipam.conf /etc/apache2/sites-available/
   sudo nano /etc/apache2/sites-available/df-ipam.conf
   sudo a2ensite df-ipam && sudo a2dissite 000-default
   sudo apache2ctl configtest && sudo systemctl reload apache2
   ```

6. **Check it** as in step 5 above. The audit log must show each person's own address; the backend trusts `X-Forwarded-For` only from Apache. With `HOST_NETWORK=true`, `make apache-up` also adds `docker-compose.apache-hostnet.yml`, so the backend trusts Apache on loopback and listens there only.

The site file matches the web container: HTTPS only with HSTS, the same Content Security Policy and headers, the app's own links working on reload, long caching for `/assets`, 2 MB request bodies (8 MB on the three import routes), slow-client timeouts, and `/health` and `/ready` not exposed. The API keeps its own per-address and per-person rate limits; Apache has no per-request limiter built in, so add `mod_evasive` or rely on the FortiGate if you need one in front.

**Upgrade with Apache:** `git pull`, then `sudo make apache-up`, `make apache-web` and the `rsync` line again. Migrations run on `apache-up`.

For Microsoft sign-in, register `https://<host name>/api/v1/auth/microsoft/callback` in Entra ID (see [authentication.md](../engineering/authentication.md#setting-it-up-in-entra-id)).

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
| Microsoft says "Need admin approval" | The tenant doesn't let users consent to apps | An Entra administrator grants admin consent once: Enterprise applications > the app > Permissions > **Grant admin consent** |
| The page loads, sign-in fails | Entra ID settings or redirect URI don't match `APP_BASE_URL`, or the browser can't reach `login.microsoftonline.com` | See [authentication.md](../engineering/authentication.md#setting-it-up-in-entra-id); the reason is shown on the sign-in page |
| "Sign-in isn't available" | The API is down, or the Entra ID settings are empty | Check `make docker-logs`; set `ENTRA_*` in `api/.env`, then `make docker-up` |
| The API won't start: `APP_ENV=production needs Microsoft Entra ID sign-in` or `needs APP_BASE_URL to start with https://` | A server without Entra ID settings, or on plain HTTP | Set the `ENTRA_*` settings and an `https://` `APP_BASE_URL` |
| "Waiting for access" | Signed in with no role | An Admin assigns one |
| `required variable ... is missing` | Compose run directly | Use the `make` commands |
| Nothing on 443 | `COMPOSE_FILE` not set | See step 3 |
| `web` keeps restarting | Certificates missing or unreadable by user 101 | See step 4, and `make docker-logs` |
| Can't sign in to PostgreSQL | Passwords changed after the volume was created | Set them back, or change them in PostgreSQL |
| Discovery shows no MACs | The server isn't on the devices' network | `HOST_NETWORK=true`, or a trunk port (not built) |
| A segment is skipped | More than 4,096 addresses | Split it into smaller segments |
| Can't save the FortiGate token | No `SECRET_ENCRYPTION_KEY` | Set it, then `make docker-up` |
| `port is already allocated` | Something else has the port | Change `POSTGRES_PORT`, `API_PORT` or `WEB_PORT` |
