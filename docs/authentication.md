# Authentication and Authorisation

## Decision

Use **simple Supabase Auth for now**. It will be replaced by the company auth later, so it stays small and isolated. This replaces the Entra design in main brief §18 and supplement §15 for the time being. Entra group-to-role mapping and the SSO configuration page wait for the company auth.

Authentication (who you are) comes from Supabase. Authorisation (what you can do) stays entirely in DF-IPAM's database, so it survives the move unchanged.

## How it works

1. The React app signs the user in with `supabase-js` (email and password).
2. `supabase-js` holds the session and refreshes the token.
3. The app sends `Authorization: Bearer <access_token>` on each API call.
4. The Go API verifies the JWT, finds the `users` row by `auth_subject`, loads roles, and checks permissions.

Supabase Row Level Security is not used. DF-IPAM data lives in its own PostgreSQL database, and Supabase is used only for sign-in.

## Supabase project setup

- Turn **off** public sign-ups. Users are invited from the Supabase dashboard.
- Use asymmetric JWT signing keys (ES256 or RS256), and confirm the JWKS URL returns keys. It returns none on the legacy HS256 secret.
- Put only the project URL and anon key in the frontend. The `service_role` key stays in `api/.env`.

The `df-ipam` project was checked on 2026-11-05: its JWKS URL returns an ES256 key, and email is the only enabled provider. Public sign-ups were still **on** and must be turned off.

## Token verification in the API

| Check | Expected |
| --- | --- |
| Signature | Against `https://<project>.supabase.co/auth/v1/.well-known/jwks.json` |
| `alg` | ES256 or RS256 only, from an allowlist |
| `iss` | `https://<project>.supabase.co/auth/v1` |
| `aud` | `authenticated` (confirm against a real token; Supabase docs show this only as an example) |
| `exp` | Not expired |
| `sub` | Matches an active `users` row |

Cache JWKS for no more than 10 minutes, as Supabase advises against caching longer. Refetch once on an unknown `kid`.

Configuration lives in two files:

```bash
# api/.env
SUPABASE_URL=https://<project>.supabase.co
SUPABASE_ANON_KEY=
SUPABASE_SERVICE_ROLE_KEY=
CORS_ALLOWED_ORIGINS=http://localhost:5173
# web/.env
VITE_SUPABASE_URL=https://<project>.supabase.co
VITE_SUPABASE_ANON_KEY=
VITE_API_URL=http://127.0.0.1:8080
```

The API derives the issuer and JWKS URL from `SUPABASE_URL`. It uses the anon key to check Supabase at startup, and the service role key only to send invitations.

## User provisioning

- A valid token with no `users` row creates an active row with **no roles**, so that user can see nothing until they are given a role.
- Roles are managed in the database, or on the User Management page by a Super Administrator. There is no bootstrap setting.
- Deactivating a user in DF-IPAM blocks them on their next request, because `users.is_active` is checked every time. A signed-out token otherwise stays valid until it expires.

To make someone the first Super Administrator, have them sign in once, then run this in `make db-shell`:

```sql
INSERT INTO user_roles (user_id, role_id)
SELECT u.id, r.id FROM users u, roles r
WHERE u.email = '<their email>' AND r.name = 'Super Administrator';
```

## Roles and permissions

Stored in `roles`, `permissions`, `role_permissions` and `user_roles`. Checked by API middleware and again in background jobs.

| Role | Purpose |
| --- | --- |
| Super Administrator | Full platform configuration and user administration |
| IPAM Administrator | Manages subnets, IP records, discovery policies and integrations |
| Network Engineer | Manages network records and performs authorised discovery |
| Infrastructure Engineer | Views inventory, manages assigned records, requests allocations |
| Security Auditor | Read-only access to inventory, audit evidence and reports |
| Business Viewer | Read-only access to specifically permitted business-service information |

| Permission | Super Admin | IPAM Admin | Network Eng | Infra Eng | Security Auditor | Business Viewer |
| --- | :-: | :-: | :-: | :-: | :-: | :-: |
| View | ✓ | ✓ | ✓ | ✓ | ✓ | Scoped |
| Create / Edit | ✓ | ✓ | ✓ | Assigned | | |
| Delete | ✓ | ✓ | | | | |
| Request allocation | ✓ | ✓ | ✓ | ✓ | | |
| Allocate / Reserve | ✓ | ✓ | ✓ | | | |
| Approve | ✓ | ✓ | | | | |
| Import | ✓ | ✓ | ✓ | | | |
| Discover | ✓ | ✓ | ✓ | | | |
| Manage discovery policies | ✓ | ✓ | | | | |
| Manage integrations | ✓ | ✓ | | | | |
| Manage users | ✓ | | | | | |
| View audit history | ✓ | ✓ | | | ✓ | |
| Export reports | ✓ | ✓ | ✓ | ✓ | ✓ | Scoped |

The role purposes come from the brief. The ticks are a proposed default, and administrators can change them. The code checks permissions, never role names. Separation of duties is enforced regardless: nobody approves their own request, and a person decides at most one step of a request. Approval workflows and notification rules are administration, so they need `users:manage`. Anyone signed in reads their own notifications and preferences.

`user_roles` has optional scope columns (organisation, business unit, site, business service). Business Viewers see only explicitly permitted business services, and infrastructure details stay hidden unless a permission allows them.

## Keeping the swap easy

Only the auth layer knows about Supabase:

```go
// api/internal/auth/verifier.go
type Identity struct {
    Subject string
    Email   string
}

type Verifier interface {
    Verify(ctx context.Context, bearerToken string) (Identity, error)
}
```

- **Backend:** `api/internal/auth/supabase.go` implements `Verifier`, and `supabase_admin.go` sends invitations.
- **Frontend:** `web/src/lib/auth.ts` is the only file importing `supabase-js`. It exposes `signIn`, `signOut`, `getAccessToken` and `onAuthChange`.

No module reads Supabase claims directly.

## Moving to company auth

1. Add a new `Verifier` and replace `web/src/lib/auth.ts`.
2. Match existing users by email and update `users.auth_provider` and `users.auth_subject`, so roles and audit history carry over.
3. Remove the Supabase settings.

If the company auth uses cookies, also add CSRF protection and the `sessions` table at that point.

## Accepted limitations while on Supabase

Acceptable for development and pilot use. Review them before production data is loaded.

- Tokens live in browser storage, so an XSS bug could steal one. The CSP in [security.md](security.md) reduces this.
- Signing out does not cancel an access token that has already been issued, until it expires. Keep the lifetime at the 1 hour default or lower.
- Sign-in depends on an external cloud service. The on-prem server and users need outbound access to it.
- Multi-factor authentication is not part of this simple setup, so the brief's "strong authentication" is not met yet.

Agents never use Supabase. They have their own credentials: see [discovery-and-reconciliation.md](discovery-and-reconciliation.md).
