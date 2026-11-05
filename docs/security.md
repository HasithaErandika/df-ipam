# Security

## Trust boundaries

| Boundary | Main controls |
| --- | --- |
| Browser to API | HTTPS, JWT verification, RBAC, validation, rate limits |
| Agent to API | Per-agent credential, least-privilege endpoints, payload limits |
| Worker to integrations | Outbound allowlist, TLS validation, read-only accounts |
| App to database | TLS, separate database roles, parameterised SQL |
| Agent to network | Approved scope, exclusions, rate limits, emergency stop |

## Threat model

Grouped by STRIDE.

| Threat | Example | Mitigation |
| --- | --- | --- |
| **Spoofing** | Forged JWT | JWKS signature check, algorithm allowlist, `iss`/`aud`/`exp` checks |
| | Rogue agent | Single-use registration, per-agent credentials, revocation |
| **Tampering** | Agent sends false evidence | Evidence never changes authoritative data; results are tied to the agent and job |
| | Audit rows altered | App role has `INSERT`/`SELECT` only on `audit_logs`, plus a blocking trigger |
| | SQL injection | Parameterised queries; allowlisted sort and filter fields |
| **Repudiation** | User denies a change | Audit log with actor, values, source IP and correlation ID |
| **Information disclosure** | Credentials leak | Encryption at rest, redacted logs, write-only API fields |
| | Viewer sees out-of-scope data | Scope filter applied in the repository layer, covered by tests |
| | Stolen token through XSS | CSP, output encoding, no `dangerouslySetInnerHTML` |
| **Denial of service** | Huge IPv6 listing | Range limits, no enumeration, page caps |
| | Discovery overloads a network | Rate and concurrency ceilings, windows, emergency stop |
| | Large file upload | Size limits, background processing |
| **Elevation of privilege** | Self-approval | Enforced in the service layer and database |
| | SSRF through integration URL | See below |
| | Command injection through Nmap | Fixed arguments, validated IPs, no shell |
| | Compromised connector tries writes | Read-only service accounts on the target system |

Specific to this system: DF-IPAM holds a detailed map of the bank's network, so read access is itself sensitive and exports are audited.

## Supabase Data API

Supabase serves the `public` schema over a REST API to anyone holding the anon key, and that key is in the browser by design. Left open, it would bypass RBAC and the audit log entirely.

- Every table has row level security on, with a policy only for `dfipam_app`.
- `anon` and `authenticated` have no grants on tables, sequences or functions.
- The browser uses `supabase-js` for sign-in only, never for data.
- The `service_role` key bypasses all of this, so it stays on the backend and is used only for invitations.
- A database test fails if any table is left open.

## SSRF protection

Applies to integration URLs and anything else that makes outbound requests.

1. Allow `https` only, against an approved host allowlist.
2. Resolve DNS, canonicalise the IP, then check it. Decimal, octal and IPv4-mapped IPv6 forms are normalised first.
3. Connect to the validated IP, so DNS rebinding can't swap it. TLS still checks the hostname.
4. Block metadata addresses (such as `169.254.169.254`), loopback and link-local always. Private ranges are allowed only if they are on the allowlist.
5. Re-validate every redirect, or disable redirects.

Users can't supply arbitrary URLs or ranges to get around this: connection URLs are limited to the allowlist, and discovery targets to approved scopes.

Nothing in DF-IPAM changes firewalls, DNS, DHCP or cloud networks.

## Secrets

- Integration credentials use AES-256-GCM envelope encryption. The master key comes from the environment or secret manager, never the database or source control.
- A key version is stored with each secret, so keys can be rotated.
- Secrets never appear in API responses, logs, errors, frontend code or source control.
- CI scans the full git history for secrets on each change, and fails on known reachable vulnerabilities.

## HTTP hardening

- TLS 1.2+ at Nginx, with HSTS
- `Content-Security-Policy`, `X-Content-Type-Options`, `Referrer-Policy`, and `frame-ancestors 'none'`
- CORS restricted to `CORS_ALLOWED_ORIGINS`
- Rate limits on the API, stricter for agent registration and exports
- Request size limits
- Errors return a generic message and a correlation ID, with detail only in the logs

Secure cookies and CSRF tokens don't apply while the API uses bearer tokens. They return if the company auth uses cookies.

## Audit

- Records actor, action, object type and ID, old and new values, timestamp, source IP, correlation ID, approval reference and result.
- The audit row is written in the **same transaction** as the change.
- Secret fields are redacted before writing.
- Retention is configurable, with the period agreed with compliance. Pruning uses a separate privileged role, not the app.
- Reservations, allocations and releases go through approval workflows, with separation of duties where required.

## Security review checklist

Used before each release.

**Auth**
- [ ] Supabase public sign-ups are off
- [ ] JWT checks cover signature, algorithm, issuer, audience and expiry
- [ ] `service_role` key is only in `api/.env` and only used for invitations
- [ ] Supabase Data API can't read any DF-IPAM table with the anon key
- [ ] Every route has a permission check, with an automated test that fails on an unprotected route
- [ ] Inactive users are rejected
- [ ] Scope limits are tested for Business Viewer

**Data**
- [ ] All SQL is parameterised
- [ ] App database role can't modify audit logs or run DDL
- [ ] Database connections use TLS with certificate verification
- [ ] Backups are encrypted and a restore has been tested

**Secrets**
- [ ] No secrets in the repo, logs, or frontend bundle
- [ ] Integration credentials are encrypted and write-only
- [ ] Rotation has been exercised

**Network and integrations**
- [ ] Each connector's account has been confirmed read-only
- [ ] TLS verification is on everywhere
- [ ] SSRF tests pass, including rebinding and redirects
- [ ] Discovery can't run outside its allowlist (tested)
- [ ] Emergency stop tested end to end
- [ ] Sensitive networks are passive-only

**Build**
- [ ] `govulncheck` and `pnpm audit` are clean or exceptions are recorded
- [ ] Security headers verified on the deployed site
- [ ] Debug endpoints and Swagger UI are off in production
