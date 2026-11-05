# DF-IPAM

Enterprise IPAM for Dialog Finance. Go backend in `api/`, React frontend in `web/`, design docs in `docs/`, Docker in `docker-compose.yml`. Run `make help` for commands. Before finishing a change, run `make lint test`.

## Read first

- `docs/README.md` for the non-negotiable rules
- `docs/database.md` before changing the schema
- `docs/requirements.md` for scope, deviations and open items
- `docs/development_phase.md` for what is done: update it when a feature changes status

## Hard rules

- **The Supabase `service_role` key stays in `api/.env`,** used only by `api/internal/auth/supabase_admin.go`. Never put it in `web/`, logs or responses.
- **PostgreSQL runs locally in Docker,** with the API and worker, through `docker-compose.yml`. Supabase provides sign-in. Its database is for testing only, through `SUPABASE_DATABASE_URL` and `SUPABASE_MIGRATION_DATABASE_URL`.
- **Every table stays closed to the Supabase Data API,** because Supabase is still used for testing. The browser never queries tables through `supabase-js`; all data goes through the Go API.
- **Env files are `api/.env` and `web/.env` only, none at the root.** Don't commit them, and don't put secrets in code, logs or API responses.
- **Discovery never changes allocation state or ownership.** Evidence stays separate from inventory, and people approve authoritative changes.
- **IPs are unique per routing domain,** never globally. Every subnet and IP query must respect routing context.
- **Integrations are read-only.**
- **Never enumerate an address range.** IPv6 counts need `math/big` in Go, `numeric(39,0)` in SQL and strings in JSON.

## Backend conventions

- Stack is Go, not Laravel, PHP or Python, despite parts of the brief.
- Keep HTTP in handlers, rules in services and SQL in repositories. Modules call each other's services only.
- Register every route with a permission through the route guard. A test fails on unprotected routes.
- Check permissions, never role names.
- Write the audit row in the same transaction as the change.
- Return DTOs, never database rows. Use the shared error format with a stable error code.
- All SQL is parameterised, and sort and filter fields are allowlisted.
- Never run a query per row. Keep files under 200 lines, and function cognitive complexity at 15 or less.

## Database conventions

- Schema changes go in a new numbered `.up.sql` and `.down.sql` pair in `api/migrations/`, followed by `make sql`. Never edit an applied migration.
- Every new table needs `ENABLE ROW LEVEL SECURITY`, a `dfipam_app_all` policy for `dfipam_app`, and explicit least-privilege grants. Without them it is either exposed or unreachable.
- Keep `audit_logs` and `discovery_observations` append-only.

## Auth

Supabase is temporary and will be replaced by company auth. Only `api/internal/auth/supabase*.go` and `web/src/lib/auth.ts` may know about Supabase. Authorisation stays in DF-IPAM's own tables.

## Frontend conventions

- Light theme only. Don't add a dark theme or theme toggle.
- Keep the blue design: colour tokens live in `web/src/index.css`.
- Show allocation and observation state as separate badges. Say "Available candidate", never "Free".
- Every data view needs loading, empty and error states.
- Dialogs are centred with a blurred background (`Modal`). No side drawers.
- Don't show fake data. Pages not built yet say so.
- Screens live in `web/src/features/<screen>`. Tabs are paths, and the query string is for filters, paging and open dialogs.
- Give each mutation a `meta.success` message, and confirm deletes and edit saves.
