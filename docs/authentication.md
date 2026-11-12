# Authentication and Authorisation

Two ways to sign in run side by side:

- **Microsoft Entra ID** (the company's sign-in), for `@dialogfinance.lk` accounts. This is the way forward.
- **Supabase email and password**, kept until it is removed (see [plan.md](plan.md)).

Authorisation stays entirely in DF-IPAM's own `users` and `roles` tables, whichever way a person signed in.

## Microsoft Entra ID

The backend does the sign-in, so the browser never holds a Microsoft token:

1. **Sign in with Microsoft** sends the browser to `/api/v1/auth/microsoft/login`, which redirects to the tenant's authorize endpoint with a random state, nonce and PKCE challenge. The attempt is kept in the API's memory for 10 minutes, tied to a short-lived cookie.
2. Microsoft returns to `/api/v1/auth/microsoft/callback`. The API checks the state against the cookie, exchanges the code with the client secret, and verifies the ID token: RS256 signature against the tenant's keys, issuer `https://login.microsoftonline.com/<tenant>/v2.0` (the tenant is pinned, never `common`), audience the client ID, the nonce, expiry, and that `tid` is the tenant.
3. The email (`email`, else `preferred_username`) must end exactly in an allowed domain (`AUTH_ALLOWED_EMAIL_DOMAINS`, default `dialogfinance.lk`). Guests (`…#EXT#@…`), look-alikes and subdomains are refused.
4. The person is found or added by their Microsoft object ID (`oid`), with `auth_provider = 'entra'`. **A new person has no role** and sees "Waiting for access" until an administrator gives one. A deactivated person is refused.
5. The API starts a session and sets an `HttpOnly`, `SameSite=Lax` cookie: `__Host-dfipam_session` over HTTPS, `dfipam_session` on plain-HTTP local use. Only the token's SHA-256 hash is stored, in `sessions`.

A session ends after 10 hours, after the idle timeout in System (default 30 minutes), at **Sign out**, or at once when the person is deactivated. Changes made with the cookie must send `X-Requested-With: DF-IPAM`, which another site can't add, so a forged form can't act for someone. Sign-ins, sign-outs and refused domains are in the audit log. A refused sign-in returns to `/?signin_error=` with a plain reason.

### Setting it up in Entra ID

1. In the Microsoft Entra admin centre, **App registrations > New registration**: name `DF-IPAM`, **Accounts in this organizational directory only**, platform **Web**, redirect URI `https://<host>/api/v1/auth/microsoft/callback`. For local use add `http://localhost:5173/api/v1/auth/microsoft/callback` (and `http://localhost:8088/...` for the container).
2. **Certificates & secrets > New client secret.** Copy the value at once; note its expiry and renew it before then.
3. **API permissions:** Microsoft Graph delegated `openid`, `profile`, `email` (the defaults). No admin consent is needed for these.
4. Optional: **Token configuration > Add optional claim > ID > email**, so the work email is sent even when it differs from the sign-in name.
5. Optional: under **Enterprise applications > DF-IPAM > Properties**, set **Assignment required** and assign the people or group allowed in. Multi-factor sign-in is enforced by your Conditional Access policies.
6. In `api/.env`: `ENTRA_TENANT_ID` (Directory ID), `ENTRA_CLIENT_ID` (Application ID), `ENTRA_CLIENT_SECRET`, and `APP_BASE_URL` set to the address people use, so the redirect URI matches. Then `make docker-up`.

For local use, leave `VITE_API_URL` empty in `web/.env`, so the app and API share one address and the cookie is sent.

### Your account

Signing in with Microsoft creates a new DF-IPAM user (a different identity from any Supabase account). Sign in once, then give that user a role from an existing administrator account, or in `make db-shell`:

```sql
UPDATE users SET role = 'admin' WHERE auth_provider = 'entra' AND lower(email) = lower('<you>@dialogfinance.lk');
```

## Supabase email and password

1. The web app signs the person in with `supabase-js`, which holds and refreshes the session.
2. The app sends `Authorization: Bearer <access_token>` on each API call.
3. The API verifies the token, finds the `users` row by its subject, checks it is active, and checks the route's permission.

The browser never reads tables through `supabase-js`. The `service_role` key is read only by `api/internal/auth/supabase_admin.go`, which checks at startup that Supabase is reachable.

### Token checks

| Check | Expected |
| --- | --- |
| Signature | Against `https://<project>.supabase.co/auth/v1/.well-known/jwks.json`, cached at most 10 minutes, refetched once on an unknown `kid` |
| `alg` | ES256 or RS256 only |
| `iss` | `https://<project>.supabase.co/auth/v1` |
| `aud` | `authenticated` |
| `exp` | Not expired |
| `sub` | An active `users` row |

```bash
# api/.env
SUPABASE_URL=https://<project>.supabase.co
SUPABASE_ANON_KEY=
SUPABASE_SERVICE_ROLE_KEY=      # backend only, never in web/
CORS_ALLOWED_ORIGINS=http://localhost:5173
# web/.env
VITE_SUPABASE_URL=https://<project>.supabase.co
VITE_SUPABASE_ANON_KEY=
VITE_API_URL=http://127.0.0.1:8080
```

### Supabase project

- **Turn off public sign-ups.** Add people in the Supabase dashboard with a password and auto-confirm on. DF-IPAM has no screen to set a password.
- Use asymmetric signing keys: the JWKS URL above must return a key.
- Set the site URL to the address people use.

## People and roles

A valid token with no `users` row creates one with **no role**: the person sees "Waiting for access" until someone who may manage users gives them a role in **Users and roles**. Deactivating someone blocks them on their next request.

Make the first administrator after they have signed in once, in `make db-shell`:

```sql
UPDATE users SET role = 'admin' WHERE lower(email) = lower('<their email>');
```

Nobody can change their own role or deactivate themselves, and no change (to a person or to a role) may leave nobody active who can manage users.

### Roles

Roles live in the `roles` table: a key, a name, a description and a list of permissions. A role with `full_access` holds every permission, including ones added later; the starting Administrator role is the only one, and it can't be limited or deleted. Others are set up in Users and roles > Roles, and a role nobody holds can be deleted. The starting roles are listed in [new.md](new.md#roles).

### Permissions

Defined in `api/internal/auth/permissions.go` and described for people in `web/src/lib/permissions.ts`. The code checks permissions, never role names; "everyone who may do X" is found with `auth.HolderSQL`.

| Permission | Lets a person |
| --- | --- |
| `inventory:view` | See discovery, servers, the register, devices, alerts and reservations |
| `inventory:edit` | Change the register and devices, import them, manage anyone's reservations |
| `network:manage` | Set up segments (including import from text), environments and departments |
| `discovery:scan` | Start a scan |
| `ip:request` | Raise IP requests |
| `ip:reserve` | Reserve addresses |
| `requests:approve` | Decide IP requests, and hear about new ones |
| `alerts:manage` | Acknowledge alerts, and hear about new High and Critical ones |
| `reports:export` | Download reports |
| `audit:view` | Read and export the audit log |
| `users:manage` | Give people roles, and set up roles |
| `system:manage` | Intervals, email, sessions and the FortiGate connection |

`web/src/app/nav.ts` hides what a person can't open, for clarity only: the API does the real check.

## Keeping the swap easy

Only `api/internal/auth/supabase*.go` and the Supabase half of `web/src/lib/auth.ts` know about Supabase; Microsoft sign-in lives in `api/internal/auth/entra.go`, `sessions.go` and `api/internal/modules/signin`. Removing Supabase means deleting its files and half, its settings, and the bearer path in `auth.Authenticate`.

## Limits of Supabase sign-in

- Tokens live in browser storage, so an XSS bug could steal one. The Content Security Policy reduces this.
- Signing out doesn't cancel an issued token until it expires (1 hour by default).
- Sign-in depends on an internet service: the server and browsers need to reach `<project>.supabase.co`.
- No multi-factor authentication.
- Idle sign-out after `session.idle_timeout_minutes` (default 30) limits unattended sessions.
