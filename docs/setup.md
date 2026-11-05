# Company Setup Guide

How to install DF-IPAM at Dialog Finance from nothing, step by step. Choose your access model first, because it decides the firewall rules, certificates and sign-in path. Later steps refer back to it.

PostgreSQL, the API and the worker run in Docker on one server. Nginx runs on the server itself, in front of them. For background, see [deployment.md](deployment.md), [security.md](security.md) and [integrations.md](integrations.md).

## Just trying it on a laptop?

```bash
make setup        # install dependencies, create env files
make docker-up    # PostgreSQL, migrations, seed, API and worker
make web          # the web app at http://localhost:5173
```

`make help` lists every command. Sign in, then follow step 11 to make yourself an administrator.

## Before you start: what isn't decided yet

These are open decisions, not technical steps.

| Needed | From | Blocks |
| --- | --- | --- |
| List of core banking and payment networks | Network and security | Any active discovery |
| Service accounts for each platform, confirmed read-only | Platform owners | Integrations |
| SMTP host and sender address | Infrastructure | Email (in-app alerts work without it) |
| RPO, RTO and retention periods | Business and compliance | Sign-off of backups and retention |
| Company auth details | Project owner | Replacing Supabase sign-in |

**The network inventory stays on your server,** in the local PostgreSQL. Supabase is used only for sign-in, so it holds user email addresses and password hashes, and no network data. Its database is for testing, and isn't part of this setup.

**Sign-in still needs an internet path to Supabase.** During an internet outage nobody can sign in, though the data is safe. That ends when the company auth replaces it, which isn't built yet.

## Step 1: Sort your networks into zones

DF-IPAM touches networks with very different exposure. Put each one in a zone, and record it in the table in step 2.

| Zone | What it means | Examples | How DF-IPAM reaches it |
| --- | --- | --- | --- |
| **Public** | Services on the internet | Supabase sign-in, AWS and Azure APIs, public DNS | The server calls out through the firewall on 443. Nothing comes in |
| **Private** | Internal, routable from the server | Office networks, server VLANs, FortiGate and vCenter management | The server calls them directly, or an agent runs nearby |
| **Restricted** | Internal, but deliberately isolated | Core banking, payment networks, DMZ, PCI segments, isolated VRFs | **Passive or API only** unless a sensitive policy is approved. Where a scan is approved, an agent inside the segment calls out to the server. The server never calls in |

Three rules hold in every zone:

1. **Agents always connect out to the server,** on 443. You never open a port into a segment for DF-IPAM.
2. **Nothing DF-IPAM does changes a network.** Connectors and agents only read.
3. **The same IP in two isolated networks isn't a conflict.** Each isolated network needs its own routing domain (step 11).

## Step 2: Choose an access model

This decides who can open DF-IPAM. It holds a map of the bank's network, so keep exposure as low as you can.

| Model | Who reaches it | Use when | Risk |
| --- | --- | --- | --- |
| **A. Private only** (recommended) | Office and VPN users | Almost always | Lowest |
| **B. Private with remote segments** | As A, plus agents in other sites or a DMZ over private links | Several sites or isolated segments | Low: each agent range is listed |
| **C. Public** | Anyone who can reach the URL, then sign-in | Only if security approves it and A or B can't work | Highest: needs a WAF or zero-trust proxy, and MFA, which isn't set up yet |

**Use A or B unless security has signed off C.** The rest of this guide marks steps that differ.

Fill this in and keep it with your change record:

| Item | Your value |
| --- | --- |
| Access model | |
| Host name, for example `ipam.corp.dialogfinance.example` | |
| Server address and VLAN | |
| Office and VPN ranges that may open the site | |
| Agent hosts (name, zone, address) | |
| FortiGate management URLs | |
| vCenter URLs | |
| Private ranges those URLs resolve to | |
| AWS accounts, Azure subscriptions | |
| Restricted networks (no active scanning) | |

## Step 3: Prepare the server

One dedicated Ubuntu Server. For under 50,000 IPs start with 4 vCPU and 8 GB, plus 250 GB of SSD for the database. These are estimates, so confirm with a load test (step 16).

Place it in a management VLAN. **Don't put it in a DMZ for model A or B.**

Install Docker Engine with the Compose plugin from Docker's own apt repository, then:

```bash
sudo apt update && sudo apt install -y nginx make gnupg git
docker compose version
```

Anyone in the `docker` group is effectively root on this server, and can read the database. Keep that group to the administrators.

## Step 4: Open the firewall

Ask the network team for exactly these rules, and block everything else.

**Outbound from the server (all models):**

| To | Port | Purpose | Zone |
| --- | --- | --- | --- |
| `<project-ref>.supabase.co` | 443/tcp | Sign-in keys and invitations | Public |
| FortiGate management addresses | 443/tcp, or your admin port | Read-only API | Private |
| vCenter | 443/tcp | Read-only API | Private |
| AWS and Azure API endpoints | 443/tcp | Read-only API | Public |
| Mail server | 587/tcp | Email | Private or public |
| Docker Hub and your package mirror | 443/tcp | Images, while installing or upgrading | Public |

If your firewall can't allow by host name, note that Supabase and cloud addresses change. Use an egress proxy or FQDN rules, not fixed IPs.

**Users' browsers, outbound (all models):** every user needs `<project-ref>.supabase.co` on 443, because **sign-in happens in the browser, directly with Supabase**. A user on a network with no internet path sees the app but can't sign in. This catches people out on private networks: test it from a real office and VPN connection in step 10.

**Inbound to the server:**

| Model | From | Port |
| --- | --- | --- |
| A | Office and VPN ranges | 443/tcp, and 80/tcp if you want the redirect |
| B | As A, plus each agent host or its NAT address | 443/tcp |
| C | Your WAF or zero-trust proxy only | 443/tcp |

**Outbound from an agent host:** the server on 443, and targets only as a policy allows: ICMP and 53/udp and tcp. SNMP on 161/udp isn't built yet.

PostgreSQL (5432) and the API (8080) are never opened to any network. Compose publishes both on `127.0.0.1` only, so only Nginx and the server's own tools reach them.

## Step 5: Set up Supabase for sign-in

Supabase holds no inventory in this setup, and its database isn't used.

1. Create a project. Note the project reference, and the anon and `service_role` keys.
2. Under **Authentication**, **turn off public sign-ups**. Skipping this lets anyone with the URL create an account.
3. Confirm the project uses asymmetric signing keys: `curl https://<project-ref>.supabase.co/auth/v1/.well-known/jwks.json` must return a key.
4. Under **Authentication, URL Configuration**, set the site URL to `https://<your host name>`.

## Step 6: Get the code and build the web app

On the server:

```bash
sudo git clone https://github.com/HasithaErandika/df-ipam.git /opt/df-ipam
```

The web app is built once, on any machine with Node 24 and pnpm. Create `web/.env` there:

```text
VITE_SUPABASE_URL=https://<project-ref>.supabase.co
VITE_SUPABASE_ANON_KEY=<anon key>
VITE_API_URL=
```

Leave `VITE_API_URL` empty, because Nginx serves the app and API from one address. Then build and copy it:

```bash
make build-web
scp -r web/dist server:/tmp/dfipam-web
```

```bash
sudo mkdir -p /var/www/df-ipam && sudo cp -r /tmp/dfipam-web/* /var/www/df-ipam/
```

## Step 7: Configure

All settings live in one file, `api/.env`. There is none at the repository root.

```bash
cd /opt/df-ipam
sudo cp api/.env.example api/.env && sudo chmod 600 api/.env
openssl rand -hex 24    # run twice: POSTGRES_PASSWORD, DFIPAM_APP_PASSWORD
openssl rand -base64 32 # SECRET_ENCRYPTION_KEY
```

Use `-hex` for the database passwords: they go into connection URLs, so they must be letters and digits only. **The passwords take effect when the database is first created.** Changing them later means changing them in PostgreSQL too.

| Setting | Value |
| --- | --- |
| `APP_ENV` | `production` |
| `CORS_ALLOWED_ORIGINS`, `APP_BASE_URL` | `https://<your host name>` |
| `SUPABASE_URL`, `SUPABASE_ANON_KEY`, `SUPABASE_SERVICE_ROLE_KEY` | From step 5 |
| `POSTGRES_PASSWORD`, `DFIPAM_APP_PASSWORD` | The two generated values |
| `DATABASE_URL`, `MIGRATION_DATABASE_URL` | Same format as the example, with those passwords. Used only by commands run on the host |
| `SECRET_ENCRYPTION_KEY` | The generated key |
| `DOCKER_SUBNET`, `DOCKER_GATEWAY` | Leave as they are, unless `172.30.0.0/24` is in use at your company |
| `SMTP_HOST`, `SMTP_PORT`, `SMTP_USERNAME`, `SMTP_PASSWORD`, `MAIL_FROM` | Your mail server. An empty `SMTP_HOST` keeps email off |
| `BACKUP_GPG_RECIPIENT` | See step 15 |

Store a copy of `SECRET_ENCRYPTION_KEY` in your secrets vault, **apart from database backups**. If it is lost, every stored integration credential has to be entered again.

Then set which internal systems DF-IPAM may call:

```text
INTEGRATION_ALLOWED_HOSTS=fw1.corp.example,.vcenter.corp.example
INTEGRATION_ALLOWED_PRIVATE_RANGES=10.20.0.0/24
INTEGRATION_CA_FILE=
```

| Setting | Meaning |
| --- | --- |
| `INTEGRATION_ALLOWED_HOSTS` | Host names connectors may reach. Empty refuses every URL. A leading dot allows subdomains |
| `INTEGRATION_ALLOWED_PRIVATE_RANGES` | **Needed for any private target.** List only the management ranges. Other private addresses are refused even for an allowed host |
| `INTEGRATION_CA_FILE` | Path **inside the container** to your internal CA file. It needs a volume mount added to `docker-compose.yml`. Certificate checks can't be turned off, so a FortiGate on its factory self-signed certificate fails until it has a certificate from this CA |

Loopback, link-local and cloud metadata addresses are always refused, whatever you list.

## Step 8: Start the backend

```bash
cd /opt/df-ipam
sudo make docker-up
sudo make docker-ps
```

This builds the image, starts PostgreSQL, applies the migrations and seed, then starts the API and worker. `migrate` should show `Exited (0)`, and the others `healthy` or `Up`. If `migrate` failed, read `sudo make docker-logs`, and don't start it again until you know why.

The database lives in the Docker volume `df-ipam_db-data`. Stopping containers keeps it. **`make db-reset` deletes it.**

## Step 9: Certificates and Nginx

| Model | Certificate |
| --- | --- |
| A, B | From your **internal CA**. Install that CA on every user device and agent host, or the browser and agents refuse the connection. Let's Encrypt HTTP validation won't work on a private host |
| C | A public CA |

```bash
sudo mkdir -p /etc/ssl/df-ipam
sudo cp fullchain.pem privkey.pem /etc/ssl/df-ipam/ && sudo chmod 600 /etc/ssl/df-ipam/privkey.pem
cd /opt/df-ipam/deploy/nginx
sudo cp df-ipam.conf /etc/nginx/sites-available/df-ipam
sudo cp agents-allow.conf /etc/nginx/df-ipam-agents.conf
```

Choose the access file:

```bash
sudo cp access-private.conf /etc/nginx/df-ipam-access.conf   # models A and B
sudo cp access-public.conf  /etc/nginx/df-ipam-access.conf   # model C only
```

Edit the host name and `<project-ref>` in the site file, your ranges in `df-ipam-access.conf`, and your agent hosts in `df-ipam-agents.conf`. Don't leave the example `10.0.0.0/8` in place: it is far wider than you need.

If a load balancer or proxy sits in front (model C), Nginx sees its address rather than the user's. Set `real_ip_header` and `set_real_ip_from` for that proxy, or audit logs and allow lists use the wrong address.

```bash
sudo ln -s /etc/nginx/sites-available/df-ipam /etc/nginx/sites-enabled/ && sudo rm -f /etc/nginx/sites-enabled/default
sudo nginx -t && sudo systemctl reload nginx
```

## Step 9a: Flood protection

Each layer stops what the one above it can't.

| Layer | Stops | Where |
| --- | --- | --- |
| Upstream firewall | **Volumetric floods** | FortiGate DoS policy, or your ISP |
| Host | SYN floods, spoofing, connection bursts | Sysctl settings and nftables, below |
| Nginx | Request floods, slow clients | Already in `df-ipam.conf` |
| API | Abuse per address and per person, overload | Built in |
| Discovery | **DF-IPAM flooding your own network** | Built in |

**No software on this server can stop a flood that fills your internet link.** The traffic is already through the link by the time it arrives, so that has to be stopped upstream. For models A and B the server isn't reachable from the internet at all, which is the best protection there is. For model C, set up a FortiGate DoS policy or an upstream scrubbing service first.

On the server:

```bash
cd /opt/df-ipam
sudo cp deploy/sysctl/99-df-ipam.conf /etc/sysctl.d/ && sudo sysctl --system
sudo nft -c -f deploy/nftables/df-ipam-flood.nft   # check it first
sudo nft -f deploy/nftables/df-ipam-flood.nft
```

To keep the nftables rules after a reboot, add `include "/opt/df-ipam/deploy/nftables/df-ipam-flood.nft"` to `/etc/nftables.conf`, and enable the `nftables` service.

**Offices behind one NAT address count as one client** at every layer that limits by address. If real people see "Too many requests", raise `rate` in the Nginx file and the nftables file. Limits per person aren't affected.

The discovery budget is 500 packets a second across all agents, and two jobs can't scan the same network at once. Jobs over the budget wait in the queue. To change it, in `make db-shell`:

```sql
UPDATE system_settings SET value = '300' WHERE key = 'discovery.max_total_pps';
```

## Step 10: Check it

```bash
curl -s http://127.0.0.1:8080/ready
```

It must say `ready`. Then check the access model really works, **from each kind of network**. If sign-in fails with a network error, the browser can't reach Supabase: go back to step 4.

| Test | From | Expected |
| --- | --- | --- |
| Open the site and sign in | Office | Works |
| Open the site and sign in | VPN | Works |
| Open the site | Outside (models A, B) | Times out, or 403 |
| `curl https://<host>/api/v1/agent-api/heartbeat` | A host not in the agents list | 403 from Nginx |
| `nc -zv <server> 5432` and `8080` | Any other machine | Refused or times out |

## Step 11: First administrator and structure

1. Invite yourself from the Supabase dashboard, and sign in once. That creates your user in the local database. You see "Waiting for access".
2. On the server, run `sudo make db-shell`, then:

   ```sql
   INSERT INTO user_roles (user_id, role_id)
   SELECT u.id, r.id FROM users u, roles r
   WHERE u.email = '<your email>' AND r.name = 'Super Administrator';
   ```

3. Add a second administrator now. Approvals, imports and discovery policies need a second person, by design.
4. In **Network Management**, create the organisation, sites and **routing domains**.

**Get routing domains right, because everything depends on them:**

| Network | Routing domain |
| --- | --- |
| Ordinary routed internal network | One domain covers it all |
| Isolated VRF, or a VDOM with its own addressing | One domain each |
| AWS VPC, Azure VNet | One domain each, created by you. A connector never creates one |
| Public address space | One domain of its own |

Ask for every network: "could this IP also exist somewhere else?" If so, it needs its own domain.

## Step 12: Connect integrations

Per platform, in order: create the read-only account, **test that it really can't write**, then add it in **Integrations**. Connectors are built to read only, but the account is the real guarantee.

| Platform | Zone | Account | Connection settings |
| --- | --- | --- | --- |
| FortiGate | Private | REST API admin with a read-only profile, trusted hosts limited to the DF-IPAM server | `https://` URL, API token |
| vCenter | Private | Read-only role at the top of the inventory | `https://` URL, username, password |
| AWS | Public | Read-only IAM role in each account, trusted with an external ID, `Describe*` only | Role ARN, external ID, regions |
| Azure | Public | Service principal with Reader, on one subscription or resource group | Tenant, client and subscription IDs, client secret |

For each connection:

1. Click **Test**.
2. Run a **Sync**.
3. In the connection's routing map, assign each VDOM, VPC or VNet it found to a routing domain. **Until you do, its evidence is kept but not matched to inventory.**
4. Check **Reconciliation Centre** for findings.

The FortiGate, AWS and Azure connectors have only been tested against fixtures built from published API shapes, and vCenter against VMware's simulator. **Try each against a real non-production system first.** Check the sync history, and expect some warnings on the first run.

DNS and DHCP connectors wait for the server types. FortiGate DHCP already comes through the FortiGate connector.

## Step 13: Import existing data

Under **Imports**, upload `Prod_Subnets.txt` and `Prod_VDOMs_Subnets.txt`. Check the preview, then have a second person approve it. The parser hasn't seen the real files yet, so check on the first run that no line was dropped.

## Step 14: Install discovery agents

Do this **only after** the sensitive-networks list is agreed. Until then, use integrations alone: they are passive.

Where to put agents:

| Zone | Agent? | Notes |
| --- | --- | --- |
| Private, routable from the server | One per site is usually enough | ICMP crosses routers. ARP doesn't, so ARP evidence needs an agent on that segment |
| Restricted, approved for scanning | One inside the segment | It calls out on 443 only. Add its address to `df-ipam-agents.conf` and the firewall |
| Restricted, not approved | **None** | Use FortiGate and platform data only |
| Cloud | **None** | Cloud inventory comes from APIs, and active scans aren't used |

On each agent host:

```bash
sudo useradd --system --no-create-home --shell /usr/sbin/nologin dfipam-agent
sudo mkdir -p /opt/df-ipam/bin /etc/df-ipam /var/lib/df-ipam-agent
sudo chown dfipam-agent:dfipam-agent /var/lib/df-ipam-agent && sudo chmod 700 /var/lib/df-ipam-agent
sudo cp agent /opt/df-ipam/bin/
sudo cp company-ca.pem /usr/local/share/ca-certificates/company-ca.crt && sudo update-ca-certificates
sudo cp agent.env.example /etc/df-ipam/agent.env
sudo cp df-ipam-agent.service /etc/systemd/system/
```

The agent requires https, and uses the host's trust store, so the CA step above is needed for models A and B.

1. In **Discovery Centre**, create an agent and copy its one-time token.
2. Put the token and server URL in `/etc/df-ipam/agent.env`. Set group `dfipam-agent` and mode 640.
3. Run `sudo systemctl enable --now df-ipam-agent`, and check it shows online.
4. **Delete the token line** from the file. The agent now uses its own stored credential.

Then add discovery rules, in this order, and have a second person approve each policy:

1. **Exclusions first:** every restricted network. Exclusions always win.
2. **Blackout windows:** change freezes and busy periods.
3. **A small policy:** one test subnet in a lab or low-risk VLAN, at low rate.
4. **Test the emergency stop** while that job runs, and confirm it halts.
5. Widen slowly.

## Step 15: Backups and retention

Create a GPG key on a machine **other than the server**, and import only the public key as root on the server. Set `BACKUP_GPG_RECIPIENT` in `api/.env`.

```bash
cd /opt/df-ipam
sudo cp deploy/systemd/df-ipam-{retention,backup}.* /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable --now df-ipam-backup.timer df-ipam-retention.timer
sudo systemctl start df-ipam-backup.service
sudo journalctl -u df-ipam-backup -n 20 --no-pager
```

Backups go to `/var/backups/df-ipam`. **The database and its backups are now on one server, so copy that folder off it** every day. If the server is lost and no copy exists, the inventory is gone.

Then **do a real restore** into a scratch database, following [runbooks.md](runbooks.md). A backup isn't proven until it has been restored.

Under **System Administration**, review retention. The defaults are starting points, not agreed policy. Audit logs aren't pruned automatically until compliance sets a period.

## Step 16: Go-live checks

Run through the checklist in [security.md](security.md), and at least these:

- [ ] Public sign-ups are off
- [ ] Database passwords and the `service_role` key are new, not ones shared during development
- [ ] `api/.env` has mode 600
- [ ] From outside, the site is unreachable (models A, B)
- [ ] Ports 5432 and 8080 can't be reached from another machine
- [ ] MFA is on (model C: required)
- [ ] Each connector's account is confirmed read-only
- [ ] Restricted networks are excluded, and the emergency stop has been tested
- [ ] Backups are copied off the server, and a restore has been tested
- [ ] `SECRET_ENCRYPTION_KEY` is stored apart from backups
- [ ] Load test run on a non-production copy: `LOADTEST_TOKEN=<token> make load-test`
- [ ] System Administration shows the worker healthy and a recent backup

## Upgrades

```bash
cd /opt/df-ipam
sudo systemctl start df-ipam-backup.service
sudo git pull
sudo make docker-up
```

`docker-up` rebuilds the image, runs new migrations and the seed, and restarts what changed. The seed adds new permissions, and leaves roles you have edited alone. Copy any new web build to `/var/www/df-ipam`. Then check `/ready`, and watch `sudo make docker-logs`.

## Commands

| Command | What it does |
| --- | --- |
| `make docker-up` | Start everything, migrating first |
| `make docker-down` | Stop everything. Data stays |
| `make docker-restart` | Restart the API and worker |
| `make docker-logs` | Follow the logs |
| `make docker-ps` | Show status |
| `make docker-migrate` | Apply migrations and seed |
| `make db-shell` | Open `psql` as the owner |
| `make db-backup` | Write an encrypted backup |
| `make db-reset CONFIRM=yes` | **Delete the database** |

## Common problems

| Symptom | Likely cause | Fix |
| --- | --- | --- |
| The page loads, sign-in fails | Browser can't reach Supabase, or the CSP is wrong | Allow it in step 4. Check `<project-ref>` in the Nginx file |
| "Waiting for access" | Signed in with no role | Assign one in User Management, or see step 11 |
| `required variable ... is missing` from Docker | Compose was run directly | Use the `make` commands, which load `api/.env` |
| `migrate` or the API can't sign in to PostgreSQL | Passwords changed after the volume was created | Set them back, or change them in PostgreSQL with `make db-shell` |
| "Too many requests" for real users | Many people behind one NAT address | Raise the Nginx and nftables rates (step 9a) |
| Every user seems to share one rate limit, or audit logs show `172.30.0.1` | `DOCKER_GATEWAY` doesn't match the Docker network | Recreate with `make docker-down`, then `make docker-up` |
| `Pool overlaps with other one` from Docker | `DOCKER_SUBNET` clashes with another network | Choose another range in `api/.env` |
| A discovery job stays queued | Over the budget, or another job is scanning that network | It starts when the other job ends |
| `port is already allocated` | Something else has 5432 or 8080 | Change `POSTGRES_PORT` or `API_PORT` in `api/.env`, and the Nginx upstream to match |
| Connection test: "refused by the outbound policy" | Host or its private range isn't allowed | Fix `INTEGRATION_ALLOWED_HOSTS` and `INTEGRATION_ALLOWED_PRIVATE_RANGES`, then `make docker-up` |
| Connection test: certificate error | Self-signed or internal certificate | Add the CA with `INTEGRATION_CA_FILE`. Don't look for a way to skip the check |
| Can't save credentials | No master key | Set `SECRET_ENCRYPTION_KEY`, then `make docker-up` |
| Sync works but no findings | Contexts aren't mapped | Map each VDOM or network to a routing domain (step 12) |
| Agent: x509 unknown authority | CA missing on the agent host | Install it (step 14) |
| Agent gets 403 | Not in `df-ipam-agents.conf` | Add its address, or its NAT address |
