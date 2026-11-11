# Authentication and Authorisation

Sign-in uses **Supabase Auth for now**; it will be replaced by the company's sign-in (see [plan.md](plan.md)). Authorisation stays entirely in DF-IPAM's own `users` table, so it survives that move unchanged.

## How it works

1. The web app signs the person in with `supabase-js` (email and password), which holds and refreshes the session.
2. The app sends `Authorization: Bearer <access_token>` on each API call.
3. The API verifies the token, finds the `users` row by its subject, checks it is active, and checks the route's permission.

Supabase is used only for sign-in. The browser never reads tables through `supabase-js`. The `service_role` key is read only by `api/internal/auth/supabase_admin.go`, which checks at startup that Supabase is reachable; there is no invitation screen.

## Token checks

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

## Supabase project

- **Turn off public sign-ups.** Add people in the Supabase dashboard with a password and auto-confirm on. DF-IPAM has no screen to set a password.
- Use asymmetric signing keys: the JWKS URL above must return a key.
- Set the site URL to the address people use.

## People and roles

A valid token with no `users` row creates one with **no role**: the person sees "Waiting for access" until an Admin gives them a role in Settings > Users. Deactivating someone blocks them on their next request.

Make the first Admin after they have signed in once, in `make db-shell`:

```sql
UPDATE users SET role = 'admin' WHERE lower(email) = lower('<their email>');
```

Admins can't change their own role or deactivate themselves, and the last active Admin can't be removed.

### Permissions

Roles are fixed in `api/internal/auth/permissions.go`. The code checks permissions, never role names.

| Permission | Admin | Requester | Viewer |
| --- | :-: | :-: | :-: |
| `inventory:view` | ✓ | ✓ | ✓ |
| `reports:export` | ✓ | ✓ | ✓ |
| `ip:request` | ✓ | ✓ | |
| `ip:reserve` | ✓ | ✓ | |
| `inventory:edit` | ✓ | | |
| `requests:approve` | ✓ | | |
| `alerts:manage` | ✓ | | |
| `audit:view` | ✓ | | |
| `users:manage` | ✓ | | |
| `system:manage` | ✓ | | |

`web/src/app/nav.ts` hides what a person can't open, for clarity only: the API does the real check.

## Keeping the swap easy

Only `api/internal/auth/supabase*.go` and `web/src/lib/auth.ts` know about Supabase. The API verifies through a `Verifier` interface returning a subject and email; the web app uses `signIn`, `signOut`, `getAccessToken` and `onAuthChange` from `lib/auth.ts`.

To move to company sign-in: add a new `Verifier`, replace `web/src/lib/auth.ts`, match existing users by email so roles and audit history carry over, then remove the Supabase settings.

## Limits while on Supabase

- Tokens live in browser storage, so an XSS bug could steal one. The Content Security Policy reduces this.
- Signing out doesn't cancel an issued token until it expires (1 hour by default).
- Sign-in depends on an internet service: the server and browsers need to reach `<project>.supabase.co`.
- No multi-factor authentication.
- Idle sign-out after `session.idle_timeout_minutes` (default 30) limits unattended sessions.
