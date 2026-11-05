# Setup, Run and Deploy

How to run DF-IPAM on your own machine, and how to deploy it to a server. To enter data and use the system, see the system guide PDF, which is shared outside git.

## What runs

| Container | Holds | Reached through |
| --- | --- | --- |
| `web` | Nginx: the built web app, HTTPS, rate limits, and the way in to the API | The browser |
| `backend` | The API and the background worker, in one process | `web` only |
| `db` | PostgreSQL, with its data in the `df-ipam_db-data` volume | `backend` only |

A `migrate` step runs first, applies migrations and the seed, and exits. Discovery agents aren't containers: they run on hosts in the networks they cover.

Supabase provides sign-in only. All network data stays in your PostgreSQL.

## Run it on your machine

### What you need

- Docker with the Compose plugin
- Go 1.26.10 or later, and Node 24 with pnpm, for the tests and commands run outside Docker
- A Supabase project for sign-in. Its URL and keys are under **Project Settings, API**

### First run

```bash
make setup       # installs dependencies, and creates api/.env and web/.env
```

Fill in `api/.env`:

| Setting | Value |
| --- | --- |
| `SUPABASE_URL`, `SUPABASE_ANON_KEY`, `SUPABASE_SERVICE_ROLE_KEY` | From your Supabase project |
| `POSTGRES_PASSWORD`, `DFIPAM_APP_PASSWORD` | Run `openssl rand -hex 24` for each |
| `DATABASE_URL`, `MIGRATION_DATABASE_URL` | As in the example, with those passwords |
| `SECRET_ENCRYPTION_KEY` | Run `openssl rand -base64 32` |

Fill in `web/.env` with `VITE_SUPABASE_URL` and `VITE_SUPABASE_ANON_KEY`. **The `service_role` key never goes in `web/.env`.** Then:

```bash
make env-check   # lists anything missing
make dev         # starts the containers, then the web app with instant reloads
```

| Address | What |
| --- | --- |
| <http://localhost:5173> | The web app, with instant reloads. Use this while developing |
| <http://localhost:8088> | The web app in its container, as it runs on a server |
| <http://127.0.0.1:8080> | The API |

Everything is published on localhost only.

### Make yourself an administrator

1. In the Supabase dashboard, open **Authentication, Users**, and add a user with a password and auto-confirm on. The app has no screen to set a password.
2. Sign in to DF-IPAM. You see "Waiting for access", and your user now exists.
3. Run `make db-shell`, then:

   ```sql
   INSERT INTO user_roles (user_id, role_id)
   SELECT u.id, r.id FROM users u, roles r
   WHERE u.email = '<your email>' AND r.name = 'Super Administrator';
   ```

4. Press **Check again**.

### Load the sample network

```bash
make seed-demo
```

This loads a sample bank network in about a minute: 23 sites, around 190 subnets, about 1,300 address records, requests in every status, discovery evidence, integrations and findings. Everything goes through the real API, so it follows every rule and is audited.

- It sends no packets, and calls no real system.
- It refuses to run with `APP_ENV=production`, and refuses to run twice.
- Existing records are left alone.
- Its people use `@example.com` addresses and can't sign in.

**It can't be removed record by record, because audit logs are append-only.** To start again, use `make db-reset CONFIRM=yes`, which deletes everything, then `make docker-up`.

### Daily commands

| Command | What it does |
| --- | --- |
| `make dev` | Start everything |
| `make docker-up` | Start or rebuild the containers, migrating first |
| `make docker-down` | Stop the containers. Data stays |
| `make docker-logs` | Follow the logs |
| `make docker-ps` | Show status |
| `make lint test` | Check formatting and types, and run the unit tests |
| `make test-db` | Run the database tests on a fresh `dfipam_test` database. Your data isn't touched |
| `make db-shell` | Open `psql` as the owner |
| `make help` | List every command |

Schema changes go in a new numbered pair in `api/migrations/`, followed by `make sql`.

## Deploy to a server

Do these in order. Your own test system should come first: nothing below has been rehearsed on a real server yet.

### Before you start

| Needed | From | Blocks |
| --- | --- | --- |
| List of core banking and payment networks | Network and security | Any active discovery |
| Read-only service accounts for each platform | Platform owners | Integrations |
| SMTP host and sender address | Infrastructure | Email. In-app alerts work without it |
| RPO, RTO and retention periods | Business and compliance | Sign-off of backups |

### 1. Sort networks into zones

| Zone | Examples | How DF-IPAM reaches it |
| --- | --- | --- |
| **Public** | Supabase, AWS and Azure APIs | The server calls out on 443. Nothing comes in |
| **Private** | Offices, server VLANs, FortiGate and vCenter management | The server calls them directly, or an agent runs nearby |
| **Restricted** | Core banking, payments, DMZ, PCI segments | **Passive or API only**, unless a sensitive policy is approved |

Agents always connect out to the server on 443, so you never open a port into a segment. Nothing DF-IPAM does changes a network.

### 2. Choose an access model

DF-IPAM holds a map of the bank's network, so keep exposure as low as you can.

| Model | Who reaches it | Use when |
| --- | --- | --- |
| **A. Private** (recommended) | Office and VPN users | Almost always |
| **B. Private with remote segments** | As A, plus agents in other sites over private links | Several sites or isolated segments |
| **C. Public** | Anyone who can reach the URL | Only if security approves it. Needs a WAF or zero-trust proxy, and MFA, which isn't set up yet |

### 3. Prepare the server

One dedicated Ubuntu Server in a management VLAN, not a DMZ. For under 50,000 IPs, start with 4 vCPU, 8 GB of memory and 250 GB of SSD. These are estimates.

Install Docker Engine with the Compose plugin from Docker's own apt repository, then:

```bash
sudo apt update && sudo apt install -y make gnupg git
```

Anyone in the `docker` group is effectively root on this server, so keep that group to administrators.

### 4. Open the firewall

**Outbound from the server:**

| To | Port | Purpose |
| --- | --- | --- |
| `<project-ref>.supabase.co` | 443/tcp | Sign-in keys |
| FortiGate, vCenter | 443/tcp, or your admin port | Read-only APIs |
| AWS and Azure APIs | 443/tcp | Read-only APIs |
| Mail server | 587/tcp | Email |
| Docker Hub, npm and Go module mirrors | 443/tcp | Building images |

**Users' browsers** need `<project-ref>.supabase.co` on 443, because sign-in happens in the browser. Without it they see the app but can't sign in.

**Inbound to the server:**

| Model | From | Port |
| --- | --- | --- |
| A | Office and VPN ranges | 443/tcp, and 80/tcp for the redirect |
| B | As A, plus each agent host or its NAT address | 443/tcp |
| C | Your WAF or zero-trust proxy only | 443/tcp |

**Outbound from an agent host:** the server on 443, and targets only as a policy allows: ICMP, and 53/udp and tcp.

### 5. Set up Supabase

1. Create a project, and note its reference and keys.
2. Under **Authentication**, **turn off public sign-ups**. Otherwise anyone with the URL can create an account.
3. Confirm it uses asymmetric signing keys: `curl https://<project-ref>.supabase.co/auth/v1/.well-known/jwks.json` must return a key.
4. Under **URL Configuration**, set the site URL to `https://<your host name>`.

### 6. Get the code and configure

```bash
sudo git clone <repository> /opt/df-ipam && cd /opt/df-ipam
sudo cp api/.env.example api/.env && sudo cp web/.env.example web/.env
sudo chmod 600 api/.env web/.env
```

In `api/.env`, as on your machine, plus:

| Setting | Value |
| --- | --- |
| `APP_ENV` | `production` |
| `COMPOSE_FILE` | `docker-compose.yml:docker-compose.prod.yml`. **Without this, nothing is published on 80 and 443** |
| `CORS_ALLOWED_ORIGINS`, `APP_BASE_URL` | `https://<your host name>` |
| `DOCKER_SUBNET`, `DOCKER_GATEWAY` | Change only if `172.30.0.0/24` is in use. Then also set `WEB_ADDRESS` and `BACKEND_ADDRESS` |
| `SMTP_HOST`, `SMTP_PORT`, `SMTP_USERNAME`, `SMTP_PASSWORD`, `MAIL_FROM` | Your mail server. Empty `SMTP_HOST` keeps email off |
| `BACKUP_GPG_RECIPIENT` | See step 12 |
| `INTEGRATION_ALLOWED_HOSTS` | Host names connectors may reach. Empty refuses every URL. A leading dot allows subdomains |
| `INTEGRATION_ALLOWED_PRIVATE_RANGES` | **Needed for any private target.** List only management ranges |

In `web/.env`, set `VITE_SUPABASE_URL` and `VITE_SUPABASE_ANON_KEY`. They are built into the web image.

Use `openssl rand -hex 24` for the database passwords, because they go into connection URLs. **They take effect when the database is first created.** Store `SECRET_ENCRYPTION_KEY` in your secrets vault, apart from backups. If it is lost, every integration credential has to be entered again.

### 7. Certificates and access lists

Use your internal CA for models A and B, and install it on every user device and agent host. Use a public CA for model C.

```bash
sudo mkdir -p /etc/ssl/df-ipam
sudo cp fullchain.pem privkey.pem /etc/ssl/df-ipam/
sudo chown root:101 /etc/ssl/df-ipam/privkey.pem && sudo chmod 640 /etc/ssl/df-ipam/privkey.pem
```

The web container reads them as user 101. Then edit the lists it mounts:

| File | Holds |
| --- | --- |
| `deploy/nginx/access-private.conf` | Who may open the site, for models A and B. List your office and VPN ranges |
| `deploy/nginx/agents-allow.conf` | Which hosts agents connect from |
| `deploy/nginx/access-public.conf` | Model C only. Set `NGINX_ACCESS_FILE=./deploy/nginx/access-public.conf` in `api/.env` |

**Don't leave the example `10.0.0.0/8` in place.**

With a load balancer in front (model C), set `real_ip_header` and `set_real_ip_from` in `web/nginx/app.inc.template`, or audit logs and rate limits use the wrong address.

### 8. Start it

```bash
sudo make docker-up
sudo make docker-ps
```

`web`, `backend` and `db` must show `healthy`, and `migrate` should show `Exited (0)`.

### 9. Flood protection

| Layer | Stops | Where |
| --- | --- | --- |
| Upstream firewall | **Volumetric floods** | FortiGate DoS policy, or your ISP |
| Host | SYN floods, spoofing, connection bursts | Below |
| Nginx | Request floods, slow clients | In the `web` container |
| API | Abuse per address and per person | Built in |
| Discovery | **DF-IPAM flooding your own network** | Built in: see [discovery-and-reconciliation.md](discovery-and-reconciliation.md) |

```bash
sudo cp deploy/sysctl/99-df-ipam.conf /etc/sysctl.d/ && sudo sysctl --system
sudo nft -c -f deploy/nftables/df-ipam-flood.nft   # check it first
sudo nft -f deploy/nftables/df-ipam-flood.nft
```

To keep the rules after a reboot, add `include "/opt/df-ipam/deploy/nftables/df-ipam-flood.nft"` to `/etc/nftables.conf`, and enable the `nftables` service.

An office behind one NAT address counts as one client. If real people see "Too many requests", raise `rate` in `web/nginx/dfipam.conf.template` and the nftables file.

The discovery ceiling is 100 packets a second across all agents, and can be raised to 500, never higher. To change it, in `make db-shell`:

```sql
UPDATE system_settings SET value = '300' WHERE key = 'discovery.max_total_pps';
```

### 10. Check it

| Test | From | Expected |
| --- | --- | --- |
| Open the site and sign in | Office and VPN | Works |
| Open the site | Outside (models A, B) | Times out, or 403 |
| `curl https://<host>/api/v1/agent-api/heartbeat` | A host not in the agents list | 403 |
| `nc -zv <server> 5432` and `8080` | Any other machine | Refused or times out |

**Check the client address.** Sign in from another machine, make a small change, and read the source address in **Audit Logs**. It must be that machine, not `172.30.0.1`. If it isn't, Docker is hiding it, and rate limits and the access lists won't work properly. Give the `web` service `network_mode: host`, and point its upstream in `web/nginx/dfipam.conf.template` at the published backend port. This hasn't been tried on a real server yet.

### 11. First administrator and data

Create your account as in [Make yourself an administrator](#make-yourself-an-administrator), and add a second administrator straight away. Approvals, imports and discovery policies need a second person.

Then enter the data, following the system guide. **Don't run `make seed-demo` here.**

Connectors have only run on sample data and VMware's simulator, so try each against a non-production system first. Create its read-only account, and test that it really can't write.

### 12. Backups and retention

Create a GPG key on a machine **other than the server**, import only the public key as root on the server, and set `BACKUP_GPG_RECIPIENT`.

```bash
cd /opt/df-ipam
sudo cp deploy/systemd/df-ipam-{retention,backup}.* /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable --now df-ipam-backup.timer df-ipam-retention.timer
sudo systemctl start df-ipam-backup.service
```

Backups go to `/var/backups/df-ipam`. **Copy that folder off the server every day.** Then do a real restore, following [runbooks.md](runbooks.md).

### 13. Discovery agents

Do this **only after** the sensitive-networks list is agreed. Until then, integrations alone are passive and safe.

| Zone | Agent? |
| --- | --- |
| Private | One per site is usually enough. ICMP crosses routers, but ARP needs an agent on that segment |
| Restricted, approved for scanning | One inside the segment. Add its address to `agents-allow.conf` and the firewall |
| Restricted, not approved | **None.** Use platform data only |
| Cloud | **None.** Inventory comes from the APIs |

Build the agent with `make build-api`, which writes `api/bin/agent`. On each agent host:

```bash
sudo useradd --system --no-create-home --shell /usr/sbin/nologin dfipam-agent
sudo mkdir -p /opt/df-ipam/bin /etc/df-ipam /var/lib/df-ipam-agent
sudo chown dfipam-agent:dfipam-agent /var/lib/df-ipam-agent && sudo chmod 700 /var/lib/df-ipam-agent
sudo cp agent /opt/df-ipam/bin/
sudo cp company-ca.pem /usr/local/share/ca-certificates/company-ca.crt && sudo update-ca-certificates
sudo cp agent.env.example /etc/df-ipam/agent.env
sudo cp df-ipam-agent.service /etc/systemd/system/
```

1. In **Discovery Centre**, create an agent and copy its one-time token.
2. Put the token and server URL in `/etc/df-ipam/agent.env`, with group `dfipam-agent` and mode 640. On a fragile link, also set `AGENT_MAX_PPS`.
3. Run `sudo systemctl enable --now df-ipam-agent`, and check it shows as active.
4. **Delete the token line.** The agent now uses its own stored credential.

Then, in this order: enter exclusions for every restricted network, add blackouts, approve one small policy on a low-risk subnet at a low rate, and **test the emergency stop** while that job runs. New subnets have discovery off, so tick **Discovery enabled** first.

### 14. Go-live checks

Run through the checklist in [security.md](security.md), and at least these:

- [ ] Public sign-ups are off
- [ ] Database passwords and the `service_role` key are new, not ones shared during development
- [ ] `api/.env` and `web/.env` have mode 600
- [ ] From outside, the site is unreachable (models A, B)
- [ ] Ports 5432 and 8080 can't be reached from another machine
- [ ] Audit logs show real client addresses
- [ ] Each connector's account is confirmed read-only
- [ ] Restricted networks are excluded, and the emergency stop has been tested
- [ ] Backups are copied off the server, and a restore has been tested
- [ ] `SECRET_ENCRYPTION_KEY` is stored apart from backups
- [ ] System Administration shows the worker healthy and a recent backup

## Upgrade a server

```bash
cd /opt/df-ipam
sudo systemctl start df-ipam-backup.service
sudo git pull
sudo make docker-up
```

This rebuilds both images, runs new migrations and the seed, and restarts what changed. The seed adds new permissions, and leaves roles you have edited alone.

Moving from the older layout, stop and disable the host's Nginx first, so the `web` container can take ports 80 and 443. Rebuild and reinstall agents after an upgrade that changes them.

## Common problems

| Symptom | Likely cause | Fix |
| --- | --- | --- |
| The page loads, sign-in fails | The browser can't reach Supabase, or `VITE_` values were wrong at build | Check step 4, fix `web/.env`, then `make docker-up` |
| "Waiting for access" | Signed in with no role | Assign one in User Management |
| `required variable ... is missing` | Compose was run directly | Use the `make` commands, which load the env files |
| Nothing on port 443 | `COMPOSE_FILE` isn't set | Set it as in step 6 |
| `web` keeps restarting | Certificates missing, or unreadable by user 101 | Check step 7, and `make docker-logs` |
| `migrate` or the backend can't sign in to PostgreSQL | Passwords changed after the volume was created | Set them back, or change them in PostgreSQL |
| "Too many requests" for real users | Many people behind one NAT address | Raise the rates as in step 9 |
| Audit logs show `172.30.0.1` | Docker is hiding the client address | See step 10 |
| `Pool overlaps with other one` | `DOCKER_SUBNET` clashes with another network | Choose another range in `api/.env` |
| `port is already allocated` | Something else has the port | Change `POSTGRES_PORT`, `API_PORT` or `WEB_PORT`, or stop the host's Nginx |
| A discovery job stays queued | No agent, outside its window, over the ceiling, or an overlapping job | It starts when the other job ends. See the policy's windows |
| Connection test: "refused by the outbound policy" | Host or its private range isn't allowed | Fix `INTEGRATION_ALLOWED_HOSTS` and `INTEGRATION_ALLOWED_PRIVATE_RANGES` |
| Connection test: certificate error | Internal certificate | Add the CA with `INTEGRATION_CA_FILE`, as a path inside the container, with a volume mount |
| Can't save credentials | No master key | Set `SECRET_ENCRYPTION_KEY`, then `make docker-up` |
