# Authentication and Authorisation

DF-IPAM holds no passwords. There are two ways in, and both end in the same session cookie:

- **Microsoft Entra ID**, for `@dialogfinance.lk` accounts. This is the way in.
- **Development sign-in** (`AUTH_BYPASS`), for while Entra ID isn't set up: one button that signs in as one named person, without a password. Never in production.

Authorisation stays entirely in DF-IPAM's own `users` and `roles` tables, whichever way a person signed in. Every `/api/v1` request is signed in by its session cookie; there are no bearer tokens.

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

### Pending accounts

An account can be made before its person first signs in: `make admin EMAIL=…`, and the development sign-in, add one with `auth_provider = 'entra'` and `auth_subject = 'pending:<email>'`. **Their first Microsoft sign-in with a verified email matching it takes that account over** (once), so the role and history given beforehand stay with them.

## Development sign-in

For while the Entra ID app registration isn't ready. In `api/.env`:

```bash
AUTH_BYPASS=true
AUTH_BYPASS_EMAIL=<you>@dialogfinance.lk
```

- The sign-in page shows **Continue with development sign-in**. It calls `POST /api/v1/auth/bypass` (with the `X-Requested-With` header), which starts an ordinary session as `AUTH_BYPASS_EMAIL`: the same cookie, idle timeout, deactivation check and audit (`provider: bypass`) as Microsoft sign-in.
- The first time, that person is added as a pending **Administrator**, so they can set everything up; their later Microsoft sign-in takes the same account over. An existing account with that email is used as it is, and a deactivated one is refused.
- **Anyone who can open the site signs in as that person.** Use it only on a trusted network. Every screen shows a warning banner while it is on, and the API logs a warning at start.
- **The API refuses to start with it when `APP_ENV=production`**, and `docker-compose.prod.yml` and `docker-compose.apache.yml` fix `APP_ENV=production`. Turn it off (`AUTH_BYPASS=false`) once Microsoft sign-in works.

## People and roles

A first Microsoft sign-in with no matching account creates one with **no role**: the person sees "Waiting for access" until someone who may manage users gives them a role in **Users and roles**. Deactivating someone blocks them on their next request.

Make the first administrator with `make admin EMAIL=<their email>`: it promotes an existing account, or adds a pending one that their first sign-in takes over. Audited.

Nobody can change their own role or deactivate themselves, and no change (to a person or to a role) may leave nobody active who can manage users.

### Roles

Roles live in the `roles` table: a key, a name, a description and a list of permissions. A role with `full_access` holds every permission, including ones added later; the starting Administrator role is the only one, and it can't be limited or deleted. Others are set up in Users and roles > Roles, and a role nobody holds can be deleted. The starting roles are listed in [screens.md](../product/screens.md#roles).

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

## Where it lives

| Part | Files |
| --- | --- |
| Sessions and the cookie check | `api/internal/auth/sessions.go`, `middleware.go` |
| Microsoft Entra ID | `api/internal/auth/entra.go`, `keycache.go`, `token.go` |
| Pending accounts | `api/internal/auth/accounts.go`, `api/cmd/admin` |
| Sign-in routes, development sign-in | `api/internal/modules/signin` |
| Browser | `web/src/lib/auth.ts` only; the API client sends the cookie and the `X-Requested-With` header |
