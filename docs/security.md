# Security

DF-IPAM holds a map of the company network, so read access is itself sensitive.

## Trust boundaries

| Boundary | Controls |
| --- | --- |
| Browser to `web` | HTTPS, Nginx access lists and rate limits |
| `web` to API | JWT verification, permissions on every route, validation, rate limits |
| API to database | `dfipam_app` role with least-privilege grants, parameterised SQL |
| Scanner to network | Read-only probes, fixed rate and concurrency limits (see [new.md](new.md#keeping-the-network-steady)) |
| API to FortiGate | `https://` only, GET only, a token from a read-only admin profile, TLS verification on by default |

## Threat model

| Threat | Example | Mitigation |
| --- | --- | --- |
| Spoofing | Forged token | JWKS signature, algorithm allowlist, `iss`, `aud`, `exp` checks; active `users` row required |
| Tampering | Audit rows altered | `dfipam_app` has only `SELECT` and `INSERT` on `audit_logs`, and a trigger blocks changes |
| | SQL injection | Parameterised queries; allowlisted sort and filter fields |
| | Evidence used to change the register | Discovery and the FortiGate write only sightings and alerts |
| | Spreadsheet formulas in imports or exports | Neutralised on the way in and out |
| Repudiation | Someone denies a change | Audit row with actor, values, source IP and correlation ID, in the same transaction |
| Disclosure | Supabase Data API with the anon key | Row level security on every table, no grants for `anon` or `authenticated`; tested |
| | Secrets leak | FortiGate token encrypted with AES-256-GCM, never returned or logged; `service_role` key only in `api/.env` |
| | Stolen token through XSS | Content Security Policy, no `dangerouslySetInnerHTML`, idle sign-out |
| Denial of service | Request flood | Limits per address and per person, an in-flight cap, Nginx limits, host SYN protection |
| | DF-IPAM flooding the network | 100 pings a second, 16 port checks in flight, slow probes every 6 hours at most, one scan at a time, manual scans a minute apart, at least 5 minutes between scans |
| | Large upload | 1 MB bodies, 5 MB imports |
| Elevation | A Requester approving | Permissions checked in the API on every route; a test fails on an unprotected route |
| | Admin locks everyone out | Nobody changes their own role; the last Admin can't be removed |

## Secrets

- `api/.env` and `web/.env` only, never committed, mode 600 on servers.
- The FortiGate token is encrypted with `SECRET_ENCRYPTION_KEY`; a key version is kept so the key can be rotated (see [runbooks.md](runbooks.md)).
- Secrets never appear in responses, logs, errors or the web bundle.

## HTTP hardening

- TLS at Nginx, with HSTS, `Content-Security-Policy`, `X-Content-Type-Options`, `Referrer-Policy` and `frame-ancestors 'none'`.
- CORS limited to `CORS_ALLOWED_ORIGINS`.
- Errors return a generic message and a correlation ID; details stay in the logs.
- Bearer tokens, so no cookies and no CSRF tokens. These return if company sign-in uses cookies.

## Network safety

- The scanner only asks questions: ping, DNS, NetBIOS, mDNS, SSDP and, on segments that ask, TCP connects that close at once. It never logs in, never sends data to services, never changes a device.
- Port checks are off by default per segment, because security tools may report them as a port scan. Agree with the security team before turning them on.
- The FortiGate client refuses non-HTTPS addresses, addresses with credentials or paths, and sends only GET requests.

## Release checklist

**Auth**
- [ ] Supabase public sign-ups are off
- [ ] `service_role` key is only in `api/.env`
- [ ] Supabase Data API can't read any table with the anon key (`make test-db-supabase`)
- [ ] The unprotected-route test passes

**Data**
- [ ] `make test-db` passes
- [ ] PostgreSQL and the API are published on localhost only
- [ ] Backups are encrypted, copied off the server, and a restore has been tested

**Secrets**
- [ ] No secrets in the repository, logs or web bundle
- [ ] Database passwords and keys are new, not ones used in development
- [ ] `SECRET_ENCRYPTION_KEY` is stored apart from backups

**Network**
- [ ] The FortiGate token comes from a read-only admin profile
- [ ] TLS verification is on for the FortiGate
- [ ] Port checks are on only where security agreed

**Build**
- [ ] `make vuln` is clean, or exceptions are recorded
- [ ] Security headers verified on the deployed site
